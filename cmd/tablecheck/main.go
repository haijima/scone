package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/fatih/color"
	"github.com/haijima/cobrax"
	"github.com/haijima/scone/internal/tablecheck"
	"github.com/mattn/go-colorable"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/tools/go/packages"
)

func main() {
	v := viper.NewWithOptions(viper.WithLogger(slog.Default()))
	fs := afero.NewOsFs()
	v.SetFs(fs)
	rootCmd := cobrax.NewRoot(v)
	rootCmd.Use = "tablecheck"
	rootCmd.Short = "tablecheck is a static analysis tool for SQL"
	rootCmd.Version = cobrax.VersionFunc()
	rootCmd.SetGlobalNormalizationFunc(cobrax.SnakeToKebab)
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Colorization settings
		color.NoColor = color.NoColor || v.GetBool("no-color")
		// Set Logger
		l := slog.New(slog.NewJSONHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: cobrax.VerbosityLevel(v)}))
		slog.SetDefault(l)
		cobrax.SetLogger(l)

		return cobrax.RootPersistentPreRunE(cmd, v, fs, args)
	}
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		dir := v.GetString("dir")
		pattern := v.GetString("pattern")
		return run(dir, pattern)
	}

	rootCmd.Flags().StringP("dir", "d", ".", "The directory to analyze")
	rootCmd.Flags().StringP("pattern", "p", "./...", "The pattern to analyze")
	_ = v.BindPFlags(rootCmd.Flags())

	rootCmd.SetOut(colorable.NewColorableStdout())
	rootCmd.SetErr(colorable.NewColorableStderr())
	if err := rootCmd.Execute(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run(dir, pattern string) error {
	pkgs, err := loadPackages(dir, pattern)
	if err != nil {
		return err
	}

	ssa, err := tablecheck.BuildSSA(pkgs[0])
	if err != nil {
		return err
	}

	queryResult, err := tablecheck.ExtractQuery(ssa)
	if err != nil {
		return err
	}

	_, err = tablecheck.CallGraph(ssa, queryResult)
	if err != nil {
		return err
	}

	return nil
}

// https://github.com/golang/tools/blob/master/go/analysis/analysistest/analysistest.go
func loadPackages(dir string, patterns ...string) ([]*packages.Package, error) {
	//env := []string{"GOPATH=" + dir, "GO111MODULE=off"} // GOPATH mode
	//
	//// Undocumented module mode. Will be replaced by something better.
	//if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
	//	env = []string{"GO111MODULE=on", "GOPROXY=off"} // module mode
	//}

	mode := packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports |
		packages.NeedTypes | packages.NeedTypesSizes | packages.NeedSyntax | packages.NeedTypesInfo |
		packages.NeedDeps | packages.NeedModule
	cfg := &packages.Config{
		Mode: mode,
		Dir:  dir,
		//Env:   append(os.Environ(), env...),
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages matched %s", patterns)
	}

	errs := make([]error, 0)
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		for _, err := range pkg.Errors {
			errs = append(errs, err)
		}
	})
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return pkgs, nil
}
