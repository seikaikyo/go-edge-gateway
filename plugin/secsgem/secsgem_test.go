package secsgem

import (
	"testing"

	"github.com/seikaikyo/go-edge-gateway/plugin"
)

func TestParseDevices(t *testing.T) {
	cfg := map[string]any{
		"devices": []any{
			map[string]any{
				"name":      "etcher-01",
				"host":      "192.168.1.100",
				"port":      5000,
				"mode":      "active",
				"device_id": 1,
			},
			map[string]any{
				"name":      "cvd-02",
				"host":      "192.168.1.101",
				"port":      5000,
				"device_id": 2,
			},
		},
	}

	devices, err := parseDevices(cfg)
	if err != nil {
		t.Fatalf("parseDevices: %v", err)
	}

	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	if devices[0].Name != "etcher-01" {
		t.Errorf("device[0].Name = %q", devices[0].Name)
	}
	if devices[0].Port != 5000 {
		t.Errorf("device[0].Port = %d", devices[0].Port)
	}
	if devices[1].Mode != "active" {
		t.Errorf("device[1].Mode = %q, want active (default)", devices[1].Mode)
	}
}

func TestPluginInit(t *testing.T) {
	p := &Plugin{}
	uplink := make(chan plugin.Message, 10)

	cfg := map[string]any{
		"devices": []any{
			map[string]any{
				"name":      "test-eq",
				"host":      "127.0.0.1",
				"port":      5000,
				"device_id": 1,
			},
		},
	}

	if err := p.Init(cfg, uplink); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if p.Name() != "secsgem" {
		t.Errorf("Name = %q", p.Name())
	}

	h := p.Health()
	if !h.OK {
		t.Error("expected Health.OK = true")
	}
	if h.Devices != 1 {
		t.Errorf("Devices = %d", h.Devices)
	}
}
