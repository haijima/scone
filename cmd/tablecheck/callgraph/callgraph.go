package callgraph

import (
	"fmt"
	"io"

	"github.com/haijima/scone/internal/tablecheck"
	"github.com/haijima/scone/internal/tablecheck/callgraph"
	"github.com/haijima/scone/internal/tablecheck/query"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewCommand(v *viper.Viper, _ afero.Fs) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "callgraph"
	cmd.Short = "Generate a call graph"
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return run(cmd, v)
	}

	cmd.Flags().StringP("dir", "d", ".", "The directory to analyze")
	cmd.Flags().StringP("pattern", "p", "./...", "The pattern to analyze")
	cmd.Flags().String("format", "dot", "The output format {dot|mermaid|text}")
	cmd.Flags().Bool("ignore-select", false, "Ignore SELECT statements")
	_ = cmd.MarkFlagDirname("dir")

	return cmd
}

func run(cmd *cobra.Command, v *viper.Viper) error {
	dir := v.GetString("dir")
	pattern := v.GetString("pattern")
	ignoreSelect := v.GetBool("ignore-select")
	opt := callgraph.CallGraphOption{
		IgnoreSelect: ignoreSelect,
	}

	ssa, queryResult, err := tablecheck.Analyze(dir, pattern)
	if err != nil {
		return err
	}
	cg, err := callgraph.BuildCallGraph(ssa, queryResult, opt)
	if err != nil {
		return err
	}

	return printGraphviz(cmd.OutOrStdout(), cg)
}

func printGraphviz(w io.Writer, cg *callgraph.CallGraph) error {
	fmt.Fprintln(w, "digraph {")
	fmt.Fprintln(w, "\trankdir=\"LR\"")
	fmt.Fprintln(w)

	for _, node := range cg.Nodes {
		// Print edges
		for _, edge := range node.Out {
			gve := &GraphvizEdge{From: edge.Caller, To: edge.Callee, Style: "bold", Weight: 100}
			if edge.SqlValue != nil {
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
			} else {
				gve.Style = "dashed"
				gve.Weight = 1000
			}
			fmt.Fprintln(w, gve)
		}
	}

	fmt.Fprintln(w)

	// Print cacheable func and table node styles
	selectOnlyNodes := make(map[string]query.QueryKind)
	for _, node := range callgraph.TopologicalSort(cg.Nodes) {
		// table node
		if node.Func == nil {
			kind := query.Select
			for _, q := range node.In {
				if q.SqlValue != nil {
					kind = max(kind, q.SqlValue.Kind)
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
		gvn := &GraphvizNode{Name: n, Style: "bold,filled", FontSize: "21"}
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
			gvn.Shape = "box"
		}
		fmt.Fprintln(w, gvn)
	}

	fmt.Fprintln(w)

	// Print node positions
	minNodeNames := make([]string, 0)
	maxNodeNames := make([]string, 0)
	for _, node := range cg.Nodes {
		if node.Func == nil {
			maxNodeNames = append(maxNodeNames, node.Name)
		} else if len(node.In) == 0 {
			minNodeNames = append(minNodeNames, node.Name)
		}
	}
	fmt.Fprintln(w, GraphvizRank("min", minNodeNames...))
	fmt.Fprintln(w, GraphvizRank("max", maxNodeNames...))

	fmt.Fprintln(w, "}")
	return nil
}
