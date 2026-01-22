package core

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/repo"
)

func newDummyIPListCache() *IPListCache {
	r := &repo.RedisRepo{Prefix: "test"}
	c := NewIPListCache(r, "", slog.Default())
	c.isTempBlacklisted = func(ctx context.Context, ip string) (bool, error) {
		return false, nil
	}
	c.isInSet = func(ctx context.Context, setKey, member string) (bool, error) {
		return false, nil
	}
	return c
}

func TestIPListCache_L1TempBlacklistHit(t *testing.T) {
	c := newDummyIPListCache()
	c.localCache.Store("1.1.1.1:black_tmp", cacheEntry{
		value:     true,
		expiresAt: time.Now().Add(time.Minute).UnixNano(),
	})

	dec, handled, err := c.CheckIP(context.Background(), "1.1.1.1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !handled {
		t.Fatal("expected handled=true")
	}
	if dec.Allowed {
		t.Fatalf("expected deny, got %+v", dec)
	}
	if dec.Reason != "ip_in_temp_blacklist_l1" {
		t.Fatalf("unexpected reason: %s", dec.Reason)
	}
}

func TestIPListCache_L1BlacklistHit(t *testing.T) {
	c := newDummyIPListCache()
	c.localCache.Store("1.1.1.1:black", cacheEntry{
		value:     true,
		expiresAt: time.Now().Add(time.Minute).UnixNano(),
	})

	dec, handled, err := c.CheckIP(context.Background(), "1.1.1.1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !handled {
		t.Fatal("expected handled=true")
	}
	if dec.Allowed {
		t.Fatalf("expected deny, got %+v", dec)
	}
	if dec.Reason != "ip_in_blacklist_l1" {
		t.Fatalf("unexpected reason: %s", dec.Reason)
	}
}

func TestIPListCache_L1WhitelistHit(t *testing.T) {
	c := newDummyIPListCache()
	c.localCache.Store("2.2.2.2:white", cacheEntry{
		value:     true,
		expiresAt: time.Now().Add(time.Minute).UnixNano(),
	})

	dec, handled, err := c.CheckIP(context.Background(), "2.2.2.2")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !handled {
		t.Fatal("expected handled=true")
	}
	if !dec.Allowed {
		t.Fatalf("expected allow, got %+v", dec)
	}
	if dec.Reason != "ip_in_whitelist_l1" {
		t.Fatalf("unexpected reason: %s", dec.Reason)
	}
}

func TestIPListCache_RepoNilDenies(t *testing.T) {
	c := &IPListCache{
		repo:       nil,
		defaultTTL: time.Minute,
		logger:     slog.Default(),
	}

	dec, handled, err := c.CheckIP(context.Background(), "3.3.3.3")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !handled {
		t.Fatal("expected handled=true")
	}
	if dec.Allowed {
		t.Fatalf("expected deny, got %+v", dec)
	}
	if dec.Reason != "iplist_repo_nil" {
		t.Fatalf("unexpected reason: %s", dec.Reason)
	}
}

func TestIPListCache_RedisUnsetDenies(t *testing.T) {
	c := &IPListCache{
		repo:       &repo.RedisRepo{Prefix: "test"},
		defaultTTL: time.Minute,
		logger:     slog.Default(),
	}

	dec, handled, err := c.CheckIP(context.Background(), "4.4.4.4")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !handled {
		t.Fatal("expected handled=true")
	}
	if dec.Allowed {
		t.Fatalf("expected deny, got %+v", dec)
	}
	if dec.Reason != "iplist_redis_nil" {
		t.Fatalf("unexpected reason: %s", dec.Reason)
	}
}

func TestIPListCache_RecordDenySetsTempBlacklist(t *testing.T) {
	c := newDummyIPListCache()
	c.hotThreshold = 2
	c.hotWindow = 10 * time.Second
	c.blacklistTTL = time.Minute

	c.incrAndExpire = func(ctx context.Context, key string, ttl time.Duration) (int64, error) {
		return 2, nil
	}
	setCalled := false
	c.setTempBlacklist = func(ctx context.Context, ip string, ttl time.Duration) error {
		setCalled = true
		return nil
	}
	publishCalled := false
	c.publish = func(ctx context.Context, channel, msg string) error {
		publishCalled = true
		return nil
	}

	c.RecordDeny(context.Background(), "5.5.5.5")

	if !setCalled {
		t.Fatal("expected temp blacklist to be set")
	}
	if !publishCalled {
		t.Fatal("expected update publish to be called")
	}
	if val, ok := c.get("5.5.5.5:black_tmp"); !ok || !val {
		t.Fatal("expected temp blacklist cached in L1")
	}
}

func BenchmarkIPListCache_CheckIP_L1(b *testing.B) {
	c := newDummyIPListCache()
	c.localCache.Store("9.9.9.9:black", cacheEntry{
		value:     true,
		expiresAt: time.Now().Add(time.Minute).UnixNano(),
	})

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = c.CheckIP(ctx, "9.9.9.9")
	}
}

func BenchmarkIPListCache_RecordDeny(b *testing.B) {
	c := newDummyIPListCache()
	c.hotThreshold = 1000000
	c.hotWindow = 10 * time.Second
	c.blacklistTTL = time.Minute

	c.incrAndExpire = func(ctx context.Context, key string, ttl time.Duration) (int64, error) {
		return 1, nil
	}

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.RecordDeny(ctx, "8.8.8.8")
	}
}
