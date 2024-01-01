package main

import (
	"github.com/haijima/scone/internal"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() { singlechecker.Main(internal.Analyzer) }
