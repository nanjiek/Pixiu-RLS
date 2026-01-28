# Testing Samples (Docker-only)

This guide assumes you use `deployments/docker-compose.nacos.yaml` and do not have `redis-cli` installed on the host.

## 1) Start the stack

```powershell
docker compose -f deployments/docker-compose.nacos.yaml up -d --build
```

## 2) Publish rules to Nacos

Option A: Nacos UI (http://localhost:8848/nacos, default user/pass: nacos/nacos)

Option B: Docker-only curl

```powershell
docker run --rm -v "${PWD}/examples/nacos-rules.json:/data/rules.json" curlimages/curl:8.6.0 -X POST "http://host.docker.internal:8848/nacos/v1/cs/configs" --data-urlencode "dataId=pixiu-rls-rules" --data-urlencode "group=DEFAULT_GROUP" --data-urlencode "content@/data/rules.json"
```

## 3) Rate limit check (docker curl)

```powershell
docker run --rm curlimages/curl:8.6.0 -H "Content-Type: application/json" -d '{"ruleId":"login_tb","dims":{"ip":"10.0.0.1","route":"/api/login"}}' http://host.docker.internal:8080/v1/allow
```

## 4) Blacklist / whitelist (redis-cli via docker)

```powershell
docker exec -it limiter-redis-cluster redis-cli -c -p 7000 SADD pixiu:rls:blacklist:ip 10.0.0.2
docker exec -it limiter-redis-cluster redis-cli -c -p 7000 SADD pixiu:rls:whitelist:ip 10.0.0.3
```

```powershell
docker run --rm curlimages/curl:8.6.0 -H "Content-Type: application/json" -d '{"ruleId":"login_tb","dims":{"ip":"10.0.0.2","route":"/api/login"}}' http://host.docker.internal:8080/v1/allow
```

```powershell
docker run --rm curlimages/curl:8.6.0 -H "Content-Type: application/json" -d '{"ruleId":"login_tb","dims":{"ip":"10.0.0.3","route":"/api/login"}}' http://host.docker.internal:8080/v1/allow
```

## 5) Temporary blacklist (hot IP)

After repeated denies, the service will add a temporary key like `pixiu:rls:blacklist:ip:tmp:<ip>`.

```powershell
docker exec -it limiter-redis-cluster redis-cli -c -p 7000 KEYS "pixiu:rls:blacklist:ip:tmp:*"
```

## 6) Redis cluster failover (basic simulation)

```powershell
docker exec -it limiter-redis-cluster sh -c "pkill -f 'redis-server.*7000'"
```

Keep sending `/v1/allow` requests and verify the service still responds while the cluster elects a new master.

## 7) Load test (k6 via docker)

```powershell
docker run --rm -i grafana/k6 run -e RLS_URL=http://host.docker.internal:8080 -e RATE=10000 -e DURATION=30s -e VUS=200 -e MAX_VUS=1000 - < scripts/k6-allow.js
```

For 1M QPS, run distributed load across multiple machines and scale Redis + RLS accordingly.
