package rules

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/rules/source"
)

// PollerConfig controls the pull loop behavior.
type PollerConfig struct {
	Interval   time.Duration
	FailPolicy string // fail-open | fail-closed
}

// Poller periodically pulls rules from an external source (e.g., Nacos).
type Poller struct {
	source     source.RuleSource
	cache      *Cache
	interval   time.Duration
	failPolicy string
	lastVer    string
	log        *slog.Logger
	mu         sync.Mutex
}

func NewPoller(src source.RuleSource, cache *Cache, cfg PollerConfig) *Poller {
	interval := cfg.Interval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Poller{
		source:     src,
		cache:      cache,
		interval:   interval,
		failPolicy: strings.ToLower(strings.TrimSpace(cfg.FailPolicy)),
		log:        slog.Default(),
	}
}

// SyncOnce pulls rules once and applies them.
func (p *Poller) SyncOnce(ctx context.Context) error {
	_, err := p.pull(ctx)
	return err
}

// Start runs the polling loop until ctx is done.
func (p *Poller) Start(ctx context.Context) {
	if _, err := p.pull(ctx); err != nil {
		p.log.Warn("nacos pull failed on startup", "error", err)
	}

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := p.pull(ctx); err != nil {
				p.log.Warn("nacos pull failed", "error", err)
			}
		}
	}
}

func (p *Poller) pull(ctx context.Context) (bool, error) {
	payload, err := p.source.Fetch(ctx)
	if err != nil {
		p.handleFailure()
		return false, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if payload.Version != "" && payload.Version == p.lastVer {
		return false, nil
	}

	ruleMap := BuildRuleMap(payload.Rules)
	if len(ruleMap) == 0 {
		p.log.Warn("nacos payload contains no valid rules")
	}

	p.cache.ReplaceAll(ruleMap)
	p.lastVer = payload.Version
	return true, nil
}

func (p *Poller) handleFailure() {
	if p.failPolicy == "fail-closed" {
		p.cache.ReplaceAll(map[string]config.Rule{})
	}
}
