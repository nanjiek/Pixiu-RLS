package util

import (
	"testing"
)

func TestValidateDims(t *testing.T) {
	tests := []struct {
		name      string
		ruleDims  []string
		inputDims map[string]string
		wantErr   bool
	}{
		{
			name:     "all dims present",
			ruleDims: []string{"ip", "route", "user_id"},
			inputDims: map[string]string{
				"ip":      "192.168.1.1",
				"route":   "/api/login",
				"user_id": "123",
			},
			wantErr: false,
		},
		{
			name:     "missing dimension",
			ruleDims: []string{"ip", "route", "user_id"},
			inputDims: map[string]string{
				"ip":    "192.168.1.1",
				"route": "/api/login",
			},
			wantErr: true,
		},
		{
			name:     "empty rule dims",
			ruleDims: []string{},
			inputDims: map[string]string{
				"ip": "192.168.1.1",
			},
			wantErr: false,
		},
		{
			name:      "extra dims in input",
			ruleDims:  []string{"ip"},
			inputDims: map[string]string{
				"ip":      "192.168.1.1",
				"route":   "/api/login",
				"user_id": "123",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDims(tt.ruleDims, tt.inputDims)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDims() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExtractDims(t *testing.T) {
	tests := []struct {
		name      string
		ruleDims  []string
		inputDims map[string]string
		want      []string
	}{
		{
			name:     "extract in order",
			ruleDims: []string{"route", "ip", "user_id"},
			inputDims: map[string]string{
				"ip":      "192.168.1.1",
				"route":   "/api/login",
				"user_id": "123",
			},
			want: []string{"/api/login", "192.168.1.1", "123"},
		},
		{
			name:     "missing dims return empty string",
			ruleDims: []string{"ip", "route", "missing"},
			inputDims: map[string]string{
				"ip":    "192.168.1.1",
				"route": "/api/login",
			},
			want: []string{"192.168.1.1", "/api/login", ""},
		},
		{
			name:      "empty rule dims",
			ruleDims:  []string{},
			inputDims: map[string]string{"ip": "192.168.1.1"},
			want:      []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractDims(tt.ruleDims, tt.inputDims)
			if len(got) != len(tt.want) {
				t.Errorf("ExtractDims() length = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ExtractDims()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestHashDims(t *testing.T) {
	tests := []struct {
		name      string
		ruleDims  []string
		inputDims map[string]string
		wantErr   bool
	}{
		{
			name:     "valid dims",
			ruleDims: []string{"ip", "route"},
			inputDims: map[string]string{
				"ip":    "192.168.1.1",
				"route": "/api/login",
			},
			wantErr: false,
		},
		{
			name:     "missing required dim",
			ruleDims: []string{"ip", "route", "user_id"},
			inputDims: map[string]string{
				"ip":    "192.168.1.1",
				"route": "/api/login",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashDims(tt.ruleDims, tt.inputDims)
			if (err != nil) != tt.wantErr {
				t.Errorf("HashDims() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && hash == "" {
				t.Error("HashDims() returned empty hash")
			}
		})
	}
}

func TestHashDimsConsistency(t *testing.T) {
	ruleDims := []string{"ip", "route", "user_id"}
	inputDims := map[string]string{
		"ip":      "192.168.1.1",
		"route":   "/api/login",
		"user_id": "123",
	}

	hash1, err1 := HashDims(ruleDims, inputDims)
	hash2, err2 := HashDims(ruleDims, inputDims)

	if err1 != nil || err2 != nil {
		t.Fatalf("HashDims() unexpected errors: %v, %v", err1, err2)
	}

	if hash1 != hash2 {
		t.Errorf("HashDims() not consistent: %s != %s", hash1, hash2)
	}
}

func TestHashDimsOrderMatters(t *testing.T) {
	inputDims := map[string]string{
		"ip":      "192.168.1.1",
		"route":   "/api/login",
		"user_id": "123",
	}

	// 不同的维度顺序应该产生不同的哈希
	hash1, _ := HashDims([]string{"ip", "route", "user_id"}, inputDims)
	hash2, _ := HashDims([]string{"route", "ip", "user_id"}, inputDims)

	if hash1 == hash2 {
		t.Error("HashDims() should produce different hashes for different dim orders")
	}
}

func BenchmarkValidateDims(b *testing.B) {
	ruleDims := []string{"ip", "route", "user_id", "app_id", "device_id"}
	inputDims := map[string]string{
		"ip":        "192.168.1.1",
		"route":     "/api/login",
		"user_id":   "123",
		"app_id":    "app-001",
		"device_id": "device-001",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateDims(ruleDims, inputDims)
	}
}

func BenchmarkExtractDims(b *testing.B) {
	ruleDims := []string{"ip", "route", "user_id", "app_id", "device_id"}
	inputDims := map[string]string{
		"ip":        "192.168.1.1",
		"route":     "/api/login",
		"user_id":   "123",
		"app_id":    "app-001",
		"device_id": "device-001",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtractDims(ruleDims, inputDims)
	}
}

func BenchmarkHashDims(b *testing.B) {
	ruleDims := []string{"ip", "route", "user_id", "app_id", "device_id"}
	inputDims := map[string]string{
		"ip":        "192.168.1.1",
		"route":     "/api/login",
		"user_id":   "123",
		"app_id":    "app-001",
		"device_id": "device-001",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = HashDims(ruleDims, inputDims)
		}
	})
}

