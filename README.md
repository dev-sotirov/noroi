# Noroi (呪い)

> _Slow and cursed by design._

A configurable HTTP backend for load and performance testing. Point your load testing tool at it and control exactly how it behaves — latency, body size, error rate, and more — all via query parameters.

---

## Quick Start

```bash
go build -o noroi ./cmd/server
./noroi --port 8080
```

Or with Docker:

```bash
docker compose up
```

This starts Noroi on `:8080`, Prometheus on `:9090`, and Grafana on `:3000`.

---

## Usage

All behavior is controlled via query parameters on the `/respond` endpoint.

```bash
# 300ms delay, 10kb response body
curl "http://localhost:8080/respond?delay=300ms&size=10kb"

# Random delay between 100ms and 500ms, 5% error rate
curl "http://localhost:8080/respond?delay_min=100ms&delay_max=500ms&error_rate=0.05"

# Always return 503
curl "http://localhost:8080/respond?status=503"

# Burn 100ms of CPU before responding
curl "http://localhost:8080/respond?cpu_ms=100"
```

### Parameters

| Parameter                 | Type     | Default | Description                                       |
| ------------------------- | -------- | ------- | ------------------------------------------------- |
| `delay`                   | duration | `0ms`   | Fixed response delay                              |
| `delay_min` / `delay_max` | duration | —       | Random delay range                                |
| `jitter`                  | duration | `0ms`   | ± random jitter on top of delay                   |
| `status`                  | int      | `200`   | HTTP status code to return                        |
| `size`                    | string   | `256`   | Response body size (`256`, `10kb`, `1mb`)         |
| `body_type`               | string   | `text`  | Body content: `text`, `json`, `binary`, `zeros`   |
| `error_rate`              | float    | `0.0`   | Fraction of requests that return 500 (e.g. `0.1`) |
| `cpu_ms`                  | int      | `0`     | Milliseconds of CPU work before responding        |
| `chunked`                 | bool     | `false` | Send as chunked transfer encoding                 |
| `chunk_delay`             | duration | `0ms`   | Delay between chunks                              |
| `compress`                | bool     | `false` | Gzip compress the response                        |

---

## Endpoints

| Method     | Path            | Description                  |
| ---------- | --------------- | ---------------------------- |
| `GET/POST` | `/respond`      | Main configurable endpoint   |
| `GET/POST` | `/echo`         | Returns request info as JSON |
| `GET`      | `/stream`       | Chunked streaming response   |
| `POST`     | `/scenario`     | Save a named scenario        |
| `GET`      | `/scenario/:id` | Replay a saved scenario      |
| `GET`      | `/metrics`      | Prometheus metrics           |
| `GET`      | `/metrics/json` | Metrics snapshot as JSON     |
| `GET`      | `/health`       | Always 200 OK                |
| `GET`      | `/ui`           | Live dashboard               |

---

## Scenarios

Save a set of params once, replay by ID:

```bash
# Save
curl -X POST http://localhost:8080/scenario \
  -H "Content-Type: application/json" \
  -d '{"id":"slow-db","params":{"delay_min":"200ms","delay_max":"800ms","error_rate":0.02}}'

# Replay
curl "http://localhost:8080/scenario/slow-db"
```

---

## Configuration

Config is loaded from `.noroi.yaml`, then overridden by env vars (`NOROI_*`), then CLI flags, then query params.

```yaml
server:
  port: 8080

defaults:
  delay: 0ms
  status_code: 200
  body_size: 256
  error_rate: 0.0

metrics:
  enabled: true
```

---

## Metrics

Noroi exposes Prometheus metrics at `/metrics`:

| Metric                           | Type      |
| -------------------------------- | --------- |
| `noroi_requests_total`           | Counter   |
| `noroi_request_duration_seconds` | Histogram |
| `noroi_simulated_delay_seconds`  | Histogram |
| `noroi_response_bytes_total`     | Counter   |
| `noroi_in_flight_requests`       | Gauge     |
| `noroi_errors_total`             | Counter   |

---

## License

[GNU GPL v3](LICENSE)
