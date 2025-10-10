# Pixiu-RLS 开发指南

## 欢迎贡献

感谢您对 Pixiu-RLS 项目的关注！本文档将帮助您快速上手项目开发。

## 开发环境搭建

### 1. 前置要求

- **Go**: 1.21+ 
- **Redis**: 7.0+
- **Git**: 2.x+
- **Make** (可选)
- **Docker** (可选，用于测试)

### 2. 克隆代码

```bash
git clone https://github.com/your-org/pixiu-rls.git
cd pixiu-rls
```

### 3. 安装依赖

```bash
go mod download
go mod verify
```

### 4. 启动 Redis

```bash
# 使用 Docker
docker run -d -p 6379:6379 --name redis-dev redis:7-alpine

# 或直接启动
redis-server
```

### 5. 运行服务

```bash
# 开发模式运行
go run ./cmd/rls-http -c configs/rls.yaml

# 或编译后运行
go build -o rls-http ./cmd/rls-http
./rls-http -c configs/rls.yaml
```

### 6. 验证

```bash
# 健康检查
curl http://localhost:8080/health

# 限流测试
curl -X POST http://localhost:8080/v1/allow \
  -H "Content-Type: application/json" \
  -d '{"ruleId":"global-default","dims":{"ip":"127.0.0.1"}}'
```

## 项目结构详解

```
Pixiu-RLS/
├── cmd/                      # 应用程序入口
│   └── rls-http/            # HTTP 服务主程序
│       └── main.go          # 主函数，依赖注入
│
├── internal/                # 内部包（不对外暴露）
│   ├── api/                 # HTTP API 层
│   │   ├── dto.go          # 数据传输对象
│   │   └── http.go         # HTTP 处理器
│   │
│   ├── config/              # 配置管理
│   │   └── config.go       # 配置结构和加载
│   │
│   ├── core/                # 核心业务逻辑
│   │   ├── engine.go       # 限流引擎（主入口）
│   │   ├── quota.go        # 配额控制器
│   │   └── strategy/       # 限流策略实现
│   │       ├── sliding.go  # 滑动窗口
│   │       ├── token.go    # 令牌桶
│   │       ├── leaky.go    # 漏桶
│   │       └── breaker_wrap.go  # 熔断装饰器
│   │
│   ├── rcu/                 # RCU 无锁快照
│   │   ├── snapshot.go     # 快照实现
│   │   └── snapshot_test.go
│   │
│   ├── repo/                # Redis 数据访问层
│   │   ├── redis.go        # Redis 客户端封装
│   │   └── lua.go          # Lua 脚本定义
│   │
│   ├── rules/               # 规则管理
│   │   └── cache.go        # 规则缓存（使用 RCU）
│   │
│   ├── types/               # 公共类型定义
│   │   └── types.go        # Decision 等类型
│   │
│   └── util/                # 工具函数
│       ├── hash.go         # 哈希函数
│       ├── dim.go          # 维度处理
│       └── conv.go         # 类型转换
│
├── configs/                 # 配置文件
│   └── rls.yaml
│
├── deployments/             # 部署配置
│   └── docker-compose.yaml
│
├── docs/                    # 文档
│   ├── API.md
│   ├── DEPLOYMENT.md
│   └── DEVELOPMENT.md (本文件)
│
├── examples/                # 示例代码
│   └── rcu_example.go
│
├── go.mod                   # Go 模块依赖
├── go.sum
└── README.md               # 项目说明
```

### 关键模块说明

#### 1. Engine (core/engine.go)

核心限流引擎，协调各个组件：

```
Allow() → 黑白名单检查 → 维度哈希 → 配额检查 → 策略执行
```

#### 2. Strategy (core/strategy/)

策略接口和具体实现：

```go
type Strategy interface {
    Allow(ctx context.Context, rule config.Rule, dimKey string, now time.Time) (types.Decision, error)
}
```

#### 3. RCU Snapshot (rcu/snapshot.go)

无锁快照机制，用于高性能规则缓存：

- `Load()`: 无锁读取（0.03ns）
- `Replace()`: 原子替换（21ns）

#### 4. Rules Cache (rules/cache.go)

规则缓存管理：

- `Resolve()`: 查找规则
- `ReloadAll()`: 全量重载
- `Upsert()`: 更新规则

## 开发工作流

### 1. 创建功能分支

```bash
git checkout -b feature/your-feature-name
```

### 2. 编写代码

遵循以下规范：

- 使用 `gofmt` 格式化代码
- 添加必要的注释
- 编写单元测试
- 更新文档

### 3. 运行测试

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test -v ./internal/core/

# 运行测试并查看覆盖率
go test -cover ./...

# 生成覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### 4. 性能基准测试

```bash
# 运行基准测试
go test -bench=. -benchmem ./internal/rcu/

# 对比优化前后性能
go test -bench=BenchmarkLoad -benchmem -count=5 ./internal/rcu/ > old.txt
# ... 修改代码 ...
go test -bench=BenchmarkLoad -benchmem -count=5 ./internal/rcu/ > new.txt
benchstat old.txt new.txt
```

### 5. 代码检查

```bash
# 格式化
go fmt ./...

# 静态检查
go vet ./...

# 使用 golangci-lint (推荐)
golangci-lint run
```

### 6. 提交代码

```bash
git add .
git commit -m "feat: add new feature"
git push origin feature/your-feature-name
```

### 7. 创建 Pull Request

在 GitHub 上创建 PR，并确保：

- PR 描述清晰
- 所有测试通过
- 代码审查通过

## 添加新功能

### 示例：添加新的限流策略

#### 1. 创建策略文件

`internal/core/strategy/custom.go`:

```go
package strategy

import (
    "context"
    "time"
    
    "github.com/nanjiek/pixiu-rls/internal/config"
    "github.com/nanjiek/pixiu-rls/internal/repo"
    "github.com/nanjiek/pixiu-rls/internal/types"
)

// Custom 自定义策略
type Custom struct {
    repo *repo.RedisRepo
}

// NewCustom 创建自定义策略实例
func NewCustom(rdb *repo.RedisRepo) *Custom {
    return &Custom{repo: rdb}
}

// Allow 实现 core.Strategy 接口
func (c *Custom) Allow(ctx context.Context, rule config.Rule, dimKey string, now time.Time) (types.Decision, error) {
    // TODO: 实现您的限流逻辑
    return types.Decision{
        Allowed:   true,
        Remaining: rule.Limit,
        Reason:    "custom_allowed",
    }, nil
}
```

#### 2. 添加测试

`internal/core/strategy/custom_test.go`:

```go
package strategy

import (
    "context"
    "testing"
    "time"
    
    "github.com/nanjiek/pixiu-rls/internal/config"
)

func TestCustom_Allow(t *testing.T) {
    strategy := NewCustom(nil)
    
    rule := config.Rule{
        RuleID: "test",
        Limit:  100,
    }
    
    decision, err := strategy.Allow(context.Background(), rule, "test-key", time.Now())
    
    if err != nil {
        t.Errorf("unexpected error: %v", err)
    }
    
    if !decision.Allowed {
        t.Error("expected allowed=true")
    }
}
```

#### 3. 注册策略

在 `cmd/rls-http/main.go` 中注册：

```go
// 创建策略实例
custom := strategy.NewCustom(rdb)

// 添加到策略映射
strategies := map[string]core.Strategy{
    "sliding_window": sliding,
    "token_bucket":   token,
    "leaky_bucket":   leaky,
    "custom":         custom,  // 新增
}
```

#### 4. 更新文档

在 `docs/API.md` 中添加算法说明。

## 编码规范

### Go 代码规范

1. **命名**：
   - 包名：小写，单数，简短
   - 变量名：驼峰命名
   - 常量：大写驼峰
   - 私有成员：小写开头
   - 公开成员：大写开头

2. **注释**：
   ```go
   // NewEngine 创建引擎实例
   // 参数：
   //   rdb: Redis 仓库
   //   strategies: 策略映射
   // 返回：
   //   *Engine: 引擎实例
   func NewEngine(rdb *repo.RedisRepo, strategies map[string]Strategy) *Engine {
       ...
   }
   ```

3. **错误处理**：
   ```go
   // ✅ 好的做法
   if err := doSomething(); err != nil {
       return fmt.Errorf("do something failed: %w", err)
   }
   
   // ❌ 不好的做法
   doSomething()  // 忽略错误
   ```

4. **上下文传递**：
   ```go
   // ✅ 始终传递 context
   func (s *Strategy) Allow(ctx context.Context, ...) error {
       ...
   }
   ```

### 测试规范

1. **测试文件命名**：`xxx_test.go`
2. **测试函数命名**：`TestXxx` 或 `TestXxx_Scenario`
3. **基准测试命名**：`BenchmarkXxx`
4. **使用表驱动测试**：

```go
func TestHashDims(t *testing.T) {
    tests := []struct {
        name      string
        ruleDims  []string
        inputDims map[string]string
        wantErr   bool
    }{
        {
            name:     "valid dims",
            ruleDims: []string{"ip"},
            inputDims: map[string]string{"ip": "192.168.1.1"},
            wantErr:  false,
        },
        // 更多测试用例...
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := HashDims(tt.ruleDims, tt.inputDims)
            if (err != nil) != tt.wantErr {
                t.Errorf("HashDims() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

## 调试技巧

### 1. 启用调试日志

```go
import "log/slog"

slog.SetLogLoggerLevel(slog.LevelDebug)
```

### 2. 使用 pprof

```go
import _ "net/http/pprof"

go func() {
    http.ListenAndServe("localhost:6060", nil)
}()
```

访问 `http://localhost:6060/debug/pprof/` 查看性能分析。

### 3. Redis 监控

```bash
# 实时监控 Redis 命令
redis-cli MONITOR

# 查看慢查询
redis-cli SLOWLOG GET 10
```

### 4. 单元测试调试

```go
func TestDebug(t *testing.T) {
    t.Log("debug info")
    t.Logf("value: %v", value)
}
```

运行：
```bash
go test -v -run TestDebug
```

## 常见开发任务

### 添加新的 API 端点

1. 在 `internal/api/dto.go` 添加请求/响应结构
2. 在 `internal/api/http.go` 添加处理器
3. 在 `RegisterRoutes()` 中注册路由
4. 添加测试
5. 更新 API 文档

### 添加新的配置项

1. 在 `internal/config/config.go` 添加配置结构
2. 在 `configs/rls.yaml` 添加默认值
3. 更新文档

### 优化性能

1. 使用 pprof 定位瓶颈
2. 编写基准测试
3. 优化代码
4. 对比优化前后的基准测试结果
5. 验证功能正确性

## 发布流程

### 版本号规范

遵循语义化版本 (SemVer)：

- **主版本号 (Major)**：不兼容的 API 修改
- **次版本号 (Minor)**：向后兼容的功能新增
- **修订号 (Patch)**：向后兼容的问题修正

### 发布步骤

1. **更新版本号**

```bash
# 创建发布分支
git checkout -b release/v1.2.0

# 更新 CHANGELOG.md
# 更新版本号
```

2. **运行完整测试**

```bash
go test ./...
go test -race ./...
go vet ./...
```

3. **创建 Tag**

```bash
git tag -a v1.2.0 -m "Release v1.2.0"
git push origin v1.2.0
```

4. **构建发布包**

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o rls-http-linux-amd64 ./cmd/rls-http

# macOS
GOOS=darwin GOARCH=amd64 go build -o rls-http-darwin-amd64 ./cmd/rls-http

# Windows
GOOS=windows GOARCH=amd64 go build -o rls-http-windows-amd64.exe ./cmd/rls-http
```

5. **创建 GitHub Release**

## 常见问题

### Q1: 如何添加新的依赖？

```bash
go get github.com/new/package
go mod tidy
```

### Q2: 测试时如何 mock Redis？

使用 `miniredis` 或接口 mock：

```go
import "github.com/alicebob/miniredis/v2"

func TestWithMockRedis(t *testing.T) {
    mr, _ := miniredis.Run()
    defer mr.Close()
    
    // 使用 mr.Addr() 作为 Redis 地址
}
```

### Q3: 如何调试 Lua 脚本？

在 Redis 中直接执行：

```bash
redis-cli EVAL "$(cat internal/repo/sliding_window.lua)" 1 test:key 1000 100 10
```

## 资源链接

- [Go 官方文档](https://golang.org/doc/)
- [Effective Go](https://golang.org/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Redis 文档](https://redis.io/documentation)
- [项目 GitHub](https://github.com/your-org/pixiu-rls)

## 获取帮助

- **Issue**: 在 GitHub 提交 Issue
- **讨论**: GitHub Discussions
- **邮件**: dev@pixiu-rls.io

---

**感谢您的贡献！** 🎉

---

**版本**: v1.0  
**最后更新**: 2024-01

