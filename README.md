# scone

Analyze SQL in source code.

```
scone is a static analysis tool for SQL

Usage:
  scone [command]

Available Commands:
  callgraph   Generate a call graph
  query       List SQL queries
  table       List tables information from queries

Flags:
      --analyze-funcs <package>#<function>#<argument index>   The names of functions to analyze additionally. format: <package>#<function>#<argument index>
      --config filename                                       configuration filename
  -d, --dir string                                            The directory to analyze (default ".")
      --exclude-files names                                   The names of files to exclude
      --exclude-functions names                               The names of functions to exclude
      --exclude-package-paths paths                           The paths of packages to exclude
      --exclude-packages names                                The names of packages to exclude
      --exclude-queries hashes                                The hashes of queries to exclude
      --exclude-query-types types                             The types of queries to exclude {select|insert|update|delete}
      --exclude-tables names                                  The names of tables to exclude
      --filter-files names                                    The names of files to filter
      --filter-functions names                                The names of functions to filter
      --filter-package-paths paths                            The paths of packages to filter
      --filter-packages names                                 The names of packages to filter
      --filter-queries hashes                                 The hashes of queries to filter
      --filter-query-types types                              The types of queries to filter {select|insert|update|delete}
      --filter-tables names                                   The names of tables to filter
  -h, --help                                                  help for scone
      --no-color                                              disable colorized output
  -p, --pattern string                                        The pattern to analyze (default "./...")
  -q, --quiet                                                 Silence all output
  -v, --verbose count                                         More output per occurrence. (e.g. -vvv)
  -V, --version                                               Print version information and quit

Use "scone [command] --help" for more information about a command.
```