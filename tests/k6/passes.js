// k6 smoke: GET /passes at 20 rps for 30 s, p95 < 500 ms.
import http from "k6/http";
import { check } from "k6";

export const options = {
  scenarios: { passes: { executor: "constant-arrival-rate", rate: 20, timeUnit: "1s", duration: "30s", preAllocatedVUs: 20, maxVUs: 60 } },
  thresholds: { http_req_duration: ["p(95)<500"], http_req_failed: ["rate<0.01"] },
};

const API = __ENV.API_URL || "http://host.docker.internal:8080";
const headers = { Authorization: `Bearer ${__ENV.TOKEN}` };

export default function () {
  const url = `${API}/passes?satellite=${__ENV.SAT_ID}&from=2026-11-27T09:00:00Z&to=2026-11-28T09:00:00Z`;
  const res = http.get(url, { headers });
  check(res, { "status 200": (r) => r.status === 200, "has passes": (r) => r.json().length > 0 });
}
