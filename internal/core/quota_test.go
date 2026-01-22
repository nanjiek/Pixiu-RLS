package core

import (
	"testing"
	"time"
)

func TestQuota_calcRetryAfter(t *testing.T) {
	q := &Quota{}

	tests := []struct {
		name  string
		scope string
		now   time.Time
		min   int64 // 最小预期值
		max   int64 // 最大预期值
	}{
		{
			name:  "minute scope - beginning of minute",
			scope: "min",
			now:   time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC),
			min:   59000,
			max:   61000,
		},
		{
			name:  "minute scope - middle of minute",
			scope: "min",
			now:   time.Date(2024, 1, 1, 10, 30, 30, 0, time.UTC),
			min:   29000,
			max:   31000,
		},
		{
			name:  "hour scope - beginning of hour",
			scope: "hour",
			now:   time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
			min:   3599000,
			max:   3601000,
		},
		{
			name:  "day scope - beginning of day",
			scope: "day",
			now:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			min:   86399000,
			max:   86401000,
		},
		{
			name:  "unknown scope",
			scope: "unknown",
			now:   time.Now(),
			min:   1000,
			max:   1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := q.calcRetryAfter(tt.scope, tt.now)
			if got < tt.min || got > tt.max {
				t.Errorf("calcRetryAfter() = %d, want between %d and %d", got, tt.min, tt.max)
			}
		})
	}
}

func TestQuota_calcRetryAfterMinute(t *testing.T) {
	q := &Quota{}

	// 测试分钟级重试时间计算
	now := time.Date(2024, 1, 1, 10, 30, 45, 0, time.UTC) // 45秒
	retryAfter := q.calcRetryAfter("min", now)

	// 应该是 (60 - 45) * 1000 + 1000 = 16000ms
	expected := int64((60-45)*1000 + 1000)
	if retryAfter != expected {
		t.Errorf("calcRetryAfter(min) = %d, want %d", retryAfter, expected)
	}
}

func TestQuota_calcRetryAfterHour(t *testing.T) {
	q := &Quota{}

	// 测试小时级重试时间计算
	now := time.Date(2024, 1, 1, 10, 30, 15, 0, time.UTC) // 30分15秒
	retryAfter := q.calcRetryAfter("hour", now)

	// 应该是 ((60 - 30) * 60 - 15) * 1000 + 1000
	expected := int64(((60-30)*60-15)*1000 + 1000)
	if retryAfter != expected {
		t.Errorf("calcRetryAfter(hour) = %d, want %d", retryAfter, expected)
	}
}

func TestQuota_calcRetryAfterDay(t *testing.T) {
	q := &Quota{}

	// 测试天级重试时间计算
	now := time.Date(2024, 1, 1, 10, 30, 15, 0, time.UTC) // 10:30:15
	retryAfter := q.calcRetryAfter("day", now)

	// 应该是 ((24 - 10) * 3600 - 30 * 60 - 15) * 1000 + 1000
	expected := int64(((24-10)*3600-30*60-15)*1000 + 1000)
	if retryAfter != expected {
		t.Errorf("calcRetryAfter(day) = %d, want %d", retryAfter, expected)
	}
}

func BenchmarkQuota_calcRetryAfter(b *testing.B) {
	q := &Quota{}
	now := time.Now()

	scopes := []string{"min", "hour", "day", "unknown"}

	for _, scope := range scopes {
		b.Run(scope, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = q.calcRetryAfter(scope, now)
			}
		})
	}
}

// Note: CheckAndIncr 方法需要 Redis 连接，因此在单元测试中难以测试
// 应该在集成测试中测试，或者使用 mock Redis 客户端

