package main

import (
	"fmt"
	"io"

	"github.com/haijima/scone/internal"
	"github.com/haijima/scone/internal/analysis"
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
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runCallgraph(cmd, v)
	}

	cmd.Flags().String("format", "dot", "The output format {dot|mermaid|text}")
	SetQueryOptionFlags(cmd)

	return cmd
}

func runCallgraph(cmd *cobra.Command, v *viper.Viper) error {
	dir := v.GetString("dir")
	pattern := v.GetString("pattern")
	opt, err := QueryOptionFromViper(v)
	if err != nil {
		return err
	}

	_, _, cgs, err := analysis.Analyze(dir, pattern, opt)
	if err != nil {
		return err
	}

	return printGraphviz(cmd.OutOrStdout(), cgs)
}

func printGraphviz(w io.Writer, cgs map[string]*analysis.CallGraph) error {
	//c := &DotCluster{ID: "callgraph", Clusters: make(map[string]*DotCluster), Nodes: make([]*DotNode, 0), Attrs: make(DotAttrs)}
	g := &internal.DotGraph{Nodes: make([]*internal.DotNode, 0), Edges: make([]*internal.DotEdge, 0)}

	for pkg, cg := range cgs {
		for _, node := range cg.Nodes {
			// Print edges
			for _, edge := range node.Out {
				if edge.SqlValue != nil {
					attrs := make(internal.DotAttrs)
					attrs["weight"] = "100"
					switch edge.SqlValue.Kind {
					case sql.Select:
						attrs["style"] = "dotted"
						//attrs["weight"] = "1"
					case sql.Insert:
						attrs["color"] = "green"
					case sql.Update:
						attrs["color"] = "orange"
					case sql.Delete:
						attrs["color"] = "red"
					default:
					}
					g.Edges = append(g.Edges, &internal.DotEdge{From: fmt.Sprintf("%s.%s", pkg, edge.Caller), To: edge.Callee, Attrs: attrs})
					g.Nodes = append(g.Nodes, &internal.DotNode{ID: fmt.Sprintf("%s.%s", pkg, edge.Caller), Attrs: map[string]string{"label": edge.Caller}})
				} else {
					attrs := make(internal.DotAttrs)
					attrs["style"] = "dashed"
					attrs["weight"] = "100"
					g.Edges = append(g.Edges, &internal.DotEdge{From: fmt.Sprintf("%s.%s", pkg, edge.Caller), To: fmt.Sprintf("%s.%s", pkg, edge.Callee), Attrs: attrs})
					g.Nodes = append(g.Nodes, &internal.DotNode{ID: fmt.Sprintf("%s.%s", pkg, edge.Caller), Attrs: map[string]string{"label": edge.Caller}})
					g.Nodes = append(g.Nodes, &internal.DotNode{ID: fmt.Sprintf("%s.%s", pkg, edge.Callee), Attrs: map[string]string{"label": edge.Callee}})
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
					n, ok := cg2.Nodes[node.Name]
					if ok {
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
			attr := make(internal.DotAttrs)
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
			g.Nodes = append(g.Nodes, &internal.DotNode{ID: name, Attrs: attr})
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
	g.Ranks = append(g.Ranks, &internal.DotRank{Name: "min", Nodes: maps.Keys(minNodeNames)})
	g.Ranks = append(g.Ranks, &internal.DotRank{Name: "max", Nodes: maps.Keys(maxNodeNames)})

	return internal.WriteDotGraph(w, *g)
}

func showLegend(w io.Writer) {
	fmt.Fprintln(w, "\tsubgraph cluster_legend {")
	fmt.Fprintln(w, "\t\tlabel=\"Legend\"")
	fmt.Fprintf(w, "\t\t\"legend_table2\"[label=\"Table\", shape=\"box\", color=\"blue\", fillcolor=\"lightblue1\", style=\"bold,filled\"]\n")
	fmt.Fprintf(w, "\t\t\"legend_table3\"[label=\"Table\", shape=\"box\", color=\"green\", fillcolor=\"darkolivegreen1\", style=\"bold,filled\"]\n")
	fmt.Fprintf(w, "\t\t\"legend_table4\"[label=\"Table\", shape=\"box\", color=\"orange\",style=\"solid\"]\n")
	fmt.Fprintf(w, "\t\t\"legend_table5\"[label=\"Table\", shape=\"box\", color=\"red\", style=\"solid\"]\n")
	fmt.Fprintf(w, "\t\t\"legend_func1\"[label=\"Func\"]\n")
	fmt.Fprintf(w, "\t\t\"legend_func2\"[label=\"Func\"]\n")
	fmt.Fprintf(w, "\t\t\"legend_func3\"[label=\"Func\"]\n")
	fmt.Fprintf(w, "\t\t\"legend_func4\"[label=\"Func\"]\n")
	fmt.Fprintf(w, "\t\t\"legend_func5\"[label=\"Func\"]\n")
	fmt.Fprintf(w, "\t\t\"legend_func2\" -> \"legend_table2\"[label=\"SELECT\", style=\"dotted\"];\n")
	fmt.Fprintf(w, "\t\t\"legend_func3\" -> \"legend_table3\"[label=\"INSERT\", color=\"green\"];\n")
	fmt.Fprintf(w, "\t\t\"legend_func4\" -> \"legend_table4\"[label=\"UPDATE\", color=\"orange\"];\n")
	fmt.Fprintf(w, "\t\t\"legend_func5\" -> \"legend_table5\"[label=\"DELETE\", color=\"red\"];\n")
	fmt.Fprintf(w, "\t\t\"legend_func1\" -> \"legend_func2\"[label=\"Function Call\", style=\"dashed\"];\n")
	fmt.Fprintf(w, "\t\t\"legend_func1\" -> \"legend_func3\"[label=\"Function Call\", style=\"dashed\"];\n")
	fmt.Fprintf(w, "\t\t\"legend_func1\" -> \"legend_func4\"[label=\"Function Call\", style=\"dashed\"];\n")
	fmt.Fprintf(w, "\t\t\"legend_func1\" -> \"legend_func5\"[label=\"Function Call\", style=\"dashed\"];\n")
	fmt.Fprintln(w, "\t}")
}
