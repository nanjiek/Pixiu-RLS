package core

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"sync/atomic"
	"time"
)

import (
	sentinel "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/base"
	"github.com/alibaba/sentinel-golang/core/circuitbreaker"

	"github.com/redis/go-redis/v9"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/repo"
	"github.com/nanjiek/pixiu-rls/internal/types"
)

type Quota struct {
	repo    *repo.RedisRepo
	logger  *slog.Logger
	resName string
	state   atomic.Int32
	openTime atomic.Int64
}

const (
	StateClosed int32 = iota
	StateOpen
	StateHalfOpen
)

var quotaScript = redis.NewScript(`
    local h_limit = tonumber(ARGV[1])
    local d_limit = tonumber(ARGV[2])
    local h_ttl   = tonumber(ARGV[3])
    local d_ttl   = tonumber(ARGV[4])
    local default_rem = tonumber(ARGV[5])

    local h_current = tonumber(redis.call("GET", KEYS[1]) or "0")
    local d_current = tonumber(redis.call("GET", KEYS[2]) or "0")

    if h_limit > 0 and h_current + 1 > h_limit then
        return {0, "hour", h_current}
    end
    if d_limit > 0 and d_current + 1 > d_limit then
        return {0, "day", d_current}
    end

    local h_new = redis.call("INCR", KEYS[1])
    local d_new = redis.call("INCR", KEYS[2])

    if h_new == 1 then redis.call("EXPIRE", KEYS[1], h_ttl) end
    if d_new == 1 then redis.call("EXPIRE", KEYS[2], d_ttl) end

    local h_rem = h_limit > 0 and (h_limit - h_new) or default_rem
    local d_rem = d_limit > 0 and (d_limit - d_new) or default_rem
    local min_rem = math.min(h_rem, d_rem)

    return {1, "ok", min_rem}
`)

// 实现 circuitbreaker.StateChangeListener 接口
// 1. 完善结构体以完全实现 circuitbreaker.StateChangeListener 接口
type quotaStateListener struct {
	q *Quota
}

// 当状态变为 Closed 时触发（必须实现）
func (l *quotaStateListener) OnTransformToClosed(from circuitbreaker.State, rule circuitbreaker.Rule) {
	// 通过 rule.Resource 判断是否是我们要监听的资源
	if rule.Resource != l.q.resName {
		return
	}
	l.q.logger.Info("Sentinel CB Transform to Closed", "from", from.String())
	l.q.state.Store(StateClosed)
}

// 当状态变为 Open 时触发（必须实现）
func (l *quotaStateListener) OnTransformToOpen(from circuitbreaker.State, rule circuitbreaker.Rule, snapshot interface{}) {
	if rule.Resource != l.q.resName {
		return
	}
	l.q.logger.Warn("Sentinel CB Transform to Open", "from", from.String())
	l.q.state.Store(StateOpen)
	l.q.openTime.Store(time.Now().Unix())
}

// 当状态变为 HalfOpen 时触发（必须实现）
func (l *quotaStateListener) OnTransformToHalfOpen(from circuitbreaker.State, rule circuitbreaker.Rule) {
	if rule.Resource != l.q.resName {
		return
	}
	l.q.logger.Info("Sentinel CB Transform to HalfOpen", "from", from.String())
	l.q.state.Store(StateHalfOpen)
	l.q.openTime.Store(time.Now().Unix()) // 重新记录进入半开的时间用于阶梯放行
}

func NewQuota(r *repo.RedisRepo, logger *slog.Logger) *Quota {
	if logger == nil {
		logger = slog.Default()
	}
	q := &Quota{
		repo:    r,
		logger:  logger,
		resName: "redis:cluster:quota",
	}
	// Sentinel 初始化
	if err := sentinel.InitDefault(); err != nil {
		logger.Warn("sentinel init failed", "err", err)
	}
	q.initSentinel()
	return q
}

func (q *Quota) initSentinel() {
	// 修复：使用实现接口的结构体注册
	circuitbreaker.RegisterStateChangeListeners(&quotaStateListener{q: q})

	_, _ = circuitbreaker.LoadRules([]*circuitbreaker.Rule{
		{
			Resource:         q.resName,
			Strategy:         circuitbreaker.ErrorCount,
			RetryTimeoutMs:   10000,
			MinRequestAmount: 100,
			StatIntervalMs:   1000,
			Threshold:        50,
		},
	})
}

func (q *Quota) getAllowedRatio() float64 {
	state := q.state.Load()
	if state == StateClosed { return 1.0 }
	if state == StateOpen { return 0.0 }

	elapsed := time.Now().Unix() - q.openTime.Load()
	switch {
	case elapsed < 5:  return 0.1
	case elapsed < 10: return 0.2
	case elapsed < 20: return 0.5
	default:           return 1.0
	}
}

func (q *Quota) CheckAndIncr(ctx context.Context, rule config.Rule, dimKey string, now time.Time) types.Decision {
	// 修复：ResTypeAPIGateway 在 base 包中
	entry, blockErr := sentinel.Entry(q.resName, sentinel.WithResourceType(base.ResTypeAPIGateway))
	if blockErr != nil {
		return types.Decision{Allowed: false, Reason: "circuit_breaker_open"}
	}
	defer entry.Exit()

	ratio := q.getAllowedRatio()
	if ratio < 1.0 && rand.Float64() > ratio {
		return types.Decision{Allowed: false, Reason: "warmup_throttled"}
	}

	// 修复：ClientForKey 逻辑
	cli := q.getClientForKey(dimKey)
	if cli == nil {
		return types.Decision{Allowed: false, Reason: "no_redis_client"}
	}

	res, err := q.runLua(ctx, cli, rule, dimKey, now)
	if err != nil {
		sentinel.TraceError(entry, err)
		q.logger.Error("quota lua execute error", "err", err)
		return types.Decision{Allowed: false, Reason: "redis_error", Err: err}
	}

	return q.parseDecision(res, now)
}

// 修复：补充 getClientForKey
func (q *Quota) getClientForKey(key string) *redis.ClusterClient {
	// 假设你的 repo 暴露了底层 ClusterClient
	// 在 Redis Cluster 下，go-redis 会自动处理分片，通常直接用 ClusterClient 即可
	return q.repo.Cli 
}

func (q *Quota) runLua(ctx context.Context, cli *redis.ClusterClient, rule config.Rule, dimKey string, now time.Time) ([]interface{}, error) {
	tCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	hashTag := fmt.Sprintf("{%s:q:%s:%s}", q.repo.Prefix, rule.RuleID, dimKey)
	keys := []string{
		fmt.Sprintf("%s:h:%s", hashTag, now.Format("2006010215")),
		fmt.Sprintf("%s:d:%s", hashTag, now.Format("20060102")),
	}

	res, err := quotaScript.Run(tCtx, cli, keys,
		rule.Quota.PerHour, rule.Quota.PerDay,
		3600+600, 86400+3600, 999999).Slice()
	return res, err
}

// 修复：补充 calcRetryAfter
func (q *Quota) calcRetryAfter(scope string, now time.Time) int64 {
	switch scope {
	case "hour":
		nextHour := now.Truncate(time.Hour).Add(time.Hour)
		return nextHour.Sub(now).Milliseconds()
	case "day":
		nextDay := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		return nextDay.Sub(now).Milliseconds()
	default:
		return 1000
	}
}

func (q *Quota) parseDecision(res []interface{}, now time.Time) types.Decision {
	if len(res) < 3 {
		return types.Decision{Allowed: false, Reason: "lua_invalid_res"}
	}

	code, _ := toInt64(res[0])
	scope, _ := res[1].(string)
	val, _ := toInt64(res[2])

	if code == 0 {
		return types.Decision{
			Allowed:      false,
			Reason:       "quota_exceeded:" + scope,
			Remaining:    0,
			RetryAfterMs: q.calcRetryAfter(scope, now),
		}
	}
	return types.Decision{Allowed: true, Reason: "quota_ok", Remaining: val}
}

func toInt64(v interface{}) (int64, bool) {
	switch val := v.(type) {
	case int64: return val, true
	case int:   return int64(val), true
	case string:
		i, err := strconv.ParseInt(val, 10, 64)
		return i, err == nil
	default: return 0, false
	}
}
