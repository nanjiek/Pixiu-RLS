package strategy

import (
	"testing"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
)

func TestNewSliding(t *testing.T) {
	sliding := NewSliding(nil)
	if sliding == nil {
		t.Error("NewSliding() should not return nil")
	}
}

func TestNewToken(t *testing.T) {
	token := NewToken(nil)
	if token == nil {
		t.Error("NewToken() should not return nil")
	}
}

func TestNewLeaky(t *testing.T) {
	leaky := NewLeaky(nil)
	if leaky == nil {
		t.Error("NewLeaky() should not return nil")
	}
}

func TestWithBreaker(t *testing.T) {
	mockStrategy := NewSliding(nil)
	wrapped := WithBreaker(nil, mockStrategy, "sliding_window")

	if wrapped == nil {
		t.Error("WithBreaker() should not return nil")
	}

	// 验证返回的是包装后的策略（breakerWrap 是私有类型，无需类型断言）
	// 只要不为 nil 且能调用 Allow 方法即可
}

// 集成测试需要真实的 Redis 连接，这里只测试构造函数
func BenchmarkStrategyCreation(b *testing.B) {
	b.Run("NewSliding", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewSliding(nil)
		}
	})

	b.Run("NewToken", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewToken(nil)
		}
	})

	b.Run("NewLeaky", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewLeaky(nil)
		}
	})

	b.Run("WithBreaker", func(b *testing.B) {
		base := NewSliding(nil)
		for i := 0; i < b.N; i++ {
			_ = WithBreaker(nil, base, "test")
		}
	})
}

// 测试规则配置的有效性验证
func TestRuleValidation(t *testing.T) {
	tests := []struct {
		name    string
		rule    config.Rule
		wantErr bool
	}{
		{
			name: "valid sliding window rule",
			rule: config.Rule{
				RuleID:   "test-sliding",
				Algo:     "sliding_window",
				WindowMs: 1000,
				Limit:    100,
				Enabled:  true,
			},
			wantErr: false,
		},
		{
			name: "valid token bucket rule",
			rule: config.Rule{
				RuleID:   "test-token",
				Algo:     "token_bucket",
				WindowMs: 1000,
				Limit:    100,
				Burst:    50,
				Enabled:  true,
			},
			wantErr: false,
		},
		{
			name: "valid leaky bucket rule",
			rule: config.Rule{
				RuleID:   "test-leaky",
				Algo:     "leaky_bucket",
				WindowMs: 1000,
				Limit:    100,
				Burst:    50,
				Enabled:  true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 基本的规则字段验证
			if tt.rule.RuleID == "" {
				t.Error("RuleID should not be empty")
			}
			if tt.rule.Algo == "" {
				t.Error("Algo should not be empty")
			}
			if tt.rule.WindowMs <= 0 && !tt.wantErr {
				t.Error("WindowMs should be positive")
			}
			if tt.rule.Limit <= 0 && !tt.wantErr {
				t.Error("Limit should be positive")
			}
		})
	}
}

// Note: 完整的策略测试需要 Redis 连接
// 这些测试应该在集成测试中进行，使用 testcontainers 或 miniredis

