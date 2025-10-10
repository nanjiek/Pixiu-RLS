package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/nanjiek/pixiu-rls/internal/rcu"
)

// UserConfig 用户配置示例
type UserConfig struct {
	MaxQPS     int
	TimeoutMs  int
	EnableAuth bool
}

func main() {
	fmt.Println("=== RCU Snapshot 使用示例 ===")

	// 示例1：基础使用
	basicUsage()

	// 示例2：并发读写
	concurrentReadWrite()

	// 示例3：规则热更新场景
	ruleHotReload()
}

// 示例1：基础使用
func basicUsage() {
	fmt.Println("1. 基础使用示例")
	fmt.Println("----------------")

	// 创建初始配置
	initConfig := &UserConfig{
		MaxQPS:     1000,
		TimeoutMs:  3000,
		EnableAuth: true,
	}

	// 创建快照
	configSnap := rcu.NewSnapshot(initConfig)

	// 读取配置（无锁，极快）
	cfg := configSnap.Load()
	fmt.Printf("初始配置: MaxQPS=%d, TimeoutMs=%d, EnableAuth=%v\n",
		cfg.MaxQPS, cfg.TimeoutMs, cfg.EnableAuth)

	// 更新配置
	newConfig := &UserConfig{
		MaxQPS:     2000,
		TimeoutMs:  5000,
		EnableAuth: false,
	}
	configSnap.Replace(newConfig)

	// 读取新配置
	cfg = configSnap.Load()
	fmt.Printf("更新后配置: MaxQPS=%d, TimeoutMs=%d, EnableAuth=%v\n\n",
		cfg.MaxQPS, cfg.TimeoutMs, cfg.EnableAuth)
}

// 示例2：并发读写
func concurrentReadWrite() {
	fmt.Println("2. 并发读写示例")
	fmt.Println("----------------")

	type Stats struct {
		RequestCount int64
		ErrorCount   int64
	}

	statsSnap := rcu.NewSnapshot(&Stats{RequestCount: 0, ErrorCount: 0})

	var wg sync.WaitGroup
	startTime := time.Now()

	// 启动100个并发读取goroutine
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10000; j++ {
				// 无锁读取，极高性能
				stats := statsSnap.Load()
				_ = stats.RequestCount
			}
		}(i)
	}

	// 启动10个并发写入goroutine
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				// 读取当前快照
				old := statsSnap.Load()

				// 创建新快照（不修改旧数据）
				newStats := &Stats{
					RequestCount: old.RequestCount + 1,
					ErrorCount:   old.ErrorCount,
				}

				// 原子替换
				statsSnap.Replace(newStats)

				time.Sleep(time.Microsecond * 10)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	finalStats := statsSnap.Load()
	fmt.Printf("并发测试完成: 耗时=%v\n", duration)
	fmt.Printf("最终统计: RequestCount=%d, ErrorCount=%d\n\n",
		finalStats.RequestCount, finalStats.ErrorCount)
}

// 示例3：规则热更新场景
func ruleHotReload() {
	fmt.Println("3. 规则热更新场景")
	fmt.Println("----------------")

	type Rule struct {
		RuleID string
		Limit  int64
		Algo   string
	}

	type RuleSet struct {
		Rules map[string]Rule
	}

	// 初始规则集
	initRules := &RuleSet{
		Rules: map[string]Rule{
			"api-login": {RuleID: "api-login", Limit: 100, Algo: "sliding_window"},
			"api-query": {RuleID: "api-query", Limit: 1000, Algo: "token_bucket"},
		},
	}

	ruleSnap := rcu.NewSnapshot(initRules)
	fmt.Printf("初始规则数量: %d\n", len(ruleSnap.Load().Rules))

	// 模拟多个请求并发查询规则
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				// 无锁读取规则（热路径）
				rules := ruleSnap.Load()
				if rule, ok := rules.Rules["api-login"]; ok {
					_ = rule.Limit
				}
			}
		}(i)
	}

	// 模拟规则热更新（冷路径）
	time.Sleep(time.Millisecond * 100)

	// 更新规则：添加新规则
	oldRules := ruleSnap.Load()
	newRulesMap := make(map[string]Rule)

	// 复制旧规则
	for k, v := range oldRules.Rules {
		newRulesMap[k] = v
	}

	// 添加新规则
	newRulesMap["api-upload"] = Rule{
		RuleID: "api-upload",
		Limit:  50,
		Algo:   "leaky_bucket",
	}

	// 修改现有规则
	newRulesMap["api-login"] = Rule{
		RuleID: "api-login",
		Limit:  200, // 限制从100提升到200
		Algo:   "sliding_window",
	}

	// 原子替换规则集
	newRules := &RuleSet{Rules: newRulesMap}
	ruleSnap.Replace(newRules)

	fmt.Println("规则已热更新！")
	fmt.Printf("更新后规则数量: %d\n", len(ruleSnap.Load().Rules))

	// 验证新规则
	currentRules := ruleSnap.Load()
	if rule, ok := currentRules.Rules["api-login"]; ok {
		fmt.Printf("api-login 新限制: %d\n", rule.Limit)
	}
	if rule, ok := currentRules.Rules["api-upload"]; ok {
		fmt.Printf("api-upload 限制: %d (新增规则)\n", rule.Limit)
	}

	wg.Wait()
	fmt.Println("\n所有请求处理完成，规则查询不受更新影响！")
}

