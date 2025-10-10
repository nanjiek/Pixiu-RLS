package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/types"
)

// MockStrategy 模拟限流策略
type MockStrategy struct {
	allowed bool
	err     error
}

func (m *MockStrategy) Allow(ctx context.Context, rule config.Rule, dimKey string, now time.Time) (types.Decision, error) {
	if m.err != nil {
		return types.Decision{Allowed: false, Reason: "mock_error"}, m.err
	}
	return types.Decision{Allowed: m.allowed, Reason: "mock_decision"}, nil
}

func TestNewEngine(t *testing.T) {
	strategies := map[string]Strategy{
		"sliding_window": &MockStrategy{allowed: true},
		"token_bucket":   &MockStrategy{allowed: true},
	}

	engine := NewEngine(nil, strategies)

	if engine == nil {
		t.Fatal("NewEngine() returned nil")
	}

	if engine.quota == nil {
		t.Error("Engine.quota should not be nil")
	}

	if len(engine.strategies) != 2 {
		t.Errorf("Engine.strategies length = %d, want 2", len(engine.strategies))
	}
}

func TestEngine_AllowDisabledRule(t *testing.T) {
	strategies := map[string]Strategy{
		"sliding_window": &MockStrategy{allowed: true},
	}

	engine := NewEngine(nil, strategies)

	rule := config.Rule{
		RuleID:  "test-rule",
		Enabled: false, // 禁用的规则
		Algo:    "sliding_window",
	}

	dims := map[string]string{
		"ip":    "192.168.1.1",
		"route": "/api/test",
	}

	decision, err := engine.Allow(context.Background(), rule, dims, time.Now())

	if decision.Allowed {
		t.Error("disabled rule should not be allowed")
	}

	if decision.Reason != "rule_disabled" {
		t.Errorf("reason = %s, want rule_disabled", decision.Reason)
	}

	if err == nil {
		t.Error("expected error for disabled rule")
	}
}

func TestEngine_AllowUnsupportedAlgorithm(t *testing.T) {
	// 创建 mock Redis repo
	strategies := map[string]Strategy{
		"sliding_window": &MockStrategy{allowed: true},
	}

	// 注意：由于 Engine 需要 repo 用于黑白名单检查，这里需要传 nil 并跳过该检查
	// 实际测试中应该 mock repo 或者修改测试逻辑
	engine := NewEngine(nil, strategies)

	rule := config.Rule{
		RuleID:  "test-rule",
		Enabled: true,
		Algo:    "unsupported_algo", // 不支持的算法
		Dims:    []string{},
	}

	// 不传 ip 维度，避免触发黑白名单检查（需要 Redis）
	dims := map[string]string{
		"route": "/api/test",
	}

	decision, err := engine.Allow(context.Background(), rule, dims, time.Now())

	if decision.Allowed {
		t.Error("unsupported algorithm should not be allowed")
	}

	if decision.Reason != "unsupported_algorithm" {
		t.Errorf("reason = %s, want unsupported_algorithm", decision.Reason)
	}

	if err == nil {
		t.Error("expected error for unsupported algorithm")
	}
}

func TestEngine_AllowMissingDimension(t *testing.T) {
	strategies := map[string]Strategy{
		"sliding_window": &MockStrategy{allowed: true},
	}

	engine := NewEngine(nil, strategies)

	rule := config.Rule{
		RuleID:  "test-rule",
		Enabled: true,
		Algo:    "sliding_window",
		Dims:    []string{"route", "user_id"}, // 需要 user_id，不用 ip 避免触发黑白名单检查
	}

	dims := map[string]string{
		"route": "/api/test",
		// missing user_id
	}

	decision, err := engine.Allow(context.Background(), rule, dims, time.Now())

	if decision.Allowed {
		t.Error("missing dimension should not be allowed")
	}

	if decision.Reason != "dim_hash_failed" {
		t.Errorf("reason = %s, want dim_hash_failed", decision.Reason)
	}

	if err == nil {
		t.Error("expected error for missing dimension")
	}
}

func TestEngine_StrategyError(t *testing.T) {
	strategies := map[string]Strategy{
		"sliding_window": &MockStrategy{
			allowed: false,
			err:     errors.New("strategy error"),
		},
	}

	engine := NewEngine(nil, strategies)

	rule := config.Rule{
		RuleID:  "test-rule",
		Enabled: true,
		Algo:    "sliding_window",
		Dims:    []string{},
	}

	dims := map[string]string{
		"route": "/api/test",  // 不用 ip 避免触发黑白名单检查
	}

	decision, err := engine.Allow(context.Background(), rule, dims, time.Now())

	if decision.Allowed {
		t.Error("strategy error should result in not allowed")
	}

	if err == nil {
		t.Error("expected error from strategy")
	}
}

func BenchmarkEngine_Allow(b *testing.B) {
	strategies := map[string]Strategy{
		"sliding_window": &MockStrategy{allowed: true},
		"token_bucket":   &MockStrategy{allowed: true},
		"leaky_bucket":   &MockStrategy{allowed: true},
	}

	engine := NewEngine(nil, strategies)

	rule := config.Rule{
		RuleID:  "bench-rule",
		Enabled: true,
		Algo:    "sliding_window",
		Dims:    []string{"route", "user_id"},  // 不用 ip
		Limit:   1000,
	}

	dims := map[string]string{
		"route":   "/api/test",
		"user_id": "12345",
	}

	ctx := context.Background()
	now := time.Now()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = engine.Allow(ctx, rule, dims, now)
		}
	})
}

func BenchmarkEngine_AllowDifferentAlgos(b *testing.B) {
	strategies := map[string]Strategy{
		"sliding_window": &MockStrategy{allowed: true},
		"token_bucket":   &MockStrategy{allowed: true},
		"leaky_bucket":   &MockStrategy{allowed: true},
	}

	engine := NewEngine(nil, strategies)

	algos := []string{"sliding_window", "token_bucket", "leaky_bucket"}

	for _, algo := range algos {
		b.Run(algo, func(b *testing.B) {
			rule := config.Rule{
				RuleID:  "bench-rule",
				Enabled: true,
				Algo:    algo,
				Dims:    []string{"route"},  // 不用 ip
				Limit:   1000,
			}

			dims := map[string]string{
				"route": "/api/test",
			}

			ctx := context.Background()
			now := time.Now()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = engine.Allow(ctx, rule, dims, now)
			}
		})
	}
}

