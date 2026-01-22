package core

import (
	"context"
	"errors"
	"time"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/repo"
	"github.com/nanjiek/pixiu-rls/internal/types" // 依赖公共类型
	"github.com/nanjiek/pixiu-rls/internal/util"
)

// Strategy 限流策略接口（所有具体策略需实现此接口）
// 定义在core包，避免对strategy包的依赖
type Strategy interface {
	Allow(ctx context.Context, rule config.Rule, dimKey string, now time.Time) (types.Decision, error)
}

// Engine 限流核心引擎
type Engine struct {
	repo       *repo.RedisRepo
	quota      *Quota
	strategies map[string]Strategy // 存储策略映射（算法名→策略实例）
}

// NewEngine 创建引擎实例（通过参数注入策略，实现依赖注入）
func NewEngine(rdb *repo.RedisRepo, strategies map[string]Strategy) *Engine {
	return &Engine{
		repo:       rdb,
		quota:      &Quota{repo: rdb},
		strategies: strategies,
	}
}

// Allow 执行限流判定（主入口）
func (e *Engine) Allow(ctx context.Context, rule config.Rule, dims map[string]string, now time.Time) (types.Decision, error) {
	// 1. 检查规则是否启用
	if !rule.Enabled {
		return types.Decision{
			Allowed: false,
			Reason:  "rule_disabled",
		}, errors.New("rule is disabled")
	}

	// 2. 黑白名单检查
	if ip, ok := dims["ip"]; ok {
		// 黑名单检查
		inBlack, err := e.repo.IsInSet(ctx, e.repo.KeyBlacklistIP(), ip)
		if err != nil {
			return types.Decision{
				Allowed: false,
				Reason:  "blacklist_check_failed",
				Err:     err,
			}, err
		}
		if inBlack {
			return types.Decision{
				Allowed: false,
				Reason:  "ip_in_blacklist",
			}, nil
		}

		// 白名单检查
		inWhite, err := e.repo.IsInSet(ctx, e.repo.KeyWhitelistIP(), ip)
		if err != nil {
			return types.Decision{
				Allowed: false,
				Reason:  "whitelist_check_failed",
				Err:     err,
			}, err
		}
		if inWhite {
			return types.Decision{
				Allowed: true,
				Reason:  "ip_in_whitelist",
			}, nil
		}
	}

	// 3. 生成维度哈希键
	dimKey, err := util.HashDims(rule.Dims, dims)
	if err != nil {
		return types.Decision{
			Allowed: false,
			Reason:  "dim_hash_failed",
			Err:     err,
		}, err
	}

	// 4. 配额检查（分钟/小时/天）
	quotaDec := e.quota.CheckAndIncr(ctx, rule, dimKey, now)
	if !quotaDec.Allowed {
		return quotaDec, nil
	}

	// 5. 执行具体限流算法（依赖接口，不依赖具体实现）
	strategy, ok := e.strategies[rule.Algo]
	if !ok {
		return types.Decision{
			Allowed: false,
			Reason:  "unsupported_algorithm",
		}, errors.New("unsupported algorithm: " + rule.Algo)
	}
	return strategy.Allow(ctx, rule, dimKey, now)
}
