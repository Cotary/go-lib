package utils

import (
	"container/list"
)

type Pair[T comparable, U any] struct {
	Key   T
	Value U
}

type ZMap[T comparable, U any] struct {
	List     *list.List
	elements map[T]*list.Element
}

func NewZMap[T comparable, U any]() *ZMap[T, U] {
	return &ZMap[T, U]{
		List:     list.New(),
		elements: make(map[T]*list.Element),
	}
}

func InitZMap[T comparable, U any](key T, value U) *ZMap[T, U] {
	return NewZMap[T, U]().Set(key, value)
}

func (m *ZMap[T, U]) Set(key T, value U) *ZMap[T, U] {
	if elem, ok := m.elements[key]; ok {
		elem.Value = Pair[T, U]{Key: key, Value: value}
	} else {
		elem := m.List.PushBack(Pair[T, U]{Key: key, Value: value})
		m.elements[key] = elem
	}
	return m
}

func (m *ZMap[T, U]) Get(key T) (U, bool) {
	if elem, ok := m.elements[key]; ok {
		return elem.Value.(Pair[T, U]).Value, true
	}
	return *new(U), false
}

func (m *ZMap[T, U]) Del(key T) *ZMap[T, U] {
	if elem, ok := m.elements[key]; ok {
		m.List.Remove(elem)
		delete(m.elements, key)
	}
	return m
}
func (m *ZMap[T, U]) Each(f func(Pair[T, U])) {
	for e := m.List.Front(); e != nil; e = e.Next() {
		f(e.Value.(Pair[T, U]))
	}
}
