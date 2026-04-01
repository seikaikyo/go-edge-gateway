package mqtt

import (
	"testing"

	"github.com/seikaikyo/go-edge-gateway/plugin"
)

func TestExtractDevice(t *testing.T) {
	tests := []struct {
		pattern string
		topic   string
		want    string
	}{
		{"machines/{device}/status", "machines/pump-01/status", "pump-01"},
		{"sensors/{+}", "sensors/temp-rack-3", "temp-rack-3"},
		{"", "raw/topic/path", "raw/topic/path"},
		{"no/match/here", "different/topic", "different/topic"},
	}

	for _, tt := range tests {
		got := extractDevice(tt.pattern, tt.topic)
		if got != tt.want {
			t.Errorf("extractDevice(%q, %q) = %q, want %q", tt.pattern, tt.topic, got, tt.want)
		}
	}
}

func TestParseConfig(t *testing.T) {
	cfg := map[string]any{
		"broker":    "tcp://192.168.1.50:1883",
		"client_id": "test-gw",
		"subscriptions": []any{
			map[string]any{
				"topic":          "machines/+/status",
				"device_pattern": "machines/{device}/status",
			},
		},
	}

	pc, err := parseConfig(cfg)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}

	if pc.Broker != "tcp://192.168.1.50:1883" {
		t.Errorf("Broker = %q", pc.Broker)
	}
	if len(pc.Subscriptions) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(pc.Subscriptions))
	}
	if pc.Subscriptions[0].Topic != "machines/+/status" {
		t.Errorf("sub topic = %q", pc.Subscriptions[0].Topic)
	}
}

func TestPluginInit(t *testing.T) {
	p := &Plugin{}
	uplink := make(chan plugin.Message, 10)

	cfg := map[string]any{
		"broker": "tcp://127.0.0.1:1883",
		"subscriptions": []any{
			map[string]any{
				"topic": "test/#",
			},
		},
	}

	if err := p.Init(cfg, uplink); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if p.Name() != "mqtt" {
		t.Errorf("Name = %q", p.Name())
	}
}
