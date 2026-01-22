package source

import (
	"context"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
)

// RulesPayload is a normalized rule set fetched from an external source.
type RulesPayload struct {
	Rules   []config.Rule
	Version string
}

// RuleSource fetches rules from an external system (e.g., Nacos).
type RuleSource interface {
	Fetch(ctx context.Context) (RulesPayload, error)
}
