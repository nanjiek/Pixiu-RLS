package repo

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/nanjiek/pixiu-rls/internal/config"
)

type RedisRepo struct {
	Cli           *redis.Client
	Prefix        string
	UpdateChannel string
}

func NewRedis(cfg *config.Config) *RedisRepo {
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Redis.Addr,
		DB:           cfg.Redis.DB,
		ReadTimeout:  800 * time.Millisecond,
		WriteTimeout: 800 * time.Millisecond,
		// 连接池配置（v9 字段）
		PoolSize:        100,              // 连接池大小
		MinIdleConns:    10,               // 最小空闲连接
		ConnMaxLifetime: 5 * time.Minute,  // 替换 MaxConnAge：连接最大存活时间
		ConnMaxIdleTime: 30 * time.Second, // 替换 IdleTimeout：空闲连接超时
		MaxRetries:      1,                // 命令重试次数
		// 重试退避策略（替换 RetryBackoff）
		MinRetryBackoff: 100 * time.Millisecond,
		MaxRetryBackoff: 300 * time.Millisecond,
	})
	return &RedisRepo{
		Cli:           rdb,
		Prefix:        cfg.Redis.Prefix,
		UpdateChannel: cfg.Redis.UpdatesChannel,
	}
}

func (r *RedisRepo) Close() error { return r.Cli.Close() }

// ---- Key helpers ----
func (r *RedisRepo) KeyRule(id string) string { return r.Prefix + ":rule:" + id }
func (r *RedisRepo) KeySW(ruleID, dimKey string) string {
	return r.Prefix + ":sw:" + ruleID + ":" + dimKey
}
func (r *RedisRepo) KeyTB(ruleID, dimKey string) string {
	return r.Prefix + ":tb:" + ruleID + ":" + dimKey
}
func (r *RedisRepo) KeyTBTS(ruleID, dimKey string) string {
	return r.Prefix + ":tbts:" + ruleID + ":" + dimKey
}
func (r *RedisRepo) KeyLBLevel(ruleID, dimKey string) string {
	return r.Prefix + ":lbq:" + ruleID + ":" + dimKey
}
func (r *RedisRepo) KeyLBTS(ruleID, dimKey string) string {
	return r.Prefix + ":lbts:" + ruleID + ":" + dimKey
}

func (r *RedisRepo) KeyQuota(scope, ruleID, dimKey, ts string) string {
	// scope: m|h|d
	return r.Prefix + ":quota:" + scope + ":" + ruleID + ":" + dimKey + ":" + ts
}

// ---- Black/White list (简化) ----
func (r *RedisRepo) IsInSet(ctx context.Context, setKey, member string) (bool, error) {
	return r.Cli.SIsMember(ctx, setKey, member).Result()
}

// KeyBlacklistIP 生成IP黑名单的集合键
func (r *RedisRepo) KeyBlacklistIP() string {
	return r.Prefix + ":blacklist:ip"
}

// KeyWhitelistIP 生成IP白名单的集合键
func (r *RedisRepo) KeyWhitelistIP() string {
	return r.Prefix + ":whitelist:ip"
}

func (r *RedisRepo) PublishUpdate(ctx context.Context, ruleID string) error {
	return r.Cli.Publish(ctx, r.UpdateChannel, ruleID).Err()
}

// IncrAndExpire 对键进行自增并设置过期时间（键不存在时创建并设置过期时间，已存在时仅自增）
func (r *RedisRepo) IncrAndExpire(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	// 先自增
	cnt, err := r.Cli.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	// 若自增后的值为1（说明是新键），则设置过期时间
	if cnt == 1 {
		if err := r.Cli.Expire(ctx, key, ttl).Err(); err != nil {
			return 0, err
		}
	}
	return cnt, nil
}
