package util

import mapset "github.com/deckarep/golang-set/v2"

type SetMap[K, V comparable] map[K]mapset.Set[V]

func NewSetMap[K, V comparable](keys ...K) SetMap[K, V] {
	m := make(SetMap[K, V])
	for _, key := range keys {
		m[key] = mapset.NewSet[V]()
	}
	return m
}

func (m SetMap[K, V]) Add(key K, value V) {
	if _, ok := m[key]; !ok {
		m[key] = mapset.NewSet[V]()
	}
	m[key].Add(value)
}

func (m SetMap[K, V]) Intersect(key K, other mapset.Set[V]) {
	if _, ok := m[key]; !ok {
		m[key] = other.Clone()
	}
	m[key] = m[key].Intersect(other)
}
