package core

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level gateway configuration.
type Config struct {
	Gateway GatewayConfig            `yaml:"gateway"`
	Uplink  UplinkConfig             `yaml:"uplink"`
	Plugins map[string]PluginConfig  `yaml:"plugins"`
}

// GatewayConfig holds gateway identity and operational settings.
type GatewayConfig struct {
	Name       string `yaml:"name"`
	LogLevel   string `yaml:"log_level"`
	HealthPort int    `yaml:"health_port"`
}

// UplinkConfig defines where plugin messages are sent.
type UplinkConfig struct {
	Type string     `yaml:"type"` // stdout, mqtt, file
	MQTT MQTTUplink `yaml:"mqtt"`
	File FileUplink `yaml:"file"`
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

	return &cfg, nil
}
