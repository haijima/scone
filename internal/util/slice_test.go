package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPairCombinate(t *testing.T) {
	got := PairCombinate([]int{1, 2, 1, 3, 2})
	assert.Equal(t, []Pair[int]{{1, 2}, {1, 3}, {2, 3}}, got)
}

func TestPairCombinateFunc(t *testing.T) {
	var got []Pair[int]
	PairCombinateFunc([]int{1, 2, 1, 3, 2}, func(a, b int) {
		got = append(got, Pair[int]{a, b})
	})

	assert.Equal(t, []Pair[int]{{1, 2}, {1, 3}, {2, 3}}, got)
}
