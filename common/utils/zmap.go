package utils

import (
	"container/list"
	"encoding/json"
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

// Has 判断指定 key 是否存在
func (m *OrderedMap[K, V]) Has(key K) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.elements[key]
	return ok
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

// Len 返回元素数量
func (m *OrderedMap[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.list.Len()
}

// Keys 返回所有 key（按插入顺序）
func (m *OrderedMap[K, V]) Keys() []K {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]K, 0, m.list.Len())
	for e := m.list.Front(); e != nil; e = e.Next() {
		keys = append(keys, e.Value.(Pair[K, V]).Key)
	}
	return keys
}

// Values 返回所有 value（按插入顺序）
func (m *OrderedMap[K, V]) Values() []V {
	m.mu.RLock()
	defer m.mu.RUnlock()

	values := make([]V, 0, m.list.Len())
	for e := m.list.Front(); e != nil; e = e.Next() {
		values = append(values, e.Value.(Pair[K, V]).Value)
	}
	return values
}

// Pairs 返回所有键值对的快照（按插入顺序）
func (m *OrderedMap[K, V]) Pairs() []Pair[K, V] {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshot()
}

func (m *OrderedMap[K, V]) snapshot() []Pair[K, V] {
	pairs := make([]Pair[K, V], 0, m.list.Len())
	for e := m.list.Front(); e != nil; e = e.Next() {
		pairs = append(pairs, e.Value.(Pair[K, V]))
	}
	return pairs
}

func (m *OrderedMap[K, V]) snapshotReverse() []Pair[K, V] {
	pairs := make([]Pair[K, V], 0, m.list.Len())
	for e := m.list.Back(); e != nil; e = e.Prev() {
		pairs = append(pairs, e.Value.(Pair[K, V]))
	}
	return pairs
}

// Each 遍历所有键值对（正序），回调返回 false 时停止遍历。
// 回调在锁外执行，可安全调用 Set/Del。
func (m *OrderedMap[K, V]) Each(f func(Pair[K, V]) bool) {
	m.mu.RLock()
	pairs := m.snapshot()
	m.mu.RUnlock()

	for _, p := range pairs {
		if !f(p) {
			break
		}
	}
}

// EachReverse 遍历所有键值对（逆序），回调返回 false 时停止遍历。
func (m *OrderedMap[K, V]) EachReverse(f func(Pair[K, V]) bool) {
	m.mu.RLock()
	pairs := m.snapshotReverse()
	m.mu.RUnlock()

	for _, p := range pairs {
		if !f(p) {
			break
		}
	}
}

// MarshalJSON 实现 json.Marshaler，按插入顺序序列化为 JSON 数组
func (m *OrderedMap[K, V]) MarshalJSON() ([]byte, error) {
	m.mu.RLock()
	pairs := m.snapshot()
	m.mu.RUnlock()
	return json.Marshal(pairs)
}

// String 返回有序 Map 的字符串表示
func (m *OrderedMap[K, V]) String() string {
	m.mu.RLock()
	pairs := m.snapshot()
	m.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("{")
	for i, pair := range pairs {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprint(pair.Key))
		sb.WriteString(": ")
		sb.WriteString(fmt.Sprint(pair.Value))
	}
	sb.WriteString("}")
	return sb.String()
}
