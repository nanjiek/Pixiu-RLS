package rules

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/rcu"
	"github.com/nanjiek/pixiu-rls/internal/repo"
)

// ImmutableRuleSet 不可变规则集，用于 RCU 快照
type ImmutableRuleSet struct {
	Rules map[string]config.Rule
}

type Cache struct {
	cfg      *config.Config
	rdb      *repo.RedisRepo
	ruleSnap *rcu.Snapshot[ImmutableRuleSet]
}

func NewCache(cfg *config.Config, r *repo.RedisRepo) *Cache {
	// 初始化空的不可变规则集
	initSet := &ImmutableRuleSet{
		Rules: make(map[string]config.Rule),
	}
	return &Cache{
		cfg:      cfg,
		rdb:      r,
		ruleSnap: rcu.NewSnapshot(initSet),
	}
}

func (c *Cache) Bootstrap(ctx context.Context) error {
	// 1) 写入 bootstrap 规则到 Redis（仅首次，不覆盖同名）
	for _, r := range c.cfg.BootstrapRules {
		key := c.rdb.KeyRule(r.RuleID)
		exists, _ := c.rdb.Cli.Exists(ctx, key).Result()
		if exists == 0 {
			b, _ := json.Marshal(r)
			if err := c.rdb.Cli.Set(ctx, key, b, 0).Err(); err != nil {
				return err
			}
		}
	}
	// 2) 全量加载到本地
	return c.ReloadAll(ctx)
}

func (c *Cache) ReloadAll(ctx context.Context) error {
	tmp := make(map[string]config.Rule)
	cursor := uint64(0)
	pattern := c.rdb.KeyRule("*")

	for {
		// 使用SCAN替代KEYS，避免阻塞Redis
		keys, newCursor, err := c.rdb.Cli.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			slog.Error("failed to scan rules", "error", err)
			return err
		}

		for _, key := range keys {
			val, err := c.rdb.Cli.Get(ctx, key).Bytes()
			if err != nil {
				slog.Warn("failed to get rule", "key", key, "error", err)
				continue
			}

			var rule config.Rule
			if err := json.Unmarshal(val, &rule); err != nil {
				slog.Warn("failed to unmarshal rule", "key", key, "error", err)
				continue
			}
			tmp[rule.RuleID] = rule
		}

		cursor = newCursor
		if cursor == 0 {
			break
		}
	}

	c.ReplaceAll(tmp)
	return nil
}

// Resolve 优化规则匹配逻辑，避免默认规则滥用
func (c *Cache) Resolve(ruleID string, dims map[string]string) (config.Rule, error) {
	// 使用 RCU 快照读取规则，无锁并发安全
	snapshot := c.ruleSnap.Load()

	if ruleID != "" {
		if r, ok := snapshot.Rules[ruleID]; ok && r.Enabled {
			return r, nil
		}
		return config.Rule{}, errors.New("rule not found or disabled")
	}

	// 按匹配前缀优先级查找（简化实现）
	for _, r := range snapshot.Rules {
		if r.Enabled && (r.Match == "*" || r.Match == dims["route"]) {
			return r, nil
		}
	}
	return config.Rule{}, errors.New("no enabled rule found")
}

func (c *Cache) StartWatcher(ctx context.Context) {
	sub := c.rdb.Cli.Subscribe(ctx, c.rdb.UpdateChannel)
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			_ = c.ReloadAll(ctx)
		case <-time.After(60 * time.Second):
			_ = c.ReloadAll(ctx) // 定时兜底
		}
	}
}

func (c *Cache) Upsert(ctx context.Context, r config.Rule) error {
	if r.RuleID == "" {
		return errors.New("ruleId required")
	}
	b, _ := json.Marshal(r)
	if err := c.rdb.Cli.Set(ctx, c.rdb.KeyRule(r.RuleID), b, 0).Err(); err != nil {
		return err
	}

	// 更新本地快照：复制当前规则集，修改后替换
	oldSnap := c.ruleSnap.Load()
	newRules := make(map[string]config.Rule, len(oldSnap.Rules)+1)
	for k, v := range oldSnap.Rules {
		newRules[k] = v
	}
	newRules[r.RuleID] = r

	newSet := &ImmutableRuleSet{
		Rules: newRules,
	}
	c.ruleSnap.Replace(newSet)

	return c.rdb.PublishUpdate(ctx, r.RuleID)
}

func (c *Cache) Get(id string) (config.Rule, bool) {
	snapshot := c.ruleSnap.Load()
	r, ok := snapshot.Rules[id]
	return r, ok
}

// GetSnapshot 返回当前的不可变规则集快照（供 Engine 使用）
func (c *Cache) GetSnapshot() *ImmutableRuleSet {
	return c.ruleSnap.Load()
}

// ReplaceAll replaces the entire rule snapshot with a new immutable set.
func (c *Cache) ReplaceAll(rules map[string]config.Rule) {
	newSet := &ImmutableRuleSet{
		Rules: rules,
	}
	c.ruleSnap.Replace(newSet)
	slog.Info("reloaded rules", "count", len(rules))
}

// BuildRuleMap normalizes a rule slice into a map keyed by RuleID.
func BuildRuleMap(rules []config.Rule) map[string]config.Rule {
	res := make(map[string]config.Rule, len(rules))
	for _, r := range rules {
		if r.RuleID == "" {
			continue
		}
		res[r.RuleID] = r
	}
	return res
}
