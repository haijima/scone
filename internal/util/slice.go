package util

import (
	"cmp"
	"slices"
)

func Intersect(a, b []string) []string {
	if a == nil {
		return b
	} else if b == nil {
		return a
	}

	seen := make(map[string]bool)
	for _, v := range a {
		seen[v] = true
	}
	r := make([]string, 0)
	for _, v := range b {
		if seen[v] {
			r = append(r, v)
		}
	}
	return r
}

func Empty(a []string) bool {
	return len(a) == 0
}

type Pair[T any] struct{ L, R T }

func PairCombinate[T cmp.Ordered](a []T) []Pair[T] {
	slices.Sort(a)
	a = slices.Compact(a)

	r := make([]Pair[T], 0, len(a)*(len(a)-1)/2)
	for i, a1 := range a {
		for j, a2 := range a {
			if i < j {
				r = append(r, Pair[T]{a1, a2})
			}
		}
	}
	return r
}
