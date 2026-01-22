package repo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

import (
	"github.com/redis/go-redis/v9"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
)

// Key templates for better readability and maintainability
const (
	keyRuleTmpl   = "%s:rule:{%s}"
	keySWTmpl     = "%s:sw:{%s}:%s"
	keyTBTmpl     = "%s:tb:{%s}:%s"
	keyLBTmpl     = "%s:lb:{%s}:%s"
	keyQuotaTmpl  = "%s:quota:%s:{%s}:%s:%s"
	keyBlacklist  = "%s:blacklist:ip"
	keyWhitelist  = "%s:whitelist:ip"
	keyHotIPTmpl  = "%s:hot:ip:%s"
	keyTmpBlkTmpl = "%s:blacklist:ip:tmp:%s"
)

// Preloaded Lua scripts
var (
	incrExpireScript = redis.NewScript(`
		local cnt = redis.call('INCR', KEYS[1])
		if cnt == 1 then
			redis.call('PEXPIRE', KEYS[1], ARGV[1])
		end
		return cnt
	`)
)

// Repo interface for abstraction (easy to mock/test)
type Repo interface {
	KeyRule(id string) string
	KeySW(ruleID, dimKey string) string
	KeyTB(ruleID, dimKey string) string
	KeyLB(ruleID, dimKey string) string
	KeyQuota(scope, ruleID, dimKey, ts string) string
	KeyBlacklistIP() string
	KeyWhitelistIP() string
	KeyHotIP(ip string) string
	KeyTempBlacklistIP(ip string) string
	IsInSet(ctx context.Context, setKey, member string) (bool, error)
	IncrAndExpire(ctx context.Context, key string, ttl time.Duration) (int64, error)
	SetTempBlacklistIP(ctx context.Context, ip string, ttl time.Duration) error
	IsTempBlacklisted(ctx context.Context, ip string) (bool, error)
	PublishUpdate(ctx context.Context, ruleID string) error
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) ([]interface{}, error)
	Close() error
}

type RedisRepo struct {
	Prefix         string
	UpdateChannel  string
	Cli            *redis.ClusterClient
	logger         *slog.Logger
	defaultTimeout time.Duration // Unified timeout config
}

// NewRedis with functional options for flexibility
func NewRedis(cfg *config.Config, logger *slog.Logger, opts ...Option) (Repo, error) {
	if logger == nil {
		logger = slog.Default()
	}

	r := &RedisRepo{
		Prefix:         cfg.Redis.Prefix,
		UpdateChannel:  cfg.Redis.UpdatesChannel,
		logger:         logger,
		defaultTimeout: 100 * time.Millisecond, // Default, can be overridden
	}

	// Apply options
	for _, opt := range opts {
		opt(r)
	}

	addrs := normalizeAddrs(cfg.Redis)
	if len(addrs) == 0 {
		return nil, errors.New("no redis addresses configured")
	}

	clusterOpts := buildClusterOptions(cfg.Redis)
	r.Cli = redis.NewClusterClient(clusterOpts)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := r.Cli.Ping(ctx).Err(); err != nil {
		logger.Error("redis cluster ping failed", "err", err)
		return nil, fmt.Errorf("redis cluster connect failed: %w", err)
	}

	return r, nil
}

// Option pattern for custom configurations
type Option func(*RedisRepo)

func WithDefaultTimeout(d time.Duration) Option {
	return func(r *RedisRepo) { r.defaultTimeout = d }
}

// withTimeout helper to reduce repetition
func (r *RedisRepo) withTimeout(ctx context.Context, opTimeout time.Duration) (context.Context, context.CancelFunc) {
	if opTimeout == 0 {
		opTimeout = r.defaultTimeout
	}
	return context.WithTimeout(ctx, opTimeout)
}

// Key generation methods (using templates)
func (r *RedisRepo) KeyRule(id string) string {
	return fmt.Sprintf(keyRuleTmpl, r.Prefix, id)
}

func (r *RedisRepo) KeySW(ruleID, dimKey string) string {
	return fmt.Sprintf(keySWTmpl, r.Prefix, ruleID, dimKey)
}

func (r *RedisRepo) KeyTB(ruleID, dimKey string) string {
	return fmt.Sprintf(keyTBTmpl, r.Prefix, ruleID, dimKey)
}

func (r *RedisRepo) KeyLB(ruleID, dimKey string) string {
	return fmt.Sprintf(keyLBTmpl, r.Prefix, ruleID, dimKey)
}

func (r *RedisRepo) KeyQuota(scope, ruleID, dimKey, ts string) string {
	return fmt.Sprintf(keyQuotaTmpl, r.Prefix, scope, ruleID, dimKey, ts)
}

func (r *RedisRepo) KeyBlacklistIP() string {
	return fmt.Sprintf(keyBlacklist, r.Prefix)
}

func (r *RedisRepo) KeyWhitelistIP() string {
	return fmt.Sprintf(keyWhitelist, r.Prefix)
}

func (r *RedisRepo) KeyHotIP(ip string) string {
	return fmt.Sprintf(keyHotIPTmpl, r.Prefix, ip)
}

func (r *RedisRepo) KeyTempBlacklistIP(ip string) string {
	return fmt.Sprintf(keyTmpBlkTmpl, r.Prefix, ip)
}

// IsInSet
func (r *RedisRepo) IsInSet(parentCtx context.Context, setKey, member string) (bool, error) {
	ctx, cancel := r.withTimeout(parentCtx, 0)
	defer cancel()
	return r.Cli.SIsMember(ctx, setKey, member).Result()
}

// IncrAndExpire
func (r *RedisRepo) IncrAndExpire(parentCtx context.Context, key string, ttl time.Duration) (int64, error) {
	ctx, cancel := r.withTimeout(parentCtx, 0)
	defer cancel()
	ttlMs := ttl.Milliseconds()
	if ttlMs <= 0 {
		ttlMs = 1
	}
	res, err := incrExpireScript.Run(ctx, r.Cli, []string{key}, ttlMs).Int64()
	if err != nil {
		return 0, fmt.Errorf("lua script execution failed for key %s: %w", key, err)
	}
	return res, nil
}

// SetTempBlacklistIP stores a temporary blacklist entry with TTL.
func (r *RedisRepo) SetTempBlacklistIP(parentCtx context.Context, ip string, ttl time.Duration) error {
	ctx, cancel := r.withTimeout(parentCtx, 0)
	defer cancel()
	if ttl <= 0 {
		ttl = time.Minute
	}
	return r.Cli.Set(ctx, r.KeyTempBlacklistIP(ip), 1, ttl).Err()
}

// IsTempBlacklisted checks whether a temporary blacklist entry exists.
func (r *RedisRepo) IsTempBlacklisted(parentCtx context.Context, ip string) (bool, error) {
	ctx, cancel := r.withTimeout(parentCtx, 0)
	defer cancel()
	res, err := r.Cli.Exists(ctx, r.KeyTempBlacklistIP(ip)).Result()
	if err != nil {
		return false, err
	}
	return res > 0, nil
}

// PublishUpdate
func (r *RedisRepo) PublishUpdate(parentCtx context.Context, ruleID string) error {
	ctx, cancel := r.withTimeout(parentCtx, 0)
	defer cancel()
	if err := r.Cli.Publish(ctx, r.UpdateChannel, ruleID).Err(); err != nil {
		return fmt.Errorf("publish update for rule %s failed: %w", ruleID, err)
	}
	return nil
}

// Eval (with longer timeout for complex scripts)
func (r *RedisRepo) Eval(parentCtx context.Context, script string, keys []string, args ...interface{}) ([]interface{}, error) {
	ctx, cancel := r.withTimeout(parentCtx, 200*time.Millisecond)
	defer cancel()
	res, err := r.Cli.Eval(ctx, script, keys, args...).Result()
	if err != nil {
		return nil, fmt.Errorf("eval script failed: %w", err)
	}
	if val, ok := res.([]interface{}); ok {
		return val, nil
	}
	return []interface{}{res}, nil
}

// Close
func (r *RedisRepo) Close() error {
	return r.Cli.Close()
}

// Helper functions
func normalizeAddrs(cfg config.RedisCfg) []string {
	if len(cfg.Addrs) > 0 {
		return cfg.Addrs
	}
	if cfg.Addr == "" {
		return nil
	}
	parts := strings.Split(cfg.Addr, ",")
	var out []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func buildClusterOptions(cfg config.RedisCfg) *redis.ClusterOptions {
	return &redis.ClusterOptions{
		Addrs:          normalizeAddrs(cfg), // Already called, but for clarity
		Password:       cfg.Password,
		ReadOnly:       false,
		RouteByLatency: true,
		PoolSize:       max(cfg.PoolSize, 100),
		MinIdleConns:   max(cfg.MinIdleConns, 10),
		DialTimeout:    durationOrDefault(cfg.DialTimeoutMs, 800),
		ReadTimeout:    durationOrDefault(cfg.ReadTimeoutMs, 800),
		WriteTimeout:   durationOrDefault(cfg.WriteTimeoutMs, 800),
		MaxRetries:     max(cfg.MaxRetries, 2),
	}
}

func max(val, def int) int {
	if val > def {
		return val
	}
	return def
}

func durationOrDefault(ms int, defMs int) time.Duration {
	if ms <= 0 {
		ms = defMs
	}
	return time.Duration(ms) * time.Millisecond
}
