package util

import mapset "github.com/deckarep/golang-set/v2"

type SetMap[K, V comparable] map[K]mapset.Set[V]

func NewSetMap[K, V comparable]() SetMap[K, V] {
	return make(SetMap[K, V])
}

func (m SetMap[K, V]) Add(key K, value V) {
	if _, ok := m[key]; !ok {
		m[key] = mapset.NewSet[V]()
	}
	m[key].Add(value)
}

func (m SetMap[K, V]) Intersect(key K, other mapset.Set[V]) {
	if _, ok := m[key]; !ok {
		m[key] = other.Clone().(mapset.Set[V])
	}
	m[key] = m[key].Intersect(other)
}