package main

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/haijima/scone/internal/analysis"
	"github.com/haijima/scone/internal/analysis/analysisutil"
	"github.com/haijima/scone/internal/analysis/query"
	internalio "github.com/haijima/scone/internal/io"
	"github.com/haijima/scone/internal/util"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewQueryCommand(v *viper.Viper, _ afero.Fs) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "query"
	cmd.Short = "List SQL queries"
	cmd.RunE = func(cmd *cobra.Command, args []string) error { return runQuery(cmd, v) }

	cmd.Flags().String("format", "table", "The output format {table|md|csv|tsv|simple}")
	cmd.Flags().StringSlice("sort", []string{"file"}, "The sort `keys` {file|function|type|table|sha1}")
	cmd.Flags().StringSlice("cols", []string{}, "The `columns` to show {"+strings.Join(headerColumns, "|")+"}")
	cmd.Flags().Bool("no-header", false, "Hide header")
	cmd.Flags().Bool("no-rownum", false, "Hide row number")
	cmd.Flags().Bool("full-package-path", false, "Show full package path")
	cmd.Flags().Bool("expand-query-group", false, "Expand query group")
	SetQueryOptionFlags(cmd)

	return cmd
}

var headerColumns = []string{"package", "package-path", "file", "function", "type", "table", "related-tables", "sha1", "query", "raw-query"}
var defaultHeaderIndex = []int{0, 1, 2, 3, 4, 5, 6, 7, 8}

func runQuery(cmd *cobra.Command, v *viper.Viper) error {
	dir := v.GetString("dir")
	pattern := v.GetString("pattern")
	format := v.GetString("format")
	cols := v.GetStringSlice("cols")
	noHeader := v.GetBool("no-header")
	noRowNum := v.GetBool("no-rownum")
	sortKeys := v.GetStringSlice("sort")
	expandQueryGroup := v.GetBool("expand-query-group")
	showFullPackagePath := v.GetBool("full-package-path")
	opt, err := QueryOptionFromViper(v)
	if err != nil {
		return err
	}

	queryGroups, _, _, err := analysis.Analyze(dir, pattern, opt)
	if err != nil {
		return err
	}

	if !mapset.NewSet(sortKeys...).IsSubset(mapset.NewSet("file", "function", "type", "table", "sha1")) {
		return errors.Newf("unknown sort key: %s", mapset.NewSet(sortKeys...).Difference(mapset.NewSet("file", "function", "type", "table", "sha1")).ToSlice())
	}
	if !slices.Contains(sortKeys, "file") {
		sortKeys = append(sortKeys, "file")
	}
	slices.SortFunc(queryGroups, sortQuery(sortKeys))

	printOpt := &PrintQueryOption{Cols: defaultHeaderIndex, NoHeader: noHeader, NoRowNum: noRowNum, ExpandQueryGroup: expandQueryGroup, ShowFullPackagePath: showFullPackagePath}
	if len(cols) > 0 {
		printOpt.Cols = make([]int, 0, len(cols))
		for _, col := range cols {
			i := slices.Index(headerColumns, col)
			if i == -1 {
				return errors.Newf("unknown columns: %s", col)
			}
			printOpt.Cols = append(printOpt.Cols, i)
		}
	}
	pkgs := mapset.NewSet[string]()
	for _, qg := range queryGroups {
		for _, q := range qg.List {
			pkgs.Add(q.Package.Pkg.Path())
		}
	}
	printOpt.pkgBasePath = util.FindCommonPrefix(pkgs.ToSlice())

	var p internalio.TablePrinter
	if format == "table" {
		maxWidth := tablewriter.MAX_ROW_WIDTH * 4
		includeRawQuery := printOpt.Cols != nil && slices.Contains(printOpt.Cols, slices.Index(headerColumns, "raw-query"))
		p = internalio.NewTablePrinter(cmd.OutOrStdout(), maxWidth, includeRawQuery)
	} else if format == "md" {
		p = internalio.NewMarkdownPrinter(cmd.OutOrStdout())
	} else if format == "simple" {
		p = internalio.NewSimplePrinter(cmd.OutOrStdout(), tablewriter.MAX_ROW_WIDTH, false)
	} else if format == "csv" {
		p = internalio.NewCSVPrinter(cmd.OutOrStdout())
	} else if format == "tsv" {
		p = internalio.NewTSVPrinter(cmd.OutOrStdout())
	} else {
		return errors.Newf("unknown format: %s", format)
	}

	if !printOpt.NoHeader {
		p.SetHeader(makeHeader(printOpt))
	}
	for i, qg := range queryGroups {
		phi := ""
		for j, q := range qg.List {
			if len(qg.List) > 1 {
				if expandQueryGroup {
					phi = fmt.Sprintf("P%d", j+1)
				} else {
					phi = "P"
				}
			}
			r := append([]string{phi}, row(q, printOpt)...)
			if !printOpt.NoRowNum {
				r = append([]string{strconv.Itoa(i + 1)}, r...)
			}
			p.AddRow(r)
			if !expandQueryGroup {
				break // only print the first query in the group
			}
		}
	}
	return p.Print()
}

func sortQuery(sortKeys []string) func(a, b *query.QueryGroup) int {
	return func(aa, bb *query.QueryGroup) int {
		a := aa.List[0]
		b := bb.List[0]
		for _, sortKey := range sortKeys {
			if sortKey == "function" && a.Func.Name() != b.Func.Name() {
				return strings.Compare(a.Func.Name(), b.Func.Name())
			} else if sortKey == "type" && a.Kind != b.Kind {
				return int(a.Kind) - int(b.Kind)
			} else if sortKey == "table" && a.MainTable != b.MainTable {
				return strings.Compare(a.MainTable, b.MainTable)
			} else if sortKey == "sha1" && a.Sha() != b.Sha() {
				return strings.Compare(a.Sha(), b.Sha())
			} else if sortKey == "file" {
				if a.Package.Pkg.Path() != b.Package.Pkg.Path() {
					return strings.Compare(a.Package.Pkg.Path(), b.Package.Pkg.Path())
				} else if a.Position().Filename != b.Position().Filename {
					return strings.Compare(a.Position().Filename, b.Position().Filename)
				} else if a.Position().Line != b.Position().Line {
					return a.Position().Line - b.Position().Line
				} else if a.Position().Column != b.Position().Column {
					return a.Position().Column - b.Position().Column
				}
			}
		}
		return 0
	}
}

type PrintQueryOption struct {
	Cols                []int
	NoHeader            bool
	NoRowNum            bool
	ExpandQueryGroup    bool
	ShowFullPackagePath bool
	pkgBasePath         string
}

var pathDirRegex = regexp.MustCompile(`([^/]+)/`)

func (opt *PrintQueryOption) ShortenPackagePath(path string) string {
	if !opt.ShowFullPackagePath && opt.pkgBasePath != "" && strings.HasPrefix(path, opt.pkgBasePath) {
		path = strings.TrimPrefix(path, opt.pkgBasePath)
		return fmt.Sprintf("%s%s", pathDirRegex.ReplaceAllStringFunc(opt.pkgBasePath, func(m string) string { return m[:1] + "/" }), path)
	}
	return path
}

func makeHeader(opt *PrintQueryOption) []string {
	header := make([]string, 0)
	if !opt.NoRowNum {
		header = append(header, "#")
	}
	header = append(header, "*")
	for _, col := range opt.Cols {
		header = append(header, strings.ReplaceAll(headerColumns[col], "-", " "))
	}
	return header
}

func row(q *query.Query, opt *PrintQueryOption) []string {
	var tables string
	if len(q.Tables) > 0 {
		tables = strings.Join(q.Tables[1:], ", ")
	}

	fullRow := []string{
		q.Package.Pkg.Name(),
		opt.ShortenPackagePath(q.Package.Pkg.Path()),
		analysisutil.FLC(q.Position()),
		q.Func.Name(),
		q.Kind.ColoredString(),
		q.MainTable,
		tables,
		q.Sha(),
		q.String(),
		q.Raw,
	}
	res := make([]string, 0, len(opt.Cols))
	for _, col := range opt.Cols {
		res = append(res, fullRow[col])
	}
	return res
}
