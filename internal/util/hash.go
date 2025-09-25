package util

import (
	"hash/fnv"
)

func FNV32(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return string([]byte{
		byte(h.Sum32() >> 24), byte(h.Sum32() >> 16),
		byte(h.Sum32() >> 8), byte(h.Sum32()),
	})
}
