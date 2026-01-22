package router

import (
	"sort"
	"strings"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/identity"
	"github.com/nanjiek/pixiu-rls/internal/rcu"
)

// RequestCtx is the input used for rule matching.
type RequestCtx struct {
	Path   string
	Method string
	Client identity.ClientKey
}

// Matcher matches rules from a route snapshot.
type Matcher struct {
	snap *rcu.Snapshot[RouteSnapshot]
}

func NewMatcher(initial *RouteSnapshot) *Matcher {
	if initial == nil {
		initial = BuildRouteSnapshot(map[string]config.Rule{})
	}
	return &Matcher{snap: rcu.NewSnapshot(initial)}
}

func (m *Matcher) Replace(snapshot *RouteSnapshot) {
	if snapshot == nil {
		snapshot = BuildRouteSnapshot(map[string]config.Rule{})
	}
	m.snap.Replace(snapshot)
}

// Match returns all matching rules ordered by priority (desc).
func (m *Matcher) Match(ctx RequestCtx) []config.Rule {
	snap := m.snap.Load()
	if snap == nil {
		return nil
	}
	var res []config.Rule

	if ctx.Path != "" {
		if rules, ok := snap.Exact[ctx.Path]; ok {
			res = append(res, filterRules(rules, ctx)...)
		}
		res = append(res, filterRules(snap.Prefix.match(ctx.Path), ctx)...)
	}
	res = append(res, filterRules(snap.Wildcard, ctx)...)

	sort.SliceStable(res, func(i, j int) bool {
		if res[i].Priority == res[j].Priority {
			return res[i].RuleID < res[j].RuleID
		}
		return res[i].Priority > res[j].Priority
	})

	return res
}

func filterRules(rules []config.Rule, ctx RequestCtx) []config.Rule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]config.Rule, 0, len(rules))
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if !matchMethod(r.Methods, ctx.Method) {
			continue
		}
		if !matchClient(r.Client, ctx.Client.Kind) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func matchMethod(methods []string, method string) bool {
	if len(methods) == 0 {
		return true
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	for _, m := range methods {
		m = strings.ToUpper(strings.TrimSpace(m))
		if m == "*" || m == method {
			return true
		}
	}
	return false
}

func matchClient(ruleClient, requestClient string) bool {
	if ruleClient == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(ruleClient), strings.TrimSpace(requestClient))
}
