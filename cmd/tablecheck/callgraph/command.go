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
	"golang.org/x/exp/maps"
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
	cmd.Flags().StringSlice("exclude-queries", []string{}, "The `SHA1s` of queries to exclude")
	cmd.Flags().StringSlice("exclude-packages", []string{}, "The `names` of packages to exclude")
	cmd.Flags().StringSlice("exclude-package-paths", []string{}, "The `paths` of packages to exclude")
	cmd.Flags().StringSlice("exclude-files", []string{}, "The `names` of files to exclude")
	cmd.Flags().StringSlice("exclude-functions", []string{}, "The `names` of functions to exclude")
	cmd.Flags().StringSlice("exclude-query-types", []string{}, "The `types` of queries to exclude {select|insert|update|delete}")
	cmd.Flags().StringSlice("exclude-tables", []string{}, "The `names` of tables to exclude")
	cmd.Flags().StringSlice("filter-queries", []string{}, "The `SHA1s` of queries to filter")
	cmd.Flags().StringSlice("filter-packages", []string{}, "The `names` of packages to filter")
	cmd.Flags().StringSlice("filter-package-paths", []string{}, "The `paths` of packages to filter")
	cmd.Flags().StringSlice("filter-files", []string{}, "The `names` of files to filter")
	cmd.Flags().StringSlice("filter-functions", []string{}, "The `names` of functions to filter")
	cmd.Flags().StringSlice("filter-query-types", []string{}, "The `types` of queries to filter {select|insert|update|delete}")
	cmd.Flags().StringSlice("filter-tables", []string{}, "The `names` of tables to filter")
	cmd.Flags().String("mode", "ssa-method", "The query analyze `mode` {ssa-method|ssa-const|ast}")
	_ = cmd.MarkFlagDirname("dir")

	return cmd
}

func run(cmd *cobra.Command, v *viper.Viper) error {
	dir := v.GetString("dir")
	pattern := v.GetString("pattern")
	excludeQueries := v.GetStringSlice("exclude-queries")
	excludePackages := v.GetStringSlice("exclude-packages")
	excludePackagePaths := v.GetStringSlice("exclude-package-paths")
	excludeFiles := v.GetStringSlice("exclude-files")
	excludeFunctions := v.GetStringSlice("exclude-functions")
	excludeQueryTypes := v.GetStringSlice("exclude-query-types")
	excludeTables := v.GetStringSlice("exclude-tables")
	filterQueries := v.GetStringSlice("filter-queries")
	filterPackages := v.GetStringSlice("filter-packages")
	filterPackagePaths := v.GetStringSlice("filter-package-paths")
	filterFiles := v.GetStringSlice("filter-files")
	filterFunctions := v.GetStringSlice("filter-functions")
	filterQueryTypes := v.GetStringSlice("filter-query-types")
	filterTables := v.GetStringSlice("filter-tables")
	modeFlg := v.GetString("mode")

	var mode query.AnalyzeMode
	if modeFlg == "ssa-method" {
		mode = query.SsaMethod
	} else if modeFlg == "ssa-const" {
		mode = query.SsaConst
	} else if modeFlg == "ast" {
		mode = query.Ast
	} else {
		return fmt.Errorf("unknown mode: %s", modeFlg)
	}

	opt := &query.Option{
		Mode:                mode,
		ExcludeQueries:      excludeQueries,
		ExcludePackages:     excludePackages,
		ExcludePackagePaths: excludePackagePaths,
		ExcludeFiles:        excludeFiles,
		ExcludeFunctions:    excludeFunctions,
		ExcludeQueryTypes:   excludeQueryTypes,
		ExcludeTables:       excludeTables,
		FilterQueries:       filterQueries,
		FilterPackages:      filterPackages,
		FilterPackagePaths:  filterPackagePaths,
		FilterFiles:         filterFiles,
		FilterFunctions:     filterFunctions,
		FilterQueryTypes:    filterQueryTypes,
		FilterTables:        filterTables,
	}
	result, err := tablecheck.Analyze(dir, pattern, opt)
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
					gve.Weight = 1000
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

	fmt.Fprintln(w, "}")
	return nil
}
