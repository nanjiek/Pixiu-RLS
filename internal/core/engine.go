package core

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/repo"
	"github.com/nanjiek/pixiu-rls/internal/types"
	"github.com/nanjiek/pixiu-rls/internal/util"
)

// Limiter defines the limiter interface used by the engine.
type Limiter interface {
	Allow(ctx context.Context, rule config.Rule, key string, now time.Time) (types.Decision, error)
}

// Engine evaluates rules and applies limiter with fail policy.
type Engine struct {
	repo       *repo.RedisRepo
	ipCache    *IPListCache
	limiter    Limiter
	failPolicy string
	logger     *slog.Logger
}

// NewEngine constructs an engine with the limiter and fail policy.
func NewEngine(rdb *repo.RedisRepo, limiter Limiter, failPolicy string) *Engine {
	if limiter == nil {
		panic("core: nil limiter")
	}
	logger := slog.Default()
	var ipCache *IPListCache
	if rdb != nil {
		ipCache = NewIPListCache(rdb, "", logger)
	}
	return &Engine{
		repo:       rdb,
		ipCache:    ipCache,
		limiter:    limiter,
		failPolicy: normalizeFailPolicy(failPolicy),
		logger:     logger,
	}
}

// Allow evaluates a single rule (kept for compatibility).
func (e *Engine) Allow(ctx context.Context, rule config.Rule, dims map[string]string, now time.Time) (types.Decision, error) {
	return e.AllowRules(ctx, []config.Rule{rule}, dims, now)
}

// AllowRules evaluates multiple rules in order and applies fail policy.
func (e *Engine) AllowRules(ctx context.Context, rules []config.Rule, dims map[string]string, now time.Time) (types.Decision, error) {
	if len(rules) == 0 {
		return types.Decision{Allowed: true, Reason: "no_rules"}, nil
	}
	if dims == nil {
		dims = map[string]string{}
	}

	anyError := false
	ipDecision, handled, err := e.checkIPLists(ctx, dims)
	if err != nil {
		anyError = true
		if e.failPolicy == "fail-closed" {
			return types.Decision{
				Allowed: false,
				Reason:  "fail_closed",
				Err:     err,
			}, nil
		}
		e.logger.Warn("fail-open due to ip list error", "err", err)
	}
	if handled {
		return ipDecision, nil
	}

	anyRule := false
	minRemainingSet := false
	var minRemaining int64

	for _, rule := range rules {
		if !rule.Enabled {
			if len(rules) == 1 {
				return types.Decision{
					Allowed: false,
					Reason:  "rule_disabled",
				}, errors.New("rule is disabled")
			}
			continue
		}

		anyRule = true
		dec, err := e.allowRule(ctx, rule, dims, now)
		if err != nil {
			anyError = true
			if e.failPolicy == "fail-open" {
				e.logger.Warn("fail-open due to limiter error", "rule_id", rule.RuleID, "algo", normalizeAlgo(rule.Algo), "err", err)
				continue
			}
			return types.Decision{
				Allowed: false,
				Reason:  "fail_closed",
				Err:     err,
			}, nil
		}

		if !dec.Allowed {
			return dec, nil
		}
		if dec.Remaining >= 0 {
			if !minRemainingSet || dec.Remaining < minRemaining {
				minRemaining = dec.Remaining
				minRemainingSet = true
			}
		}
	}

	if !anyRule {
		return types.Decision{Allowed: true, Reason: "no_enabled_rules"}, nil
	}

	out := types.Decision{
		Allowed: true,
		Reason:  "allowed",
	}
	if minRemainingSet {
		out.Remaining = minRemaining
	}
	if anyError && e.failPolicy == "fail-open" {
		out.Reason = "fail_open"
	}
	return out, nil
}

func (e *Engine) allowRule(ctx context.Context, rule config.Rule, dims map[string]string, now time.Time) (types.Decision, error) {
	if e.repo == nil {
		err := errors.New("repo is nil")
		return types.Decision{Allowed: false, Reason: "repo_unavailable", Err: err}, err
	}

	dimKey, err := util.HashDims(rule.Dims, dims)
	if err != nil {
		return types.Decision{Allowed: false, Reason: "dim_hash_failed", Err: err}, err
	}

	algo := normalizeAlgo(rule.Algo)
	var key string
	switch algo {
	case "token_bucket":
		key = e.repo.KeyTB(rule.RuleID, dimKey)
	case "sliding_window":
		key = e.repo.KeySW(rule.RuleID, dimKey)
	case "leaky_bucket":
		key = e.repo.KeyLB(rule.RuleID, dimKey)
	default:
		err = errors.New("unsupported algorithm: " + algo)
		return types.Decision{Allowed: false, Reason: "unsupported_algorithm", Err: err}, err
	}

	dec, err := e.limiter.Allow(ctx, rule, key, now)
	if err != nil {
		if dec.Reason == "" {
			dec.Reason = "limiter_failed"
		}
		dec.Err = err
		return dec, err
	}
	if !dec.Allowed && e.ipCache != nil {
		ip := strings.TrimSpace(dims["ip"])
		if ip != "" {
			e.ipCache.RecordDeny(ctx, ip)
		}
	}
	return dec, nil
}

func (e *Engine) checkIPLists(ctx context.Context, dims map[string]string) (types.Decision, bool, error) {
	ip := strings.TrimSpace(dims["ip"])
	if ip == "" {
		return types.Decision{}, false, nil
	}
	if e.ipCache != nil {
		return e.ipCache.CheckIP(ctx, ip)
	}
	if e.repo == nil {
		return types.Decision{}, false, errors.New("repo is nil")
	}

	inBlack, err := e.repo.IsInSet(ctx, e.repo.KeyBlacklistIP(), ip)
	if err != nil {
		return types.Decision{Allowed: false, Reason: "blacklist_check_failed", Err: err}, false, err
	}
	if inBlack {
		return types.Decision{Allowed: false, Reason: "ip_in_blacklist"}, true, nil
	}

	inWhite, err := e.repo.IsInSet(ctx, e.repo.KeyWhitelistIP(), ip)
	if err != nil {
		return types.Decision{Allowed: false, Reason: "whitelist_check_failed", Err: err}, false, err
	}
	if inWhite {
		return types.Decision{Allowed: true, Reason: "ip_in_whitelist"}, true, nil
	}
	return types.Decision{}, false, nil
}

func normalizeAlgo(algo string) string {
	algo = strings.ToLower(strings.TrimSpace(algo))
	if algo == "" {
		return "token_bucket"
	}
	return algo
}

func normalizeFailPolicy(policy string) string {
	policy = strings.ToLower(strings.TrimSpace(policy))
	if policy != "fail-open" && policy != "fail-closed" {
		return "fail-closed"
	}
	return policy
}

func (e *Engine) Close() {
	if e.ipCache != nil {
		e.ipCache.Close()
	}
}
