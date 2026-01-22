package limiter

import (
	"context"
	"errors"
	"time"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/repo"
	"github.com/nanjiek/pixiu-rls/internal/types"
	"github.com/nanjiek/pixiu-rls/internal/util"
)

// SlidingWindow enforces rate limits with a sliding window script.
type SlidingWindow struct {
	repo *repo.RedisRepo
}

func NewSlidingWindow(rdb *repo.RedisRepo) *SlidingWindow {
	if rdb == nil {
		panic("limiter: nil redis repo")
	}
	return &SlidingWindow{repo: rdb}
}

func (s *SlidingWindow) Allow(ctx context.Context, rule config.Rule, key string, now time.Time) (types.Decision, error) {
	if rule.WindowMs <= 0 || rule.Limit <= 0 {
		err := errors.New("invalid rule")
		return types.Decision{Allowed: false, Reason: "invalid_rule", Err: err}, err
	}
	if key == "" {
		err := errors.New("empty key")
		return types.Decision{Allowed: false, Reason: "empty_key", Err: err}, err
	}

	res, err := repo.ScriptSliding.Run(ctx, s.repo.Cli, []string{key}, now.UnixMilli(), rule.WindowMs, rule.Limit).Result()
	if err != nil {
		return types.Decision{Allowed: false, Reason: "limiter_eval_failed", Err: err}, err
	}

	results, ok := res.([]interface{})
	if !ok || len(results) < 2 {
		err = errors.New("invalid script response")
		return types.Decision{Allowed: false, Reason: "invalid_script_response", Err: err}, err
	}

	allowed := util.ToInt64(results[0]) == 1
	remaining := util.ToInt64(results[1])

	decision := types.Decision{
		Allowed:   allowed,
		Remaining: remaining,
		Reason:    "allowed",
	}
	if !allowed {
		decision.Reason = "rate_limited"
	}
	return decision, nil
}
