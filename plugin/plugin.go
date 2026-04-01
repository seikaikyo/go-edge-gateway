// Package plugin defines the interface that all edge-gateway plugins must implement.
package plugin

import (
	"context"
	"time"
)

// Message is the universal data envelope that plugins use to report
// device data upstream. Core's uplink module consumes these.
type Message struct {
	Source  string         `json:"source"`  // plugin name
	Device  string         `json:"device"`  // device identifier from config
	Topic   string         `json:"topic"`   // data category (e.g. "modbus/poll", "secsgem/event")
	Payload map[string]any `json:"payload"` // actual data
	Ts      time.Time      `json:"ts"`
}

// HealthStatus reports a plugin's current state.
type HealthStatus struct {
	OK          bool      `json:"ok"`
	Devices     int       `json:"devices"`
	ActiveConns int       `json:"active_conns"`
	LastError   string    `json:"last_error,omitempty"`
	LastSeen    time.Time `json:"last_seen,omitempty"`
}

// Plugin is the interface every protocol adapter must satisfy.
type Plugin interface {
	// Name returns the plugin identifier (e.g. "secsgem", "modbus", "mqtt").
	Name() string

	// Init receives plugin-specific config and the uplink channel.
	Init(cfg map[string]any, uplink chan<- Message) error

	// Start begins device communication. It blocks until ctx is cancelled.
	Start(ctx context.Context) error

	// Stop gracefully shuts down connections.
	Stop() error

	// Health returns current plugin health.
	Health() HealthStatus
}
