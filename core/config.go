package core

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level gateway configuration.
type Config struct {
	Gateway     GatewayConfig           `yaml:"gateway"`
	API         APIConfig               `yaml:"api"`
	Scan        ScanConfig              `yaml:"scan"`
	Coordinator CoordinatorConfig       `yaml:"coordinator"`
	Uplink      UplinkConfig            `yaml:"uplink"`
	Plugins     map[string]PluginConfig `yaml:"plugins"`
}

// APIConfig holds HTTP listener and authentication settings.
//
// Defaults are deliberately restrictive. The server binds to loopback and the
// /api routes stay closed until an API token is supplied through the
// environment; exposing the API on another interface has to be explicit.
type APIConfig struct {
	// Listen is the bind address for the HTTP server. Empty means loopback.
	// Overridden by the --listen flag or EDGE_GATEWAY_LISTEN.
	Listen string `yaml:"listen"`

	// TokenEnv names the environment variable holding the API token.
	// Empty means EDGE_GATEWAY_API_TOKEN.
	TokenEnv string `yaml:"token_env"`

	// AllowedOrigins lists browser origins permitted to call /api in addition
	// to same-origin requests. Empty means same-origin only.
	AllowedOrigins []string `yaml:"allowed_origins"`
}

// ScanConfig constrains what the on-demand scan API is allowed to reach.
type ScanConfig struct {
	// AllowedTargets is a CIDR allow list for TCP scan targets. Empty falls
	// back to loopback plus the RFC1918 private ranges.
	AllowedTargets []string `yaml:"allowed_targets"`
}

// CoordinatorConfig defines cloud coordinator connection settings.
type CoordinatorConfig struct {
	URL               string `yaml:"url"`
	NodeID            string `yaml:"node_id"`
	Location          string `yaml:"location"`
	HeartbeatInterval string `yaml:"heartbeat_interval"` // e.g. "30s"
}

// GatewayConfig holds gateway identity and operational settings.
type GatewayConfig struct {
	Name       string `yaml:"name"`
	LogLevel   string `yaml:"log_level"`
	HealthPort int    `yaml:"health_port"`
}

// UplinkConfig defines where plugin messages are sent.
type UplinkConfig struct {
	Type          string     `yaml:"type"` // stdout, mqtt, file, coordinator
	MQTT          MQTTUplink `yaml:"mqtt"`
	File          FileUplink `yaml:"file"`
	BatchInterval string     `yaml:"batch_interval"` // for coordinator type, e.g. "5s"
	MaxBatchSize  int        `yaml:"max_batch_size"` // for coordinator type
}

// MQTTUplink configures the MQTT uplink target.
type MQTTUplink struct {
	Broker      string `yaml:"broker"`
	TopicPrefix string `yaml:"topic_prefix"`
	QoS         int    `yaml:"qos"`
	ClientID    string `yaml:"client_id"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
}

// FileUplink configures file-based message logging.
type FileUplink struct {
	Path string `yaml:"path"`
}

// PluginConfig holds the enabled flag and raw settings for a single plugin.
type PluginConfig struct {
	Enabled  bool           `yaml:"enabled"`
	Settings map[string]any `yaml:"-"`
}

// UnmarshalYAML implements custom unmarshalling so that all keys except
// "enabled" are collected into Settings.
func (pc *PluginConfig) UnmarshalYAML(node *yaml.Node) error {
	// Decode entire node into a generic map first.
	var raw map[string]any
	if err := node.Decode(&raw); err != nil {
		return err
	}

	// Extract "enabled" flag.
	if v, ok := raw["enabled"]; ok {
		if b, ok := v.(bool); ok {
			pc.Enabled = b
		}
	}
	delete(raw, "enabled")

	pc.Settings = raw
	return nil
}

// LoadConfig reads and parses a YAML config file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Defaults
	if cfg.Gateway.LogLevel == "" {
		cfg.Gateway.LogLevel = "info"
	}
	if cfg.Gateway.HealthPort == 0 {
		cfg.Gateway.HealthPort = 8080
	}
	if cfg.Uplink.Type == "" {
		cfg.Uplink.Type = "stdout"
	}
	if cfg.API.Listen == "" {
		cfg.API.Listen = DefaultListenHost
	}
	if cfg.API.TokenEnv == "" {
		cfg.API.TokenEnv = DefaultTokenEnv
	}

	return &cfg, nil
}
