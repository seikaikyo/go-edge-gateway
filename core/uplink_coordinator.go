package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/seikaikyo/go-edge-gateway/plugin"
)

// coordinatorUplink batches plugin messages and POSTs them to the coordinator's /events/ingest endpoint.
type coordinatorUplink struct {
	url           string
	nodeID        string
	client        *http.Client
	logger        *slog.Logger
	batchInterval time.Duration
	maxBatchSize  int

	mu      sync.Mutex
	buffer  []ingestEvent
	done    chan struct{}
	stopped bool
}

type ingestEvent struct {
	EventID   string         `json:"event_id"`
	Timestamp time.Time      `json:"timestamp"`
	Source    string         `json:"source"`
	Type      string         `json:"type"`
	Severity  string         `json:"severity"`
	Data      map[string]any `json:"data"`
}

type ingestPayload struct {
	NodeID string        `json:"node_id"`
	Events []ingestEvent `json:"events"`
}

func newCoordinatorUplink(cfg *Config, logger *slog.Logger) *coordinatorUplink {
	interval := 5 * time.Second
	if cfg.Uplink.BatchInterval != "" {
		if d, err := time.ParseDuration(cfg.Uplink.BatchInterval); err == nil {
			interval = d
		}
	}

	maxBatch := 100
	if cfg.Uplink.MaxBatchSize > 0 {
		maxBatch = cfg.Uplink.MaxBatchSize
	}

	u := &coordinatorUplink{
		url:           cfg.Coordinator.URL + "/events/ingest",
		nodeID:        cfg.Coordinator.NodeID,
		client:        &http.Client{Timeout: 10 * time.Second},
		logger:        logger,
		batchInterval: interval,
		maxBatchSize:  maxBatch,
		buffer:        make([]ingestEvent, 0, maxBatch),
		done:          make(chan struct{}),
	}

	go u.flushLoop()
	logger.Info("coordinator uplink started", "url", u.url, "interval", interval, "max_batch", maxBatch)
	return u
}

func (u *coordinatorUplink) Send(msg plugin.Message) error {
	evt := ingestEvent{
		EventID:   fmt.Sprintf("%s-%s-%d", msg.Source, msg.Device, msg.Ts.UnixNano()),
		Timestamp: msg.Ts,
		Source:    fmt.Sprintf("%s:%s", msg.Source, msg.Device),
		Type:      msg.Topic,
		Severity:  "info",
		Data:      msg.Payload,
	}

	u.mu.Lock()
	u.buffer = append(u.buffer, evt)
	shouldFlush := len(u.buffer) >= u.maxBatchSize
	u.mu.Unlock()

	if shouldFlush {
		u.flush()
	}

	return nil
}

func (u *coordinatorUplink) Close() error {
	u.mu.Lock()
	u.stopped = true
	u.mu.Unlock()
	close(u.done)
	u.flush()
	return nil
}

func (u *coordinatorUplink) flushLoop() {
	ticker := time.NewTicker(u.batchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			u.flush()
		case <-u.done:
			return
		}
	}
}

func (u *coordinatorUplink) flush() {
	u.mu.Lock()
	if len(u.buffer) == 0 {
		u.mu.Unlock()
		return
	}
	batch := u.buffer
	u.buffer = make([]ingestEvent, 0, u.maxBatchSize)
	u.mu.Unlock()

	payload := ingestPayload{
		NodeID: u.nodeID,
		Events: batch,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		u.logger.Error("coordinator uplink marshal failed", "error", err)
		return
	}

	resp, err := u.client.Post(u.url, "application/json", bytes.NewReader(body))
	if err != nil {
		u.logger.Warn("coordinator uplink POST failed", "error", err, "events", len(batch))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		u.logger.Warn("coordinator uplink unexpected status", "status", resp.StatusCode, "events", len(batch))
		return
	}

	u.logger.Debug("events flushed to coordinator", "count", len(batch))
}
