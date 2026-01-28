import http from "k6/http";
import { check } from "k6";

const baseUrl = __ENV.RLS_URL || "http://host.docker.internal:8080";
const urls = (__ENV.RLS_URLS || baseUrl)
  .split(",")
  .map((s) => s.trim())
  .filter((s) => s.length > 0);
const ruleId = __ENV.RULE_ID || "login_tb";
const route = __ENV.ROUTE || "/api/login";
const fixedIP = __ENV.FIXED_IP || "";

export const options = {
  scenarios: {
    steady: {
      executor: "constant-arrival-rate",
      rate: Number(__ENV.RATE || 10000),
      timeUnit: "1s",
      duration: __ENV.DURATION || "30s",
      preAllocatedVUs: Number(__ENV.VUS || 200),
      maxVUs: Number(__ENV.MAX_VUS || 1000),
    },
  },
};

export default function () {
  const url = urls[__ITER % urls.length];
  const n = __ITER % 1000000;
  const ip = fixedIP || `10.${(n >> 16) & 0xff}.${(n >> 8) & 0xff}.${n & 0xff}`;
  const payload = JSON.stringify({
    ruleId: ruleId,
    dims: { ip: ip, route: route },
  });
  const params = { headers: { "Content-Type": "application/json" } };
  const res = http.post(`${url}/v1/allow`, payload, params);
  check(res, {
    "status is 200 or 429": (r) => r.status === 200 || r.status === 429,
  });
}
