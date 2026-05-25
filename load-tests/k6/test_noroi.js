// test_noroi.js
// A k6 script to test the Noroi HTTP backend

import http from "k6/http";
import { check } from "k6";
import { Trend, Rate, Counter } from "k6/metrics";

// Custom metrics
const responseTimeTrend = new Trend("response_time", true);
const errorRate = new Rate("error_rate");
const requestCounter = new Counter("requests_count");

// Test configuration
const baseUrl = "http://localhost:8080/respond";

// Test scenarios
const scenarios = [
  {
    name: "Fixed Delay",
    params: "delay=300ms&size=10kb",
  },
  {
    name: "Random Delay",
    params: "delay_min=100ms&delay_max=500ms&error_rate=0.05",
  },
  {
    name: "High Error Rate",
    params: "error_rate=0.2",
  },
  {
    name: "CPU Load",
    params: "cpu_ms=100",
  },
];

export const options = {
  thresholds: {
    http_req_failed: ["rate<0.1"], // Less than 10% of requests should fail
    http_req_duration: ["p(95)<500"], // 95% of requests should be below 500ms
  },
  scenarios: {
    fixed_delay: {
      executor: "per-vu-iterations",
      vus: 5000,
      iterations: 20,
      maxDuration: "1m",
      exec: "fixedDelayTest",
    },
    random_delay: {
      executor: "per-vu-iterations",
      vus: 5000,
      iterations: 20,
      maxDuration: "1m",
      exec: "randomDelayTest",
    },
    high_error_rate: {
      executor: "per-vu-iterations",
      vus: 5000,
      iterations: 20,
      maxDuration: "1m",
      exec: "highErrorRateTest",
    },
    cpu_load: {
      executor: "per-vu-iterations",
      vus: 5000,
      iterations: 20,
      maxDuration: "1m",
      exec: "cpuLoadTest",
    },
  },
};

// Test functions for each scenario
export function fixedDelayTest() {
  runTest(scenarios[0]);
}

export function randomDelayTest() {
  runTest(scenarios[1]);
}

export function highErrorRateTest() {
  runTest(scenarios[2]);
}

export function cpuLoadTest() {
  runTest(scenarios[3]);
}

function runTest(scenario) {
  const url = `${baseUrl}?${scenario.params}`;
  const res = http.get(url);

  // Track custom metrics
  responseTimeTrend.add(res.timings.duration);
  errorRate.add(res.status !== 200);
  requestCounter.add(1);

  // Validate response
  check(res, {
    "status is 200": (r) => r.status === 200,
    "response time acceptable": (r) => r.timings.duration < 1000,
  });

}
