# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2026-05-22

### Added
- Rate limiter TTL-based eviction and max IP cap to prevent unbounded memory growth
- `RateLimit.TrustForwarded` config flag to control X-Forwarded-For trust (default: false)
- `RateLimit.TTL` config field for limiter eviction timeout (default: 5 minutes)
- Parameter bounds for production safety: `cpu_ms` (max 10s), `chunks` (max 100k), `delay`/`chunk_delay` (max 1h)
- Error logging for write/gzip/flush failures in response handlers
- Error handling for Prometheus Gather() and JSON Encode() operations
- Request body size validation with explicit 413 Payload Too Large responses
- JSON parsing with `DisallowUnknownFields` and trailing JSON detection

### Changed
- **BREAKING**: `RateLimit.TrustForwarded` defaults to false. If behind a trusted proxy, explicitly set to true in config
- Jitter calculation now produces symmetric ±range (was off-by-one)
- Response write errors now logged instead of silently suppressed
- Scenario storage now deep-clones maps to prevent external mutation

### Fixed
- Map aliasing vulnerability in scenario storage
- Unbounded rate limiter state (could grow unbounded with spoofed IPs)
- Jitter asymmetry: could produce -jitter to +jitter-1, now -jitter to +jitter
- Silent write/gzip/flush errors in response handlers
- Silent Prometheus Gather() and JSON Encode() errors
- JSON body parsing: replaced `io.LimitReader` (silent truncation) with `http.MaxBytesReader` (explicit 413)

### Security
- Rate limiter evicts old IP entries after TTL (prevents memory DoS from IP spoofing)
- X-Forwarded-For untrusted by default (requires explicit opt-in via config)
- Request parameter bounds prevent resource exhaustion attacks
- Scenario maps protected from external mutation
