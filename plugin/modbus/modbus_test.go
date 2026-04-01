package modbus

import (
	"testing"
	"time"

	"github.com/seikaikyo/go-edge-gateway/plugin"
)

func TestParseDevices(t *testing.T) {
	cfg := map[string]any{
		"devices": []any{
			map[string]any{
				"name":          "sensor-1",
				"host":          "192.168.1.100",
				"port":          502,
				"unit_id":       1,
				"poll_interval": "2s",
				"registers": []any{
					map[string]any{
						"name":    "temperature",
						"address": 0,
						"type":    "holding",
						"count":   1,
						"scale":   0.1,
						"unit":    "celsius",
					},
				},
			},
		},
	}

	devices, err := parseDevices(cfg)
	if err != nil {
		t.Fatalf("parseDevices: %v", err)
	}

	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	dev := devices[0]
	if dev.Name != "sensor-1" {
		t.Errorf("name = %q, want sensor-1", dev.Name)
	}
	if dev.Host != "192.168.1.100" {
		t.Errorf("host = %q", dev.Host)
	}
	if dev.Port != 502 {
		t.Errorf("port = %d", dev.Port)
	}
	if dev.PollInterval != 2*time.Second {
		t.Errorf("poll_interval = %v", dev.PollInterval)
	}
	if len(dev.Registers) != 1 {
		t.Fatalf("expected 1 register, got %d", len(dev.Registers))
	}

	reg := dev.Registers[0]
	if reg.Name != "temperature" {
		t.Errorf("reg name = %q", reg.Name)
	}
	if reg.Scale != 0.1 {
		t.Errorf("reg scale = %f", reg.Scale)
	}
}

func TestPluginInit(t *testing.T) {
	p := &Plugin{}
	uplink := make(chan plugin.Message, 10)

	cfg := map[string]any{
		"devices": []any{
			map[string]any{
				"name": "test",
				"host": "127.0.0.1",
				"port": 502,
				"registers": []any{
					map[string]any{
						"name":    "temp",
						"address": 0,
						"type":    "holding",
					},
				},
			},
		},
	}

	if err := p.Init(cfg, uplink); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if p.Name() != "modbus" {
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
