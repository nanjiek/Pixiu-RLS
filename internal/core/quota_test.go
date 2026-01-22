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
		min   int64 // 最小预期值 (ms)
		max   int64 // 最大预期值 (ms)
	}{
		{
			name:  "hour scope - middle of hour",
			scope: "hour",
			// 10:30:00 -> 下一整点 11:00:00，差 30 分钟 = 1800,000 ms
			now:   time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC),
			min:   1800000, 
			max:   1801000, // 允许少量 buffer
		},
		{
			name:  "day scope - middle of day",
			scope: "day",
			// 10:00:00 -> 明天 00:00:00，差 14 小时 = 14 * 3600 * 1000 = 50,400,000 ms
			now:   time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
			min:   50400000,
			max:   50401000,
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
				t.Errorf("calcRetryAfter(%s) = %d, want between %d and %d", tt.scope, got, tt.min, tt.max)
			}
		})
	}
}

func TestQuota_calcRetryAfterHour(t *testing.T) {
	q := &Quota{}

	// 测试小时级重试时间计算：10:30:15
	now := time.Date(2024, 1, 1, 10, 30, 15, 0, time.UTC)
	retryAfter := q.calcRetryAfter("hour", now)

	// 计算逻辑：11:00:00.000 - 10:30:15.000 = 29分45秒 = 1785,000 ms
	expected := time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC).Sub(now).Milliseconds()
	if retryAfter != expected {
		t.Errorf("calcRetryAfter(hour) = %d, want %d", retryAfter, expected)
	}
}

func TestQuota_calcRetryAfterDay(t *testing.T) {
	q := &Quota{}

	// 测试天级重试时间计算：10:30:15
	now := time.Date(2024, 1, 1, 10, 30, 15, 0, time.UTC)
	retryAfter := q.calcRetryAfter("day", now)

	// 计算逻辑：次日 00:00:00 - 10:30:15
	expected := time.Date(2024, 1, 1, 10, 30, 15, 0, time.UTC).AddDate(0, 0, 1).Truncate(24 * time.Hour).Sub(now.Truncate(time.Second)).Milliseconds()
	
    // 注意：由于代码中 day 计算没加 1s buffer，我们直接比对
	if retryAfter < expected-2000 || retryAfter > expected+2000 {
		t.Errorf("calcRetryAfter(day) = %d, expected around %d", retryAfter, expected)
	}
}

func BenchmarkQuota_calcRetryAfter(b *testing.B) {
	q := &Quota{}
	now := time.Now()

	// 移除 "min"
	scopes := []string{"hour", "day", "unknown"}

	for _, scope := range scopes {
		b.Run(scope, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = q.calcRetryAfter(scope, now)
			}
		})
	}
}
