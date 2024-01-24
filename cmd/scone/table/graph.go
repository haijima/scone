package table

// Graph
type Graph struct {
	edges   map[string][]string
	visited map[string]bool
	nodeMap map[string]bool
}

func NewGraph() *Graph {
	return &Graph{
		edges:   make(map[string][]string),
		visited: make(map[string]bool),
		nodeMap: make(map[string]bool),
	}
}

func (g *Graph) AddNode(node string) {
	g.nodeMap[node] = true
}

func (g *Graph) AddEdge(u, v string) {
	g.edges[u] = append(g.edges[u], v)
	g.edges[v] = append(g.edges[v], u)
}

func (g *Graph) DFS(node string, component *[]string) {
	g.visited[node] = true
	*component = append(*component, node)
	for _, v := range g.edges[node] {
		if !g.visited[v] {
			g.DFS(v, component)
		}
	}
}

func (g *Graph) FindConnectedComponents() [][]string {
	var components [][]string
	for node := range g.nodeMap {
		if !g.visited[node] {
			var component []string
			g.DFS(node, &component)
			components = append(components, component)
		}
	}
	return components
}
