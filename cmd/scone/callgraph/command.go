package callgraph

import (
	"fmt"
	"io"

	"github.com/haijima/scone/cmd/scone/option"
	"github.com/haijima/scone/internal/analysis"
	"github.com/haijima/scone/internal/analysis/callgraph"
	"github.com/haijima/scone/internal/analysis/query"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/exp/maps"
)

func NewCommand(v *viper.Viper, _ afero.Fs) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "callgraph"
	cmd.Short = "Generate a call graph"
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return run(cmd, v)
	}

	cmd.Flags().String("format", "dot", "The output format {dot|mermaid|text}")
	option.SetQueryOptionFlags(cmd)

	return cmd
}

func run(cmd *cobra.Command, v *viper.Viper) error {
	dir := v.GetString("dir")
	pattern := v.GetString("pattern")
	opt, err := option.QueryOptionFromViper(v)
	if err != nil {
		return err
	}

	result, err := analysis.Analyze(dir, pattern, opt)
	if err != nil {
		return err
	}

	cgs := make([]*callgraph.CallGraph, 0, len(result))
	for _, res := range result {
		cg, err := callgraph.BuildCallGraph(res.SSA, res.QueryResult)
		if err != nil {
			return err
		}
		cgs = append(cgs, cg)
	}

	return printGraphviz(cmd.OutOrStdout(), cgs)
}

func printGraphviz(w io.Writer, cgs []*callgraph.CallGraph) error {
	fmt.Fprintln(w, "digraph {")
	fmt.Fprintln(w, "\trankdir=\"LR\"")
	fmt.Fprintln(w)

	showLegend(w)

	fmt.Fprintln(w, "\tsubgraph callgraph {")
	for _, cg := range cgs {
		for _, node := range cg.Nodes {
			// Print edges
			for _, edge := range node.Out {
				if edge.SqlValue != nil {
					gve := &GraphvizEdge{From: fmt.Sprintf("%s%s", cg.Package.Path(), edge.Caller), To: edge.Callee, Style: "bold", Weight: 100}
					fmt.Fprintf(w, "\t\"%s%s\"[label=\"%s\"]\n", cg.Package.Path(), edge.Caller, edge.Caller)
					switch edge.SqlValue.Kind {
					case query.Select:
						gve.Style = "dotted"
						gve.Weight = 1
					case query.Insert:
						gve.Color = "green"
					case query.Update:
						gve.Color = "orange"
					case query.Delete:
						gve.Color = "red"
					default:
					}
					fmt.Fprintln(w, gve)
				} else {
					gve := &GraphvizEdge{From: fmt.Sprintf("%s%s", cg.Package.Path(), edge.Caller), To: fmt.Sprintf("%s%s", cg.Package.Path(), edge.Callee), Style: "bold", Weight: 100}
					fmt.Fprintf(w, "\t\"%s%s\"[label=\"%s\"]\n", cg.Package.Path(), edge.Caller, edge.Caller)
					fmt.Fprintf(w, "\t\"%s%s\"[label=\"%s\"]\n", cg.Package.Path(), edge.Callee, edge.Callee)
					gve.Style = "dashed"
					fmt.Fprintln(w, gve)
				}
			}
		}

		fmt.Fprintln(w)

	}

	fmt.Fprintln(w)

	// Print cacheable func and table node styles
	selectOnlyNodes := make(map[string]query.QueryKind)
	for _, cg := range cgs {
		for _, node := range callgraph.TopologicalSort(cg.Nodes) {
			// table node
			if node.Func == nil {
				kind := query.Select
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
			kind := query.Unknown
			for _, edge := range node.Out {
				if edge.SqlValue != nil {
					selectOnly = selectOnly && edge.SqlValue.Kind == query.Select
				} else {
					_, ok := selectOnlyNodes[edge.Callee]
					selectOnly = selectOnly && ok
				}
				kind = max(kind, selectOnlyNodes[edge.Callee])
			}
			if selectOnly && kind != query.Unknown {
				selectOnlyNodes[node.Name] = kind
			}
		}

		for n, k := range selectOnlyNodes {
			gvn := &GraphvizNode{Name: fmt.Sprintf("%s%s", cg.Package.Path(), n), Style: "bold,filled", FontSize: "21"}
			if k == query.Select {
				gvn.Color = "blue"
				gvn.FillColor = "lightblue1"
			} else if k == query.Insert {
				gvn.Color = "green"
				gvn.FillColor = "darkolivegreen1"
			} else if k == query.Update {
				gvn.Style = "solid"
				gvn.Color = "orange"
			} else if k == query.Delete {
				gvn.Style = "solid"
				gvn.Color = "red"
			}
			if cg.Nodes[n].Func == nil {
				gvn.Name = n
				gvn.Shape = "box"
			}
			fmt.Fprintln(w, gvn)
		}

		// Reset
		selectOnlyNodes = make(map[string]query.QueryKind)
	}

	fmt.Fprintln(w)

	// Print node positions
	minNodeNames := make(map[string]bool)
	maxNodeNames := make(map[string]bool)
	for _, cg := range cgs {
		for _, node := range cg.Nodes {
			if node.Func == nil {
				maxNodeNames[node.Name] = true
			} else if len(node.In) == 0 {
				minNodeNames[fmt.Sprintf("%s%s", cg.Package.Path(), node.Name)] = true
			}
		}
	}
	fmt.Fprintln(w, GraphvizRank("min", maps.Keys(minNodeNames)...))
	fmt.Fprintln(w, GraphvizRank("max", maps.Keys(maxNodeNames)...))

	fmt.Fprintln(w, "\t}")
	fmt.Fprintln(w, "}")
	return nil
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
