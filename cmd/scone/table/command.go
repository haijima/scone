package table

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/fatih/color"
	"github.com/haijima/scone/internal/analysis"
	"github.com/haijima/scone/internal/analysis/callgraph"
	"github.com/haijima/scone/internal/analysis/query"
	"github.com/haijima/scone/internal/util"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/exp/maps"
)

func NewCommand(v *viper.Viper, _ afero.Fs) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "table"
	cmd.Short = "List tables information from queries"
	cmd.RunE = func(cmd *cobra.Command, args []string) error { return run(cmd, v) }

	cmd.Flags().Bool("summary", false, "Print summary only")

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
	additionalFuncs := v.GetStringSlice("analyze-funcs")
	summarizeOnly := v.GetBool("summary")

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
		AdditionalFuncs:     additionalFuncs,
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

	return printResult(cmd.OutOrStdout(), result, cgs, PrintOption{SummarizeOnly: summarizeOnly})
}

type PrintOption struct {
	SummarizeOnly bool
}

func printResult(w io.Writer, result []*analysis.QueryResultWithSSA, cgs []*callgraph.CallGraph, opt PrintOption) error {
	// Create table and query slice
	tm := make(map[string]bool)
	var tables []string
	queries := make([]*query.Query, 0)
	for _, res := range result {
		for _, q := range res.QueryResult.Queries {
			queries = append(queries, q)
			for _, t := range q.Tables {
				tm[t] = true
			}
		}
	}
	tables = maps.Keys(tm)
	slices.Sort(tables)

	connTables, collocationMap := clusterize(tables, queries, cgs)

	filterColumns := make(map[string][]string)
	for _, q := range queries {
		for t, cols := range q.FilterColumnMap {
			filterColumns[t] = util.Intersect(filterColumns[t], cols)
		}
	}

	maxKindMap := make(map[string]query.QueryKind)
	kindsMap := make(map[string]map[query.QueryKind]bool)
	for _, t := range tables {
		maxKindMap[t] = query.Unknown
		kindsMap[t] = make(map[query.QueryKind]bool)
		for _, cg := range cgs {
			n, ok := cg.Nodes[t]
			if ok {
				for _, q := range n.In {
					if q.SqlValue != nil {
						maxKindMap[t] = max(maxKindMap[t], q.SqlValue.Kind)
						kindsMap[t][q.SqlValue.Kind] = true
					}
				}
			}
		}
	}

	// Print result summary
	fmt.Fprintf(w, "%s\n", color.CyanString("Summary"))
	fmt.Fprintf(w, "  %s : %d\n", color.MagentaString("queries       "), len(queries))
	fmt.Fprintf(w, "  %s : %d\n", color.MagentaString("tables        "), len(tables))
	fmt.Fprintf(w, "  %s :\n", color.MagentaString("cacheability  "))
	hardCoded := make([]string, 0)
	readThrough := make([]string, 0)
	writeThrough := make([]string, 0)
	for t, kind := range maxKindMap {
		switch kind {
		case query.Select:
			hardCoded = append(hardCoded, t)
		case query.Insert:
			readThrough = append(readThrough, t)
		case query.Delete, query.Replace, query.Update:
			writeThrough = append(writeThrough, t)
		default:
		}
	}
	if len(hardCoded) > 0 {
		slices.Sort(hardCoded)
		fmt.Fprintf(w, "    %s : %d\t%q\n", color.BlueString("hard coded   "), len(hardCoded), hardCoded)
	}
	if len(readThrough) > 0 {
		slices.Sort(readThrough)
		fmt.Fprintf(w, "    %s : %d\t%q\n", color.GreenString("read-through "), len(readThrough), readThrough)
	}
	if len(writeThrough) > 0 {
		slices.Sort(writeThrough)
		fmt.Fprintf(w, "    %s : %d\t%q\n", color.YellowString("write-through"), len(writeThrough), writeThrough)
	}
	fmt.Fprintf(w, "  %s : %d\n", color.MagentaString("table clusters"), len(connTables))
	slices.SortFunc(connTables, func(a, b []string) int { return len(b) - len(a) })
	for _, tables := range connTables {
		slices.Sort(tables)
		fmt.Fprintf(w, "    %q\n", tables)
	}
	notEmptyFilteredColumns := make(map[string][]string)
	for k, v := range filterColumns {
		if len(v) > 0 {
			notEmptyFilteredColumns[k] = v
		}
	}
	fmt.Fprintf(w, "  %s : found in %d table(s)\n", color.MagentaString("partition keys"), len(notEmptyFilteredColumns))
	for t, cols := range notEmptyFilteredColumns {
		fmt.Fprintf(w, "    %s: %q\n", t, cols)
	}
	fmt.Fprintln(w)

	if opt.SummarizeOnly {
		return nil
	}

	for _, t := range tables {
		var colorFunc func(format string, a ...interface{}) string
		switch maxKindMap[t] {
		case query.Select:
			colorFunc = color.New(color.FgBlack, color.BgBlue).SprintfFunc()
		case query.Insert:
			colorFunc = color.New(color.FgBlack, color.BgGreen).SprintfFunc()
		case query.Delete:
			colorFunc = color.New(color.FgBlack, color.BgRed).SprintfFunc()
		case query.Replace, query.Update:
			colorFunc = color.New(color.FgBlack, color.BgYellow).SprintfFunc()
		default:
			colorFunc = color.New(color.FgBlack, color.BgWhite).SprintfFunc()
		}
		fmt.Fprintln(w, colorFunc(" %s ", t))

		fmt.Fprintf(w, "  %s\t:", color.MagentaString("query types"))
		ks := maps.Keys(kindsMap[t])
		slices.Sort(ks)
		for _, k := range ks {
			if kindsMap[t][k] {
				switch k {
				case query.Select:
					fmt.Fprintf(w, " %s", color.BlueString(k.String()))
				case query.Insert:
					fmt.Fprintf(w, " %s", color.GreenString(k.String()))
				case query.Delete:
					fmt.Fprintf(w, " %s", color.RedString(k.String()))
				case query.Replace, query.Update:
					fmt.Fprintf(w, " %s", color.YellowString(k.String()))
				default:
					fmt.Fprintf(w, " %s", k.String())
				}
			}
		}
		fmt.Fprintln(w)

		fmt.Fprintf(w, "  %s\t: ", color.MagentaString("cacheability"))
		switch maxKindMap[t] {
		case query.Select:
			fmt.Fprintln(w, color.BlueString("Hard coded"))
		case query.Insert:
			fmt.Fprintln(w, color.GreenString("Read-through"))
		case query.Delete, query.Replace, query.Update:
			fmt.Fprintln(w, "Write-through")
		case query.Unknown:
			fmt.Fprintln(w, color.HiBlackString("Unknown"))
		}

		fmt.Fprintf(w, "  %s\t: %q\n", color.MagentaString("collocation"), collocationMap[t])

		for _, ts := range connTables {
			if slices.Contains(ts, t) {
				fmt.Fprintf(w, "  %s\t: %q\n", color.MagentaString("cluster"), ts)
			}
		}

		if len(filterColumns[t]) > 0 {
			fmt.Fprintf(w, "  %s\t: %q\n", color.MagentaString("partition key"), filterColumns[t])
			fmt.Fprintf(w, "  \t\t  %s\n", color.HiBlackString("It is likely that this table will always be filtered by these column(s)"))
		}

		qs := make([]*query.Query, 0)
		for _, q := range queries {
			if slices.Contains(q.Tables, t) {
				qs = append(qs, q)
			}
		}
		fmt.Fprintf(w, "  %s\t: %d\n", color.MagentaString("queries"), len(qs))

		table := tablewriter.NewWriter(w)
		table.SetColWidth(tablewriter.MAX_ROW_WIDTH * 5)
		table.SetAutoWrapText(true)
		table.SetAutoFormatHeaders(true)
		table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
		table.SetAlignment(tablewriter.ALIGN_LEFT)
		table.SetColumnAlignment([]int{tablewriter.ALIGN_LEFT, tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_LEFT, tablewriter.ALIGN_LEFT, tablewriter.ALIGN_LEFT, tablewriter.ALIGN_LEFT})
		table.SetCenterSeparator("")
		table.SetColumnSeparator("")
		table.SetRowSeparator("")
		table.SetHeaderLine(false)
		table.SetBorder(false)
		//table.SetTablePadding("\t") // pad with tabs
		table.SetTablePadding(" ")
		table.SetNoWhiteSpace(true)
		table.SetRowLine(false)

		table.SetHeader([]string{"", "#", "file", "function", "t", "query"})
		for i, q := range qs {
			file := fmt.Sprintf("%s:%d:%d", filepath.Base(q.Position().Filename), q.Position().Line, q.Position().Column)
			k := ""
			switch q.Kind {
			case query.Select:
				k = color.BlueString(q.Kind.String()[:1])
			case query.Insert:
				k = color.GreenString(q.Kind.String()[:1])
			case query.Delete:
				k = color.RedString(q.Kind.String()[:1])
			case query.Replace, query.Update:
				k = color.YellowString(q.Kind.String()[:1])
			default:
				k = color.WhiteString(q.Kind.String()[:1])
			}
			table.Append([]string{"   ", strconv.Itoa(i + 1), file, q.Func.Name(), k, q.Raw})
		}
		table.Render()
		fmt.Fprintln(w)
	}

	return nil
}

func clusterize(tables []string, queries []*query.Query, cgs []*callgraph.CallGraph) ([][]string, map[string][]string) {
	g := NewGraph()
	// Add tables as nodes
	for _, t := range tables {
		g.AddNode(t)
	}

	// Extract tables updated in the same transaction
	for _, cg := range cgs {
		for _, r := range cg.Nodes {
			if r.IsNotRoot() {
				continue
			}
			// walk from the root nodes
			queue := []string{r.Name}
			tablesInTx := make([]string, 0)
			for len(queue) > 0 {
				callee := queue[0]
				queue = queue[1:]
				if n, exists := cg.Nodes[callee]; exists {
					if n.IsTable() {
						tablesInTx = append(tablesInTx, callee)
					} else {
						for _, e := range n.Out {
							if e.IsFuncCall() || e.SqlValue.Kind != query.Select { // ignore select queries
								queue = append(queue, e.Callee)
							}
						}
					}
				}
			}
			// add edges between updated tables under the same root node
			for _, t1 := range tablesInTx {
				for _, t2 := range tablesInTx {
					if t1 < t2 {
						g.AddEdge(t1, t2)
					}
				}
			}
		}
	}

	// extract tables used in the same query
	for _, q := range queries {
		for _, t1 := range q.Tables {
			for _, t2 := range q.Tables {
				if t1 < t2 {
					g.AddEdge(t1, t2)
				}
			}
		}
	}

	// find connected components
	connGraphs := g.FindConnectedComponents()          // tables and functions
	connTables := make([][]string, 0, len(connGraphs)) // tables only
	for _, nodes := range connGraphs {
		ts := make([]string, 0, len(nodes))
		for _, node := range nodes {
			for _, cg := range cgs {
				if n, exists := cg.Nodes[node]; exists && n.IsTable() {
					ts = append(ts, node)
					break // next node
				}
			}
		}
		if len(ts) > 0 {
			connTables = append(connTables, ts)
		}
	}

	// find collocation
	collocationMap := make(map[string][]string)
	for _, t := range tables {
		for _, e := range g.edges[t] {
			if !slices.Contains(collocationMap[t], e) {
				for _, cg := range cgs {
					if _, exists := cg.Nodes[e]; exists && cg.Nodes[e].IsTable() {
						collocationMap[t] = append(collocationMap[t], e)
					}
				}
			}
		}
	}
	for t, cols := range collocationMap {
		slices.Sort(cols)
		collocationMap[t] = cols
	}

	return connTables, collocationMap
}
