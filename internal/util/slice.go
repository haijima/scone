package util

import (
	"cmp"
	"slices"
)

type Pair[T any] struct{ L, R T }

func PairCombinate[S ~[]E, E cmp.Ordered](x S) []Pair[E] {
	slices.Sort(x)
	x = slices.Compact(x)

	pairs := make([]Pair[E], 0, len(x)*(len(x)-1)/2)
	for i, x1 := range x {
		for j, x2 := range x {
			if i < j {
				pairs = append(pairs, Pair[E]{x1, x2})
			}
		}
	}
	return pairs
}

func PairCombinateFunc[S ~[]E, E cmp.Ordered](a S, fn func(a, b E)) {
	for _, p := range PairCombinate(a) {
		fn(p.L, p.R)
	}
}
