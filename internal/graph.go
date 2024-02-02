package internal

import mapset "github.com/deckarep/golang-set/v2"

// Graph
type Graph struct {
	Edges   map[string][]string
	visited map[string]bool
	NodeMap map[string]bool
}

func NewGraph(nodes ...string) *Graph {
	g := &Graph{
		Edges:   make(map[string][]string),
		visited: make(map[string]bool),
		NodeMap: make(map[string]bool),
	}
	for _, node := range nodes {
		g.AddNode(node)
	}
	return g
}

func (g *Graph) AddNode(node string) {
	g.NodeMap[node] = true
}

func (g *Graph) AddEdge(u, v string) {
	g.Edges[u] = append(g.Edges[u], v)
	g.Edges[v] = append(g.Edges[v], u)
}

func (g *Graph) DFS(node string, component *mapset.Set[string]) {
	g.visited[node] = true
	(*component).Add(node)
	for _, v := range g.Edges[node] {
		if !g.visited[v] {
			g.DFS(v, component)
		}
	}
}

func (g *Graph) FindConnectedComponents() []mapset.Set[string] {
	var components []mapset.Set[string]
	for node := range g.NodeMap {
		if !g.visited[node] {
			component := mapset.NewSet[string]()
			g.DFS(node, &component)
			components = append(components, component)
		}
	}
	return components
}
