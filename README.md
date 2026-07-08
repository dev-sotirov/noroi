# Noroi (呪い)

> *Slow and cursed by design.*

[![CI](https://github.com/dev-sotirov/noroi/actions/workflows/ci.yml/badge.svg)](https://github.com/dev-sotirov/noroi/actions/workflows/ci.yml)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)

A configurable HTTP backend for load and performance testing. Point your load testing tool at it and control exactly how it behaves — latency, body size, error rate, and more — all via query parameters.

---

## Table of Contents

- [Quick Start](#quick-start)
- [Installation](#installation)
- [Usage](#usage)
- [Endpoints](#endpoints)
- [Scenarios](#scenarios)
- [Configuration](#configuration)
- [Metrics](#metrics)
- [Contributing](#contributing)
- [Releases](#releases)
- [License](#license)

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

## Installation

### Download a binary

Grab a prebuilt binary for Linux, macOS, or Windows (`amd64`/`arm64`) from the [Releases page](https://github.com/dev-sotirov/noroi/releases). Each release also includes a `SHA256SUMS` file for verification:

```bash
curl -LO https://github.com/dev-sotirov/noroi/releases/latest/download/noroi_<version>_linux_amd64.tar.gz
tar -xzf noroi_<version>_linux_amd64.tar.gz
./noroi --port 8080
```

### Docker

Multi-platform images (`linux/amd64`, `linux/arm64`) are published to GHCR on every release:

```bash
docker run -p 8080:8080 ghcr.io/dev-sotirov/noroi:<version>
```

### Go install

```bash
go install github.com/dev-sotirov/noroi/cmd/server@latest
```

### Build from source

```bash
git clone https://github.com/dev-sotirov/noroi.git
cd noroi
go build -o noroi ./cmd/server
```

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

| Parameter                 | Type     | Default | Description                                        |
| -------------------------- | -------- | ------- | --------------------------------------------------- |
| `delay`                   | duration | `0ms`   | Fixed response delay                                |
| `delay_min` / `delay_max` | duration | —       | Random delay range                                  |
| `jitter`                  | duration | `0ms`   | ± random jitter on top of delay                     |
| `status`                  | int      | `200`   | HTTP status code to return                          |
| `size`                    | string   | `256`   | Response body size (`256`, `10kb`, `1mb`)           |
| `body_type`               | string   | `text`  | Body content: `text`, `json`, `binary`, `zeros`     |
| `error_rate`              | float    | `0.0`   | Fraction of requests that return 500 (e.g. `0.1`)   |
| `cpu_ms`                  | int      | `0`     | Milliseconds of CPU work before responding          |
| `chunked`                 | bool     | `false` | Send as chunked transfer encoding                   |
| `chunk_delay`              | duration | `0ms`   | Delay between chunks                                |
| `compress`                | bool     | `false` | Gzip compress the response                          |

---

## Endpoints

| Method     | Path            | Description                   |
| ---------- | --------------- | ------------------------------ |
| `GET/POST` | `/respond`      | Main configurable endpoint     |
| `GET/POST` | `/echo`         | Returns request info as JSON   |
| `GET`      | `/stream`       | Chunked streaming response     |
| `POST`     | `/scenario`     | Save a named scenario          |
| `GET`      | `/scenario/:id` | Replay a saved scenario        |
| `GET`      | `/metrics`      | Prometheus metrics              |
| `GET`      | `/metrics/json` | Metrics snapshot as JSON        |
| `GET`      | `/health`       | Always 200 OK                    |
| `GET`      | `/ui`           | Live dashboard                  |

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

Config is loaded from `.noroi.yaml`, then overridden by environment variables (`NOROI_*`), then CLI flags, then query parameters — each layer overrides the one before it.

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

For example, `NOROI_SERVER_PORT=9000` overrides `server.port` from the YAML file, and `?status=503` on a request overrides everything for that single call.

---

## Metrics

Noroi exposes Prometheus metrics at `/metrics`:

| Metric                            | Type      |
| ---------------------------------- | --------- |
| `noroi_requests_total`            | Counter   |
| `noroi_request_duration_seconds`  | Histogram |
| `noroi_simulated_delay_seconds`   | Histogram |
| `noroi_response_bytes_total`      | Counter   |
| `noroi_in_flight_requests`        | Gauge     |
| `noroi_errors_total`              | Counter   |

A `docker compose up` stack includes Prometheus and Grafana pre-wired to scrape these.

---

## Contributing

Contributions are welcome! The project uses a standard fork-and-PR workflow:

1. Fork the repo and create a branch off `main`.
2. Make your changes — run `gofmt`, `go vet`, and `go test ./...` locally before pushing.
3. Open a PR against `main`. CI (formatting, tests, vet, lint, build) runs automatically on every PR.

`main` always reflects the latest merged work. Official releases are cut separately — see [Releases](#releases) below — so merging to `main` doesn't publish anything on its own.

### Development

```bash
just # list available recipes
```

See the `Justfile` for common dev tasks (build, test, lint, run).

---

## Releases

Noroi does not maintain long-term support branches for older versions — if you hit a bug, please update to the latest release, as fixes are only made going forward.

Releases are promoted deliberately from `main` to a `release` branch via PR, then tagged. Publishing a GitHub Release against that tag triggers the release pipeline, which:

- Builds binaries for Linux, macOS, and Windows (`amd64`/`arm64`) and attaches them to the release along with a `SHA256SUMS` checksum file.
- Publishes a multi-platform image to GHCR:

  ```
  ghcr.io/dev-sotirov/noroi:<version>
  ```

---

## License

[GNU GPL v3](https://github.com/dev-sotirov/noroi/blob/main/LICENSE)
