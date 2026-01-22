package rules

import (
	"context"
	"errors"
	"testing"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/rules/source"
)

type fakeSource struct {
	payload source.RulesPayload
	err     error
}

func (f *fakeSource) Fetch(ctx context.Context) (source.RulesPayload, error) {
	if f.err != nil {
		return source.RulesPayload{}, f.err
	}
	return f.payload, nil
}

func TestPollerSyncOnceUpdatesSnapshot(t *testing.T) {
	cache := NewCache(&config.Config{}, nil)
	src := &fakeSource{
		payload: source.RulesPayload{
			Version: "v1",
			Rules: []config.Rule{
				{RuleID: "r1", Algo: "token_bucket", WindowMs: 1000, Limit: 10, Enabled: true},
			},
		},
	}

	poller := NewPoller(src, cache, PollerConfig{})
	if err := poller.SyncOnce(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	snap := cache.GetSnapshot()
	if len(snap.Rules) != 1 || snap.Rules["r1"].RuleID != "r1" {
		t.Fatalf("unexpected snapshot: %#v", snap.Rules)
	}
}

func TestPollerSkipsSameVersion(t *testing.T) {
	cache := NewCache(&config.Config{}, nil)
	src := &fakeSource{
		payload: source.RulesPayload{
			Version: "v1",
			Rules: []config.Rule{
				{RuleID: "r1", Algo: "token_bucket", WindowMs: 1000, Limit: 10, Enabled: true},
			},
		},
	}

	poller := NewPoller(src, cache, PollerConfig{})
	if err := poller.SyncOnce(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	first := cache.GetSnapshot()

	src.payload = source.RulesPayload{
		Version: "v1",
		Rules: []config.Rule{
			{RuleID: "r2", Algo: "token_bucket", WindowMs: 1000, Limit: 10, Enabled: true},
		},
	}
	if err := poller.SyncOnce(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	second := cache.GetSnapshot()
	if first != second {
		t.Fatalf("snapshot should not be replaced on same version")
	}
	if _, ok := second.Rules["r1"]; !ok {
		t.Fatalf("unexpected snapshot contents: %#v", second.Rules)
	}
}

func TestPollerFailClosedClearsRules(t *testing.T) {
	cache := NewCache(&config.Config{}, nil)
	cache.ReplaceAll(map[string]config.Rule{
		"r1": {RuleID: "r1", Algo: "token_bucket", WindowMs: 1000, Limit: 10, Enabled: true},
	})

	src := &fakeSource{err: errors.New("boom")}
	poller := NewPoller(src, cache, PollerConfig{FailPolicy: "fail-closed"})

	if err := poller.SyncOnce(context.Background()); err == nil {
		t.Fatalf("expected error")
	}

	snap := cache.GetSnapshot()
	if len(snap.Rules) != 0 {
		t.Fatalf("expected empty snapshot")
	}
}
