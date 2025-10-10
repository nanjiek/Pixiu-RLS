package rules

import (
	"testing"
	"time"

	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/rcu"
)

func TestImmutableRuleSet(t *testing.T) {
	rules := map[string]config.Rule{
		"rule1": {RuleID: "rule1", Limit: 100, Enabled: true},
		"rule2": {RuleID: "rule2", Limit: 200, Enabled: true},
	}

	ruleSet := &ImmutableRuleSet{Rules: rules}

	if len(ruleSet.Rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(ruleSet.Rules))
	}

	if rule, ok := ruleSet.Rules["rule1"]; !ok || rule.Limit != 100 {
		t.Error("rule1 not found or incorrect")
	}
}

func TestCacheSnapshot(t *testing.T) {
	cfg := &config.Config{
		BootstrapRules: []config.Rule{
			{RuleID: "test-rule", Match: "/api/test", Enabled: true, Limit: 100},
		},
	}

	// 创建快照
	initSet := &ImmutableRuleSet{
		Rules: map[string]config.Rule{
			"test-rule": cfg.BootstrapRules[0],
		},
	}
	snap := rcu.NewSnapshot(initSet)

	// 测试读取
	loaded := snap.Load()
	if len(loaded.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(loaded.Rules))
	}

	// 测试更新
	newRule := config.Rule{RuleID: "new-rule", Match: "/api/new", Enabled: true, Limit: 200}
	newRules := make(map[string]config.Rule)
	for k, v := range loaded.Rules {
		newRules[k] = v
	}
	newRules["new-rule"] = newRule

	newSet := &ImmutableRuleSet{Rules: newRules}
	snap.Replace(newSet)

	// 验证更新
	updated := snap.Load()
	if len(updated.Rules) != 2 {
		t.Errorf("expected 2 rules after update, got %d", len(updated.Rules))
	}

	if rule, ok := updated.Rules["new-rule"]; !ok || rule.Limit != 200 {
		t.Error("new-rule not found or incorrect after update")
	}
}

func TestCacheConcurrentRead(t *testing.T) {
	initSet := &ImmutableRuleSet{
		Rules: map[string]config.Rule{
			"rule1": {RuleID: "rule1", Limit: 100, Enabled: true},
		},
	}
	snap := rcu.NewSnapshot(initSet)

	// 并发读取
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			for j := 0; j < 1000; j++ {
				rules := snap.Load()
				if len(rules.Rules) == 0 {
					t.Error("expected non-empty rules")
				}
			}
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestCacheConcurrentReadWrite(t *testing.T) {
	initSet := &ImmutableRuleSet{
		Rules: map[string]config.Rule{
			"rule1": {RuleID: "rule1", Limit: 100, Enabled: true},
		},
	}
	snap := rcu.NewSnapshot(initSet)

	done := make(chan bool)

	// 并发读取
	for i := 0; i < 50; i++ {
		go func() {
			for j := 0; j < 500; j++ {
				_ = snap.Load()
			}
			done <- true
		}()
	}

	// 并发写入
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 50; j++ {
				old := snap.Load()
				newRules := make(map[string]config.Rule)
				for k, v := range old.Rules {
					newRules[k] = v
				}
				// 添加新规则
				newRules[string(rune('a'+id))] = config.Rule{
					RuleID:  string(rune('a' + id)),
					Limit:   int64(id * 100),
					Enabled: true,
				}
				snap.Replace(&ImmutableRuleSet{Rules: newRules})
				time.Sleep(time.Microsecond)
			}
			done <- true
		}(i)  // 传递 i 参数
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 60; i++ {
		<-done
	}

	// 验证最终状态
	final := snap.Load()
	if len(final.Rules) < 1 {
		t.Error("expected at least 1 rule in final state")
	}
}

func BenchmarkCacheLoad(b *testing.B) {
	rules := make(map[string]config.Rule)
	for i := 0; i < 100; i++ {
		id := string(rune('a' + i%26)) + string(rune('0' + i%10))
		rules[id] = config.Rule{
			RuleID:  id,
			Limit:   int64(i * 100),
			Enabled: true,
		}
	}

	snap := rcu.NewSnapshot(&ImmutableRuleSet{Rules: rules})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = snap.Load()
		}
	})
}

func BenchmarkCacheReplace(b *testing.B) {
	snap := rcu.NewSnapshot(&ImmutableRuleSet{
		Rules: map[string]config.Rule{
			"rule1": {RuleID: "rule1", Limit: 100, Enabled: true},
		},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		old := snap.Load()
		newRules := make(map[string]config.Rule)
		for k, v := range old.Rules {
			newRules[k] = v
		}
		newRules["new"] = config.Rule{RuleID: "new", Limit: 200, Enabled: true}
		snap.Replace(&ImmutableRuleSet{Rules: newRules})
	}
}

// 模拟实际使用场景的基准测试
func BenchmarkCacheResolve(b *testing.B) {
	rules := make(map[string]config.Rule)
	for i := 0; i < 10; i++ {
		id := "rule-" + string(rune('0'+i))
		rules[id] = config.Rule{
			RuleID:  id,
			Match:   "/api/" + string(rune('0'+i)),
			Limit:   int64(i * 100),
			Enabled: true,
		}
	}

	snap := rcu.NewSnapshot(&ImmutableRuleSet{Rules: rules})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			snapshot := snap.Load()
			// 模拟规则查找
			if _, ok := snapshot.Rules["rule-5"]; !ok {
				b.Error("rule not found")
			}
		}
	})
}

