package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/seikaikyo/go-edge-gateway/plugin"
)

func init() {
	plugin.Register("mqtt", func() plugin.Plugin { return &Plugin{} })
}

// SubscriptionDef describes a topic subscription.
type SubscriptionDef struct {
	Topic         string `yaml:"topic"`
	DevicePattern string `yaml:"device_pattern"` // e.g. "machines/{device}/status"
	QoS           byte   `yaml:"qos"`
}

// PluginConfig holds MQTT device-side subscriber settings.
type PluginConfig struct {
	Broker        string            `yaml:"broker"`
	ClientID      string            `yaml:"client_id"`
	Username      string            `yaml:"username"`
	Password      string            `yaml:"password"`
	Subscriptions []SubscriptionDef `yaml:"subscriptions"`
}

// Plugin implements the MQTT device-side subscriber.
type Plugin struct {
	cfg    PluginConfig
	client pahomqtt.Client
	uplink chan<- plugin.Message
	logger *slog.Logger
	mu     sync.Mutex
	msgCnt int
	lastOK time.Time
	lastErr string
}

func (p *Plugin) Name() string { return "mqtt" }

func (p *Plugin) Init(cfg map[string]any, uplink chan<- plugin.Message) error {
	p.uplink = uplink
	p.logger = slog.Default().With("plugin", "mqtt")

	pcfg, err := parseConfig(cfg)
	if err != nil {
		return fmt.Errorf("mqtt config: %w", err)
	}
	p.cfg = pcfg

	p.logger.Info("mqtt plugin initialised",
		"broker", p.cfg.Broker,
		"subscriptions", len(p.cfg.Subscriptions),
	)
	return nil
}

func (p *Plugin) Start(ctx context.Context) error {
	clientID := p.cfg.ClientID
	if clientID == "" {
		clientID = fmt.Sprintf("edge-gateway-mqtt-%d", time.Now().UnixNano()%10000)
	}

	opts := pahomqtt.NewClientOptions().
		AddBroker(p.cfg.Broker).
		SetClientID(clientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetOnConnectHandler(p.onConnect).
		SetConnectionLostHandler(func(_ pahomqtt.Client, err error) {
			p.logger.Warn("connection lost", "error", err)
			p.setError(err.Error())
		})

	if p.cfg.Username != "" {
		opts.SetUsername(p.cfg.Username)
		opts.SetPassword(p.cfg.Password)
	}

	p.client = pahomqtt.NewClient(opts)

	p.logger.Info("connecting to MQTT broker", "broker", p.cfg.Broker)
	token := p.client.Connect()
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt connect: %w", err)
	}

	// Block until context is cancelled.
	<-ctx.Done()
	return nil
}

func (p *Plugin) Stop() error {
	if p.client != nil && p.client.IsConnected() {
		p.client.Disconnect(1000)
	}
	return nil
}

func (p *Plugin) Health() plugin.HealthStatus {
	p.mu.Lock()
	defer p.mu.Unlock()

	connected := p.client != nil && p.client.IsConnected()
	return plugin.HealthStatus{
		OK:          connected && p.lastErr == "",
		Devices:     p.msgCnt,
		ActiveConns: boolToInt(connected),
		LastError:   p.lastErr,
		LastSeen:    p.lastOK,
	}
}

// onConnect subscribes to all configured topics when the MQTT connection is established.
func (p *Plugin) onConnect(client pahomqtt.Client) {
	p.logger.Info("connected to MQTT broker")
	p.setOK()

	for _, sub := range p.cfg.Subscriptions {
		s := sub // capture
		token := client.Subscribe(s.Topic, s.QoS, func(_ pahomqtt.Client, msg pahomqtt.Message) {
			p.handleMessage(s, msg)
		})
		token.Wait()
		if err := token.Error(); err != nil {
			p.logger.Error("subscribe failed", "topic", s.Topic, "error", err)
		} else {
			p.logger.Info("subscribed", "topic", s.Topic)
		}
	}
}

// handleMessage converts an MQTT message to a plugin.Message.
func (p *Plugin) handleMessage(sub SubscriptionDef, msg pahomqtt.Message) {
	deviceName := extractDevice(sub.DevicePattern, msg.Topic())

	// Try to parse payload as JSON.
	payload := make(map[string]any)
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		// Not JSON — wrap raw payload.
		payload["raw"] = string(msg.Payload())
	}
	payload["mqtt_topic"] = msg.Topic()

	p.mu.Lock()
	p.msgCnt++
	p.lastOK = time.Now()
	p.lastErr = ""
	p.mu.Unlock()

	m := plugin.Message{
		Source:  "mqtt",
		Device:  deviceName,
		Topic:   "mqtt/" + msg.Topic(),
		Payload: payload,
		Ts:      time.Now(),
	}

	select {
	case p.uplink <- m:
	default:
		p.logger.Warn("uplink channel full, dropping MQTT message", "topic", msg.Topic())
	}
}

// extractDevice maps an MQTT topic to a device name using the pattern.
// Pattern: "machines/{device}/status", Topic: "machines/pump-01/status" → "pump-01"
func extractDevice(pattern, topic string) string {
	if pattern == "" {
		return topic
	}

	patParts := strings.Split(pattern, "/")
	topParts := strings.Split(topic, "/")

	for i, pp := range patParts {
		if (pp == "{device}" || pp == "{+}") && i < len(topParts) {
			return topParts[i]
		}
	}

	return topic
}

func (p *Plugin) setError(msg string) {
	p.mu.Lock()
	p.lastErr = msg
	p.mu.Unlock()
}

func (p *Plugin) setOK() {
	p.mu.Lock()
	p.lastErr = ""
	p.lastOK = time.Now()
	p.mu.Unlock()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func parseConfig(cfg map[string]any) (PluginConfig, error) {
	pc := PluginConfig{
		Broker:   getString(cfg, "broker"),
		ClientID: getString(cfg, "client_id"),
		Username: getString(cfg, "username"),
		Password: getString(cfg, "password"),
	}

	if pc.Broker == "" {
		return pc, fmt.Errorf("missing 'broker' key")
	}

	if rawSubs, ok := cfg["subscriptions"].([]any); ok {
		for _, raw := range rawSubs {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			sub := SubscriptionDef{
				Topic:         getString(m, "topic"),
				DevicePattern: getString(m, "device_pattern"),
			}
			if v, ok := m["qos"].(int); ok {
				sub.QoS = byte(v)
			}
			pc.Subscriptions = append(pc.Subscriptions, sub)
		}
	}

	return pc, nil
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
