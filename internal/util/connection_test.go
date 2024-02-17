package util

import (
	"slices"
	"testing"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/stretchr/testify/assert"
)

func TestConnection(t *testing.T) {
	c := NewConnection("a", "b", "c", "d")
	c.Connect("a", "b")
	c.Connect("b", "c")

	assert.Equal(t, 4, len(c))
	assert.Equal(t, mapset.NewSet("a", "b"), c.GetConnection("a", 1))
	assert.Equal(t, mapset.NewSet("a", "b", "c"), c.GetConnection("a", 2))
	assert.Equal(t, mapset.NewSet("a", "b", "c"), c.GetConnection("a", -1))
	assert.Equal(t, mapset.NewSet("d"), c.GetConnection("d", -1))

	clusters := c.GetClusters()
	slices.SortFunc(clusters, func(a, b mapset.Set[string]) int { return slices.Compare(mapset.Sorted(a), mapset.Sorted(b)) })
	assert.Equal(t, []mapset.Set[string]{mapset.NewSet("a", "b", "c"), mapset.NewSet("d")}, clusters)
}
