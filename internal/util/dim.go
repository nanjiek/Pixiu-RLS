package util

import (
	"fmt"
	"hash/fnv"
)

// ValidateDims 验证输入维度是否符合规则定义的维度列表
func ValidateDims(ruleDims []string, inputDims map[string]string) error {
	for _, dim := range ruleDims {
		if _, ok := inputDims[dim]; !ok {
			return fmt.Errorf("missing required dimension: %s", dim)
		}
	}
	return nil
}

// ExtractDims 从输入维度中提取规则需要的维度（按规则顺序）
func ExtractDims(ruleDims []string, inputDims map[string]string) []string {
	res := make([]string, 0, len(ruleDims))
	for _, dim := range ruleDims {
		res = append(res, inputDims[dim]) // 不存在的 dim 会返回空字符串
	}
	return res
}

// HashDims 基于规则维度生成哈希键
// 算法：FNV-1a 64 位
func HashDims(ruleDims []string, inputDims map[string]string) (string, error) {
	// 校验必需维度
	if err := ValidateDims(ruleDims, inputDims); err != nil {
		return "", err
	}

	parts := ExtractDims(ruleDims, inputDims)
	return FNV64(fmt.Sprintf("%v", parts)), nil
}

// FNV64 使用 FNV-1a 64 位哈希算法，返回 16 进制字符串
func FNV64(s string) string {
	h := fnv.New64a() // 64-bit FNV-1a
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum64())
}
