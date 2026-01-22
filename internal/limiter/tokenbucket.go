package limiter

import (
	"context"
	_ "embed"
	"errors"
	"strconv"
	"time"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/types"
)

//go:embed script.lua
var tokenBucketScript string

// ScriptExecutor executes a Lua script and returns raw results.
type ScriptExecutor interface {
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) ([]interface{}, error)
}

// TokenBucket applies token bucket algorithm via Lua script.
type TokenBucket struct {
	exec      ScriptExecutor
	script    string
	ttlFactor int64
}

func NewTokenBucket(exec ScriptExecutor) *TokenBucket {
	if exec == nil {
		panic("limiter: nil ScriptExecutor")
	}
	return &TokenBucket{
		exec:      exec,
		script:    tokenBucketScript,
		ttlFactor: 2,
	}
}

func (t *TokenBucket) Allow(ctx context.Context, rule config.Rule, key string, now time.Time) (types.Decision, error) {
	if rule.WindowMs <= 0 || rule.Limit <= 0 {
		err := errors.New("invalid rule")
		return types.Decision{Allowed: false, Reason: "invalid_rule", Err: err}, err
	}
	if key == "" {
		err := errors.New("empty key")
		return types.Decision{Allowed: false, Reason: "empty_key", Err: err}, err
	}

	nowMs := now.UnixNano() / int64(time.Millisecond)
	ttlMs := rule.WindowMs * t.ttlFactor
	if ttlMs <= 0 {
		ttlMs = rule.WindowMs
	}

	res, err := t.exec.Eval(ctx, t.script, []string{key}, rule.Limit, rule.WindowMs, rule.Burst, nowMs, ttlMs)
	if err != nil {
		return types.Decision{Allowed: false, Reason: "limiter_eval_failed", Err: err}, err
	}
	if len(res) < 3 {
		err = errors.New("invalid script response")
		return types.Decision{Allowed: false, Reason: "invalid_script_response", Err: err}, err
	}

	allowed, ok := toInt64(res[0])
	if !ok {
		err = errors.New("invalid allowed value")
		return types.Decision{Allowed: false, Reason: "invalid_script_response", Err: err}, err
	}
	remaining, ok := toInt64(res[1])
	if !ok {
		err = errors.New("invalid remaining value")
		return types.Decision{Allowed: false, Reason: "invalid_script_response", Err: err}, err
	}
	resetMs, ok := toInt64(res[2])
	if !ok {
		err = errors.New("invalid reset value")
		return types.Decision{Allowed: false, Reason: "invalid_script_response", Err: err}, err
	}

	decision := types.Decision{
		Allowed:   allowed > 0,
		Remaining: remaining,
		Reason:    "allowed",
	}
	if !decision.Allowed {
		decision.Reason = "rate_limited"
	}
	if resetMs > nowMs {
		decision.RetryAfterMs = resetMs - nowMs
	}
	return decision, nil
}

func toInt64(v interface{}) (int64, bool) {
	switch val := v.(type) {
	case int64:
		return val, true
	case int:
		return int64(val), true
	case float64:
		return int64(val), true
	case string:
		if parsed, err := strconv.ParseInt(val, 10, 64); err == nil {
			return parsed, true
		}
	}
	return 0, false
}
