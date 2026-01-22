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

// LeakyBucket enforces rate limits with a leaky bucket script.
type LeakyBucket struct {
	repo *repo.RedisRepo
}

func NewLeakyBucket(rdb *repo.RedisRepo) *LeakyBucket {
	if rdb == nil {
		panic("limiter: nil redis repo")
	}
	return &LeakyBucket{repo: rdb}
}

func (l *LeakyBucket) Allow(ctx context.Context, rule config.Rule, key string, now time.Time) (types.Decision, error) {
	if rule.WindowMs <= 0 || rule.Limit <= 0 {
		err := errors.New("invalid rule")
		return types.Decision{Allowed: false, Reason: "invalid_rule", Err: err}, err
	}
	if key == "" {
		err := errors.New("empty key")
		return types.Decision{Allowed: false, Reason: "empty_key", Err: err}, err
	}

	maxQueue := rule.Burst
	if maxQueue <= 0 {
		maxQueue = rule.Limit
	}

	ratePerMs := float64(rule.Limit) / float64(rule.WindowMs)

	ttlMs := int64(float64(maxQueue) / ratePerMs)
	if ttlMs < 1000 {
		ttlMs = 1000
	}
	ttlMs += 1000

	res, err := repo.ScriptLeaky.Run(ctx, l.repo.Cli, []string{key}, ratePerMs, now.UnixMilli(), maxQueue, ttlMs).Result()
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
