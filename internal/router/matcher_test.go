package router

import (
	"testing"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/identity"
)

func TestMatcherMatchOrder(t *testing.T) {
	rules := map[string]config.Rule{
		"r1": {
			RuleID:   "r1",
			Match:    "/api",
			Methods:  []string{"GET"},
			Client:   identity.KindUser,
			Priority: 10,
			Enabled:  true,
		},
		"r2": {
			RuleID:   "r2",
			Match:    "/api",
			Priority: 5,
			Enabled:  true,
		},
		"r3": {
			RuleID:   "r3",
			Match:    "/v1/*",
			Priority: 7,
			Enabled:  true,
		},
	}

	snap := BuildRouteSnapshot(rules)
	matcher := NewMatcher(snap)

	got := matcher.Match(RequestCtx{
		Path:   "/api",
		Method: "GET",
		Client: identity.ClientKey{Kind: identity.KindUser},
	})

	if len(got) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(got))
	}
	if got[0].RuleID != "r1" || got[1].RuleID != "r2" {
		t.Fatalf("unexpected order: %v", []string{got[0].RuleID, got[1].RuleID})
	}
}

func TestMatcherFiltersMethodAndClient(t *testing.T) {
	rules := map[string]config.Rule{
		"r1": {
			RuleID:  "r1",
			Match:   "/api",
			Methods: []string{"POST"},
			Client:  identity.KindUser,
			Enabled: true,
		},
		"r2": {
			RuleID:  "r2",
			Match:   "/api",
			Client:  identity.KindIP,
			Enabled: true,
		},
	}

	matcher := NewMatcher(BuildRouteSnapshot(rules))
	got := matcher.Match(RequestCtx{
		Path:   "/api",
		Method: "GET",
		Client: identity.ClientKey{Kind: identity.KindUser},
	})
	if len(got) != 0 {
		t.Fatalf("expected no rules, got %d", len(got))
	}
}

func TestMatcherPrefixAndWildcard(t *testing.T) {
	rules := map[string]config.Rule{
		"r1": {RuleID: "r1", Match: "/v1/*", Enabled: true},
		"r2": {RuleID: "r2", Match: "*", Enabled: true},
	}

	matcher := NewMatcher(BuildRouteSnapshot(rules))
	got := matcher.Match(RequestCtx{Path: "/v1/a", Method: "GET"})
	if len(got) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(got))
	}
}
