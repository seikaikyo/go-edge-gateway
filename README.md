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

# The scan API needs a token; without one /api refuses every request.
export EDGE_GATEWAY_API_TOKEN=$(openssl rand -hex 24)

# Run with config (binds 127.0.0.1 unless --listen says otherwise)
./edge-gateway --config edge-gateway.yaml

# Check health (unauthenticated)
curl http://localhost:8080/health

# Call the scan API
curl -H "Authorization: Bearer $EDGE_GATEWAY_API_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"mode":"tcp","host":"192.168.1.200","port":502}' \
     http://localhost:8080/api/scan/quick
```

## Scan API Security

The scan API drives outbound Modbus connections, so it is closed by default:

| Control | Default | Where to change |
|---|---|---|
| Bind address | `127.0.0.1` | `--listen`, `EDGE_GATEWAY_LISTEN`, or `api.listen` |
| `/api/*` authentication | Bearer token, required | `EDGE_GATEWAY_API_TOKEN` (name via `api.token_env`) |
| Scan target range | Loopback plus RFC1918 | `scan.allowed_targets` (CIDR list) |
| RTU `serial_port` | Devices the host enumerates | not configurable |
| Browser origins | Same-origin only | `api.allowed_origins` |

Notes:

- No token means `/api/*` answers 503. It never runs unauthenticated.
- Link-local (`169.254.0.0/16`, `fe80::/10`) is refused whatever `allowed_targets` says.
- State-changing requests must send `Content-Type: application/json`.
- Device connection errors come back as a fixed message; the cause is in the log
  only, so the API cannot be used to map an internal network.

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
