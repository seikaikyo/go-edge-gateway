package core

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	content := `
gateway:
  name: "test-gw"
  log_level: debug
  health_port: 9090

uplink:
  type: stdout

plugins:
  modbus:
    enabled: true
    devices:
      - name: "sensor"
        host: "127.0.0.1"
        port: 502

  secsgem:
    enabled: false
    devices:
      - name: "etcher"
        host: "10.0.0.1"
        port: 5000

  mqtt:
    enabled: false
    broker: "tcp://localhost:1883"
`
	f, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfg, err := LoadConfig(f.Name())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Gateway.Name != "test-gw" {
		t.Errorf("Gateway.Name = %q", cfg.Gateway.Name)
	}
	if cfg.Gateway.LogLevel != "debug" {
		t.Errorf("LogLevel = %q", cfg.Gateway.LogLevel)
	}
	if cfg.Gateway.HealthPort != 9090 {
		t.Errorf("HealthPort = %d", cfg.Gateway.HealthPort)
	}
	if cfg.Uplink.Type != "stdout" {
		t.Errorf("Uplink.Type = %q", cfg.Uplink.Type)
	}

	if len(cfg.Plugins) != 3 {
		t.Fatalf("expected 3 plugins, got %d", len(cfg.Plugins))
	}

	modbus := cfg.Plugins["modbus"]
	if !modbus.Enabled {
		t.Error("modbus should be enabled")
	}

	secsgem := cfg.Plugins["secsgem"]
	if secsgem.Enabled {
		t.Error("secsgem should be disabled")
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	content := `
gateway:
  name: "minimal"
plugins: {}
`
	f, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	cfg, err := LoadConfig(f.Name())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Gateway.LogLevel != "info" {
		t.Errorf("default LogLevel = %q, want info", cfg.Gateway.LogLevel)
	}
	if cfg.Gateway.HealthPort != 8080 {
		t.Errorf("default HealthPort = %d, want 8080", cfg.Gateway.HealthPort)
	}
	if cfg.Uplink.Type != "stdout" {
		t.Errorf("default Uplink.Type = %q, want stdout", cfg.Uplink.Type)
	}
}
