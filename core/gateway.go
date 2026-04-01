package core

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/seikaikyo/go-edge-gateway/plugin"
)

// Gateway is the main orchestrator that manages plugin lifecycles and the uplink.
type Gateway struct {
	cfg     *Config
	plugins []plugin.Plugin
	uplink  chan plugin.Message
	logger  *slog.Logger
}

// NewGateway creates a gateway from the given config.
func NewGateway(cfg *Config, logger *slog.Logger) *Gateway {
	return &Gateway{
		cfg:    cfg,
		uplink: make(chan plugin.Message, 256),
		logger: logger,
	}
}

// Init discovers and initialises all enabled plugins.
func (g *Gateway) Init() error {
	for name, pcfg := range g.cfg.Plugins {
		if !pcfg.Enabled {
			g.logger.Info("plugin disabled, skipping", "plugin", name)
			continue
		}

		factory := plugin.Get(name)
		if factory == nil {
			return fmt.Errorf("plugin %q enabled but not registered (missing import?)", name)
		}

		p := factory()
		if err := p.Init(pcfg.Settings, g.uplink); err != nil {
			return fmt.Errorf("init plugin %q: %w", name, err)
		}

		g.plugins = append(g.plugins, p)
		g.logger.Info("plugin initialised", "plugin", name)
	}

	if len(g.plugins) == 0 {
		g.logger.Warn("no plugins enabled")
	}
	return nil
}

// Run starts all plugins, the uplink consumer, and the coordinator. Blocks until ctx is cancelled.
func (g *Gateway) Run(ctx context.Context) error {
	// Start uplink consumer.
	uplinkDone := make(chan struct{})
	go func() {
		defer close(uplinkDone)
		g.consumeUplink(ctx)
	}()

	// Start coordinator (if configured).
	coord := NewCoordinator(g.cfg.Coordinator, g, g.logger)
	if coord != nil {
		go coord.Run(ctx)
	}

	// Start all plugins concurrently.
	var wg sync.WaitGroup
	errCh := make(chan error, len(g.plugins))

	for _, p := range g.plugins {
		wg.Add(1)
		go func(p plugin.Plugin) {
			defer wg.Done()
			g.logger.Info("plugin starting", "plugin", p.Name())
			if err := p.Start(ctx); err != nil {
				g.logger.Error("plugin stopped with error", "plugin", p.Name(), "error", err)
				errCh <- fmt.Errorf("plugin %s: %w", p.Name(), err)
			}
		}(p)
	}

	// Wait for context cancellation.
	<-ctx.Done()
	g.logger.Info("shutdown signal received")

	// Stop all plugins.
	for _, p := range g.plugins {
		if err := p.Stop(); err != nil {
			g.logger.Error("plugin stop error", "plugin", p.Name(), "error", err)
		}
	}

	wg.Wait()
	close(g.uplink)
	<-uplinkDone

	return nil
}

// Health returns aggregated health from all plugins.
func (g *Gateway) Health() map[string]plugin.HealthStatus {
	result := make(map[string]plugin.HealthStatus, len(g.plugins))
	for _, p := range g.plugins {
		result[p.Name()] = p.Health()
	}
	return result
}

// consumeUplink reads messages from the uplink channel and dispatches them.
func (g *Gateway) consumeUplink(ctx context.Context) {
	sink := NewUplink(g.cfg, g.logger)
	defer sink.Close()

	for {
		select {
		case msg, ok := <-g.uplink:
			if !ok {
				return
			}
			if err := sink.Send(msg); err != nil {
				g.logger.Error("uplink send failed", "error", err)
			}
		case <-ctx.Done():
			// Drain remaining messages.
			for msg := range g.uplink {
				_ = sink.Send(msg)
			}
			return
		}
	}
}
