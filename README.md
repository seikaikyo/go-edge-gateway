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
to add devices, just edit the config file.

## Plugins

| Plugin | Protocol | Use Case | Status |
|--------|----------|----------|--------|
| **secsgem** | HSMS-SS (SEMI E37) | Semiconductor equipment | Implemented |
| **modbus** | Modbus TCP | PLC, sensors, traditional factory | Implemented |
| **mqtt** | MQTT 3.1.1/5.0 | IoT devices with native MQTT | Implemented |

Each plugin ships with tests (`plugin/*/`).

## Modbus Scanner

The former `go-modbus-scanner` is merged into this binary under
`internal/scan/`: a device-discovery API plus an embedded React UI served
from the same port. Scan a Modbus network, inspect register reads, then
export the result directly into gateway plugin config.

Scanner endpoints (mounted under `/api`):

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/scan` | Start a full scan job |
| POST | `/api/scan/quick` | Quick scan |
| POST | `/api/read` | One-off register read |
| GET | `/api/jobs` | List scan jobs |
| GET | `/api/jobs/{id}` | Get a scan job |
| GET | `/api/serial/ports` | List serial ports |
| POST | `/api/scan/jobs/{id}/to-config` | Export scan result to gateway config |

The scanner UI source lives in `web/scanner-ui/` and is embedded at build time.

## Cloud Coordinator

The gateway can register with a cloud coordinator (dashai-go) on startup,
send heartbeats with plugin health and system metrics, and batch-upload
device events. Configure via the `coordinator` block in YAML.

## Quick Start

```bash
# Build
go build -o edge-gateway ./cmd/edge-gateway/

# Run with config
./edge-gateway --config edge-gateway.yaml

# Check health (also serves the scanner UI on this port)
curl http://localhost:8080/health
```

Requires Go 1.26+.

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

See `edge-gateway.yaml` for the full config and `edge-gateway.demo.yaml`
for a cloud demo profile (all plugins disabled).

## Build Tags

```bash
# Full build (all plugins)
go build ./cmd/edge-gateway/

# Semiconductor only
go build -tags secsgem ./cmd/edge-gateway/

# Traditional factory (no SECS/GEM)
go build -tags "modbus,mqtt" ./cmd/edge-gateway/
```

## Deployment

A `Dockerfile` and `render.yaml` are provided for container deployment.
The Render service uses the Docker runtime, region Singapore, Starter plan,
with `/health` as the health-check path. Push to deploy.

## Development

This project follows spec-driven development with OpenSpec.
See `openspec/changes/` for current plans.

AI-assisted development with Claude.

## License

MIT
