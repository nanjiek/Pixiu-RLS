# Pixiu-RLS 百万级限流器重构设计

## 目标
- 1M RPS 级别吞吐，p99 < 10ms
- 可用性优先于强一致性
- 支持按 userId/IP/API Key 识别客户端
- 支持多规则叠加与最严格规则生效

## 需求摘要
### 功能性
- 通过 id、ip 或 api key 识别用户
- 根据可配置规则限制请求
- 返回准确错误码与限流响应头

### 非功能性
- 低延迟检查（<10ms）
- 可扩展到 1M RPS
- 高可用优先


Note: Circuit breaker is optional and out of scope for the baseline refactor.
## 架构概览
- 部署位置：API 网关/负载均衡入口层
- 控制面：规则管理 + 动态下发
- 数据面：无锁规则读 + 令牌桶限流
- 存储面：Redis Cluster 作为限流状态唯一来源

## 关键概念
- Rule：限流规则（算法、窗口、阈值、维度、优先级）
- Client：限流对象（userId/IP/API Key）
- Request：请求上下文（路径、方法、headers、时间）

## 规则匹配与执行
- 规则匹配基于请求上下文与客户端标识
- 多规则叠加时，任意拒绝即拒绝
- 路由与规则匹配使用 RCU 快照：
  - 规则快照：无锁读取规则集
  - 路由快照：无锁读取路由/匹配索引，规则更新时整体替换

## 接口草图
```go
type ClientKey struct {
    Kind string // "user" | "ip" | "api_key"
    ID   string
    Key  string
}

type ClientResolver interface {
    Resolve(req *http.Request) (ClientKey, error)
}

type RequestCtx struct {
    Path   string
    Method string
    Header http.Header
    Client ClientKey
    Now    time.Time
}

type RuleMatcher interface {
    Match(ctx RequestCtx) []config.Rule
}

type Limiter interface {
    Allow(ctx context.Context, rule config.Rule, clientKey string, now time.Time) (types.Decision, error)
}
```

## Lua 脚本伪码（Token Bucket）
```lua
-- KEYS[1]: rls:tb:{ruleId}:{clientKey}
-- ARGV[1]: limit
-- ARGV[2]: window_ms
-- ARGV[3]: burst
-- ARGV[4]: now_ms
-- ARGV[5]: ttl_ms

local limit = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local burst = tonumber(ARGV[3])
local now_ms = tonumber(ARGV[4])
local ttl_ms = tonumber(ARGV[5])

local tokens = tonumber(redis.call("HGET", KEYS[1], "tokens"))
local last = tonumber(redis.call("HGET", KEYS[1], "last_refill"))
if tokens == nil then
  tokens = limit + burst
  last = now_ms
end

local rate_per_ms = limit / window_ms
local delta = math.max(0, now_ms - last)
local refill = delta * rate_per_ms
tokens = math.min(limit + burst, tokens + refill)
last = now_ms

local allowed = 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
end

local reset_ms = 0
if tokens < 1 then
  local need = 1 - tokens
  reset_ms = now_ms + math.floor(need / rate_per_ms)
else
  -- Calculate when the next token becomes available
  reset_ms = now_ms + math.floor(1 / rate_per_ms)
end
redis.call("HSET", KEYS[1], "tokens", tokens, "last_refill", last)
redis.call("PEXPIRE", KEYS[1], ttl_ms)

return { allowed, math.floor(tokens), reset_ms }
```

## Nacos dynamic rules (Pull mode)
### Config
- `nacos.addr` / `nacos.namespace` / `nacos.group` / `nacos.dataId`
- `nacos.username` / `nacos.password` (optional)
- `nacos.pollIntervalMs` (default 5000)
- `nacos.timeoutMs` (default 2000)
- `nacos.failPolicy`: `fail-open` | `fail-closed`

### Pull flow
1) Periodic poll from Nacos by `dataId + group + namespace`
2) Compare version (MD5/etag); skip if unchanged
3) Parse rules (JSON/YAML) -> validate -> build immutable rule set
4) Build route snapshot (rule index) and replace via RCU
5) On error: keep last good snapshot, emit metrics/logs

### Notes
- Pull mode favors simplicity; update latency equals poll interval
- Use RCU snapshot for both rules and route index
- Keep the last successful version and timestamp for observability

### Suggested modules
- `internal/rules/source/nacos.go`: puller + parsing + validation
- `internal/rules/poller.go`: schedule + backoff + last-good snapshot
- `internal/router/snapshot.go`: route index built from rule snapshot

## 模块结构与职责
- `internal/api/http.go`：API 入口，返回 200/429 + RateLimit 头
- `internal/core/engine.go`：编排 matcher + limiter + fail policy
- `internal/router/snapshot.go`：路由索引快照（RCU），由规则快照构建
- `internal/router/matcher.go`：基于路由快照匹配规则，合并最严格决策
- `internal/identity/resolver.go`：解析 clientId（JWT/IP/API Key）
- `internal/rules/cache.go`：规则快照（RCU）
- `internal/rules/source/nacos.go`：Nacos 拉取规则（pull），版本校验
- `internal/rules/poller.go`：拉取调度、重试、保留 last-good
- `internal/limiter/tokenbucket.go`：令牌桶 + Lua 调用
- `internal/limiter/script.lua`：Lua 脚本
- `internal/repo/redis.go`：Redis 客户端、Lua eval、分片路由
- `internal/config/config.go`：规则定义与系统配置（含 Nacos）
- `internal/types/types.go`：决策结果结构体

## 重构步骤（规划版）
1) 扩展 `internal/config/config.go`：规则匹配字段、优先级、失败策略、Nacos 配置
2) 实现 `internal/identity/resolver.go`：解析 clientId 并归一化
3) 实现 `internal/rules/source/nacos.go` + `internal/rules/poller.go`：pull 拉取、版本校验、last-good
4) 扩展 `internal/rules/cache.go`：RCU 规则快照更新接口
5) 新增 `internal/router/snapshot.go` + `internal/router/matcher.go`：路由索引快照 + 规则匹配
6) 新增 `internal/limiter/script.lua` + `internal/limiter/tokenbucket.go`：Lua 令牌桶
7) 扩展 `internal/repo/redis.go`：Lua eval、分片与连接池配置
8) 改造 `internal/core/engine.go`：多规则合并 + fail-open/close
9) 更新 `internal/api/http.go`：统一响应头与错误码
10) 增加测试与压测：解析、匹配、Lua、Nacos 解析、吞吐指标
