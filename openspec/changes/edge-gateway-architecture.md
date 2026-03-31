---
title: Edge Gateway Plugin Architecture
type: feature
status: in-progress
created: 2026-04-01
---

# Edge Gateway Plugin Architecture

## Background

Taiwan manufacturing reality:
- Most factories use Modbus TCP/RS-485, some use MQTT
- SECS/GEM is semiconductor-only
- OPC UA is rare (budget-dependent)
- Typical setup: device → Windows PC (C#/VB6) → Excel report

This gateway replaces the Windows PC with a single Go binary that speaks
multiple industrial protocols via a plugin system.

## Architecture

```
                    ┌─────────────────────────────────┐
                    │         edge-gateway             │
                    │                                  │
                    │  ┌───────────────────────────┐   │
                    │  │          core              │   │
                    │  │  config / log / health /   │   │
                    │  │  plugin registry / uplink  │   │
                    │  └─────────┬─────────────────┘   │
                    │            │                      │
                    │  ┌─────────┼─────────────────┐   │
                    │  │  plugin.Register(name, fn) │   │
                    │  └─────────┼─────────────────┘   │
                    │            │                      │
                    │  ┌─────┐ ┌─────┐ ┌──────┐       │
                    │  │SECS │ │Mod- │ │MQTT  │  ...   │
                    │  │/GEM │ │bus  │ │      │        │
                    │  └──┬──┘ └──┬──┘ └──┬───┘       │
                    └─────┼───────┼───────┼────────────┘
                          │       │       │
                     HSMS TCP  Modbus TCP  MQTT broker
                          │       │       │
                      半導體設備  PLC/感測器  IoT 設備
```

## Core Module

Responsibilities:
- YAML config loading and validation
- Structured logging (slog)
- Plugin lifecycle management (init → start → stop → cleanup)
- Health endpoint (HTTP /health)
- Uplink interface (plugins report data through core, core decides where to send)
- Graceful shutdown (SIGTERM/SIGINT)
- Metrics collection (per-plugin message count, error count, uptime)

### Plugin Interface

```go
// plugin/plugin.go

type Message struct {
    Source    string            // plugin name
    Device   string            // device identifier
    Topic    string            // data category
    Payload  map[string]any    // actual data
    Ts       time.Time
}

type Plugin interface {
    // Name returns the plugin identifier (e.g. "secsgem", "modbus")
    Name() string

    // Init receives plugin-specific config and the uplink channel
    Init(cfg map[string]any, uplink chan<- Message) error

    // Start begins device communication (blocking, run in goroutine)
    Start(ctx context.Context) error

    // Stop gracefully shuts down
    Stop() error

    // Health returns plugin health status
    Health() HealthStatus
}

type HealthStatus struct {
    OK          bool
    Devices     int
    ActiveConns int
    LastError   string
    LastSeen    time.Time
}
```

### Config Format

```yaml
# edge-gateway.yaml

gateway:
  name: "fab-3-line-a"
  log_level: info        # debug / info / warn / error
  health_port: 8080

uplink:
  type: mqtt             # mqtt / stdout / file
  mqtt:
    broker: "tcp://10.0.0.1:1883"
    topic_prefix: "factory/fab3/lineA"
    qos: 1

plugins:
  secsgem:
    enabled: true
    devices:
      - name: "etcher-01"
        host: 192.168.1.100
        port: 5000
        mode: passive      # active / passive
        device_id: 1
      - name: "etcher-02"
        host: 192.168.1.101
        port: 5000
        mode: passive
        device_id: 1

  modbus:
    enabled: true
    devices:
      - name: "temp-sensor-rack"
        host: 192.168.1.200
        port: 502
        unit_id: 1
        poll_interval: 1s
        registers:
          - name: "temperature"
            address: 0
            type: holding
            count: 1
            scale: 0.1      # raw * 0.1 = actual value
            unit: "celsius"
          - name: "humidity"
            address: 1
            type: holding
            count: 1
            scale: 0.1
            unit: "%"

  mqtt:
    enabled: false
    broker: "tcp://192.168.1.50:1883"
    subscriptions:
      - topic: "machines/+/status"
        device_pattern: "machines/{device}/status"
      - topic: "sensors/#"
        device_pattern: "sensors/{+}"
```

## Plugin 1: SECS/GEM

Import go-factory-io as a Go module. Wraps the existing HSMS driver.

| Item | Detail |
|------|--------|
| Dependency | `github.com/dashfactory/go-factory-io` |
| Protocol | HSMS-SS (TCP) |
| Features | S1F1/F2 online, S1F13/F14 establish, S6F11 event report |
| Config | host, port, mode (active/passive), device_id |
| Message mapping | SECS-II message → `plugin.Message{Topic: "secsgem/event", Payload: ...}` |

Key difference from go-factory-io standalone: here it runs as a plugin
managed by core, not as its own CLI process.

## Plugin 2: Modbus TCP

| Item | Detail |
|------|--------|
| Library | `github.com/goburrow/modbus` (mature, MIT) |
| Protocol | Modbus TCP (port 502) |
| Features | Read holding/input registers, coils; polling-based |
| Config | host, port, unit_id, poll_interval, register map |
| Message mapping | Register values → `plugin.Message{Topic: "modbus/poll", Payload: {"temperature": 25.3}}` |

Register map in YAML lets users define which addresses to read, with
human-readable names and scaling factors. No code change needed to add
new sensors — just edit YAML.

## Plugin 3: MQTT (device-side)

| Item | Detail |
|------|--------|
| Library | `github.com/eclipse/paho.mqtt.golang` (Eclipse, mature) |
| Protocol | MQTT 3.1.1 / 5.0 |
| Features | Subscribe to device topics, parse JSON/binary payloads |
| Config | broker, subscriptions with topic patterns |
| Message mapping | MQTT message → `plugin.Message{Topic: "mqtt/...", Payload: parsed}` |

Note: This is the device-side MQTT plugin (subscribing to devices that
publish via MQTT). The uplink MQTT (sending data to MES/cloud) is handled
by core's uplink module — they are separate concerns.

## Directory Structure

```
go-edge-gateway/
├── cmd/
│   └── edge-gateway/
│       └── main.go              # CLI entry point
├── core/
│   ├── config.go                # YAML config loader
│   ├── gateway.go               # Plugin lifecycle orchestrator
│   ├── health.go                # HTTP /health endpoint
│   ├── uplink.go                # Uplink interface (MQTT/stdout/file)
│   └── uplink_mqtt.go           # MQTT uplink implementation
├── plugin/
│   ├── plugin.go                # Plugin interface + Message type
│   ├── registry.go              # Plugin registration
│   ├── secsgem/
│   │   └── secsgem.go           # SECS/GEM plugin (wraps go-factory-io)
│   ├── modbus/
│   │   └── modbus.go            # Modbus TCP plugin
│   └── mqtt/
│       └── mqtt.go              # MQTT device-side plugin
├── edge-gateway.yaml            # Example config
├── openspec/
├── go.mod
├── go.sum
├── Dockerfile
├── LICENSE
└── README.md
```

## Build Tags (optional, future)

```bash
# Full build (default)
go build ./cmd/edge-gateway/

# Semiconductor only
go build -tags secsgem ./cmd/edge-gateway/

# Traditional factory only (no SECS/GEM)
go build -tags "modbus,mqtt" ./cmd/edge-gateway/
```

## Impact

| Item | Files |
|------|-------|
| New repo | `github.com/seikaikyo/go-edge-gateway` |
| New files | ~15 Go files |
| Dependencies | go-factory-io, goburrow/modbus, paho.mqtt, gopkg.in/yaml.v3 |
| Deployment | Single binary, Docker, or systemd on industrial PC |

## Implementation Plan

### Phase 1: Core + Plugin Interface
1. `plugin/plugin.go` — Plugin interface, Message type
2. `plugin/registry.go` — Registration mechanism
3. `core/config.go` — YAML config loader
4. `core/gateway.go` — Lifecycle orchestrator (init/start/stop)
5. `core/health.go` — HTTP /health
6. `core/uplink.go` — Uplink interface (stdout first, MQTT later)
7. `cmd/edge-gateway/main.go` — CLI entry

### Phase 2: Modbus Plugin (most common, easiest to test)
1. `plugin/modbus/modbus.go` — Modbus TCP polling + register mapping
2. Test with Modbus simulator (diagslave / ModRSsim2)

### Phase 3: SECS/GEM Plugin
1. `plugin/secsgem/secsgem.go` — Wrap go-factory-io HSMS driver
2. Test with go-factory-io simulator

### Phase 4: MQTT Plugin
1. `plugin/mqtt/mqtt.go` — Subscribe + parse device topics
2. Test with mosquitto local broker

### Phase 5: MQTT Uplink + Integration
1. `core/uplink_mqtt.go` — Send aggregated data to MES/cloud
2. End-to-end test: Modbus sensor → gateway → MQTT broker → subscriber

## Test Plan

| Phase | Test Method |
|-------|-------------|
| Core | Unit test: config loading, plugin lifecycle, health endpoint |
| Modbus | diagslave simulator on localhost:502 |
| SECS/GEM | go-factory-io built-in simulator |
| MQTT | mosquitto broker on localhost:1883 |
| Integration | All 3 plugins → stdout uplink, verify Message format |
| Docker | `docker build` + `docker run` with example config |

## Checklist

- [ ] Phase 1: Core + Plugin interface
- [ ] Phase 2: Modbus TCP plugin
- [ ] Phase 3: SECS/GEM plugin
- [ ] Phase 4: MQTT device plugin
- [ ] Phase 5: MQTT uplink + integration test
- [ ] README with architecture diagram
- [ ] Dockerfile
- [ ] Example edge-gateway.yaml
- [ ] CI: GitHub Actions (build + test)
