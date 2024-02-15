package util

import (
	"testing"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/stretchr/testify/assert"
)

func TestNewSetMap(t *testing.T) {
	setMap := NewSetMap[string, string]("a", "b", "c")
	if len(setMap) != 3 {
		t.Errorf("NewSetMap() = %v, want %v", len(setMap), 3)
	}
}

func TestSetMap_Add(t *testing.T) {
	setMap := NewSetMap[string, string]("a", "b", "c")
	setMap.Add("a", "b")
	setMap.Add("a", "c")
	setMap.Add("b", "c")
	setMap.Add("d", "e")

	assert.Equal(t, mapset.NewSet[string]("b", "c"), setMap["a"])
	assert.Equal(t, mapset.NewSet[string]("c"), setMap["b"])
	assert.Equal(t, mapset.NewSet[string](), setMap["c"])
	assert.Equal(t, mapset.NewSet[string]("e"), setMap["d"])
}

func TestSetMap_Intersect(t *testing.T) {
	setMap := NewSetMap[string, string]("a", "b", "c")
	setMap.Add("a", "b")
	setMap.Add("a", "c")
	setMap.Add("b", "c")

	setMap.Intersect("a", mapset.NewSet[string]("c", "d"))
	setMap.Intersect("b", mapset.NewSet[string]("a", "c"))
	setMap.Intersect("d", mapset.NewSet[string]("e"))

	assert.Equal(t, mapset.NewSet[string]("c"), setMap["a"])
	assert.Equal(t, mapset.NewSet[string]("c"), setMap["b"])
	assert.Equal(t, mapset.NewSet[string](), setMap["c"])
	assert.Equal(t, mapset.NewSet[string]("e"), setMap["d"])
}
