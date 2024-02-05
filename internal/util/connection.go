package util

import (
	mapset "github.com/deckarep/golang-set/v2"
)

type Connection SetMap[string, string]

func NewConnection(elements ...string) Connection {
	return Connection(NewSetMap[string, string](elements...))
}

func (c Connection) Connect(u, v string) {
	c[u].Add(v)
	c[v].Add(u)
}

func (c Connection) GetConnection(elem string, distance int) mapset.Set[string] {
	if distance < 0 {
		distance = len(c) * (len(c) - 1) / 2
	}
	visited := mapset.NewSet(elem)
	queue := c[elem].Clone()
	for i := 0; i < distance && !queue.IsEmpty(); i++ {
		visited = visited.Union(queue)
		nextQueue := mapset.NewSet[string]()
		for _, q := range queue.ToSlice() {
			nextQueue = nextQueue.Union(c[q])
		}
		queue = nextQueue.Difference(visited)
	}
	return visited
}

func (c Connection) GetClusters() []mapset.Set[string] {
	visited := mapset.NewSet[string]()
	clusters := make([]mapset.Set[string], 0)
	for elem := range c {
		if !visited.Contains(elem) {
			cluster := c.GetConnection(elem, -1)
			clusters = append(clusters, cluster)
			visited = visited.Union(cluster)
		}
	}
	return clusters
}
