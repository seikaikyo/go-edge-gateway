package modbus

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"sync"
	"time"

	modbusclient "github.com/goburrow/modbus"
	"github.com/seikaikyo/go-edge-gateway/plugin"
)

func init() {
	plugin.Register("modbus", func() plugin.Plugin { return &Plugin{} })
}

// RegisterDef describes a single register to read.
type RegisterDef struct {
	Name    string  `yaml:"name"`
	Address int     `yaml:"address"`
	Type    string  `yaml:"type"`  // holding, input, coil, discrete
	Count   int     `yaml:"count"` // number of registers
	Scale   float64 `yaml:"scale"` // raw * scale = actual
	Unit    string  `yaml:"unit"`
}

// DeviceDef describes a Modbus TCP device.
type DeviceDef struct {
	Name         string        `yaml:"name"`
	Host         string        `yaml:"host"`
	Port         int           `yaml:"port"`
	UnitID       byte          `yaml:"unit_id"`
	PollInterval time.Duration `yaml:"poll_interval"`
	Registers    []RegisterDef `yaml:"registers"`
}

// Plugin implements the Modbus TCP protocol adapter.
type Plugin struct {
	devices []DeviceDef
	uplink  chan<- plugin.Message
	logger  *slog.Logger
	mu      sync.Mutex
	conns   int
	lastErr string
	lastOK  time.Time
}

func (p *Plugin) Name() string { return "modbus" }

func (p *Plugin) Init(cfg map[string]any, uplink chan<- plugin.Message) error {
	p.uplink = uplink
	p.logger = slog.Default().With("plugin", "modbus")

	devices, err := parseDevices(cfg)
	if err != nil {
		return fmt.Errorf("modbus config: %w", err)
	}
	p.devices = devices

	p.logger.Info("modbus plugin initialised", "devices", len(p.devices))
	return nil
}

func (p *Plugin) Start(ctx context.Context) error {
	var wg sync.WaitGroup

	for i := range p.devices {
		wg.Add(1)
		go func(dev DeviceDef) {
			defer wg.Done()
			p.pollDevice(ctx, dev)
		}(p.devices[i])
	}

	wg.Wait()
	return nil
}

func (p *Plugin) Stop() error { return nil }

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

// pollDevice runs the polling loop for a single Modbus device.
func (p *Plugin) pollDevice(ctx context.Context, dev DeviceDef) {
	addr := fmt.Sprintf("%s:%d", dev.Host, dev.Port)
	logger := p.logger.With("device", dev.Name, "addr", addr)

	handler := modbusclient.NewTCPClientHandler(addr)
	handler.SlaveId = dev.UnitID
	handler.Timeout = 5 * time.Second

	interval := dev.PollInterval
	if interval == 0 {
		interval = time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	client := modbusclient.NewClient(handler)
	connected := false

	for {
		select {
		case <-ctx.Done():
			if connected {
				handler.Close()
			}
			return
		case <-ticker.C:
			if !connected {
				if err := handler.Connect(); err != nil {
					logger.Error("connect failed", "error", err)
					p.setError(err.Error())
					continue
				}
				connected = true
				p.addConn()
				logger.Info("connected")
			}

			payload := make(map[string]any)
			var readErr error

			for _, reg := range dev.Registers {
				val, err := readRegister(client, reg)
				if err != nil {
					logger.Error("read register failed", "register", reg.Name, "error", err)
					readErr = err
					continue
				}
				payload[reg.Name] = val
				if reg.Unit != "" {
					payload[reg.Name+"_unit"] = reg.Unit
				}
			}

			if readErr != nil {
				p.setError(readErr.Error())
				// Reconnect on next tick.
				handler.Close()
				connected = false
				p.removeConn()
				continue
			}

			p.setOK()

			msg := plugin.Message{
				Source:  "modbus",
				Device:  dev.Name,
				Topic:   "modbus/poll",
				Payload: payload,
				Ts:      time.Now(),
			}

			select {
			case p.uplink <- msg:
			default:
				logger.Warn("uplink channel full, dropping message")
			}
		}
	}
}

func readRegister(client modbusclient.Client, reg RegisterDef) (float64, error) {
	count := uint16(reg.Count)
	if count == 0 {
		count = 1
	}
	addr := uint16(reg.Address)

	var results []byte
	var err error

	switch reg.Type {
	case "holding", "":
		results, err = client.ReadHoldingRegisters(addr, count)
	case "input":
		results, err = client.ReadInputRegisters(addr, count)
	default:
		return 0, fmt.Errorf("unsupported register type: %s", reg.Type)
	}

	if err != nil {
		return 0, err
	}

	if len(results) < 2 {
		return 0, fmt.Errorf("insufficient data: got %d bytes", len(results))
	}

	raw := float64(binary.BigEndian.Uint16(results[:2]))
	scale := reg.Scale
	if scale == 0 {
		scale = 1
	}
	return raw * scale, nil
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

// parseDevices extracts DeviceDef slices from the raw YAML config map.
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
			Name:   getString(m, "name"),
			Host:   getString(m, "host"),
			Port:   getInt(m, "port", 502),
			UnitID: byte(getInt(m, "unit_id", 1)),
		}

		if v := getString(m, "poll_interval"); v != "" {
			d, err := time.ParseDuration(v)
			if err == nil {
				dev.PollInterval = d
			}
		}

		if rawRegs, ok := m["registers"].([]any); ok {
			for _, rr := range rawRegs {
				rm, ok := rr.(map[string]any)
				if !ok {
					continue
				}
				reg := RegisterDef{
					Name:    getString(rm, "name"),
					Address: getInt(rm, "address", 0),
					Type:    getString(rm, "type"),
					Count:   getInt(rm, "count", 1),
					Scale:   getFloat(rm, "scale", 1),
					Unit:    getString(rm, "unit"),
				}
				dev.Registers = append(dev.Registers, reg)
			}
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

func getFloat(m map[string]any, key string, def float64) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return def
}
