# Pixiu-RLS API 文档

## 概述

Pixiu-RLS 提供 RESTful API 接口，用于限流判断和规则管理。所有接口返回 JSON 格式数据。

## 基础信息

- **基础 URL**: `http://localhost:8080` (默认)
- **API 版本**: v1
- **内容类型**: `application/json`

## 通用响应格式

### 成功响应

```json
{
  "code": 200,
  "data": { ...},
  "message": "success"
}
```

### 错误响应

```json
{
  "code": 400,
  "error": "error message",
  "message": "failed"
}
```

## API 接口

### 1. 限流判断

#### 请求

```http
POST /v1/allow
Content-Type: application/json
```

**请求体**：

```json
{
  "ruleId": "api-login",
  "dims": {
    "ip": "192.168.1.1",
    "route": "/api/login",
    "user_id": "12345"
  }
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `ruleId` | string | 是 | 规则 ID |
| `dims` | object | 否 | 维度键值对，不传时自动从请求中提取 IP 和路由 |

**维度说明**：
- 如果未提供 `ip`，系统会自动从请求的 RemoteAddr 提取
- 如果未提供 `route`，系统会自动使用请求的 URL.Path

#### 响应

**允许通过**：

```json
{
  "code": 200,
  "data": {
    "allowed": true,
    "remaining": 95,
    "retryAfterMs": 0,
    "reason": "sliding_window_allowed"
  },
  "message": "success"
}
```

**限流拒绝**：

```json
{
  "code": 200,
  "data": {
    "allowed": false,
    "remaining": 0,
    "retryAfterMs": 1000,
    "reason": "sliding_window_exceeded"
  },
  "message": "success"
}
```

**响应字段**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `allowed` | boolean | 是否允许请求通过 |
| `remaining` | int64 | 剩余可用配额 |
| `retryAfterMs` | int64 | 建议重试时间（毫秒），0 表示无需等待 |
| `reason` | string | 判定原因 |

**原因代码**：

| Reason | 说明 |
|--------|------|
| `sliding_window_allowed` | 滑动窗口算法允许 |
| `sliding_window_exceeded` | 滑动窗口超限 |
| `token_bucket_allowed` | 令牌桶允许 |
| `token_bucket_no_tokens` | 令牌桶令牌不足 |
| `leaky_bucket_allowed` | 漏桶允许 |
| `leaky_bucket_overflow` | 漏桶溢出 |
| `quota_exceeded:min` | 分钟级配额超限 |
| `quota_exceeded:hour` | 小时级配额超限 |
| `quota_exceeded:day` | 天级配额超限 |
| `ip_in_blacklist` | IP 在黑名单中 |
| `ip_in_whitelist` | IP 在白名单中（允许） |
| `rule_disabled` | 规则已禁用 |
| `unsupported_algorithm` | 不支持的算法 |
| `dim_hash_failed` | 维度哈希失败（缺少必需维度） |

### 2. 创建规则

#### 请求

```http
POST /v1/rules
Content-Type: application/json
```

**请求体**：

```json
{
  "ruleId": "api-login",
  "match": "/api/login",
  "algo": "sliding_window",
  "windowMs": 1000,
  "limit": 10,
  "burst": 5,
  "dims": ["ip", "user_id"],
  "enabled": true,
  "quota": {
    "perMinute": 100,
    "perHour": 1000,
    "perDay": 10000
  },
  "breaker": {
    "enabled": true,
    "rlDenyThreshold": 20,
    "rlDenyWindowMs": 10000,
    "minOpenMs": 8000,
    "halfOpenProbePercent": 10,
    "halfOpenMinPass": 5,
    "halfOpenMaxFail": 3
  }
}
```

**请求字段**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `ruleId` | string | 是 | 规则唯一标识 |
| `match` | string | 是 | 路由匹配模式，支持通配符 `*` |
| `algo` | string | 是 | 限流算法：`sliding_window`、`token_bucket`、`leaky_bucket` |
| `windowMs` | int64 | 是 | 时间窗口（毫秒） |
| `limit` | int64 | 是 | 速率限制 |
| `burst` | int64 | 否 | 突发容量（令牌桶和漏桶使用） |
| `dims` | []string | 是 | 限流维度列表，如 `["ip", "route", "user_id"]` |
| `enabled` | boolean | 否 | 是否启用，默认 true |
| `quota` | object | 否 | 配额配置 |
| `quota.perMinute` | int64 | 否 | 分钟级配额，0 或不设置表示不限制 |
| `quota.perHour` | int64 | 否 | 小时级配额，0 或不设置表示不限制 |
| `quota.perDay` | int64 | 否 | 天级配额，0 或不设置表示不限制 |
| `breaker` | object | 否 | 熔断器配置 |
| `breaker.enabled` | boolean | 否 | 是否启用熔断 |
| `breaker.rlDenyThreshold` | int | 否 | 触发熔断的拒绝次数阈值 |
| `breaker.rlDenyWindowMs` | int64 | 否 | 统计窗口（毫秒） |
| `breaker.minOpenMs` | int64 | 否 | 熔断状态最小持续时间（毫秒） |
| `breaker.halfOpenProbePercent` | int | 否 | 半开状态探测百分比 |
| `breaker.halfOpenMinPass` | int | 否 | 半开状态通过次数阈值 |
| `breaker.halfOpenMaxFail` | int | 否 | 半开状态失败次数阈值 |

#### 响应

```json
{
  "code": 200,
  "data": {
    "ruleId": "api-login"
  },
  "message": "success"
}
```

### 3. 获取规则

#### 请求

```http
GET /v1/rules/{ruleId}
```

**路径参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `ruleId` | string | 规则 ID |

#### 响应

```json
{
  "code": 200,
  "data": {
    "ruleId": "api-login",
    "match": "/api/login",
    "algo": "sliding_window",
    "windowMs": 1000,
    "limit": 10,
    "burst": 5,
    "dims": ["ip", "user_id"],
    "enabled": true,
    "quota": {
      "perMinute": 100,
      "perHour": 1000,
      "perDay": 10000
    },
    "breaker": {
      "enabled": true,
      "rlDenyThreshold": 20,
      "rlDenyWindowMs": 10000,
      "minOpenMs": 8000
    }
  },
  "message": "success"
}
```

### 4. 更新规则

#### 请求

```http
PUT /v1/rules/{ruleId}
Content-Type: application/json
```

**请求体**：与创建规则相同

#### 响应

```json
{
  "code": 200,
  "data": {
    "ruleId": "api-login"
  },
  "message": "success"
}
```

## 使用示例

### cURL 示例

#### 1. 检查限流

```bash
curl -X POST http://localhost:8080/v1/allow \
  -H "Content-Type: application/json" \
  -d '{
    "ruleId": "api-login",
    "dims": {
      "ip": "192.168.1.1",
      "user_id": "12345"
    }
  }'
```

#### 2. 创建规则

```bash
curl -X POST http://localhost:8080/v1/rules \
  -H "Content-Type: application/json" \
  -d '{
    "ruleId": "api-login",
    "match": "/api/login",
    "algo": "token_bucket",
    "windowMs": 1000,
    "limit": 10,
    "burst": 5,
    "dims": ["ip"],
    "enabled": true
  }'
```

#### 3. 获取规则

```bash
curl http://localhost:8080/v1/rules/api-login
```

#### 4. 更新规则

```bash
curl -X PUT http://localhost:8080/v1/rules/api-login \
  -H "Content-Type: application/json" \
  -d '{
    "ruleId": "api-login",
    "match": "/api/login",
    "algo": "sliding_window",
    "windowMs": 1000,
    "limit": 20,
    "dims": ["ip"],
    "enabled": true
  }'
```

### Python 示例

```python
import requests

# 限流检查
def check_rate_limit(rule_id, dims):
    url = "http://localhost:8080/v1/allow"
    payload = {
        "ruleId": rule_id,
        "dims": dims
    }
    response = requests.post(url, json=payload)
    return response.json()

# 创建规则
def create_rule(rule):
    url = "http://localhost:8080/v1/rules"
    response = requests.post(url, json=rule)
    return response.json()

# 使用示例
result = check_rate_limit("api-login", {"ip": "192.168.1.1"})
if result["data"]["allowed"]:
    print("请求允许通过")
else:
    print(f"请求被限流，建议 {result['data']['retryAfterMs']}ms 后重试")
```

### Go 示例

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
)

type AllowRequest struct {
    RuleID string            `json:"ruleId"`
    Dims   map[string]string `json:"dims"`
}

type AllowResponse struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Data    struct {
        Allowed      bool   `json:"allowed"`
        Remaining    int64  `json:"remaining"`
        RetryAfterMs int64  `json:"retryAfterMs"`
        Reason       string `json:"reason"`
    } `json:"data"`
}

func CheckRateLimit(ruleID string, dims map[string]string) (*AllowResponse, error) {
    req := AllowRequest{
        RuleID: ruleID,
        Dims:   dims,
    }
    
    body, _ := json.Marshal(req)
    resp, err := http.Post(
        "http://localhost:8080/v1/allow",
        "application/json",
        bytes.NewBuffer(body),
    )
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var result AllowResponse
    json.NewDecoder(resp.Body).Decode(&result)
    return &result, nil
}

func main() {
    result, _ := CheckRateLimit("api-login", map[string]string{
        "ip": "192.168.1.1",
    })
    
    if result.Data.Allowed {
        fmt.Println("请求允许通过")
    } else {
        fmt.Printf("请求被限流: %s\n", result.Data.Reason)
    }
}
```

### JavaScript 示例

```javascript
// 限流检查
async function checkRateLimit(ruleId, dims) {
    const response = await fetch('http://localhost:8080/v1/allow', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            ruleId: ruleId,
            dims: dims
        })
    });
    return await response.json();
}

// 创建规则
async function createRule(rule) {
    const response = await fetch('http://localhost:8080/v1/rules', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify(rule)
    });
    return await response.json();
}

// 使用示例
(async () => {
    const result = await checkRateLimit('api-login', {
        ip: '192.168.1.1'
    });
    
    if (result.data.allowed) {
        console.log('请求允许通过');
    } else {
        console.log(`请求被限流: ${result.data.reason}`);
        console.log(`建议 ${result.data.retryAfterMs}ms 后重试`);
    }
})();
```

## 错误码

| HTTP 状态码 | 说明 |
|-----------|------|
| 200 | 成功 |
| 400 | 请求参数错误 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |

## 最佳实践

### 1. 维度设计

建议的维度组合：

- **用户级限流**：`["user_id"]`
- **IP 级限流**：`["ip"]`
- **接口级限流**：`["route"]`
- **用户+接口限流**：`["user_id", "route"]`
- **IP+接口限流**：`["ip", "route"]`
- **细粒度限流**：`["ip", "route", "user_id"]`

### 2. 算法选择

- **滑动窗口**：精确限流，适合严格控制场景
- **令牌桶**：允许突发，适合正常流量有波动的场景
- **漏桶**：恒定速率，适合需要平滑输出的场景

### 3. 配额设置

建议配额从宽到严：

```
perDay > perHour > perMinute
```

示例：
- 分钟：1000 次
- 小时：10000 次（< 60 * 1000）
- 天：100000 次（< 24 * 10000）

### 4. 熔断配置

合理的熔断配置示例：

```json
{
  "enabled": true,
  "rlDenyThreshold": 20,        // 10秒内拒绝20次触发熔断
  "rlDenyWindowMs": 10000,      // 10秒统计窗口
  "minOpenMs": 8000,            // 熔断至少持续8秒
  "halfOpenProbePercent": 10,   // 半开时10%请求用于探测
  "halfOpenMinPass": 5,         // 连续通过5次关闭熔断
  "halfOpenMaxFail": 3          // 失败3次重新打开熔断
}
```

## 监控和告警

### 建议监控指标

1. **QPS**：每秒请求数
2. **拒绝率**：被限流的请求比例
3. **响应时间**：P50, P95, P99 延迟
4. **熔断次数**：熔断触发频率

### Redis 键监控

建议监控 Redis 中的键数量：

```bash
redis-cli --scan --pattern "pixiu:rls:*" | wc -l
```

## 常见问题

### Q1: 如何处理限流响应？

**A**: 客户端应该根据 `retryAfterMs` 字段进行退避重试：

```javascript
if (!result.data.allowed) {
    const retryAfter = result.data.retryAfterMs;
    await sleep(retryAfter);
    // 重试请求
}
```

### Q2: 规则更新需要多久生效？

**A**: 通过 API 更新规则后，会通过 Redis Pub/Sub 通知所有实例，通常在 1 秒内生效。

### Q3: 支持批量检查吗？

**A**: 当前版本不支持批量检查，每个请求需要单独调用 `/v1/allow` 接口。

### Q4: 如何查看所有规则？

**A**: 当前版本可以通过 Redis 直接查询：

```bash
redis-cli --scan --pattern "pixiu:rls:rule:*"
```

未来版本会提供 `GET /v1/rules` 接口列出所有规则。

## 参考资料

- [快速入门](./RCU_QUICKSTART.md)
- [部署指南](./DEPLOYMENT.md)
- [开发指南](./DEVELOPMENT.md)
- [架构设计](./RCU_ARCHITECTURE.md)

---

**版本**: v1.0  
**最后更新**: 2024-01

