package rcu

import (
	"sync/atomic"
)

// Snapshot 是一个基于 RCU（Read-Copy-Update）机制的无锁快照容器
// 特性：
// - 读操作无锁，性能极高，适合读多写少场景
// - 写操作通过原子指针替换实现，无需对旧数据加锁
// - 保证读操作看到的数据是一致的快照
// 
// 使用场景：
// - 配置/规则的热更新
// - 频繁读取、偶尔更新的共享数据结构
type Snapshot[T any] struct {
	ptr atomic.Pointer[T]
}

// NewSnapshot 创建一个新的快照容器并初始化
func NewSnapshot[T any](init *T) *Snapshot[T] {
	s := &Snapshot[T]{}
	s.ptr.Store(init)
	return s
}

// Load 读取当前快照（无锁操作，极高性能）
// 返回的指针指向不可变数据，可以安全地并发读取
func (s *Snapshot[T]) Load() *T {
	return s.ptr.Load()
}

// Replace 用新快照替换当前快照（写操作）
// 注意：调用者需确保传入的数据是新分配的副本，避免后续修改
func (s *Snapshot[T]) Replace(next *T) {
	s.ptr.Store(next)
}
