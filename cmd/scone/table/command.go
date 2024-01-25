package table

import (
	"fmt"
	"io"
	"slices"
	"strconv"

	mapset "github.com/deckarep/golang-set/v2"
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

	queries, tables, cgs, err := analysis.Analyze(dir, pattern, opt)
	if err != nil {
		return err
	}
	return printResult(cmd.OutOrStdout(), queries, tables, cgs, PrintOption{SummarizeOnly: summarizeOnly})
}

type PrintOption struct{ SummarizeOnly bool }

func printResult(w io.Writer, queries []*query.Query, tables mapset.Set[string], cgs []*callgraph.CallGraph, opt PrintOption) error {
	filterColumns := util.NewSetMap[string, string]()
	kindsMap := util.NewSetMap[string, query.QueryKind]()
	for _, q := range queries {
		for t, cols := range q.FilterColumnMap {
			filterColumns.Intersect(t, cols)
		}
		for _, t := range q.Tables {
			kindsMap.Add(t, q.Kind)
		}
	}
	connTables, collocationMap := clusterize(tables, queries, cgs)

	printSummary(w, tables, queries, connTables, filterColumns, kindsMap)
	if !opt.SummarizeOnly {
		for _, t := range mapset.Sorted(tables) {
			printTableResult(w, t, queries, connTables, collocationMap[t], filterColumns[t], kindsMap[t])
		}
	}
	return nil
}

func clusterize(tables mapset.Set[string], queries []*query.Query, cgs []*callgraph.CallGraph) ([]mapset.Set[string], map[string]mapset.Set[string]) {
	g := NewGraph()

	// Add tables as nodes
	for _, t := range tables.ToSlice() {
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
			callgraph.Walk(cg, r, func(n *callgraph.Node) bool {
				for _, edge := range n.Out {
					if edge.IsQuery() && edge.SqlValue.Kind != query.Select {
						tablesInTx = append(tablesInTx, edge.Callee)
					}
				}
				return false
			})
			// add edges between updated tables under the same root node
			util.PairCombinateFunc(tablesInTx, g.AddEdge)
		}
	}

	// extract tables used in the same query
	for _, q := range queries {
		util.PairCombinateFunc(q.Tables, g.AddEdge)
	}

	// find connected components
	connGraphs := g.FindConnectedComponents()                    // tables and functions
	connTables := make([]mapset.Set[string], 0, len(connGraphs)) // tables only
	for _, nodes := range connGraphs {
		ts := tables.Intersect(nodes)
		if ts.Cardinality() > 0 {
			connTables = append(connTables, ts)
		}
	}
	slices.SortFunc(connTables, func(a, b mapset.Set[string]) int { return b.Cardinality() - a.Cardinality() })

	// find collocation
	collocationMap := util.NewSetMap[string, string]()
	for _, t := range tables.ToSlice() {
		collocationMap.Add(t, t)
		for _, e := range g.edges[t] {
			if tables.Contains(e) {
				collocationMap.Add(t, e)
			}
		}
	}

	return connTables, collocationMap
}

func printSummary(w io.Writer, tables mapset.Set[string], queries []*query.Query, connTables []mapset.Set[string], filterColumns map[string]mapset.Set[string], kindsMap map[string]mapset.Set[query.QueryKind]) {
	fmt.Fprintf(w, "%s\n", color.CyanString("Summary"))
	fmt.Fprintf(w, "  %s : %d\n", color.MagentaString("queries       "), len(queries))
	fmt.Fprintf(w, "  %s : %d\n", color.MagentaString("tables        "), tables.Cardinality())
	fmt.Fprintf(w, "  %s :\n", color.MagentaString("cacheability  "))
	var hardCoded, readThrough, writeThrough []string
	for _, t := range mapset.Sorted(tables) {
		switch slices.Max(kindsMap[t].ToSlice()) {
		case query.Select:
			hardCoded = append(hardCoded, t)
		case query.Insert:
			readThrough = append(readThrough, t)
		case query.Delete, query.Replace, query.Update:
			writeThrough = append(writeThrough, t)
		default:
		}
	}
	fmt.Fprintf(w, "    %s : %d\t%q\n", color.BlueString("hard coded   "), len(hardCoded), hardCoded)
	fmt.Fprintf(w, "    %s : %d\t%q\n", color.GreenString("read-through "), len(readThrough), readThrough)
	fmt.Fprintf(w, "    %s : %d\t%q\n", color.YellowString("write-through"), len(writeThrough), writeThrough)
	fmt.Fprintf(w, "  %s : %d\n", color.MagentaString("table clusters"), len(connTables))
	for _, ts := range connTables {
		fmt.Fprintf(w, "    %q\n", mapset.Sorted(ts))
	}
	fmt.Fprintf(w, "  %s : found in %d table(s)\n", color.MagentaString("partition keys"), len(slices.DeleteFunc(maps.Values(filterColumns), func(m mapset.Set[string]) bool { return m.Cardinality() == 0 })))
	for t, cols := range filterColumns {
		if cols.Cardinality() > 0 {
			fmt.Fprintf(w, "    %s: %q\n", t, mapset.Sorted(cols))
		}
	}
	fmt.Fprintln(w)
}

func printTableResult(w io.Writer, table string, queries []*query.Query, connTables []mapset.Set[string], collocationTables mapset.Set[string], filterColumns mapset.Set[string], kinds mapset.Set[query.QueryKind]) {
	maxKind := slices.Max(kinds.ToSlice())
	switch maxKind {
	case query.Select:
		fmt.Fprintln(w, color.New(color.FgBlack, color.BgBlue).Sprintf(" %s ", table))
	case query.Insert:
		fmt.Fprintln(w, color.New(color.FgBlack, color.BgGreen).Sprintf(" %s ", table))
	case query.Delete:
		fmt.Fprintln(w, color.New(color.FgBlack, color.BgRed).Sprintf(" %s ", table))
	case query.Replace, query.Update:
		fmt.Fprintln(w, color.New(color.FgBlack, color.BgYellow).Sprintf(" %s ", table))
	default:
		fmt.Fprintln(w, color.New(color.FgBlack, color.BgWhite).Sprintf(" %s ", table))
	}

	fmt.Fprintf(w, "  %s\t:", color.MagentaString("query types"))
	for _, k := range mapset.Sorted(kinds) {
		if k != query.Unknown {
			fmt.Fprintf(w, " %s", k.ColoredString())
		} else {
			fmt.Fprintf(w, " %s", k.String())
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "  %s\t: ", color.MagentaString("cacheability"))
	switch maxKind {
	case query.Select:
		fmt.Fprintln(w, maxKind.Color("Hard coded"))
	case query.Insert:
		fmt.Fprintln(w, maxKind.Color("Read-through"))
	case query.Delete, query.Replace, query.Update:
		fmt.Fprintln(w, "Write-through")
	case query.Unknown:
		fmt.Fprintln(w, maxKind.Color("Unknown"))
	}

	fmt.Fprintf(w, "  %s\t: %q\n", color.MagentaString("collocation"), mapset.Sorted(collocationTables.Difference(mapset.NewSet(table))))

	for _, ts := range connTables {
		if ts.Contains(table) {
			fmt.Fprintf(w, "  %s\t: %q\n", color.MagentaString("cluster"), mapset.Sorted(ts))
		}
	}

	if filterColumns.Cardinality() > 0 {
		fmt.Fprintf(w, "  %s\t: %q\n", color.MagentaString("partition key"), mapset.Sorted(filterColumns))
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
		k := "?"
		if q.Kind != query.Unknown {
			k = q.Kind.Color(q.Kind.String()[:1])
		}
		p.AddRow([]string{"   ", strconv.Itoa(i + 1), q.FLC(), q.Func.Name(), k, q.Raw})
	}
	p.Print()
	fmt.Fprintln(w)
}
