package utils

import (
	"container/list"
	"fmt"
	"strings"
	"sync"
)

// Pair 表示键值对
type Pair[K comparable, V any] struct {
	Key   K
	Value V
}

// OrderedMap 是一个保持插入顺序的并发安全 Map
type OrderedMap[K comparable, V any] struct {
	mu       sync.RWMutex
	list     *list.List
	elements map[K]*list.Element
}

// NewOrderedMap 创建一个空的 OrderedMap
func NewOrderedMap[K comparable, V any]() *OrderedMap[K, V] {
	return &OrderedMap[K, V]{
		list:     list.New(),
		elements: make(map[K]*list.Element),
	}
}

// InitOrderedMap 创建并初始化一个包含单个键值对的 OrderedMap
func InitOrderedMap[K comparable, V any](key K, value V) *OrderedMap[K, V] {
	return NewOrderedMap[K, V]().Set(key, value)
}

// Set 添加或更新键值对（更新时保持原顺序）
func (m *OrderedMap[K, V]) Set(key K, value V) *OrderedMap[K, V] {
	m.mu.Lock()
	defer m.mu.Unlock()

	if elem, ok := m.elements[key]; ok {
		elem.Value = Pair[K, V]{Key: key, Value: value}
	} else {
		elem = m.list.PushBack(Pair[K, V]{Key: key, Value: value})
		m.elements[key] = elem
	}
	return m
}

// Get 获取指定 key 的值
func (m *OrderedMap[K, V]) Get(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if elem, ok := m.elements[key]; ok {
		return elem.Value.(Pair[K, V]).Value, true
	}
	var zero V
	return zero, false
}

// Del 删除指定 key
func (m *OrderedMap[K, V]) Del(key K) *OrderedMap[K, V] {
	m.mu.Lock()
	defer m.mu.Unlock()

	if elem, ok := m.elements[key]; ok {
		m.list.Remove(elem)
		delete(m.elements, key)
	}
	return m
}

// Each 遍历所有键值对（正序）
func (m *OrderedMap[K, V]) Each(f func(Pair[K, V])) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for e := m.list.Front(); e != nil; e = e.Next() {
		f(e.Value.(Pair[K, V]))
	}
}

// EachReverse 遍历所有键值对（逆序）
func (m *OrderedMap[K, V]) EachReverse(f func(Pair[K, V])) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for e := m.list.Back(); e != nil; e = e.Prev() {
		f(e.Value.(Pair[K, V]))
	}
}

// String 返回有序 Map 的字符串表示
func (m *OrderedMap[K, V]) String() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("{")
	first := true
	for e := m.list.Front(); e != nil; e = e.Next() {
		if !first {
			sb.WriteString(", ")
		}
		pair := e.Value.(Pair[K, V])
		_, _ = fmt.Fprintf(&sb, "%v: %v", pair.Key, pair.Value)
		first = false
	}
	sb.WriteString("}")
	return sb.String()
}
