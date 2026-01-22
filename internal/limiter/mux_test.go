package limiter

import (
	"context"
	"testing"
	"time"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/types"
)

type mockLimiter struct {
	allowed bool
}

func (m *mockLimiter) Allow(ctx context.Context, rule config.Rule, key string, now time.Time) (types.Decision, error) {
	return types.Decision{Allowed: m.allowed, Reason: "mock"}, nil
}

func TestMux_DefaultAlgo(t *testing.T) {
	mux := NewMux("token_bucket", map[string]Limiter{
		"token_bucket": &mockLimiter{allowed: true},
	})
	dec, err := mux.Allow(context.Background(), config.Rule{}, "k1", time.Now())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !dec.Allowed {
		t.Fatalf("expected allowed, got %+v", dec)
	}
}

func TestMux_UnsupportedAlgo(t *testing.T) {
	mux := NewMux("token_bucket", map[string]Limiter{
		"token_bucket": &mockLimiter{allowed: true},
	})
	_, err := mux.Allow(context.Background(), config.Rule{Algo: "unknown"}, "k1", time.Now())
	if err == nil {
		t.Fatal("expected error for unsupported algorithm")
	}
}
