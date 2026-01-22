package strategy

import (
	"context"
	"time"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/repo"
	"github.com/nanjiek/pixiu-rls/internal/types" // 依赖公共类型，不依赖core包
	"github.com/nanjiek/pixiu-rls/internal/util"
)

// Leaky 漏桶算法实现
type Leaky struct {
	repo *repo.RedisRepo
}

// NewLeaky 创建漏桶实例
func NewLeaky(rdb *repo.RedisRepo) *Leaky {
	return &Leaky{repo: rdb}
}

// Allow 实现core.Strategy接口
func (l *Leaky) Allow(ctx context.Context, rule config.Rule, dimKey string, now time.Time) (types.Decision, error) {
	// 生成Redis键
	levelKey := l.repo.KeyLB(rule.RuleID, dimKey)

	// 计算漏水速率（每秒允许的请求数）
	ratePerMs := float64(rule.Limit) / float64(rule.WindowMs)
	maxQueue := rule.Limit + rule.Burst // 最大队列长度=基础限制+突发容量
	ttlMs := int64(0)
	if ratePerMs > 0 {
		ttlMs = int64(float64(maxQueue) / ratePerMs)
	}
	if ttlMs < 1000 {
		ttlMs = 1000
	}
	ttlMs += 1000

	// 执行Lua脚本（原子操作）
	res, err := repo.ScriptLeaky.Run(ctx, l.repo.Cli, []string{levelKey},
		ratePerMs, now.UnixMilli(), maxQueue, ttlMs).Result()
	if err != nil {
		return types.Decision{
			Allowed: false,
			Reason:  "leaky_lua_failed",
			Err:     err,
		}, err
	}

	// 解析脚本返回结果
	results := res.([]interface{})
	allowed := util.ToInt64(results[0]) == 1
	currentLevel := util.ToInt64(results[1])

	if !allowed {
		return types.Decision{
			Allowed:      false,
			Remaining:    maxQueue - currentLevel,
			RetryAfterMs: 100, // 建议100ms后重试
			Reason:       "leaky_bucket_full",
		}, nil
	}

	return types.Decision{
		Allowed:   true,
		Remaining: maxQueue - currentLevel,
		Reason:    "leaky_allowed",
	}, nil
}
