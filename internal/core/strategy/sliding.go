package strategy

import (
	"context"
	"log/slog"
	"time"

	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/repo"
	"github.com/nanjiek/pixiu-rls/internal/types" // 依赖公共类型
	"github.com/nanjiek/pixiu-rls/internal/util"
)

// Sliding 滑动窗口算法实现
type Sliding struct {
	repo *repo.RedisRepo
}

// NewSliding 创建滑动窗口实例
func NewSliding(rdb *repo.RedisRepo) *Sliding {
	return &Sliding{repo: rdb}
}

// Allow 实现core.Strategy接口
func (s *Sliding) Allow(ctx context.Context, rule config.Rule, dimKey string, now time.Time) (types.Decision, error) {
	key := s.repo.KeySW(rule.RuleID, dimKey)
	nowMs := now.UnixMilli()

	// 执行 Lua 脚本
	res, err := repo.ScriptSliding.Run(ctx, s.repo.Cli, []string{key},
		nowMs, rule.WindowMs, rule.Limit).Result()
	if err != nil {
		slog.Error("sliding script error", "err", err)
		return types.Decision{Allowed: false, Reason: "sliding_lua_failed", Err: err}, err
	}

	results := res.([]interface{})
	allowed := util.ToInt64(results[0]) == 1
	count := util.ToInt64(results[1])

	if !allowed {
		return types.Decision{
			Allowed:      false,
			Remaining:    0,
			RetryAfterMs: rule.WindowMs,
			Reason:       "sliding_window_exceeded",
		}, nil
	}

	return types.Decision{
		Allowed:   true,
		Remaining: rule.Limit - count,
		Reason:    "sliding_window_allowed",
	}, nil
}
