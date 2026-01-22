package core

import (
	"context"
	"time"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/repo"
	"github.com/nanjiek/pixiu-rls/internal/types" // 使用公共类型
)

// Quota 配额控制器（分钟/小时/天级限制）
type Quota struct {
	repo *repo.RedisRepo
}

// CheckAndIncr 检查并增加配额计数
func (q *Quota) CheckAndIncr(ctx context.Context, rule config.Rule, dimKey string, now time.Time) types.Decision {
	// 检查单个时间粒度的配额
	check := func(scope string, limit int64, ttl time.Duration, timestamp string) types.Decision {
		if limit <= 0 {
			return types.Decision{Allowed: true} // 配额为0表示不限制
		}

		key := q.repo.KeyQuota(rule.RuleID, dimKey, scope, timestamp)
		cnt, err := q.repo.IncrAndExpire(ctx, key, ttl)
		if err != nil {
			return types.Decision{
				Allowed: false,
				Reason:  "quota_incr_failed",
				Err:     err,
			}
		}

		if cnt > limit {
			return types.Decision{
				Allowed:      false,
				Remaining:    0,
				RetryAfterMs: q.calcRetryAfter(scope, now),
				Reason:       "quota_exceeded:" + scope,
			}
		}

		return types.Decision{
			Allowed:   true,
			Remaining: limit - cnt,
			Reason:    "quota_ok:" + scope,
		}
	}

	// 分钟级检查
	minDec := check("min", rule.Quota.PerMinute, 90*time.Minute, now.Format("200601021504"))
	if !minDec.Allowed {
		return minDec
	}

	// 小时级检查
	hourDec := check("hour", rule.Quota.PerHour, 3*time.Hour, now.Format("2006010215"))
	if !hourDec.Allowed {
		return hourDec
	}

	// 天级检查
	dayDec := check("day", rule.Quota.PerDay, 30*time.Hour, now.Format("20060102"))
	if !dayDec.Allowed {
		return dayDec
	}

	return types.Decision{Allowed: true, Reason: "quota_all_ok"}
}

// 计算建议重试时间（毫秒）
func (q *Quota) calcRetryAfter(scope string, now time.Time) int64 {
	switch scope {
	case "min":
		// 剩余分钟数*60秒
		return int64(60-now.Second())*1000 + 1000
	case "hour":
		// 剩余小时数*3600秒
		return int64((60-now.Minute())*60-now.Second())*1000 + 1000
	case "day":
		// 剩余天数*86400秒
		return int64((24-now.Hour())*3600-now.Minute()*60-now.Second())*1000 + 1000
	default:
		return 1000 // 默认1秒
	}
}
