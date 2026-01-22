package limiter

import (
	"context"
	"errors"
	"strings"
	"time"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/types"
)

// Limiter defines the limiter interface used by the engine.
type Limiter interface {
	Allow(ctx context.Context, rule config.Rule, key string, now time.Time) (types.Decision, error)
}

// Mux routes to a limiter by rule.Algo with a default fallback.
type Mux struct {
	defaultAlgo string
	limiters    map[string]Limiter
}

func NewMux(defaultAlgo string, limiters map[string]Limiter) *Mux {
	if len(limiters) == 0 {
		panic("limiter: empty limiter map")
	}
	// Normalize limiter map keys
	normalized := make(map[string]Limiter, len(limiters))
	for k, v := range limiters {
		normalized[normalizeAlgo(k)] = v
	}
	if strings.TrimSpace(defaultAlgo) == "" {
		defaultAlgo = "token_bucket"
	}
	return &Mux{
		defaultAlgo: normalizeAlgo(defaultAlgo),
		limiters:    limiters,
	}
}

func (m *Mux) Allow(ctx context.Context, rule config.Rule, key string, now time.Time) (types.Decision, error) {
	algo := normalizeAlgo(rule.Algo)
	if algo == "" {
		algo = m.defaultAlgo
	}
	lim, ok := m.limiters[algo]
	if !ok || lim == nil {
		return types.Decision{Allowed: false, Reason: "unsupported_algorithm"}, errors.New("unsupported algorithm: " + algo)
	}
	return lim.Allow(ctx, rule, key, now)
}

func normalizeAlgo(algo string) string {
	return strings.ToLower(strings.TrimSpace(algo))
}
