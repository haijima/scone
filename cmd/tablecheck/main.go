package main

import (
	"github.com/haijima/scone/internal"
	"golang.org/x/tools/go/analysis/unitchecker"
)

func main() { unitchecker.Main(internal.Analyzer) }
