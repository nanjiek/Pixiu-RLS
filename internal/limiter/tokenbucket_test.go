package limiter

import (
	"context"
	"errors"
	"testing"
	"time"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
)

type fakeExec struct {
	result []interface{}
	err    error
	keys   []string
	args   []interface{}
}

func (f *fakeExec) Eval(ctx context.Context, script string, keys []string, args ...interface{}) ([]interface{}, error) {
	f.keys = keys
	f.args = args
	return f.result, f.err
}

func TestTokenBucketAllow(t *testing.T) {
	exec := &fakeExec{
		result: []interface{}{int64(1), int64(9), int64(1000)},
	}
	tb := NewTokenBucket(exec)

	rule := config.Rule{RuleID: "r1", Limit: 10, WindowMs: 1000, Burst: 0}
	dec, err := tb.Allow(context.Background(), rule, "k1", time.UnixMilli(100))
	if err != nil {
		t.Fatalf("allow failed: %v", err)
	}
	if !dec.Allowed || dec.Remaining != 9 {
		t.Fatalf("unexpected decision: %#v", dec)
	}
	if len(exec.keys) != 1 || exec.keys[0] != "k1" {
		t.Fatalf("unexpected keys: %#v", exec.keys)
	}
	if len(exec.args) < 5 {
		t.Fatalf("unexpected args: %#v", exec.args)
	}
}

func TestTokenBucketRateLimited(t *testing.T) {
	exec := &fakeExec{
		result: []interface{}{int64(0), int64(0), int64(2000)},
	}
	tb := NewTokenBucket(exec)

	rule := config.Rule{RuleID: "r1", Limit: 1, WindowMs: 1000, Burst: 0}
	dec, err := tb.Allow(context.Background(), rule, "k1", time.UnixMilli(1000))
	if err != nil {
		t.Fatalf("allow failed: %v", err)
	}
	if dec.Allowed || dec.RetryAfterMs == 0 {
		t.Fatalf("unexpected decision: %#v", dec)
	}
}

func TestTokenBucketError(t *testing.T) {
	exec := &fakeExec{err: errors.New("boom")}
	tb := NewTokenBucket(exec)

	rule := config.Rule{RuleID: "r1", Limit: 1, WindowMs: 1000}
	if _, err := tb.Allow(context.Background(), rule, "k1", time.Now()); err == nil {
		t.Fatalf("expected error")
	}
}

func TestTokenBucketInvalidResponse(t *testing.T) {
	exec := &fakeExec{
		result: []interface{}{struct{}{}, int64(1), int64(1000)},
	}
	tb := NewTokenBucket(exec)

	rule := config.Rule{RuleID: "r1", Limit: 1, WindowMs: 1000}
	if _, err := tb.Allow(context.Background(), rule, "k1", time.Now()); err == nil {
		t.Fatalf("expected error")
	}
}
