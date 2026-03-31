# go-edge-gateway

Modular Edge Gateway for manufacturing environments. A single Go binary that
bridges industrial devices to MES/cloud systems via a plugin architecture.

## Architecture

```
                  edge-gateway (single binary)
          ┌──────────┬──────────┬──────────┐
          │ SECS/GEM │ Modbus   │  MQTT    │  ... plugins
          │ plugin   │ plugin   │  plugin  │
          └────┬─────┴────┬─────┴────┬─────┘
               │          │          │
          HSMS TCP    Modbus TCP   MQTT 3.1.1
               │          │          │
          Semiconductor  PLC /     IoT
          Equipment    Sensors    Devices
```

Plugins are independently enabled via YAML config. No code change needed
to add devices — just edit the config file.

## Plugins

| Plugin | Protocol | Use Case | Status |
|--------|----------|----------|--------|
| **secsgem** | HSMS-SS (SEMI E37) | Semiconductor equipment | Planned |
| **modbus** | Modbus TCP | PLC, sensors, traditional factory | Planned |
| **mqtt** | MQTT 3.1.1/5.0 | IoT devices with native MQTT | Planned |

## Quick Start

```bash
# Build
go build -o edge-gateway ./cmd/edge-gateway/

# Run with config
./edge-gateway --config edge-gateway.yaml

# Check health
curl http://localhost:8080/health
```

## Config Example

```yaml
gateway:
  name: "fab-3-line-a"
  log_level: info
  health_port: 8080

uplink:
  type: stdout    # stdout / mqtt / file

plugins:
  modbus:
    enabled: true
    devices:
      - name: "temp-sensor"
        host: 192.168.1.200
        port: 502
        unit_id: 1
        poll_interval: 1s
        registers:
          - name: "temperature"
            address: 0
            type: holding
            scale: 0.1
            unit: "celsius"

  secsgem:
    enabled: false

  mqtt:
    enabled: false
```

## Build Tags

```bash
# Full build (all plugins)
go build ./cmd/edge-gateway/

# Semiconductor only
go build -tags secsgem ./cmd/edge-gateway/

# Traditional factory (no SECS/GEM)
go build -tags "modbus,mqtt" ./cmd/edge-gateway/
```

## Development

This project follows spec-driven development with OpenSpec.
See `openspec/changes/` for current plans.

AI-assisted development with Claude.

## License

MIT
