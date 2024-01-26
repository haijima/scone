package table

import mapset "github.com/deckarep/golang-set/v2"

// Graph
type Graph struct {
	edges   map[string][]string
	visited map[string]bool
	nodeMap map[string]bool
}

func NewGraph(nodes ...string) *Graph {
	g := &Graph{
		edges:   make(map[string][]string),
		visited: make(map[string]bool),
		nodeMap: make(map[string]bool),
	}
	for _, node := range nodes {
		g.AddNode(node)
	}
	return g
}

func (g *Graph) AddNode(node string) {
	g.nodeMap[node] = true
}

func (g *Graph) AddEdge(u, v string) {
	g.edges[u] = append(g.edges[u], v)
	g.edges[v] = append(g.edges[v], u)
}

func (g *Graph) DFS(node string, component *mapset.Set[string]) {
	g.visited[node] = true
	(*component).Add(node)
	for _, v := range g.edges[node] {
		if !g.visited[v] {
			g.DFS(v, component)
		}
	}
}

func (g *Graph) FindConnectedComponents() []mapset.Set[string] {
	var components []mapset.Set[string]
	for node := range g.nodeMap {
		if !g.visited[node] {
			component := mapset.NewSet[string]()
			g.DFS(node, &component)
			components = append(components, component)
		}
	}
	return components
}
