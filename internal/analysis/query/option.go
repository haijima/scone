package query

import "go/token"

type AnalyzeMode int

const (
	SsaMethod AnalyzeMode = iota
	SsaConst
	Ast
)

type Option struct {
	Mode                AnalyzeMode
	ExcludeQueries      []string
	ExcludePackages     []string
	ExcludePackagePaths []string
	ExcludeFiles        []string
	ExcludeFunctions    []string
	ExcludeQueryTypes   []string
	ExcludeTables       []string
	FilterQueries       []string
	FilterPackages      []string
	FilterPackagePaths  []string
	FilterFiles         []string
	FilterFunctions     []string
	FilterQueryTypes    []string
	FilterTables        []string
	AdditionalFuncs     []string

	queryCommentPositions []token.Pos
	isIgnoredFunc         func(pos token.Pos) bool
}
