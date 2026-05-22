# PRD: Noroi — Go Configurable Load Test Backend Server

**Document version:** 1.0  
**Status:** Ready for implementation  
**Stack:** Go (Golang)

---

## 1. Overview

### 1.1 Purpose

Build a standalone HTTP backend server written in Go, specifically designed to be the **target** of load and performance tests. It simulates real-world backend behavior — response delays, variable body sizes, error rates, CPU work, streaming — all controlled through request parameters. It also exposes metrics so the operator can observe behavior and compare test runs.

### 1.2 Goals

- Give load testing tools (k6, wrk, hey, ab, Locust, etc.) a realistic, controllable target
- Require zero external dependencies to run (single binary)
- Be trivially deployable: local, Docker, Kubernetes
- Produce Prometheus-compatible metrics consumable by Grafana or similar
- Be entirely stateless (except in-memory metrics counters)

### 1.3 Non-Goals

- Not a proxy or reverse proxy
- Not a persistent data store
- Not a public-facing production service
- Not a full APM or tracing solution

---

## 2. Tech Stack

| Concern | Choice | Notes |
|---|---|---|
| Language | Go 1.22+ | Single binary, great concurrency |
| HTTP Router | `chi` (github.com/go-chi/chi) | Lightweight, idiomatic |
| Metrics | `prometheus/client_golang` | Standard, Grafana-compatible |
| Logging | `zerolog` (github.com/rs/zerolog) | Structured, zero-alloc |
| Config | `viper` (github.com/spf13/viper) | File + env + flags layering |
| Body generation | Pre-allocated buffer pool | Avoid per-request allocations |
| Testing | Standard `testing` + `httptest` | No external test framework needed |

---

## 3. Project Structure

```
noroi/
├── cmd/
│   └── server/
│       └── main.go               # Entry point, flag parsing, server startup
├── internal/
│   ├── config/
│   │   └── config.go             # Config struct, loader (file + env + flags)
│   ├── handler/
│   │   ├── respond.go            # Main configurable endpoint
│   │   ├── echo.go               # Echo endpoint
│   │   ├── stream.go             # Chunked streaming endpoint
│   │   ├── health.go             # Health check endpoint
│   │   └── scenario.go           # Scenario store + replay
│   ├── middleware/
│   │   ├── metrics.go            # Prometheus instrumentation middleware
│   │   ├── logger.go             # Request logging middleware
│   │   └── ratelimit.go          # Optional in-server rate limiter
│   ├── metrics/
│   │   └── registry.go           # Prometheus registry, metric definitions
│   ├── body/
│   │   └── generator.go          # Response body generation (size, content type)
│   └── scenario/
│       └── store.go              # In-memory scenario store (map + RWMutex)
├── .noroi.yaml                   # Default config file (checked into repo)
├── Dockerfile                    # Multi-stage build → minimal final image
├── docker-compose.yml            # Server + Prometheus + Grafana stack
├── .env.example
├── go.mod
├── go.sum
└── README.md
```

---

## 4. Configuration

### 4.1 Config Layers (lowest → highest priority)

```
.noroi.yaml → environment variables → CLI flags → query parameters (per-request)
```

### 4.2 `.noroi.yaml` Schema

```yaml
server:
  port: 8080
  read_timeout: 10s
  write_timeout: 30s
  idle_timeout: 60s

defaults:
  delay: 0ms
  jitter: 0ms
  status_code: 200
  body_size: 256        # bytes
  body_type: text       # text | json | binary | zeros
  error_rate: 0.0       # 0.0 – 1.0

metrics:
  enabled: true
  path: /metrics

logging:
  level: info           # debug | info | warn | error
  format: json          # json | pretty
```

### 4.3 Environment Variable Mapping

Each config key maps to an env var with the prefix `NOROI_`:

```
NOROI_SERVER_PORT=8080
NOROI_DEFAULTS_DELAY=100ms
NOROI_DEFAULTS_ERROR_RATE=0.05
NOROI_METRICS_ENABLED=true
```

### 4.4 CLI Flags

```
--port         int     HTTP port (default: 8080)
--delay        string  Default response delay (default: "0ms")
--error-rate   float   Default error rate 0.0–1.0 (default: 0)
--log-level    string  Log level (default: "info")
--config       string  Path to .noroi.yaml (default: "./.noroi.yaml")
```

---

## 5. Endpoints

### 5.1 Summary Table

| Method | Path | Description |
|---|---|---|
| GET/POST | `/respond` | Main configurable endpoint |
| GET/POST | `/echo` | Returns request metadata as JSON |
| GET | `/stream` | Chunked streaming response |
| POST | `/scenario` | Save a named scenario |
| GET | `/scenario/:id` | Replay a saved scenario |
| GET | `/metrics` | Prometheus metrics |
| GET | `/metrics/json` | Metrics as plain JSON (for UI polling) |
| GET | `/health` | Always returns 200 OK |
| GET | `/ready` | Returns 200 when server is ready |
| GET | `/ui` | Simple browser dashboard (static HTML) |

---

### 5.2 `GET /respond` — Main Endpoint

This is the primary endpoint. All behavior is controlled by **query parameters**. Query parameters override config file defaults.

#### Query Parameters

| Parameter | Type | Default | Description |
|---|---|---|---|
| `delay` | duration | `0ms` | Fixed response delay. Examples: `200ms`, `1s`, `1500ms` |
| `delay_min` | duration | — | Min delay for random range. Used with `delay_max`. |
| `delay_max` | duration | — | Max delay for random range. Used with `delay_min`. |
| `jitter` | duration | `0ms` | ± random jitter added to `delay`. Example: `50ms` |
| `status` | int | `200` | HTTP status code to return. Example: `503` |
| `size` | bytes/string | `256` | Response body size. Supports units: `1kb`, `10kb`, `1mb` |
| `body_type` | string | `text` | Body content type: `text`, `json`, `binary`, `zeros` |
| `error_rate` | float | `0.0` | Fraction of requests that return HTTP 500. Example: `0.1` |
| `cpu_ms` | int | `0` | Burn CPU for N milliseconds before responding |
| `chunked` | bool | `false` | Send response as chunked transfer encoding |
| `chunk_delay` | duration | `0ms` | Delay between chunks when `chunked=true` |
| `chunks` | int | `5` | Number of chunks when `chunked=true` |
| `compress` | bool | `false` | Gzip compress the response body |
| `close` | bool | `false` | Force-close the connection after responding (no keep-alive) |

#### Behavior Rules

- If both `delay` and `delay_min`/`delay_max` are given, the range takes priority.
- `error_rate` is evaluated per-request using `math/rand`. When triggered, the server returns HTTP 500 with a JSON error body, ignoring all other params.
- `cpu_ms` runs a tight loop burning CPU **on the goroutine serving the request**, before the delay is applied.
- `size` values are capped at `100mb` to prevent abuse.
- `body_type=json` returns a valid JSON object `{"data": "<random_string>"}` padded to the requested size.
- `body_type=binary` returns random bytes with `Content-Type: application/octet-stream`.
- `body_type=zeros` returns zero bytes (highly compressible, good for testing compression overhead).

#### Example Requests

```bash
# 300ms delay, 10kb body, 200 OK
curl "http://localhost:8080/respond?delay=300ms&size=10kb"

# Random delay 100–500ms, 5% error rate
curl "http://localhost:8080/respond?delay_min=100ms&delay_max=500ms&error_rate=0.05"

# Simulate overloaded server: 503, 2s delay
curl "http://localhost:8080/respond?status=503&delay=2s"

# CPU-heavy response: burn 100ms CPU, then respond
curl "http://localhost:8080/respond?cpu_ms=100&size=1kb"
```

#### Response Headers (always included)

```
X-Request-ID: <uuid>
X-Simulated-Delay: 300ms
X-Body-Size: 10240
X-Timestamp: <unix_ms>
```

---

### 5.3 `GET /echo` — Echo Endpoint

Returns a JSON object describing the incoming request. Useful for verifying load test tool behavior.

#### Response Body

```json
{
  "method": "GET",
  "path": "/echo",
  "query": { "foo": "bar" },
  "headers": { "User-Agent": "k6/0.49.0", "..." : "..." },
  "remote_addr": "127.0.0.1:54321",
  "content_length": 0,
  "body": "",
  "timestamp_ms": 1716300000000,
  "server_id": "noroi-01"
}
```

---

### 5.4 `GET /stream` — Chunked Streaming Endpoint

Sends a response in chunks with a configurable delay between each chunk.

#### Query Parameters

| Parameter | Type | Default | Description |
|---|---|---|---|
| `chunks` | int | `10` | Number of chunks to send |
| `chunk_size` | bytes/string | `256` | Size of each chunk |
| `chunk_delay` | duration | `100ms` | Delay between chunks |
| `status` | int | `200` | HTTP status code |

---

### 5.5 `POST /scenario` — Save Scenario

Saves a named set of parameters as a reusable scenario. Stored in memory only (lost on restart).

#### Request Body

```json
{
  "id": "heavy-load",
  "description": "Simulates a slow DB query under load",
  "params": {
    "delay_min": "200ms",
    "delay_max": "800ms",
    "size": "5kb",
    "body_type": "json",
    "error_rate": 0.02
  }
}
```

#### Response: `201 Created`

```json
{ "id": "heavy-load", "replay_url": "/scenario/heavy-load" }
```

---

### 5.6 `GET /scenario/:id` — Replay Scenario

Behaves exactly like `/respond` with the saved scenario's params. Any query param passed at replay time overrides the saved value.

---

### 5.7 `GET /metrics` — Prometheus Metrics

Standard Prometheus text format. Mount this as a scrape target in `prometheus.yml`.

#### Exposed Metrics

| Metric | Type | Description |
|---|---|---|
| `noroi_requests_total` | Counter | Total requests, labels: `method`, `path`, `status` |
| `noroi_request_duration_seconds` | Histogram | Full request latency, labels: `method`, `path` |
| `noroi_simulated_delay_seconds` | Histogram | Delay actually applied (before response) |
| `noroi_response_bytes_total` | Counter | Total bytes sent in response bodies |
| `noroi_in_flight_requests` | Gauge | Current number of in-flight requests |
| `noroi_errors_total` | Counter | Total simulated error responses |
| `noroi_cpu_burn_duration_seconds` | Histogram | Time spent in CPU burn |

---

### 5.8 `GET /metrics/json` — JSON Metrics Snapshot

Returns current metric values as JSON. Intended for the `/ui` dashboard to poll.

```json
{
  "snapshot_ms": 1716300000000,
  "requests_total": 15420,
  "errors_total": 154,
  "in_flight": 12,
  "rps_1m": 256.4,
  "latency_p50_ms": 201,
  "latency_p90_ms": 480,
  "latency_p99_ms": 812,
  "bytes_sent_total": 15790080
}
```

---

### 5.9 `GET /ui` — Browser Dashboard

A minimal single-page dashboard served as a static HTML file embedded in the binary (using Go `embed`). It polls `/metrics/json` every second and displays:

- Live requests/sec graph (last 60 seconds)
- Error rate %
- Latency p50 / p90 / p99 gauges
- In-flight requests counter
- Total requests and bytes sent

No external JS frameworks. Use vanilla JS + Canvas or a small charting lib embedded inline.

---

### 5.10 `GET /health` and `GET /ready`

- `/health` — Always returns `200 OK` with `{"status": "ok"}`. Never fails.
- `/ready` — Returns `200 OK` once the server has fully started (all middleware wired, metrics registered). Returns `503` during startup.

---

## 6. Middleware Stack

Applied in this order (outermost → innermost):

```
Request In
    │
    ▼
[1] Logger Middleware          — log method, path, remote addr
    │
    ▼
[2] Request ID Middleware      — generate/attach X-Request-ID
    │
    ▼
[3] Metrics Middleware         — start timer, increment in-flight gauge
    │
    ▼
[4] Recover Middleware         — catch panics, return 500
    │
    ▼
[5] Handler (respond/echo/...) — apply delay, cpu burn, generate body
    │
    ▼
[6] Metrics Middleware         — record duration, status, bytes on response
    │
    ▼
Response Out
```

---

## 7. Body Generation

The body generator must be **fast and allocation-efficient**.

### Requirements

- Maintain a **sync.Pool** of byte buffers for common sizes (1kb, 4kb, 16kb, 64kb, 256kb, 1mb)
- For sizes not matching a pool bucket, allocate once and discard
- `text` type: alphanumeric ASCII, pre-generated 64kb block, sliced/repeated to size
- `json` type: valid JSON, padded with a `"padding"` field to reach target size
- `binary` type: use `crypto/rand` seeded source (not crypto-secure, just random-looking)
- `zeros` type: `bytes.Repeat([]byte{0}, size)` — highly compressible

---

## 8. Error Handling

| Scenario | Behavior |
|---|---|
| Invalid `delay` param | Return `400 Bad Request` with JSON error |
| Invalid `status` param (not 100–599) | Return `400 Bad Request` |
| `size` exceeds 100mb cap | Return `400 Bad Request` |
| `error_rate` not in 0.0–1.0 | Return `400 Bad Request` |
| Unknown scenario ID | Return `404 Not Found` |
| Panic in handler | Recover middleware catches, returns `500`, logs stack trace |

All error responses use this JSON envelope:

```json
{
  "error": true,
  "code": 400,
  "message": "invalid delay: 'abc' is not a valid duration",
  "request_id": "a1b2c3d4"
}
```

---

## 9. Deployment

### 9.1 Binary

```bash
go build -o noroi ./cmd/server
./noroi --port 8080 --log-level info
```

### 9.2 Dockerfile (Multi-Stage)

```dockerfile
# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o noroi ./cmd/server

# Final stage
FROM scratch
COPY --from=builder /app/noroi /noroi
EXPOSE 8080
ENTRYPOINT ["/noroi"]
```

### 9.3 Docker Compose (Full Observability Stack)

`docker-compose.yml` should bring up:

1. `noroi` — the app itself on port `8080`
2. `prometheus` — scraping `noroi:8080/metrics` every 5s
3. `grafana` — pre-provisioned with a dashboard for `noroi_*` metrics

Include a pre-built Grafana dashboard JSON in `./grafana/dashboards/noroi.json` covering all `noroi_*` metrics.

---

## 10. Testing Requirements

| Layer | Tool | Coverage Target |
|---|---|---|
| Unit | `testing` + `httptest` | All handlers, param parsing, body generator |
| Integration | `httptest.Server` | Full request lifecycle per endpoint |
| Concurrency | `-race` flag | All tests must pass with `-race` |
| Benchmarks | `testing.B` | `BenchmarkNoroiRespond` with varying sizes and delays |

Key test cases:
- `/respond` with all combinations of params
- `error_rate=1.0` always returns 500
- `delay=500ms` actually delays ≥ 500ms
- Body size matches `size` param within ±5 bytes
- Metrics counters increment correctly under concurrent load
- Scenario save → replay produces identical behavior

---

## 11. Implementation Notes & Constraints

1. **No global mutable state** except the metrics registry and scenario store (both must be safe for concurrent access).
2. **The delay must be interruptible** — use `time.After` inside a `select` with the request context, so client disconnects don't hold goroutines.
3. **CPU burn** must be a tight loop checking `time.Now()`, not `time.Sleep`. Use `runtime.Gosched()` occasionally to avoid starving the scheduler.
4. **All durations** in query params follow Go's `time.ParseDuration` format: `ms`, `s`, `m`.
5. **Size strings** support both raw integers (bytes) and suffixed strings: `kb`, `mb` (case-insensitive).
6. The `/ui` HTML must be embedded using `//go:embed` so the binary is fully self-contained.
7. Server must handle `SIGTERM` and `SIGINT` with a **graceful shutdown**: stop accepting new connections, wait up to 15s for in-flight requests to complete.
8. Log lines must include `request_id`, `method`, `path`, `status`, `duration_ms`, `body_bytes`.

---

## 12. Acceptance Criteria

- [ ] Single `go build` produces a working binary with no external runtime dependencies
- [ ] `GET /respond?delay=200ms&size=5kb` returns a `~5kb` body after `~200ms`
- [ ] `GET /respond?error_rate=1.0` always returns HTTP 500
- [ ] `GET /respond?delay_min=100ms&delay_max=300ms` returns within that range (verified over 100 requests)
- [ ] `GET /metrics` returns valid Prometheus text format parseable by `promtool`
- [ ] `GET /metrics/json` returns correct live counters
- [ ] `POST /scenario` + `GET /scenario/:id` replays correctly
- [ ] `GET /ui` loads in browser and shows live data
- [ ] All tests pass with `go test -race ./...`
- [ ] Docker image builds and runs with `docker compose up`
- [ ] Server shuts down gracefully on `SIGTERM` (in-flight requests complete)
