package router

import (
	"strings"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
)

// RouteSnapshot is an immutable index built from rules.
type RouteSnapshot struct {
	Exact    map[string][]config.Rule
	Prefix   *trieNode
	Wildcard []config.Rule
}

type trieNode struct {
	children map[rune]*trieNode
	rules    []config.Rule
}

func newTrie() *trieNode {
	return &trieNode{children: make(map[rune]*trieNode)}
}

func (t *trieNode) insert(prefix string, rule config.Rule) {
	node := t
	for _, ch := range prefix {
		if node.children == nil {
			node.children = make(map[rune]*trieNode)
		}
		next := node.children[ch]
		if next == nil {
			next = &trieNode{children: make(map[rune]*trieNode)}
			node.children[ch] = next
		}
		node = next
	}
	node.rules = append(node.rules, rule)
}

func (t *trieNode) match(path string) []config.Rule {
	if t == nil {
		return nil
	}
	node := t
	var out []config.Rule
	for _, ch := range path {
		if node == nil {
			break
		}
		if len(node.rules) > 0 {
			out = append(out, node.rules...)
		}
		node = node.children[ch]
	}
	if node != nil && len(node.rules) > 0 {
		out = append(out, node.rules...)
	}
	return out
}

// BuildRouteSnapshot builds a route index from a rule map.
func BuildRouteSnapshot(rules map[string]config.Rule) *RouteSnapshot {
	snap := &RouteSnapshot{
		Exact:    make(map[string][]config.Rule),
		Prefix:   newTrie(),
		Wildcard: make([]config.Rule, 0),
	}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		match := strings.TrimSpace(rule.Match)
		if match == "" || match == "*" {
			snap.Wildcard = append(snap.Wildcard, rule)
			continue
		}
		if strings.HasSuffix(match, "*") && len(match) > 1 {
			prefix := strings.TrimSuffix(match, "*")
			snap.Prefix.insert(prefix, rule)
			continue
		}
		snap.Exact[match] = append(snap.Exact[match], rule)
	}
	return snap
}
