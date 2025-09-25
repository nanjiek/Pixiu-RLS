package strategy

import (
	"context"
	"time"

	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/repo"
	"github.com/nanjiek/pixiu-rls/internal/types" // 依赖公共类型
	"github.com/nanjiek/pixiu-rls/internal/util"
)

// Token 令牌桶算法实现
type Token struct {
	repo *repo.RedisRepo
}

// NewToken 创建令牌桶实例
func NewToken(rdb *repo.RedisRepo) *Token {
	return &Token{repo: rdb}
}

// Allow 实现core.Strategy接口
func (t *Token) Allow(ctx context.Context, rule config.Rule, dimKey string, now time.Time) (types.Decision, error) {
	// 生成Redis键
	tokenKey := t.repo.KeyTB(rule.RuleID, dimKey)
	tsKey := t.repo.KeyTBTS(rule.RuleID, dimKey)

	// 计算令牌补充速率（每毫秒补充的令牌数）
	refillPerMs := float64(rule.Limit) / float64(rule.WindowMs)
	maxTokens := rule.Limit + rule.Burst // 最大令牌数=基础限制+突发容量

	// 执行Lua脚本（补充令牌+判断是否允许）
	res, err := repo.ScriptToken.Run(ctx, t.repo.Cli, []string{tokenKey, tsKey},
		maxTokens, refillPerMs, now.UnixMilli()).Result()
	if err != nil {
		return types.Decision{
			Allowed: false,
			Reason:  "token_lua_failed",
			Err:     err,
		}, err
	}

	// 解析结果
	results := res.([]interface{})
	allowed := util.ToInt64(results[0]) == 1
	remaining := util.ToInt64(results[1])

	if !allowed {
		return types.Decision{
			Allowed:      false,
			Remaining:    remaining,
			RetryAfterMs: 100, // 建议100ms后重试
			Reason:       "token_bucket_empty",
		}, nil
	}

	return types.Decision{
		Allowed:   true,
		Remaining: remaining,
		Reason:    "token_allowed",
	}, nil
}
