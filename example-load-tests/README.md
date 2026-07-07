# Performance Testing Guide

This repository contains load testing scripts for **k6** and **Locust**. Both are configured to target the Noroi HTTP backend (defaulting to `http://localhost:8080`).

---

## 1. k6 Tests

[k6](https://k6.io/) is a modern, developer-centric performance testing tool.

### Prerequisites
- [Install k6](https://k6.io/docs/getting-started/installation/) on your machine.

### Running the Tests
To run the standard test suite:
```bash
k6 run load-tests/k6/test_noroi.js
```

### Script Details
- **Scenarios**: Fixed Delay, Random Delay, High Error Rate, and CPU Load.
- **VUs**: 10 Virtual Users per scenario.
- **Target**: `http://localhost:8080/respond`

---

## 2. Locust Tests

[Locust](https://locust.io/) is a Python-based load testing framework.

### Prerequisites
- Python 3.x installed.
- Install Locust (preferably in a virtual environment):
  ```bash
  python -m venv .venv
  source .venv/bin/activate
  pip install locust
  ```

### Running via Web UI
To start the Locust web interface:
```bash
locust -f load-tests/locust/http_user.py
```
1. Open your browser to `http://localhost:8089`.
2. Enter the host: `http://localhost:8080`.
3. Set the number of users and hatch rate.
4. Click **Start swarming**.

### Running in Headless Mode
To run without the UI for a specific duration:
```bash
locust -f load-tests/locust/fast_http_user.py --headless -u 10 -r 1 --run-time 1m
# locust -f load-tests/locust/fast_http_user.py --headless -u 20000 -r 20000 --run-time 1m --processes -1 or number of cores -1 (If the CPU is with 8 core we use --processes 7 sinse one is dedicated got the master process and 7 wonker - total 8 cores)
```

### Script Variants
- `load-tests/locust/fast_http_user.py`: Uses `FastHttpUser` (gevent-based, higher performance).
- `load-tests/locust/http_user.py`: Uses standard `HttpUser` (python-requests based).

---

## Test Scenarios Summary

| Scenario | Parameters | Description |
| :--- | :--- | :--- |
| **Fixed Delay** | `delay=300ms&size=10kb` | Simulates a consistent latency and payload. |
| **Random Delay** | `delay_min=100ms&delay_max=500ms` | Simulates jitter and occasional errors (5%). |
| **High Error Rate**| `error_rate=0.2` | Tests how the system handles 20% failure rate. |
| **CPU Load** | `cpu_ms=100` | Simulates intensive server-side computation.