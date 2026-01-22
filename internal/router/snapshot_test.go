package router

import (
	"testing"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
)

func TestBuildRouteSnapshot(t *testing.T) {
	rules := map[string]config.Rule{
		"r1": {RuleID: "r1", Match: "/api", Enabled: true},
		"r2": {RuleID: "r2", Match: "/v1/*", Enabled: true},
		"r3": {RuleID: "r3", Match: "*", Enabled: true},
	}

	snap := BuildRouteSnapshot(rules)
	if len(snap.Exact) != 1 {
		t.Fatalf("exact size = %d", len(snap.Exact))
	}
	if len(snap.Wildcard) != 1 {
		t.Fatalf("wildcard size = %d", len(snap.Wildcard))
	}

	got := snap.Prefix.match("/v1/test")
	if len(got) != 1 || got[0].RuleID != "r2" {
		t.Fatalf("prefix match failed: %#v", got)
	}
}
