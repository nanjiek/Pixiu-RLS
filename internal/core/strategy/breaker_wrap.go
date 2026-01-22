package strategy

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/core" // 关键：引用 core.Strategy 接口
	"github.com/nanjiek/pixiu-rls/internal/repo"
	"github.com/nanjiek/pixiu-rls/internal/types"
)

type breakerWrap struct {
	repo  *repo.RedisRepo
	inner core.Strategy // 关键：用 core.Strategy
	name  string
}

// 工厂：用熔断器包装任意策略实现（sliding/token/leaky）
func WithBreaker(r *repo.RedisRepo, inner core.Strategy, algoName string) core.Strategy {
	return &breakerWrap{repo: r, inner: inner, name: algoName}
}

// ---- Redis keys ----
func (b *breakerWrap) keyState(ruleID, dimKey string) string {
	return fmt.Sprintf("%s:brk:%s:%s:state", b.repo.Prefix, ruleID, dimKey)
} // "open"|"half"|"closed"
func (b *breakerWrap) keyUntil(ruleID, dimKey string) string {
	return fmt.Sprintf("%s:brk:%s:%s:until", b.repo.Prefix, ruleID, dimKey)
} // ms
func (b *breakerWrap) keyRLDeny(ruleID, dimKey string) string {
	return fmt.Sprintf("%s:brk:%s:%s:rldeny", b.repo.Prefix, ruleID, dimKey)
} // 限流拒绝计数窗口
func (b *breakerWrap) keyHalfPass(ruleID, dimKey string) string {
	return fmt.Sprintf("%s:brk:%s:%s:half:pass", b.repo.Prefix, ruleID, dimKey)
}
func (b *breakerWrap) keyHalfFail(ruleID, dimKey string) string {
	return fmt.Sprintf("%s:brk:%s:%s:half:fail", b.repo.Prefix, ruleID, dimKey)
}

func (b *breakerWrap) getState(ctx context.Context, ruleID, dimKey string) (string, int64) {
	state, _ := b.repo.Cli.Get(ctx, b.keyState(ruleID, dimKey)).Result()
	until, _ := b.repo.Cli.Get(ctx, b.keyUntil(ruleID, dimKey)).Int64()
	if state == "" {
		state = "closed"
	}
	return state, until
}

func (b *breakerWrap) setOpen(ctx context.Context, rule config.Rule, dimKey string, nowMs int64) {
	until := nowMs + rule.Breaker.MinOpenMs
	_ = b.repo.Cli.Set(ctx, b.keyState(rule.RuleID, dimKey), "open", 0).Err()
	_ = b.repo.Cli.Set(ctx, b.keyUntil(rule.RuleID, dimKey), until, 0).Err()
	_ = b.repo.Cli.Del(ctx, b.keyHalfPass(rule.RuleID, dimKey), b.keyHalfFail(rule.RuleID, dimKey)).Err()
	slog.Info("breaker open", "rule", rule.RuleID, "dim", dimKey, "until", until)
}

func (b *breakerWrap) setHalf(ctx context.Context, rule config.Rule, dimKey string) {
	_ = b.repo.Cli.Set(ctx, b.keyState(rule.RuleID, dimKey), "half", 0).Err()
	_ = b.repo.Cli.Del(ctx, b.keyHalfPass(rule.RuleID, dimKey), b.keyHalfFail(rule.RuleID, dimKey)).Err()
	slog.Info("breaker half-open", "rule", rule.RuleID, "dim", dimKey)
}

func (b *breakerWrap) setClosed(ctx context.Context, rule config.Rule, dimKey string) {
	_ = b.repo.Cli.Set(ctx, b.keyState(rule.RuleID, dimKey), "closed", 0).Err()
	_ = b.repo.Cli.Del(ctx, b.keyHalfPass(rule.RuleID, dimKey), b.keyHalfFail(rule.RuleID, dimKey)).Err()
	slog.Info("breaker closed", "rule", rule.RuleID, "dim", dimKey)
}

func stableSample(dimKey string, percent int) bool {
	// 简单稳定采样（FNV-1a 32）
	var h uint32 = 2166136261
	for i := 0; i < len(dimKey); i++ {
		h ^= uint32(dimKey[i])
		h *= 16777619
	}
	if percent <= 0 {
		return false
	}
	if percent >= 100 {
		return true
	}
	return int(h%100) < percent
}

func (b *breakerWrap) Allow(ctx context.Context, rule config.Rule, dimKey string, now time.Time) (types.Decision, error) {
	// 未开启熔断：直接走原策略
	if !rule.Breaker.Enabled {
		return b.inner.Allow(ctx, rule, dimKey, now)
	}

	nowMs := now.UnixMilli()
	state, until := b.getState(ctx, rule.RuleID, dimKey)

	switch state {
	case "open":
		if until == 0 || nowMs < until {
			return types.Decision{
				Allowed:      false,
				Remaining:    0,
				RetryAfterMs: max64(until-nowMs, 0),
				Reason:       "circuit_open",
			}, nil
		}
		// 冷却到期 → half-open
		b.setHalf(ctx, rule, dimKey)
		fallthrough

	case "half":
		// 半开采样：未命中则直接拒绝（不计失败）
		if !stableSample(dimKey, rule.Breaker.HalfOpenProbePercent) {
			return types.Decision{Allowed: false, Reason: "probe_dropped"}, nil
		}
		// 命中探测 → 调原策略
		dec, err := b.inner.Allow(ctx, rule, dimKey, now)
		if err != nil {
			return dec, err
		}
		if dec.Allowed {
			if rule.Breaker.HalfOpenMinPass > 0 {
				pass, _ := b.repo.Cli.Incr(ctx, b.keyHalfPass(rule.RuleID, dimKey)).Result()
				if int(pass) >= rule.Breaker.HalfOpenMinPass {
					b.setClosed(ctx, rule, dimKey)
				}
			}
			return dec, nil
		}
		// 半开但被限流拒绝 → 算失败，达阈值则重回 open
		if rule.Breaker.HalfOpenMaxFail > 0 {
			fail, _ := b.repo.Cli.Incr(ctx, b.keyHalfFail(rule.RuleID, dimKey)).Result()
			if int(fail) >= rule.Breaker.HalfOpenMaxFail {
				b.setOpen(ctx, rule, dimKey, nowMs)
			}
		}
		return dec, nil

	default: // "closed"
		dec, err := b.inner.Allow(ctx, rule, dimKey, now)
		if err != nil {
			return dec, err
		}
		// 关闭态下：若持续“限流拒绝”达到阈值 → 打开熔断
		if !dec.Allowed && rule.Breaker.RLDenyThreshold > 0 && rule.Breaker.RLDenyWindowMs > 0 {
			key := b.keyRLDeny(rule.RuleID, dimKey)
			n, _ := b.repo.Cli.Incr(ctx, key).Result()
			_ = b.repo.Cli.Expire(ctx, key, time.Duration(rule.Breaker.RLDenyWindowMs)*time.Millisecond).Err()
			if int(n) >= rule.Breaker.RLDenyThreshold {
				b.setOpen(ctx, rule, dimKey, nowMs)
				return types.Decision{
					Allowed:      false,
					Remaining:    0,
					RetryAfterMs: rule.Breaker.MinOpenMs,
					Reason:       "circuit_open_by_rl_exceed",
				}, nil
			}
		}
		return dec, nil
	}
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
