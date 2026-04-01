package secsgem

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/dashfactory/go-factory-io/pkg/message/secs2"
	"github.com/dashfactory/go-factory-io/pkg/simulator"
	"github.com/dashfactory/go-factory-io/pkg/validator"
	"github.com/seikaikyo/go-edge-gateway/plugin"
)

func init() {
	plugin.Register("secsgem", func() plugin.Plugin { return &Plugin{} })
}

// DeviceDef describes a SECS/GEM equipment endpoint.
type DeviceDef struct {
	Name      string `yaml:"name"`
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	Mode      string `yaml:"mode"` // active or passive
	DeviceID  int    `yaml:"device_id"`
	SessionID int    `yaml:"session_id"`
}

// Plugin implements the SECS/GEM protocol adapter using go-factory-io.
type Plugin struct {
	devices []DeviceDef
	hosts   []*simulator.Host
	uplink  chan<- plugin.Message
	logger  *slog.Logger
	mu      sync.Mutex
	conns   int
	lastErr string
	lastOK  time.Time
}

func (p *Plugin) Name() string { return "secsgem" }

func (p *Plugin) Init(cfg map[string]any, uplink chan<- plugin.Message) error {
	p.uplink = uplink
	p.logger = slog.Default().With("plugin", "secsgem")

	devices, err := parseDevices(cfg)
	if err != nil {
		return fmt.Errorf("secsgem config: %w", err)
	}
	p.devices = devices

	p.logger.Info("secsgem plugin initialised", "devices", len(p.devices))
	return nil
}

func (p *Plugin) Start(ctx context.Context) error {
	var wg sync.WaitGroup

	for i := range p.devices {
		wg.Add(1)
		go func(dev DeviceDef) {
			defer wg.Done()
			p.runDevice(ctx, dev)
		}(p.devices[i])
	}

	wg.Wait()
	return nil
}

func (p *Plugin) Stop() error {
	p.mu.Lock()
	hosts := p.hosts
	p.mu.Unlock()

	for _, h := range hosts {
		h.Close()
	}
	return nil
}

func (p *Plugin) Health() plugin.HealthStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return plugin.HealthStatus{
		OK:          p.lastErr == "",
		Devices:     len(p.devices),
		ActiveConns: p.conns,
		LastError:   p.lastErr,
		LastSeen:    p.lastOK,
	}
}

// runDevice manages the HSMS connection lifecycle for a single equipment.
func (p *Plugin) runDevice(ctx context.Context, dev DeviceDef) {
	addr := fmt.Sprintf("%s:%d", dev.Host, dev.Port)
	logger := p.logger.With("device", dev.Name, "addr", addr)

	sessionID := uint16(dev.SessionID)
	if sessionID == 0 {
		sessionID = uint16(dev.DeviceID)
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		host := simulator.NewHost(addr, sessionID, logger)
		host.SetInterceptor(p.makeInterceptor(dev.Name))

		p.mu.Lock()
		p.hosts = append(p.hosts, host)
		p.mu.Unlock()

		logger.Info("connecting to equipment")
		if err := host.Connect(ctx); err != nil {
			logger.Error("connect failed", "error", err)
			p.setError(err.Error())
			host.Close()
			// Wait before retry (T5 equivalent).
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
				continue
			}
		}

		p.addConn()
		logger.Info("connected, establishing communication")

		// S1F13 Establish Communication.
		if _, err := host.EstablishComm(ctx); err != nil {
			logger.Error("establish comm failed", "error", err)
			p.setError(err.Error())
		} else {
			p.setOK()
			logger.Info("communication established")
		}

		// Block until context is cancelled — the interceptor handles incoming messages.
		select {
		case <-ctx.Done():
			host.Close()
			p.removeConn()
			return
		case <-host.Session().Done():
			logger.Warn("session disconnected, will reconnect")
			p.removeConn()
			p.setError("disconnected")
			host.Close()
			// Reconnect after T5.
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
			}
		}
	}
}

// makeInterceptor creates a MessageInterceptor that converts SECS-II messages
// to plugin.Message and sends them to the uplink channel.
func (p *Plugin) makeInterceptor(deviceName string) simulator.MessageInterceptor {
	return func(dir simulator.Direction, stream, function byte, body *secs2.Item, results []validator.ValidationResult) {
		p.setOK()

		payload := map[string]any{
			"direction": string(dir),
			"stream":    int(stream),
			"function":  int(function),
		}

		if body != nil {
			payload["body"] = body.String()
		}

		if len(results) > 0 {
			validationResults := make([]map[string]any, 0, len(results))
			for _, r := range results {
				validationResults = append(validationResults, map[string]any{
					"level":   r.Level.String(),
					"path":    r.Path,
					"message": r.Message,
				})
			}
			payload["validation"] = validationResults
		}

		topic := fmt.Sprintf("secsgem/S%dF%d", stream, function)
		if dir == simulator.DirRX {
			topic = fmt.Sprintf("secsgem/rx/S%dF%d", stream, function)
		}

		msg := plugin.Message{
			Source:  "secsgem",
			Device:  deviceName,
			Topic:   topic,
			Payload: payload,
			Ts:      time.Now(),
		}

		select {
		case p.uplink <- msg:
		default:
			p.logger.Warn("uplink channel full, dropping SECS message",
				"device", deviceName,
				"sf", fmt.Sprintf("S%dF%d", stream, function),
			)
		}
	}
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

func (p *Plugin) addConn() {
	p.mu.Lock()
	p.conns++
	p.mu.Unlock()
}

func (p *Plugin) removeConn() {
	p.mu.Lock()
	if p.conns > 0 {
		p.conns--
	}
	p.mu.Unlock()
}

func parseDevices(cfg map[string]any) ([]DeviceDef, error) {
	rawDevices, ok := cfg["devices"]
	if !ok {
		return nil, fmt.Errorf("missing 'devices' key")
	}

	devList, ok := rawDevices.([]any)
	if !ok {
		return nil, fmt.Errorf("'devices' must be a list")
	}

	var devices []DeviceDef
	for _, raw := range devList {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		dev := DeviceDef{
			Name:     getString(m, "name"),
			Host:     getString(m, "host"),
			Port:     getInt(m, "port", 5000),
			Mode:     getString(m, "mode"),
			DeviceID: getInt(m, "device_id", 1),
		}
		if dev.Mode == "" {
			dev.Mode = "active"
		}
		devices = append(devices, dev)
	}
	return devices, nil
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt(m map[string]any, key string, def int) int {
	switch v := m[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return def
}
