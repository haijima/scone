package util

import (
	"cmp"
	"slices"
)

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

func PairCombinateFunc[T cmp.Ordered](a []T, fn func(a, b T)) {
	for _, p := range PairCombinate(a) {
		fn(p.L, p.R)
	}
}
