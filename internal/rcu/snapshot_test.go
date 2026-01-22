package rcu

import (
	"sync"
	"testing"
	"time"
)

type TestData struct {
	Value int
	Name  string
}

// TestBasicUsage 测试基本用法
func TestBasicUsage(t *testing.T) {
	initial := &TestData{Value: 100, Name: "initial"}
	snap := NewSnapshot(initial)

	// 读取初始值
	data := snap.Load()
	if data.Value != 100 || data.Name != "initial" {
		t.Errorf("expected Value=100, Name=initial, got Value=%d, Name=%s", data.Value, data.Name)
	}

	// 更新快照
	updated := &TestData{Value: 200, Name: "updated"}
	snap.Replace(updated)

	// 读取更新后的值
	data = snap.Load()
	if data.Value != 200 || data.Name != "updated" {
		t.Errorf("expected Value=200, Name=updated, got Value=%d, Name=%s", data.Value, data.Name)
	}
}

// TestConcurrentRead 测试并发读取
func TestConcurrentRead(t *testing.T) {
	initial := &TestData{Value: 42, Name: "test"}
	snap := NewSnapshot(initial)

	var wg sync.WaitGroup
	numReaders := 1000

	// 启动多个并发读取
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data := snap.Load()
			if data.Value != 42 {
				t.Errorf("expected Value=42, got %d", data.Value)
			}
		}()
	}

	wg.Wait()
}

// TestConcurrentReadWrite 测试并发读写
func TestConcurrentReadWrite(t *testing.T) {
	initial := &TestData{Value: 0, Name: "v0"}
	snap := NewSnapshot(initial)

	var wg sync.WaitGroup
	numReaders := 100
	numWriters := 10
	iterations := 100

	// 启动并发读取
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				data := snap.Load()
				// 只要能读取到数据就算成功
				_ = data.Value
				time.Sleep(time.Microsecond)
			}
		}()
	}

	// 启动并发写入
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations/10; j++ {
				newData := &TestData{
					Value: id*1000 + j,
					Name:  "updated",
				}
				snap.Replace(newData)
				time.Sleep(10 * time.Microsecond)
			}
		}(i)
	}

	wg.Wait()
}

// BenchmarkLoad 基准测试：读取性能
func BenchmarkLoad(b *testing.B) {
	data := &TestData{Value: 100, Name: "benchmark"}
	snap := NewSnapshot(data)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = snap.Load()
		}
	})
}

// BenchmarkReplace 基准测试：写入性能
func BenchmarkReplace(b *testing.B) {
	data := &TestData{Value: 100, Name: "benchmark"}
	snap := NewSnapshot(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		newData := &TestData{Value: i, Name: "updated"}
		snap.Replace(newData)
	}
}

// BenchmarkReadWrite 基准测试：混合读写（90% 读，10% 写）
func BenchmarkReadWrite(b *testing.B) {
	data := &TestData{Value: 100, Name: "benchmark"}
	snap := NewSnapshot(data)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%10 == 0 {
				// 10% 写操作
				newData := &TestData{Value: i, Name: "updated"}
				snap.Replace(newData)
			} else {
				// 90% 读操作
				_ = snap.Load()
			}
			i++
		}
	})
}

// BenchmarkMapSnapshot 基准测试：大 map 快照的读性能
func BenchmarkMapSnapshot(b *testing.B) {
	type MapData struct {
		Items map[string]int
	}

	// 准备一个包含 1000 个元素的 map
	items := make(map[string]int)
	for i := 0; i < 1000; i++ {
		items[string(rune('a'+i%26))+string(rune('0'+i%10))] = i
	}

	data := &MapData{Items: items}
	snap := NewSnapshot(data)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m := snap.Load()
			// 模拟查找操作
			_ = m.Items["a0"]
		}
	})
}

type rwSnapshot struct {
	mu   sync.RWMutex
	data *TestData
}

func (s *rwSnapshot) Load() *TestData {
	s.mu.RLock()
	data := s.data
	s.mu.RUnlock()
	return data
}

func (s *rwSnapshot) Replace(next *TestData) {
	s.mu.Lock()
	s.data = next
	s.mu.Unlock()
}

// BenchmarkRWMutexLoad 基准测试：RWMutex 读性能
func BenchmarkRWMutexLoad(b *testing.B) {
	data := &TestData{Value: 100, Name: "benchmark"}
	snap := &rwSnapshot{data: data}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = snap.Load()
		}
	})
}

// BenchmarkRWMutexReplace 基准测试：RWMutex 写性能
func BenchmarkRWMutexReplace(b *testing.B) {
	data := &TestData{Value: 100, Name: "benchmark"}
	snap := &rwSnapshot{data: data}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		newData := &TestData{Value: i, Name: "updated"}
		snap.Replace(newData)
	}
}

// BenchmarkRWMutexReadWrite 基准测试：RWMutex 混合读写（90% 读，10% 写）
func BenchmarkRWMutexReadWrite(b *testing.B) {
	data := &TestData{Value: 100, Name: "benchmark"}
	snap := &rwSnapshot{data: data}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%10 == 0 {
				newData := &TestData{Value: i, Name: "updated"}
				snap.Replace(newData)
			} else {
				_ = snap.Load()
			}
			i++
		}
	})
}

// BenchmarkRWMutexMapSnapshot 基准测试：RWMutex 大 map 读取性能
func BenchmarkRWMutexMapSnapshot(b *testing.B) {
	type MapData struct {
		Items map[string]int
	}

	items := make(map[string]int)
	for i := 0; i < 1000; i++ {
		items[string(rune('a'+i%26))+string(rune('0'+i%10))] = i
	}

	data := &MapData{Items: items}
	snap := struct {
		mu   sync.RWMutex
		data *MapData
	}{data: data}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			snap.mu.RLock()
			m := snap.data
			_ = m.Items["a0"]
			snap.mu.RUnlock()
		}
	})
}

