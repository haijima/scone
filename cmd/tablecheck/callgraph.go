package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/haijima/scone/internal/tablecheck"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewCallGraphCmd(v *viper.Viper, _ afero.Fs) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "callgraph"
	cmd.Short = "Generate a call graph"
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		dir := v.GetString("dir")
		pattern := v.GetString("pattern")
		ignoreSelect := v.GetBool("ignore-select")
		opt := tablecheck.CallGraphOption{
			IgnoreSelect: ignoreSelect,
		}
		return run(cmd, dir, pattern, opt)
	}

	cmd.Flags().StringP("dir", "d", ".", "The directory to analyze")
	cmd.Flags().StringP("pattern", "p", "./...", "The pattern to analyze")
	cmd.Flags().String("format", "dot", "The output format {dot|mermaid|text}")
	cmd.Flags().Bool("ignore-select", false, "Ignore SELECT statements")
	_ = cmd.MarkFlagDirname("dir")

	return cmd
}

func run(cmd *cobra.Command, dir, pattern string, opt tablecheck.CallGraphOption) error {
	ssa, queryResult, err := tablecheck.Analyze(dir, pattern)
	if err != nil {
		return err
	}
	cg, err := tablecheck.CallGraph(ssa, queryResult, opt)
	if err != nil {
		return err
	}

	return printGraphviz(cmd.OutOrStdout(), cg)
}

func printGraphviz(w io.Writer, cg *tablecheck.CallGraphResult) error {
	fmt.Fprintln(w, "digraph {")
	fmt.Fprintln(w, "\trankdir=\"LR\"")

	for _, node := range cg.Nodes {
		// Print table node styles
		if node.Func == nil {
			gvn := &GraphvizNode{Name: node.Name, Shape: "box", Style: "solid", FontSize: "21"}

			maxKind := tablecheck.Select
			for _, edge := range node.In {
				if edge.SqlValue != nil {
					maxKind = max(maxKind, edge.SqlValue.Kind)
				}
			}
			switch maxKind {
			case tablecheck.Select:
				gvn.Style = "bold"
				gvn.Color = "blue"
			case tablecheck.Insert:
				gvn.Style = "bold"
				gvn.Color = "green"
			case tablecheck.Update:
				gvn.Color = "orange"
			case tablecheck.Delete:
				gvn.Color = "red"
			default:
			}
			fmt.Fprintln(w, gvn)

			fmt.Fprintf(w, "\t{rank = max; \"%s\"}\n", node.Name)
		} else if len(node.In) == 0 {
			fmt.Fprintf(w, "\t{rank = min; \"%s\"}\n", node.Name)
		}

		// Print edges
		for _, edge := range node.Out {
			gve := &GraphvizEdge{From: edge.Caller, To: edge.Callee, Style: "solid"}
			if edge.SqlValue != nil {
				switch edge.SqlValue.Kind {
				case tablecheck.Select:
					gve.Style = "dotted"
					//gve.Color = "blue"
				case tablecheck.Insert:
					gve.Color = "green"
					gve.Weight = 100
				case tablecheck.Update:
					gve.Style = "bold"
					gve.Color = "orange"
					gve.Weight = 100
				case tablecheck.Delete:
					gve.Style = "bold"
					gve.Color = "red"
					gve.Weight = 100
				default:
				}
			} else {
				gve.Style = "dashed"
				gve.Weight = 1000
			}
			fmt.Fprintln(w, gve)
		}
	}

	// Print cacheable func node styles
	selectOnlyNodes := make(map[string]tablecheck.QueryKind)
	for _, node := range tablecheck.TopologicalSort(cg.Nodes) {
		if node.Func == nil {
			continue
		}

		selectOnly := true
		kind := tablecheck.Unknown
		for _, edge := range node.Out {
			if edge.SqlValue != nil {
				if edge.SqlValue.Kind != tablecheck.Select {
					selectOnly = false
					break
				}

				tableNode := cg.Nodes[edge.Callee]
				for _, query := range tableNode.In {
					if query.SqlValue != nil {
						kind = max(kind, query.SqlValue.Kind)
					}
				}
			} else {
				if _, ok := selectOnlyNodes[edge.Callee]; !ok {
					selectOnly = false
					break
				}
				kind = max(kind, selectOnlyNodes[edge.Callee])
			}
		}
		if selectOnly && kind != tablecheck.Unknown {
			selectOnlyNodes[node.Name] = kind
		}
	}

	for n, k := range selectOnlyNodes {
		gvn := &GraphvizNode{Name: n, Style: "filled", FillColor: "darkseagreen1", FontSize: "21"}
		if k == tablecheck.Select {
			gvn.Color = "blue"
			gvn.FillColor = "lightblue1"
		} else if k == tablecheck.Insert {
			gvn.Color = "green"
			gvn.FillColor = "darkolivegreen1"
		} else if k == tablecheck.Update || k == tablecheck.Delete {
			gvn.Color = "orangered"
			gvn.FillColor = "rosybrown1"
		}
		fmt.Fprintln(w, gvn)
	}

	fmt.Fprintln(w, "}")
	return nil
}

type GraphvizNode struct {
	Name      string
	Shape     string
	Style     string
	Color     string
	FillColor string
	FontSize  string
}

func (n *GraphvizNode) String() string {
	attributes := make([]string, 0)
	if n.Shape != "" {
		attributes = append(attributes, fmt.Sprintf("shape=%s", n.Shape))
	}
	if n.Style != "" {
		attributes = append(attributes, fmt.Sprintf("style=%s", n.Style))
	}
	if n.Color != "" {
		attributes = append(attributes, fmt.Sprintf("color=%s", n.Color))
	}
	if n.FillColor != "" {
		attributes = append(attributes, fmt.Sprintf("fillcolor=%s", n.FillColor))
	}
	if n.FontSize != "" {
		attributes = append(attributes, fmt.Sprintf("fontsize=%q", n.FontSize))
	}
	if len(attributes) == 0 {
		return fmt.Sprintf("\t\"%s\";", n.Name)
	}
	return fmt.Sprintf("\t\"%s\"[%s];", n.Name, strings.Join(attributes, ", "))
}

type GraphvizEdge struct {
	From     string
	To       string
	Style    string
	Color    string
	PenWidth string
	Weight   int
}

func (e *GraphvizEdge) String() string {
	attributes := make([]string, 0)
	if e.Style != "" {
		attributes = append(attributes, fmt.Sprintf("style=%s", e.Style))
	}
	if e.Color != "" {
		attributes = append(attributes, fmt.Sprintf("color=%s", e.Color))
	}
	if e.PenWidth != "" {
		attributes = append(attributes, fmt.Sprintf("penwidth=%s", e.PenWidth))
	}
	if e.Weight != 0 {
		attributes = append(attributes, fmt.Sprintf("weight=%d", e.Weight))
	}
	if len(attributes) == 0 {
		return fmt.Sprintf("\t\"%s\" -> \"%s\";", e.From, e.To)
	}
	return fmt.Sprintf("\t\"%s\" -> \"%s\"[%s];", e.From, e.To, strings.Join(attributes, ", "))
}
