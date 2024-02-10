package main

import (
	"fmt"
	"io"

	"github.com/haijima/scone/internal/analysis"
	internalio "github.com/haijima/scone/internal/io"
	"github.com/haijima/scone/internal/sql"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/exp/maps"
)

func NewCallgraphCommand(v *viper.Viper, _ afero.Fs) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "callgraph"
	cmd.Short = "Generate a call graph"
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return runCallgraph(cmd, v)
	}

	cmd.Flags().String("format", "dot", "The output format {dot|mermaid|text}")
	SetQueryOptionFlags(cmd)

	return cmd
}

func runCallgraph(cmd *cobra.Command, v *viper.Viper) error {
	dir := v.GetString("dir")
	pattern := v.GetString("pattern")
	opt := QueryOptionFromViper(v)

	_, cgs, err := analysis.Analyze(dir, pattern, opt)
	if err != nil {
		return err
	}

	return printGraphviz(cmd.OutOrStdout(), cgs)
}

func printGraphviz(w io.Writer, cgs map[string]*analysis.CallGraph) error {
	g := &internalio.DotGraph{Nodes: make([]*internalio.DotNode, 0), Edges: make([]*internalio.DotEdge, 0)}

	for pkg, cg := range cgs {
		for _, node := range cg.Nodes {
			// Print edges
			for _, edge := range node.Out {
				if edge.SqlValue != nil {
					attrs := make(internalio.DotAttrs)
					attrs["weight"] = "100"
					switch edge.SqlValue.Kind {
					case sql.Select:
						attrs["style"] = "dotted"
					case sql.Insert:
						attrs["color"] = "green"
					case sql.Update:
						attrs["color"] = "orange"
					case sql.Delete:
						attrs["color"] = "red"
					default:
					}
					g.Edges = append(g.Edges, &internalio.DotEdge{From: fmt.Sprintf("%s.%s", pkg, edge.Caller), To: edge.Callee, Attrs: attrs})
					g.Nodes = append(g.Nodes, &internalio.DotNode{ID: fmt.Sprintf("%s.%s", pkg, edge.Caller), Attrs: map[string]string{"label": edge.Caller}})
				} else {
					attrs := make(internalio.DotAttrs)
					attrs["style"] = "dashed"
					attrs["weight"] = "100"
					g.Edges = append(g.Edges, &internalio.DotEdge{From: fmt.Sprintf("%s.%s", pkg, edge.Caller), To: fmt.Sprintf("%s.%s", pkg, edge.Callee), Attrs: attrs})
					g.Nodes = append(g.Nodes, &internalio.DotNode{ID: fmt.Sprintf("%s.%s", pkg, edge.Caller), Attrs: map[string]string{"label": edge.Caller}})
					g.Nodes = append(g.Nodes, &internalio.DotNode{ID: fmt.Sprintf("%s.%s", pkg, edge.Callee), Attrs: map[string]string{"label": edge.Callee}})
				}
			}
		}

		fmt.Fprintln(w)
	}

	fmt.Fprintln(w)

	// Print cacheable func and table node styles
	selectOnlyNodes := make(map[string]sql.QueryKind)
	for pkg, cg := range cgs {
		for _, node := range analysis.TopologicalSort(cg.Nodes) {
			// table node
			if node.Func == nil {
				kind := sql.Select
				for _, cg2 := range cgs {
					if n, ok := cg2.Nodes[node.Name]; ok {
						for _, q := range n.In {
							if q.SqlValue != nil {
								kind = max(kind, q.SqlValue.Kind)
							}
						}
					}
				}
				selectOnlyNodes[node.Name] = kind
				continue
			}
			// func node
			selectOnly := true
			kind := sql.Unknown
			for _, edge := range node.Out {
				if edge.SqlValue != nil {
					selectOnly = selectOnly && edge.SqlValue.Kind == sql.Select
				} else {
					_, ok := selectOnlyNodes[edge.Callee]
					selectOnly = selectOnly && ok
				}
				kind = max(kind, selectOnlyNodes[edge.Callee])
			}
			if selectOnly && kind != sql.Unknown {
				selectOnlyNodes[node.Name] = kind
			}
		}

		for n, k := range selectOnlyNodes {
			name := fmt.Sprintf("%s.%s", pkg, n)
			attr := make(internalio.DotAttrs)
			if k == sql.Select {
				attr["color"] = "blue"
				attr["fillcolor"] = "lightblue1"
			} else if k == sql.Insert {
				attr["color"] = "green"
				attr["fillcolor"] = "darkolivegreen1"
			} else if k == sql.Update {
				attr["color"] = "orange"
			} else if k == sql.Delete {
				attr["color"] = "red"
			}
			if cg.Nodes[n].Func == nil {
				name = n
				attr["style"] = "bold"
				attr["shape"] = "box"
			}
			g.Nodes = append(g.Nodes, &internalio.DotNode{ID: name, Attrs: attr})
		}

		// Reset
		selectOnlyNodes = make(map[string]sql.QueryKind)
	}

	fmt.Fprintln(w)

	// Print node positions
	minNodeNames := make(map[string]bool)
	maxNodeNames := make(map[string]bool)
	for pkg, cg := range cgs {
		for _, node := range cg.Nodes {
			if node.Func == nil {
				maxNodeNames[node.Name] = true
			} else if len(node.In) == 0 {
				minNodeNames[fmt.Sprintf("%s.%s", pkg, node.Name)] = true
			}
		}
	}
	g.Ranks = append(g.Ranks, &internalio.DotRank{Name: "min", Nodes: maps.Keys(minNodeNames)})
	g.Ranks = append(g.Ranks, &internalio.DotRank{Name: "max", Nodes: maps.Keys(maxNodeNames)})

	return internalio.WriteDotGraph(w, *g)
}
