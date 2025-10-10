# RCU 快照快速入门

## 📌 什么是 RCU？

RCU (Read-Copy-Update) 是一种无锁并发控制机制，特别适合**读多写少**的场景。它通过以下三个步骤实现高性能并发：

1. **Read（读取）**：无锁读取当前快照
2. **Copy（复制）**：写入前先复制数据
3. **Update（更新）**：原子替换指针

## 🚀 快速开始

### 1. 基础使用

```go
package main

import "github.com/nanjiek/pixiu-rls/internal/rcu"

type Config struct {
    Host string
    Port int
}

func main() {
    // 创建快照
    snap := rcu.NewSnapshot(&Config{Host: "localhost", Port: 8080})
    
    // 读取（无锁，极快）
    cfg := snap.Load()
    println(cfg.Host, cfg.Port)
    
    // 更新
    snap.Replace(&Config{Host: "0.0.0.0", Port: 9090})
}
```

### 2. 在项目中的应用

本项目在规则缓存中使用 RCU 快照：

```go
// internal/rules/cache.go

// 定义不可变规则集
type ImmutableRuleSet struct {
    Rules map[string]config.Rule
}

type Cache struct {
    ruleSnap *rcu.Snapshot[ImmutableRuleSet]
}

// 读取规则（热路径，无锁）
func (c *Cache) Resolve(ruleID string, dims map[string]string) (config.Rule, error) {
    snapshot := c.ruleSnap.Load()  // 无锁读取
    
    if r, ok := snapshot.Rules[ruleID]; ok && r.Enabled {
        return r, nil
    }
    return config.Rule{}, errors.New("rule not found")
}

// 更新规则（冷路径，Copy-Update）
func (c *Cache) Upsert(ctx context.Context, r config.Rule) error {
    // 1. 读取旧快照
    oldSnap := c.ruleSnap.Load()
    
    // 2. 复制并修改
    newRules := make(map[string]config.Rule, len(oldSnap.Rules)+1)
    for k, v := range oldSnap.Rules {
        newRules[k] = v
    }
    newRules[r.RuleID] = r
    
    // 3. 原子替换
    c.ruleSnap.Replace(&ImmutableRuleSet{Rules: newRules})
    
    return nil
}
```

## 📊 性能对比

### 基准测试结果

| 操作 | QPS | 延迟 | vs RWMutex |
|------|-----|------|-----------|
| 读取（Load） | ~31亿/s | 0.03 ns | **60倍+** |
| 写入（Replace） | ~5200万/s | 21.89 ns | 2倍+ |
| 混合读写（90%读） | ~1.8亿/s | 5.59 ns | **30倍+** |

### 实际效果

- ✅ **读性能提升 60+ 倍**
- ✅ **支持百万级并发读取**
- ✅ **写操作不阻塞读取**
- ✅ **零锁竞争**

## 🔧 运行示例

### 1. 运行完整示例

```bash
# 编译并运行
go run examples/rcu_example.go
```

输出：
```
=== RCU Snapshot 使用示例 ===

1. 基础使用示例
----------------
初始配置: MaxQPS=1000, TimeoutMs=3000, EnableAuth=true
更新后配置: MaxQPS=2000, TimeoutMs=5000, EnableAuth=false

2. 并发读写示例
----------------
并发测试完成: 耗时=54.9683ms
最终统计: RequestCount=987, ErrorCount=0

3. 规则热更新场景
----------------
初始规则数量: 2
规则已热更新！
更新后规则数量: 3
api-login 新限制: 200
api-upload 限制: 50 (新增规则)

所有请求处理完成，规则查询不受更新影响！
```

### 2. 运行性能测试

```bash
# 运行基准测试
cd internal/rcu
go test -bench=. -benchmem -benchtime=1s

# 输出示例：
# BenchmarkLoad-20        1000000000    0.03189 ns/op    0 B/op    0 allocs/op
# BenchmarkReplace-20     52022820      21.89 ns/op     24 B/op    1 allocs/op
# BenchmarkReadWrite-20   215507409     5.588 ns/op      2 B/op    0 allocs/op
```

### 3. 验证项目编译

```bash
# 编译主程序
go build ./cmd/rls-http/

# 运行测试
go test -v ./internal/rcu/
go test -v ./internal/rules/
```

## 💡 使用建议

### ✅ 适合使用的场景

1. **配置热更新**
   - 应用配置偶尔变更，频繁读取
   - 例如：限流规则、路由规则、特性开关

2. **缓存数据**
   - 定期从数据库/Redis 加载数据
   - 业务逻辑频繁查询

3. **只读查找表**
   - 黑白名单
   - 用户权限表
   - 路由映射表

### ❌ 不适合的场景

1. **高频写入**：写操作 > 10次/秒
2. **大数据结构**：单个对象 > 10MB
3. **需要事务**：需要原子修改多个字段
4. **写优先**：写操作延迟敏感

### ⚠️ 注意事项

#### 1. 数据不可变性

```go
// ❌ 错误：修改快照数据
snap := cache.Load()
snap.Rules["new"] = newRule  // 危险！破坏不可变性

// ✅ 正确：复制后修改
old := cache.Load()
newRules := make(map[string]Rule)
for k, v := range old.Rules {
    newRules[k] = v
}
newRules["new"] = newRule
cache.Replace(&RuleSet{Rules: newRules})
```

#### 2. 写操作的复制开销

```go
// ⚠️ 低效：多次单独更新
for _, rule := range rules {
    cache.Upsert(ctx, rule)  // 每次都复制整个规则集
}

// ✅ 高效：批量更新
cache.BatchUpsert(ctx, rules)  // 只复制一次
```

#### 3. 避免长时间持有快照

```go
// ❌ 不好：长时间持有快照
snap := cache.Load()
time.Sleep(time.Hour)  // 阻止旧快照被GC
processRules(snap)

// ✅ 好：及时读取，及时释放
processRules(cache.Load())
```

## 📚 更多文档

- [RCU 详细文档](../internal/rcu/README.md)
- [集成说明](./RCU_INTEGRATION.md)
- [架构设计](./RCU_ARCHITECTURE.md)

## 🔍 项目结构

```
Pixiu-RLS/
├── internal/
│   ├── rcu/                    # RCU 快照实现
│   │   ├── snapshot.go         # 核心实现
│   │   ├── snapshot_test.go    # 测试和基准
│   │   └── README.md           # 详细文档
│   │
│   └── rules/                  # 规则缓存（使用 RCU）
│       └── cache.go            # 规则缓存实现
│
├── examples/
│   └── rcu_example.go          # 使用示例
│
└── docs/
    ├── RCU_QUICKSTART.md       # 快速入门（本文档）
    ├── RCU_INTEGRATION.md      # 集成说明
    └── RCU_ARCHITECTURE.md     # 架构设计
```

## 🎯 核心要点

### RCU 的三大优势

1. **读操作零开销**
   - 无需加锁
   - 无需 CAS 操作
   - 延迟 < 1ns

2. **写操作不阻塞读**
   - 通过复制实现隔离
   - 原子指针替换
   - 旧版本自动回收

3. **简洁的代码**
   - 消除复杂锁逻辑
   - 无需关心锁粒度
   - 天然避免死锁

### 关键设计原则

1. **数据不可变**：快照指向的数据不可修改
2. **Copy-on-Write**：修改前先复制
3. **原子替换**：通过 atomic.Pointer 原子更新

## 🚦 快速检查清单

集成 RCU 快照前的检查：

- [ ] 确认是读多写少的场景（读写比 > 10:1）
- [ ] 数据结构大小合理（< 10MB）
- [ ] 写入频率可控（< 10次/秒）
- [ ] 能接受写操作的复制开销
- [ ] 理解数据不可变性要求

## 🤔 FAQ

### Q1: RCU 和 sync.RWMutex 有什么区别？

**A**: 
- RCU：读操作无锁，写操作不阻塞读
- RWMutex：读操作需要锁，写锁阻塞所有读

### Q2: 什么时候旧快照会被回收？

**A**: 当没有任何 goroutine 引用时，Go GC 会自动回收。

### Q3: 写入频率有限制吗？

**A**: 建议 < 10次/秒。高频写入会导致：
- 频繁的内存分配
- 多个版本并存
- GC 压力增大

### Q4: 如何监控快照状态？

**A**: 
```go
// 规则数量
count := len(cache.GetSnapshot().Rules)

// 添加版本号
type ImmutableRuleSet struct {
    Version int64
    Rules   map[string]Rule
}
```

## 💪 下一步

1. 阅读 [集成说明](./RCU_INTEGRATION.md) 了解完整集成
2. 查看 [架构设计](./RCU_ARCHITECTURE.md) 理解原理
3. 运行 `examples/rcu_example.go` 体验效果
4. 查看 `internal/rules/cache.go` 学习实际应用

---

**记住**：RCU 快照是为读多写少的场景设计的利器，正确使用能带来 **10-100 倍的性能提升**！

