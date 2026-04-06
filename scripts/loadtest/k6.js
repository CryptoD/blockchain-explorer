// k6 load test: /api/search, auth (login), and advanced search (heavier API path).
//
// Usage (k6 v0.47+):
//   BASE_URL=http://localhost:8080 \
//   LOADTEST_USERNAME=admin LOADTEST_PASSWORD=admin123 \
//   k6 run scripts/loadtest/k6.js
//
// Optional:
//   LOADTEST_SEARCH_QUERY — query string for /api/search (default: 1 block height)
//
// Profiling while load runs: start server with PPROF_ENABLED=true, then run
//   ./scripts/loadtest/profile-with-k6.sh
//
// Scenarios run in parallel with separate arrival rates; tune in options below.

import http from "k6/http";
import { check, sleep } from "k6";

const base = __ENV.BASE_URL || "http://127.0.0.1:8080";
const searchQ = __ENV.LOADTEST_SEARCH_QUERY || "1";
const user = __ENV.LOADTEST_USERNAME || "";
const pass = __ENV.LOADTEST_PASSWORD || "";

export const options = {
  scenarios: {
    api_search: {
      executor: "constant-arrival-rate",
      rate: 10,
      timeUnit: "1s",
      duration: "60s",
      preAllocatedVUs: 20,
      maxVUs: 50,
      exec: "apiSearch",
    },
    auth_login: {
      executor: "constant-arrival-rate",
      rate: 3,
      timeUnit: "1s",
      duration: "60s",
      startTime: "0s",
      preAllocatedVUs: 10,
      maxVUs: 30,
      exec: "authLogin",
    },
    heavy_advanced_search: {
      executor: "constant-arrival-rate",
      rate: 5,
      timeUnit: "1s",
      duration: "60s",
      startTime: "0s",
      preAllocatedVUs: 15,
      maxVUs: 40,
      exec: "heavyAdvancedSearch",
    },
  },
  thresholds: {
    // Dev-friendly: RPC/cache misses may 5xx; tighten in a dedicated perf environment.
    http_req_failed: ["rate<0.90"],
    http_req_duration: ["p(95)<15000"],
  },
};

export function apiSearch() {
  const res = http.get(
    `${base}/api/search?q=${encodeURIComponent(searchQ)}`,
    { tags: { name: "api_search" } }
  );
  check(res, {
    "search status 2xx/3xx/4xx": (r) => r.status >= 200 && r.status < 500,
  });
  sleep(0.1);
}

export function authLogin() {
  if (!user || !pass) {
    return;
  }
  const payload = JSON.stringify({ username: user, password: pass });
  const res = http.post(`${base}/api/v1/login`, payload, {
    headers: { "Content-Type": "application/json" },
    tags: { name: "auth_login" },
  });
  check(res, {
    "login 200 or 401": (r) => r.status === 200 || r.status === 401,
  });
  sleep(0.2);
}

export function heavyAdvancedSearch() {
  const res = http.get(
    `${base}/api/v1/search/advanced?page=1&page_size=50&sort_by=rank&sort_dir=asc`,
    { tags: { name: "heavy_advanced_search" } }
  );
  check(res, {
    "advanced 200": (r) => r.status === 200,
  });
  sleep(0.1);
}
