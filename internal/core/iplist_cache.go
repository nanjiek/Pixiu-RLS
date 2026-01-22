package core

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/repo"
	"github.com/nanjiek/pixiu-rls/internal/types"
)

type cacheEntry struct {
	value     bool
	expiresAt int64
}

// IPListCache provides a two-level cache for blacklist/whitelist checks.
type IPListCache struct {
	repo          *repo.RedisRepo
	localCache    sync.Map
	defaultTTL    time.Duration
	hotEnabled    bool
	hotThreshold  int64
	hotWindow     time.Duration
	blacklistTTL  time.Duration
	updateChannel string
	logger        *slog.Logger
	cancel        context.CancelFunc

	isTempBlacklisted func(ctx context.Context, ip string) (bool, error)
	isInSet           func(ctx context.Context, setKey, member string) (bool, error)
	incrAndExpire     func(ctx context.Context, key string, ttl time.Duration) (int64, error)
	setTempBlacklist  func(ctx context.Context, ip string, ttl time.Duration) error
	publish           func(ctx context.Context, channel, msg string) error
}

func NewIPListCache(r *repo.RedisRepo, updateChan string, logger *slog.Logger) *IPListCache {
	if logger == nil {
		logger = slog.Default()
	}
	if updateChan == "" && r != nil {
		updateChan = r.Prefix + ":iplist_updates"
	}
	c := &IPListCache{
		repo:          r,
		updateChannel: updateChan,
		defaultTTL:    5 * time.Minute,
		hotEnabled:    true,
		hotThreshold:  10,
		hotWindow:     time.Minute,
		blacklistTTL:  10 * time.Minute,
		logger:        logger,
	}
	if r != nil {
		c.isTempBlacklisted = r.IsTempBlacklisted
		c.isInSet = r.IsInSet
		c.incrAndExpire = r.IncrAndExpire
		c.setTempBlacklist = r.SetTempBlacklistIP
		if r.Cli != nil {
			c.publish = func(ctx context.Context, channel, msg string) error {
				return r.Cli.Publish(ctx, channel, msg).Err()
			}
		}
	}
	if r != nil && r.Cli != nil && updateChan != "" {
		ctx, cancel := context.WithCancel(context.Background())
		c.cancel = cancel
		go c.watchUpdates(ctx)
	}
	return c
}

// CheckIP checks blacklist/whitelist with L1 cache and Redis as source of truth.
// Safety-first: any Redis error results in deny.
func (c *IPListCache) CheckIP(ctx context.Context, ip string) (types.Decision, bool, error) {
	if ip == "" {
		return types.Decision{}, false, nil
	}
	if c.repo == nil {
		err := errors.New("repo is nil")
		c.logger.Error("ip list repo is nil", "err", err)
		return types.Decision{Allowed: false, Reason: "iplist_repo_nil", Err: err}, true, nil
	}
	if c.isTempBlacklisted == nil || c.isInSet == nil {
		err := errors.New("redis accessors not set")
		c.logger.Error("ip list redis accessors not set", "err", err)
		return types.Decision{Allowed: false, Reason: "iplist_redis_nil", Err: err}, true, nil
	}

	tempKey := ip + ":black_tmp"
	if val, ok := c.get(tempKey); ok && val {
		return types.Decision{Allowed: false, Reason: "ip_in_temp_blacklist_l1"}, true, nil
	}
	inTemp, err := c.isTempBlacklisted(ctx, ip)
	if err != nil {
		c.logger.Error("temp blacklist check failed", "err", err)
		return types.Decision{Allowed: false, Reason: "temp_blacklist_check_failed", Err: err}, true, nil
	}
	if inTemp {
		c.setWithTTL(tempKey, true, c.blacklistTTL)
		return types.Decision{Allowed: false, Reason: "ip_in_temp_blacklist_l2"}, true, nil
	}

	blackKey := ip + ":black"
	if val, ok := c.get(blackKey); ok && val {
		return types.Decision{Allowed: false, Reason: "ip_in_blacklist_l1"}, true, nil
	}
	inBlack, err := c.isInSet(ctx, c.repo.KeyBlacklistIP(), ip)
	if err != nil {
		c.logger.Error("blacklist check failed", "err", err)
		return types.Decision{Allowed: false, Reason: "blacklist_check_failed", Err: err}, true, nil
	}
	if inBlack {
		c.setWithTTL(blackKey, true, c.defaultTTL)
		return types.Decision{Allowed: false, Reason: "ip_in_blacklist_l2"}, true, nil
	}

	whiteKey := ip + ":white"
	if val, ok := c.get(whiteKey); ok {
		if val {
			return types.Decision{Allowed: true, Reason: "ip_in_whitelist_l1"}, true, nil
		}
	} else {
		inWhite, err := c.isInSet(ctx, c.repo.KeyWhitelistIP(), ip)
		if err != nil {
			c.logger.Error("whitelist check failed", "err", err)
			return types.Decision{Allowed: false, Reason: "whitelist_check_failed", Err: err}, true, nil
		}
		c.set(whiteKey, inWhite)
		if inWhite {
			return types.Decision{Allowed: true, Reason: "ip_in_whitelist_l2"}, true, nil
		}
	}

	return types.Decision{}, false, nil
}

// RecordDeny tracks rate limit denials and applies temporary blacklist.
func (c *IPListCache) RecordDeny(ctx context.Context, ip string) {
	if !c.hotEnabled || ip == "" {
		return
	}
	if c.repo == nil {
		return
	}
	if c.incrAndExpire == nil || c.setTempBlacklist == nil {
		return
	}
	if c.hotThreshold <= 0 || c.hotWindow <= 0 || c.blacklistTTL <= 0 {
		return
	}

	tempKey := ip + ":black_tmp"
	if val, ok := c.get(tempKey); ok && val {
		return
	}

	cnt, err := c.incrAndExpire(ctx, c.repo.KeyHotIP(ip), c.hotWindow)
	if err != nil {
		c.logger.Error("hot ip counter failed", "err", err)
		return
	}
	if cnt < c.hotThreshold {
		return
	}

	if err := c.setTempBlacklist(ctx, ip, c.blacklistTTL); err != nil {
		c.logger.Error("set temp blacklist failed", "err", err)
		return
	}
	c.setWithTTL(tempKey, true, c.blacklistTTL)
	c.publishUpdate(ctx)
}

func (c *IPListCache) get(key string) (bool, bool) {
	if val, ok := c.localCache.Load(key); ok {
		entry := val.(cacheEntry)
		if time.Now().UnixNano() <= entry.expiresAt {
			return entry.value, true
		}
		c.localCache.Delete(key)
	}
	return false, false
}

func (c *IPListCache) set(key string, value bool) {
	c.setWithTTL(key, value, c.defaultTTL)
}

func (c *IPListCache) setWithTTL(key string, value bool, ttl time.Duration) {
	if ttl <= 0 {
		ttl = c.defaultTTL
	}
	c.localCache.Store(key, cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(ttl).UnixNano(),
	})
}

func (c *IPListCache) watchUpdates(ctx context.Context) {
	sub := c.repo.Cli.Subscribe(ctx, c.updateChannel)
	defer sub.Close()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				c.logger.Warn("pubsub channel closed, stopping watcher")
				return
			}
			c.logger.Debug("received cache invalidation", "channel", msg.Channel)
			c.clear()
		}
	}
}

func (c *IPListCache) clear() {
	c.localCache.Range(func(key, value any) bool {
		c.localCache.Delete(key)
		return true
	})
}

func (c *IPListCache) publishUpdate(ctx context.Context) {
	if c.publish == nil || c.updateChannel == "" {
		return
	}
	if err := c.publish(ctx, c.updateChannel, "iplist_update"); err != nil {
		c.logger.Warn("ip list publish update failed", "err", err)
	}
}

// Close stops the update watcher.
func (c *IPListCache) Close() {
	if c.cancel != nil {
		c.cancel()
		c.logger.Info("IPListCache closed, watcher stopped")
	}
}
