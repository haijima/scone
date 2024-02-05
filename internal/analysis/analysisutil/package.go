package analysisutil

import (
	"github.com/cockroachdb/errors"
	"golang.org/x/tools/go/packages"
)

// https://github.com/golang/tools/blob/master/go/analysis/analysistest/analysistest.go
func LoadPackages(dir string, patterns ...string) ([]*packages.Package, error) {
	mode := packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports |
		packages.NeedTypes | packages.NeedTypesSizes | packages.NeedSyntax | packages.NeedTypesInfo |
		packages.NeedDeps | packages.NeedModule
	cfg := &packages.Config{Mode: mode, Dir: dir}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}

	if len(pkgs) == 0 {
		return nil, errors.Newf("no packages matched %s", patterns)
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
