package core

import (
	"context"
	"errors"
	"testing"
	"time"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/repo"
	"github.com/nanjiek/pixiu-rls/internal/types"
)

type mockLimiter struct {
	allowed   bool
	remaining int64
	err       error
}

func (m *mockLimiter) Allow(ctx context.Context, rule config.Rule, key string, now time.Time) (types.Decision, error) {
	if m.err != nil {
		return types.Decision{Allowed: false, Reason: "mock_error"}, m.err
	}
	return types.Decision{Allowed: m.allowed, Remaining: m.remaining, Reason: "mock_decision"}, nil
}

func newTestRepo() *repo.RedisRepo {
	return &repo.RedisRepo{Prefix: "test"}
}

func TestNewEngine_DefaultFailPolicy(t *testing.T) {
	engine := NewEngine(newTestRepo(), &mockLimiter{allowed: true}, "")
	if engine.failPolicy != "fail-closed" {
		t.Fatalf("failPolicy = %s, want fail-closed", engine.failPolicy)
	}
}

func TestAllowRules_NoRules(t *testing.T) {
	engine := NewEngine(newTestRepo(), &mockLimiter{allowed: true}, "fail-closed")
	dec, err := engine.AllowRules(context.Background(), nil, map[string]string{"route": "/api"}, time.Now())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !dec.Allowed || dec.Reason != "no_rules" {
		t.Fatalf("unexpected decision: %+v", dec)
	}
}

func TestAllowRules_SingleDisabledRule(t *testing.T) {
	engine := NewEngine(newTestRepo(), &mockLimiter{allowed: true}, "fail-closed")
	rule := config.Rule{RuleID: "r1", Enabled: false}
	dec, err := engine.AllowRules(context.Background(), []config.Rule{rule}, map[string]string{"route": "/api"}, time.Now())
	if err == nil {
		t.Fatalf("expected error for disabled rule")
	}
	if dec.Allowed || dec.Reason != "rule_disabled" {
		t.Fatalf("unexpected decision: %+v", dec)
	}
}

func TestAllowRules_FailClosedOnLimiterError(t *testing.T) {
	engine := NewEngine(newTestRepo(), &mockLimiter{err: errors.New("boom")}, "fail-closed")
	rule := config.Rule{RuleID: "r1", Enabled: true, Algo: "token_bucket", WindowMs: 1000, Limit: 1}
	dec, err := engine.AllowRules(context.Background(), []config.Rule{rule}, map[string]string{"route": "/api"}, time.Now())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if dec.Allowed || dec.Reason != "fail_closed" {
		t.Fatalf("unexpected decision: %+v", dec)
	}
}

func TestAllowRules_FailOpenOnLimiterError(t *testing.T) {
	engine := NewEngine(newTestRepo(), &mockLimiter{err: errors.New("boom")}, "fail-open")
	rule := config.Rule{RuleID: "r1", Enabled: true, Algo: "token_bucket", WindowMs: 1000, Limit: 1}
	dec, err := engine.AllowRules(context.Background(), []config.Rule{rule}, map[string]string{"route": "/api"}, time.Now())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !dec.Allowed || dec.Reason != "fail_open" {
		t.Fatalf("unexpected decision: %+v", dec)
	}
}

func TestAllowRules_DenyWins(t *testing.T) {
	engine := NewEngine(newTestRepo(), &mockLimiter{allowed: true, remaining: 2}, "fail-closed")
	rules := []config.Rule{
		{RuleID: "r1", Enabled: true, Algo: "token_bucket", WindowMs: 1000, Limit: 10},
		{RuleID: "r2", Enabled: true, Algo: "token_bucket", WindowMs: 1000, Limit: 10},
	}

	dims := map[string]string{"route": "/api"}
	dec, err := engine.AllowRules(context.Background(), rules, dims, time.Now())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !dec.Allowed {
		t.Fatalf("unexpected decision: %+v", dec)
	}

	denyLimiter := &mockLimiter{allowed: false, remaining: 0}
	engine = NewEngine(newTestRepo(), denyLimiter, "fail-closed")
	dec, err = engine.AllowRules(context.Background(), rules, dims, time.Now())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if dec.Allowed {
		t.Fatalf("expected deny, got: %+v", dec)
	}
}
