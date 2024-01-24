package table

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/fatih/color"
	"github.com/haijima/scone/cmd/scone/option"
	"github.com/haijima/scone/internal/analysis"
	"github.com/haijima/scone/internal/analysis/callgraph"
	"github.com/haijima/scone/internal/analysis/query"
	internalio "github.com/haijima/scone/internal/io"
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
	option.SetQueryOptionFlags(cmd)

	_ = cmd.MarkFlagDirname("dir")

	return cmd
}

func run(cmd *cobra.Command, v *viper.Viper) error {
	dir := v.GetString("dir")
	pattern := v.GetString("pattern")
	summarizeOnly := v.GetBool("summary")
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

	queries, tables := analysis.GetQueriesAndTablesFromResult(result)
	return printResult(cmd.OutOrStdout(), queries, tables, cgs, PrintOption{SummarizeOnly: summarizeOnly})
}

type PrintOption struct {
	SummarizeOnly bool
}

func printResult(w io.Writer, queries []*query.Query, tables []string, cgs []*callgraph.CallGraph, opt PrintOption) error {
	filterColumns := make(map[string][]string)
	maxKindMap := make(map[string]query.QueryKind)
	kindsMap := make(map[string]map[query.QueryKind]bool)
	for _, q := range queries {
		for t, cols := range q.FilterColumnMap {
			filterColumns[t] = util.Intersect(filterColumns[t], cols)
		}
		for _, t := range q.Tables {
			maxKindMap[t] = max(maxKindMap[t], q.Kind)
			if _, exists := kindsMap[t]; !exists {
				kindsMap[t] = make(map[query.QueryKind]bool)
			}
			kindsMap[t][q.Kind] = true
		}
	}
	connTables, collocationMap := clusterize(tables, queries, cgs)

	printSummary(w, queries, tables, connTables, filterColumns, maxKindMap)
	if !opt.SummarizeOnly {
		for _, t := range tables {
			printTableResult(w, t, queries, maxKindMap, kindsMap, collocationMap, connTables, filterColumns)
		}
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
			tablesInTx := make([]string, 0)
			callgraph.Walk(cg, r, func(edge *callgraph.Edge) bool {
				if edge.IsQuery() {
					if edge.SqlValue.Kind != query.Select {
						tablesInTx = append(tablesInTx, edge.Callee)
					}
					return true
				}
				return false
			})
			// add edges between updated tables under the same root node
			for _, p := range util.PairCombinate(tablesInTx) {
				g.AddEdge(p.L, p.R)
			}
		}
	}

	// extract tables used in the same query
	for _, q := range queries {
		for _, p := range util.PairCombinate(q.Tables) {
			g.AddEdge(p.L, p.R)
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
	slices.SortFunc(connTables, func(a, b []string) int { return len(b) - len(a) })
	for _, ts := range connTables {
		slices.Sort(ts)
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

func printSummary(w io.Writer, queries []*query.Query, tables []string, connTables [][]string, filterColumns map[string][]string, maxKindMap map[string]query.QueryKind) {
	fmt.Fprintf(w, "%s\n", color.CyanString("Summary"))
	fmt.Fprintf(w, "  %s : %d\n", color.MagentaString("queries       "), len(queries))
	fmt.Fprintf(w, "  %s : %d\n", color.MagentaString("tables        "), len(tables))
	fmt.Fprintf(w, "  %s :\n", color.MagentaString("cacheability  "))
	var hardCoded, readThrough, writeThrough []string
	for _, t := range tables {
		switch maxKindMap[t] {
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
		fmt.Fprintf(w, "    %s : %d\t%q\n", color.BlueString("hard coded   "), len(hardCoded), hardCoded)
	}
	if len(readThrough) > 0 {
		fmt.Fprintf(w, "    %s : %d\t%q\n", color.GreenString("read-through "), len(readThrough), readThrough)
	}
	if len(writeThrough) > 0 {
		fmt.Fprintf(w, "    %s : %d\t%q\n", color.YellowString("write-through"), len(writeThrough), writeThrough)
	}
	fmt.Fprintf(w, "  %s : %d\n", color.MagentaString("table clusters"), len(connTables))
	for _, ts := range connTables {
		fmt.Fprintf(w, "    %q\n", ts)
	}
	fmt.Fprintf(w, "  %s : found in %d table(s)\n", color.MagentaString("partition keys"), len(slices.DeleteFunc(maps.Values(filterColumns), util.Empty)))
	for t, cols := range filterColumns {
		if len(cols) > 0 {
			fmt.Fprintf(w, "    %s: %q\n", t, cols)
		}
	}
	fmt.Fprintln(w)
}

func printTableResult(w io.Writer, table string, queries []*query.Query, maxKindMap map[string]query.QueryKind, kindsMap map[string]map[query.QueryKind]bool, collocationMap map[string][]string, connTables [][]string, filterColumns map[string][]string) {
	var colorFunc func(format string, a ...interface{}) string
	switch maxKindMap[table] {
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
	fmt.Fprintln(w, colorFunc(" %s ", table))

	fmt.Fprintf(w, "  %s\t:", color.MagentaString("query types"))
	ks := maps.Keys(kindsMap[table])
	slices.Sort(ks)
	for _, k := range ks {
		if kindsMap[table][k] {
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
	switch maxKindMap[table] {
	case query.Select:
		fmt.Fprintln(w, color.BlueString("Hard coded"))
	case query.Insert:
		fmt.Fprintln(w, color.GreenString("Read-through"))
	case query.Delete, query.Replace, query.Update:
		fmt.Fprintln(w, "Write-through")
	case query.Unknown:
		fmt.Fprintln(w, color.HiBlackString("Unknown"))
	}

	fmt.Fprintf(w, "  %s\t: %q\n", color.MagentaString("collocation"), collocationMap[table])

	for _, ts := range connTables {
		if slices.Contains(ts, table) {
			fmt.Fprintf(w, "  %s\t: %q\n", color.MagentaString("cluster"), ts)
		}
	}

	if len(filterColumns[table]) > 0 {
		fmt.Fprintf(w, "  %s\t: %q\n", color.MagentaString("partition key"), filterColumns[table])
		fmt.Fprintf(w, "  \t\t  %s\n", color.HiBlackString("It is likely that this table will always be filtered by these column(s)"))
	}

	qs := make([]*query.Query, 0)
	for _, q := range queries {
		if slices.Contains(q.Tables, table) {
			qs = append(qs, q)
		}
	}
	fmt.Fprintf(w, "  %s\t: %d\n", color.MagentaString("queries"), len(qs))

	p := internalio.NewSimplePrinter(w, tablewriter.MAX_ROW_WIDTH*5, true)
	p.SetHeader([]string{"", "#", "file", "function", "t", "query"})
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
			k = q.Kind.String()[:1]
		}
		p.AddRow([]string{"   ", strconv.Itoa(i + 1), file, q.Func.Name(), k, q.Raw})
	}
	p.Print()
	fmt.Fprintln(w)
}
