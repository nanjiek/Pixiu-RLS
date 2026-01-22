package config

import (
	"os"
)

import (
	"gopkg.in/yaml.v3"
)

// ServerCfg —— HTTP 服务端口/地址配置
type ServerCfg struct {
	HTTPAddr string `yaml:"httpAddr"` // 监听地址，例如 ":8080" 或 "0.0.0.0:8080"
}

// RedisCfg —— Redis 连接与命名空间配置
type RedisCfg struct {
	Addr               string   `yaml:"addr"`               // Redis address, e.g. "127.0.0.1:6379"
	Addrs              []string `yaml:"addrs"`              // Optional shard addresses
	Password           string   `yaml:"password"`           // Redis password
	DB                 int      `yaml:"db"`                 // Redis DB index
	Prefix             string   `yaml:"prefix"`             // Key prefix
	UpdatesChannel     string   `yaml:"updatesChannel"`     // Pub/Sub channel for rule updates
	PoolSize           int      `yaml:"poolSize"`           // Connection pool size
	MinIdleConns       int      `yaml:"minIdleConns"`       // Minimum idle connections
	ConnMaxLifetimeSec int      `yaml:"connMaxLifetimeSec"` // Max connection lifetime (sec)
	ConnMaxIdleTimeSec int      `yaml:"connMaxIdleTimeSec"` // Max idle time (sec)
	MaxRetries         int      `yaml:"maxRetries"`         // Command retry count
	MinRetryBackoffMs  int      `yaml:"minRetryBackoffMs"`  // Min retry backoff (ms)
	MaxRetryBackoffMs  int      `yaml:"maxRetryBackoffMs"`  // Max retry backoff (ms)
	ReadTimeoutMs      int      `yaml:"readTimeoutMs"`      // Read timeout (ms)
	WriteTimeoutMs     int      `yaml:"writeTimeoutMs"`     // Write timeout (ms)
	DialTimeoutMs      int      `yaml:"dialTimeoutMs"`      // Dial timeout (ms)
}

// Features —— 特性开关
type Features struct {
	Audit         string `yaml:"audit"`         // 审计模式："redis_stream" | "none" （后续可扩展 "kafka" 等）
	LocalFallback bool   `yaml:"localFallback"` // Redis 故障是否启用本地退化（仅建议开发/测试场景开启）
	FailPolicy    string `yaml:"failPolicy"`    // fail-open | fail-closed
}

// NacosCfg - Nacos config center (pull mode)
type NacosCfg struct {
	Addr           string `yaml:"addr"`           // Nacos address, e.g. "http://127.0.0.1:8848"
	Namespace      string `yaml:"namespace"`      // tenant/namespace
	Group          string `yaml:"group"`          // rule group, default DEFAULT_GROUP
	DataID         string `yaml:"dataId"`         // config dataId
	Username       string `yaml:"username"`       // optional
	Password       string `yaml:"password"`       // optional
	PollIntervalMs int    `yaml:"pollIntervalMs"` // default 5000
	TimeoutMs      int    `yaml:"timeoutMs"`      // default 2000
	FailPolicy     string `yaml:"failPolicy"`     // fail-open | fail-closed
	Format         string `yaml:"format"`         // json | yaml (auto-detect if empty)
}

func (n NacosCfg) Enabled() bool {
	return n.Addr != "" && n.DataID != ""
}

// QuotaCfg —— 配额（分钟/小时/天）
type QuotaCfg struct {
	PerMinute int64 `yaml:"perMinute"` // 分钟内最大请求数（<=0 表示不限制）
	PerHour   int64 `yaml:"perHour"`   // 小时内最大请求数（<=0 表示不限制）
	PerDay    int64 `yaml:"perDay"`    // 天内最大请求数（<=0 表示不限制）
}

// BreakerCfg —— 熔断器配置（可针对规则+维度细粒度生效）
type BreakerCfg struct {
	Enabled bool `json:"enabled" yaml:"enabled"` // 是否开启熔断

	// 触发熔断：当“被限流拒绝”在窗口内累计到阈值时打开熔断
	RLDenyThreshold int   `json:"rlDenyThreshold" yaml:"rlDenyThreshold"` // 窗口内的拒绝阈值（如 20）
	RLDenyWindowMs  int64 `json:"rlDenyWindowMs"  yaml:"rlDenyWindowMs"`  // 统计窗口（毫秒），如 10000

	// Open（全开断路）状态的最小保持时长（冷却时间）
	MinOpenMs int64 `json:"minOpenMs" yaml:"minOpenMs"` // 如 8000

	// Half-Open（半开探测）阶段的采样/通过/失败阈值
	HalfOpenProbePercent int `json:"halfOpenProbePercent" yaml:"halfOpenProbePercent"` // 探测采样百分比，如 10 表示 10%
	HalfOpenMinPass      int `json:"halfOpenMinPass"      yaml:"halfOpenMinPass"`      // 连续通过次数达到后关闭熔断（回到 Closed）
	HalfOpenMaxFail      int `json:"halfOpenMaxFail"      yaml:"halfOpenMaxFail"`      // 半开阶段失败达到阈值则回到 Open
}

// Rule —— 单条限流规则
type Rule struct {
	RuleID   string     `yaml:"ruleId"   json:"ruleId"`   // 规则唯一 ID
	Match    string     `yaml:"match"    json:"match"`    // 路由匹配（示例："/api/login" 或 "*"）
	Methods  []string   `yaml:"methods" json:"methods"`   // HTTP methods
	Client   string     `yaml:"client"  json:"client"`    // client kind
	Priority int        `yaml:"priority" json:"priority"` // higher wins
	Algo     string     `yaml:"algo"     json:"algo"`     // 算法："sliding_window" | "token_bucket" | "leaky_bucket"
	WindowMs int64      `yaml:"windowMs" json:"windowMs"` // 时间窗口（毫秒），不同算法语义略有不同
	Limit    int64      `yaml:"limit"    json:"limit"`    // 基础速率/上限（例如每窗口允许的次数）
	Burst    int64      `yaml:"burst"    json:"burst"`    // 允许的突发容量（令牌桶/漏桶会用到）
	Dims     []string   `yaml:"dims"     json:"dims"`     // 维度声明（如 ["ip","route","appId"]）
	Quota    QuotaCfg   `yaml:"quota"    json:"quota"`    // 分钟/小时/天级配额
	Enabled  bool       `yaml:"enabled"  json:"enabled"`  // 是否启用此规则
	Breaker  BreakerCfg `yaml:"breaker"  json:"breaker"`  // 熔断配置（可选）
}

// Config —— 全量配置
type Config struct {
	Server         ServerCfg `yaml:"server"`         // 服务配置
	Redis          RedisCfg  `yaml:"redis"`          // Redis 配置
	Features       Features  `yaml:"features"`       // 特性开关
	Nacos          NacosCfg  `yaml:"nacos"`          // Nacos dynamic rules config
	BootstrapRules []Rule    `yaml:"bootstrapRules"` // 启动时注入的初始规则（如无则可留空）
}

// Load —— 从 YAML 文件加载配置
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	expanded := os.ExpandEnv(string(b))
	var c Config
	if err := yaml.Unmarshal([]byte(expanded), &c); err != nil {
		return nil, err
	}
	return &c, nil
}
