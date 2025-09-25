package types

// Decision 限流判定结果
// 原位于core包，移至公共类型包避免循环依赖
type Decision struct {
	Allowed      bool   // 是否允许请求
	Remaining    int64  // 剩余可用配额
	RetryAfterMs int64  // 建议重试时间(毫秒)
	Reason       string // 判定原因
	Err          error  // 错误信息(如有)
}
