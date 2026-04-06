# winet2mqtt

A lightweight bridge that connects to Sungrow WiNet-S/S2 solar inverter dongles and publishes sensor data to an MQTT broker.

## Features

- Connects to WiNet dongles via WebSocket (auto-detects HTTP/HTTPS)
- Discovers all connected devices (inverters, meters, batteries, etc.)
- Polls realtime metrics, MPPT string data, and battery data
- Publishes to MQTT with clean topic structure
- Health check endpoint for container orchestration
- Minimal Docker image (~5MB, distroless)
- Graceful shutdown with proper cleanup

## Quick Start

### Docker (recommended)

```bash
docker run -d \
  --name winet2mqtt \
  -e WINET2MQTT_WINET_HOST=192.168.1.100 \
  -e WINET2MQTT_MQTT_URL=tcp://mqtt-broker:1883 \
  -p 8080:8080 \
  ghcr.io/<your-org>/winet2mqtt:latest
```

### Docker Compose

```yaml
services:
  winet2mqtt:
    image: ghcr.io/<your-org>/winet2mqtt:latest
    restart: unless-stopped
    environment:
      WINET2MQTT_WINET_HOST: "192.168.1.100"
      WINET2MQTT_MQTT_URL: "tcp://mqtt-broker:1883"
      # WINET2MQTT_MQTT_USERNAME: ""
      # WINET2MQTT_MQTT_PASSWORD: ""
      # WINET2MQTT_POLL_INTERVAL: "10"
    ports:
      - "8080:8080"
```

### Binary

```bash
winet2mqtt server \
  --winet-host 192.168.1.100 \
  --mqtt-url tcp://localhost:1883
```

## Configuration

All flags can be set via environment variables with the `WINET2MQTT_` prefix. Dashes become underscores.

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--winet-host` | `WINET2MQTT_WINET_HOST` | *(required)* | WiNet dongle IP or hostname |
| `--winet-username` | `WINET2MQTT_WINET_USERNAME` | `admin` | WiNet username |
| `--winet-password` | `WINET2MQTT_WINET_PASSWORD` | `pw8888` | WiNet password |
| `--mqtt-url` | `WINET2MQTT_MQTT_URL` | *(required)* | MQTT broker URL (`tcp://host:1883`) |
| `--mqtt-username` | `WINET2MQTT_MQTT_USERNAME` | | MQTT username |
| `--mqtt-password` | `WINET2MQTT_MQTT_PASSWORD` | | MQTT password |
| `--mqtt-prefix` | `WINET2MQTT_MQTT_PREFIX` | `homeassistant` | MQTT topic prefix (HA discovery) |
| `--poll-interval` | `WINET2MQTT_POLL_INTERVAL` | `10` | Poll interval in seconds (1-3600) |
| `--health-port` | `WINET2MQTT_HEALTH_PORT` | `8080` | Health check HTTP port |

## MQTT Topics (Home Assistant Discovery)

Uses the [HA MQTT discovery](https://www.home-assistant.io/integrations/mqtt/#mqtt-discovery) protocol. Devices and sensors are auto-registered in Home Assistant when the MQTT integration is enabled.

```
<prefix>/sensor/<MODEL>_<SERIAL>/config                    → Device registration (retained)
<prefix>/sensor/<MODEL>_<SERIAL>/<sensor-slug>/config      → Sensor config with device_class, unit, etc. (retained)
<prefix>/sensor/<MODEL>_<SERIAL>/<sensor-slug>/state       → {"value": 3.14, "unit_of_measurement": "kW"}
```

Example with default prefix `homeassistant`:
```
homeassistant/sensor/SH10RS_A2582008920/config
homeassistant/sensor/SH10RS_A2582008920/daily_pv_yield/config
homeassistant/sensor/SH10RS_A2582008920/daily_pv_yield/state → {"value": 12.5, "unit_of_measurement": "kWh"}
```

Numeric sensors include `device_class` (power, voltage, current, energy, temperature, frequency) and `state_class` (measurement or total_increasing) for proper HA energy dashboard integration.

## Health Checks

```
GET /healthz  → {"status":"ok","checks":{"mqtt":{"status":"ok"},"winet":{"status":"ok"}}}
GET /readyz   → (same)
```

Returns `200` when healthy, `503` when any check fails.

## Building

```bash
make build    # Build binary
make test     # Run tests
make lint     # Run linter
make docker   # Build Docker image
```

## Acknowledgements

This project was inspired by [winet-extractor](https://github.com/MichaelEFlip/winet-extractor) by MichaelEFlip, whose work on reverse-engineering the WiNet WebSocket protocol made this possible. Thank you!

## License

[MIT](LICENSE)
