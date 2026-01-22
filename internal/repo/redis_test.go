package repo

import (
	"testing"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
)

func TestNormalizeAddrs(t *testing.T) {
	cfg := config.RedisCfg{Addr: "127.0.0.1:6379, 127.0.0.2:6379"}
	addrs := normalizeAddrs(cfg)
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addrs, got %d", len(addrs))
	}
	if addrs[0] != "127.0.0.1:6379" || addrs[1] != "127.0.0.2:6379" {
		t.Fatalf("unexpected addrs: %#v", addrs)
	}
}

func TestKeyTemplates(t *testing.T) {
	r := &RedisRepo{Prefix: "pixiu"}
	if got := r.KeyRule("r1"); got != "pixiu:rule:{r1}" {
		t.Fatalf("KeyRule = %s", got)
	}
	if got := r.KeySW("r1", "d1"); got != "pixiu:sw:{r1}:d1" {
		t.Fatalf("KeySW = %s", got)
	}
	if got := r.KeyTB("r1", "d1"); got != "pixiu:tb:{r1}:d1" {
		t.Fatalf("KeyTB = %s", got)
	}
	if got := r.KeyLB("r1", "d1"); got != "pixiu:lb:{r1}:d1" {
		t.Fatalf("KeyLB = %s", got)
	}
	if got := r.KeyQuota("min", "r1", "d1", "202401"); got != "pixiu:quota:min:{r1}:d1:202401" {
		t.Fatalf("KeyQuota = %s", got)
	}
}
