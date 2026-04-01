package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/seikaikyo/go-edge-gateway/plugin"
)

// Uplink dispatches plugin messages to the configured destination.
type Uplink interface {
	Send(msg plugin.Message) error
	Close() error
}

// NewUplink creates the appropriate uplink based on config.
func NewUplink(cfg *Config, logger *slog.Logger) Uplink {
	switch cfg.Uplink.Type {
	case "file":
		return newFileUplink(cfg.Uplink.File.Path, logger)
	case "mqtt":
		return newMQTTUplink(cfg.Uplink.MQTT, logger)
	case "coordinator":
		return newCoordinatorUplink(cfg, logger)
	default:
		return &stdoutUplink{logger: logger}
	}
}

// stdoutUplink prints messages to stdout as JSON lines.
type stdoutUplink struct {
	logger *slog.Logger
}

func (u *stdoutUplink) Send(msg plugin.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func (u *stdoutUplink) Close() error { return nil }

// fileUplink writes JSON lines to a file.
type fileUplink struct {
	file   *os.File
	enc    *json.Encoder
	logger *slog.Logger
}

func newFileUplink(path string, logger *slog.Logger) *fileUplink {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		logger.Error("failed to open uplink file, falling back to stdout", "path", path, "error", err)
		return &fileUplink{file: os.Stdout, enc: json.NewEncoder(os.Stdout), logger: logger}
	}
	logger.Info("file uplink opened", "path", path)
	return &fileUplink{file: f, enc: json.NewEncoder(f), logger: logger}
}

func (u *fileUplink) Send(msg plugin.Message) error {
	return u.enc.Encode(msg)
}

func (u *fileUplink) Close() error {
	if u.file != os.Stdout {
		return u.file.Close()
	}
	return nil
}

// mqttUplink publishes plugin messages to an MQTT broker.
type mqttUplink struct {
	cfg    MQTTUplink
	client pahomqtt.Client
	logger *slog.Logger
}

func newMQTTUplink(cfg MQTTUplink, logger *slog.Logger) *mqttUplink {
	clientID := cfg.ClientID
	if clientID == "" {
		clientID = fmt.Sprintf("edge-gw-uplink-%d", time.Now().UnixNano()%10000)
	}

	opts := pahomqtt.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(clientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second)

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}

	client := pahomqtt.NewClient(opts)
	token := client.Connect()
	token.Wait()
	if err := token.Error(); err != nil {
		logger.Error("mqtt uplink connect failed, messages will be lost", "error", err)
	} else {
		logger.Info("mqtt uplink connected", "broker", cfg.Broker)
	}

	return &mqttUplink{cfg: cfg, client: client, logger: logger}
}

func (u *mqttUplink) Send(msg plugin.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	topic := msg.Topic
	if u.cfg.TopicPrefix != "" {
		topic = u.cfg.TopicPrefix + "/" + msg.Topic
	}

	qos := byte(u.cfg.QoS)
	token := u.client.Publish(topic, qos, false, data)
	token.Wait()
	return token.Error()
}

func (u *mqttUplink) Close() error {
	if u.client != nil && u.client.IsConnected() {
		u.client.Disconnect(1000)
	}
	return nil
}
