# scone

[![CI](https://github.com/haijima/scone/actions/workflows/ci.yaml/badge.svg?branch=main)](https://github.com/haijima/scone/actions/workflows/ci.yaml)
[![Go Reference](https://pkg.go.dev/badge/github.com/haijima/scone.svg)](https://pkg.go.dev/github.com/haijima/scone)
[![Go report](https://goreportcard.com/badge/github.com/haijima/scone)](https://goreportcard.com/report/github.com/haijima/scone)

Analyze SQL in source code.

```
scone is a static analysis tool for SQL

Usage:
  scone [command]

Available Commands:
  callgraph   Generate a call graph
  crud        Show the CRUD operations for each endpoint
  genconf     Generate configuration file
  loop        Find N+1 queries
  query       List SQL queries
  table       List tables information from queries

Flags:
      --analyze-funcs <func pattern>@<argument index>   The names of functions to analyze additionally. format: <func pattern>@<argument index>
      --config filename                                 configuration filename
  -d, --dir string                                      The directory to analyze (default ".")
      --filter pattern                                  filter queries by pattern
  -h, --help                                            help for scone
      --no-color                                        disable colorized output
  -p, --pattern string                                  The pattern to analyze (default "./...")
  -q, --quiet                                           Silence all output
  -v, --verbose count                                   More output per occurrence. (e.g. -vvv)
  -V, --version                                         Print version information and quit

Use "scone [command] --help" for more information about a command.
```

## Installation

You can install scone using the following command:

``` sh
go install github.com/haijima/scone/cmd/scone@latest
```

MacOS users can install scone using [Homebrew](https://brew.sh/) (See also [haijima/homebrew-tap](http://github.com/haijima/homebrew-tap)):

``` sh
brew install haijima/tap/scone
```

or you can download binaries from [Releases](https://github.com/haijima/scone/releases).

## Examples

``` sh
scone query --dir path/to/project --filter="queryType!='SELECT' && 'users' in tables"
scone table --dir path/to/project
scone crud --dir path/to/project
scone loop --dir path/to/project
scone callgraph --dir path/to/project
```

## Commands and Options

### Commands

- `scone query`: List SQL queries
- `scone table`: List tables information from queries
- `scone callgraph`: Generate a call graph

### Options

#### Global Options

- `--config string` : Config file (default is `$XDG_CONFIG_HOME/.scone.yaml`)
- `--no-color`: Disable colorized output
- `-q, --quiet`: Quiet output
- `--verbosity int`: Verbosity level (default `0`)
- `--analyze-funcs <func pattern>@<argument index>`: The names of functions to analyze additionally. format: `<func pattern>@<argument index>`
- `--filter pattern`: Filter queries by pattern [for more information](#filter)
- `-d, --dir string`: The directory to analyze (default `.`)
- `-p, --pattern string`: The pattern to analyze (default `./...`)


#### Options for `scone query`

- `--cols columns`: The columns to show {`package`|`package-path`|`file`|`function`|`type`|`tables`|`hash`|`query`|`raw-query`}
- `--expand-query-group`: Expand query group
- `--format string`: The output format {`table`|`md`|`csv`|`tsv`|`simple`} (default `"table"`)
- `--full-package-path`: Show full package path
- `--no-header`: Hide header
- `--no-rownum`: Hide row number
- `--sort keys`: The sort keys {`file`|`function`|`type`|`tables`|`hash`} (default `[file]`)


#### Options for `scone table`

- `--collapse-phi`: Collapse phi queries
- `--summary`: Print summary only


#### Options for `scone crud`

- `--format string`: The output format {`table`|`md`|`csv`|`tsv`|`html`|`simple`} (default `"table"`)


#### Options for `scone loop`

- `--format string`: The output format {`table`|`md`|`csv`|`tsv`|`html`|`simple`} (default `"table"`)


#### Options for `scone callgraph`

- `--format string`: The output format {`dot`|`mermaid`|`text`} (default `"dot"`)


#### filter

Use [common expression language (CEL)](https://cel.dev/) to filter log lines.

The following variables are defined
- `pkgName`: string
- `pkgPath`: string
- `file`: string
- `func`: string
- `queryType`: string
- `tables`: list\[string\]
- `hash`: string

Example:
```
--filter "(queryType=='UPDATE' || queryType=='DELETE') && file=='main.go' && 'users' in tables"
```

- [CEL Spec](https://github.com/google/cel-spec/blob/master/doc/langdef.md)
    - [List of Standard Definitions](https://github.com/google/cel-spec/blob/master/doc/langdef.md#list-of-standard-definitions)
- [CEL Go implementation](https://github.com/google/cel-go)


## Comments

You can add comments to the SQL query by using the following format:

``` go
// scone:sql SELECT * FROM users
_, err := db.Query(UnAnalyzableQuery)

// scone:ignore
NonDbConnectFunction("SQL like string")
```

## License

This tool is licensed under the MIT License. See the [LICENSE](https://github.com/haijima/scone/blob/main/LICENSE) file for details.
