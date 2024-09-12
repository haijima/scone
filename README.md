# scone

Analyze SQL in source code.

```
scone is a static analysis tool for SQL

Usage:
  scone [command]

Available Commands:
  callgraph   Generate a call graph
  genconf     Generate configuration file
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