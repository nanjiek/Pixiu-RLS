package util

// ToInt64 安全地把 interface{} 转换为 int64
// 兼容 Redis Lua 返回的 int64 / float64 / uint64
func ToInt64(v interface{}) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	case uint64:
		return int64(x)
	default:
		return 0
	}
}
