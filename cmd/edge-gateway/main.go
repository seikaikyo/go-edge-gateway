package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/seikaikyo/go-edge-gateway/core"

	// Import plugins — each init() calls plugin.Register.
	_ "github.com/seikaikyo/go-edge-gateway/plugin/modbus"
	_ "github.com/seikaikyo/go-edge-gateway/plugin/mqtt"
	_ "github.com/seikaikyo/go-edge-gateway/plugin/secsgem"
)

var version = "dev"

func main() {
	cfgPath := flag.String("config", "edge-gateway.yaml", "path to config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("edge-gateway", version)
		os.Exit(0)
	}

	// Load config.
	cfg, err := core.LoadConfig(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Setup logger.
	level := slog.LevelInfo
	switch cfg.Gateway.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	logger.Info("edge-gateway starting",
		"name", cfg.Gateway.Name,
		"version", version,
		"plugins_configured", len(cfg.Plugins),
	)

	// Create gateway.
	gw := core.NewGateway(cfg, logger)
	if err := gw.Init(); err != nil {
		logger.Error("gateway init failed", "error", err)
		os.Exit(1)
	}

	// Start health server.
	hs := core.NewHealthServer(gw, cfg.Gateway.HealthPort, logger)
	go func() {
		if err := hs.ListenAndServe(); err != nil {
			logger.Debug("health server stopped", "error", err)
		}
	}()

	// Run gateway until signal.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := gw.Run(ctx); err != nil {
		logger.Error("gateway exited with error", "error", err)
		os.Exit(1)
	}

	hs.Close()
	logger.Info("edge-gateway stopped")
}
