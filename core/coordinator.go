package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"time"
)

// Coordinator handles registration and heartbeat with the cloud coordinator.
type Coordinator struct {
	cfg       CoordinatorConfig
	gateway   *Gateway
	client    *http.Client
	logger    *slog.Logger
	startTime time.Time
	interval  time.Duration
}

type registerPayload struct {
	NodeID       string            `json:"node_id"`
	NodeType     string            `json:"node_type"`
	Version      string            `json:"version"`
	Location     string            `json:"location,omitempty"`
	Capabilities []string          `json:"capabilities"`
	Endpoints    map[string]string `json:"endpoints,omitempty"`
	Metadata     map[string]any    `json:"metadata,omitempty"`
}

type heartbeatPayload struct {
	NodeID        string         `json:"node_id"`
	Status        string         `json:"status"`
	UptimeSeconds int64          `json:"uptime_seconds"`
	Plugins       map[string]any `json:"plugins,omitempty"`
	System        map[string]any `json:"system,omitempty"`
}

// NewCoordinator creates a coordinator client. Returns nil if URL is not configured.
func NewCoordinator(cfg CoordinatorConfig, gw *Gateway, logger *slog.Logger) *Coordinator {
	if cfg.URL == "" {
		return nil
	}

	interval := 30 * time.Second
	if cfg.HeartbeatInterval != "" {
		if d, err := time.ParseDuration(cfg.HeartbeatInterval); err == nil {
			interval = d
		}
	}

	return &Coordinator{
		cfg:     cfg,
		gateway: gw,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger:    logger,
		startTime: time.Now(),
		interval:  interval,
	}
}

// Run registers with the coordinator and sends periodic heartbeats.
// Blocks until ctx is cancelled.
func (c *Coordinator) Run(ctx context.Context) {
	// Register on startup (retry until success or ctx cancelled).
	for {
		if err := c.register(ctx); err != nil {
			c.logger.Error("coordinator register failed, retrying in 5s", "error", err)
			select {
			case <-time.After(5 * time.Second):
				continue
			case <-ctx.Done():
				return
			}
		}
		c.logger.Info("registered with coordinator", "url", c.cfg.URL, "node_id", c.cfg.NodeID)
		break
	}

	// Heartbeat loop.
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.heartbeat(ctx); err != nil {
				c.logger.Warn("heartbeat failed", "error", err)
			} else {
				c.logger.Debug("heartbeat sent")
			}
		case <-ctx.Done():
			return
		}
	}
}

func (c *Coordinator) register(ctx context.Context) error {
	caps := c.gateway.pluginNames()

	payload := registerPayload{
		NodeID:       c.cfg.NodeID,
		NodeType:     "edge-gateway",
		Version:      "0.1.0",
		Location:     c.cfg.Location,
		Capabilities: caps,
		Endpoints: map[string]string{
			"health": fmt.Sprintf("http://localhost:%d/health", c.gateway.cfg.Gateway.HealthPort),
		},
		Metadata: map[string]any{
			"os":   runtime.GOOS + "/" + runtime.GOARCH,
			"go":   runtime.Version(),
		},
	}

	return c.post(ctx, "/edge/register", payload)
}

func (c *Coordinator) heartbeat(ctx context.Context) error {
	health := c.gateway.Health()

	// Determine overall status.
	status := "healthy"
	pluginData := make(map[string]any, len(health))
	for name, h := range health {
		pluginData[name] = map[string]any{
			"status":  boolToStatus(h.OK),
			"devices": h.Devices,
		}
		if !h.OK {
			status = "degraded"
		}
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	payload := heartbeatPayload{
		NodeID:        c.cfg.NodeID,
		Status:        status,
		UptimeSeconds: int64(time.Since(c.startTime).Seconds()),
		Plugins:       pluginData,
		System: map[string]any{
			"memory_mb":  m.Alloc / 1024 / 1024,
			"goroutines": runtime.NumGoroutine(),
		},
	}

	return c.post(ctx, "/edge/heartbeat", payload)
}

func (c *Coordinator) post(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	return nil
}

// pluginNames returns the names of all initialised plugins.
func (g *Gateway) pluginNames() []string {
	names := make([]string, len(g.plugins))
	for i, p := range g.plugins {
		names[i] = p.Name()
	}
	return names
}

func boolToStatus(ok bool) string {
	if ok {
		return "running"
	}
	return "error"
}
