package util

import (
	"testing"
)

func TestFNV64(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int
		wantSame bool
	}{
		{
			name:     "basic string",
			input:    "hello",
			wantLen:  16, // 64-bit hash = 16 hex chars
			wantSame: false,
		},
		{
			name:     "empty string",
			input:    "",
			wantLen:  16,
			wantSame: false,
		},
		{
			name:     "long string",
			input:    "this is a very long string for testing hash function",
			wantLen:  16,
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := FNV64(tt.input)
			if len(hash) != tt.wantLen {
				t.Errorf("FNV64() hash length = %d, want %d", len(hash), tt.wantLen)
			}
		})
	}
}

func TestFNV64Consistency(t *testing.T) {
	// 相同输入应该产生相同输出
	input := "test-consistency"
	hash1 := FNV64(input)
	hash2 := FNV64(input)

	if hash1 != hash2 {
		t.Errorf("FNV64() not consistent: %s != %s", hash1, hash2)
	}
}

func TestFNV64Different(t *testing.T) {
	// 不同输入应该产生不同输出（大概率）
	hash1 := FNV64("input1")
	hash2 := FNV64("input2")

	if hash1 == hash2 {
		t.Error("FNV64() produced same hash for different inputs")
	}
}

func TestFNV32(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"basic", "hello"},
		{"empty", ""},
		{"numbers", "12345"},
		{"special", "!@#$%^&*()"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := FNV32(tt.input)
			if len(hash) != 4 {
				t.Errorf("FNV32() should return 4 bytes, got %d", len(hash))
			}
		})
	}
}

func BenchmarkFNV64(b *testing.B) {
	inputs := []string{
		"short",
		"medium length string",
		"this is a very long string that will be hashed multiple times for benchmarking",
	}

	for _, input := range inputs {
		b.Run("len="+string(rune('0'+len(input)/10)), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = FNV64(input)
			}
		})
	}
}

func BenchmarkFNV32(b *testing.B) {
	inputs := []string{
		"short",
		"medium length string",
		"this is a very long string that will be hashed multiple times for benchmarking",
	}

	for _, input := range inputs {
		b.Run("len="+string(rune('0'+len(input)/10)), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = FNV32(input)
			}
		})
	}
}

