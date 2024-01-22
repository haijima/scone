package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/haijima/cobrax"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-colorable"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var v *viper.Viper
var rootCmd *cobra.Command

func init() {
	cobra.OnInitialize(func() {
		// Colorization settings
		color.NoColor = color.NoColor || v.GetBool("no-color")
		// Set Logger
		l := slog.New(tint.NewHandler(rootCmd.ErrOrStderr(), &tint.Options{Level: cobrax.VerbosityLevel(v), NoColor: color.NoColor, TimeFormat: time.Kitchen}))
		slog.SetDefault(l)
		cobrax.SetLogger(l)
	})
}

func main() {
	v = viper.NewWithOptions(viper.WithLogger(slog.Default()))
	fs := afero.NewOsFs()
	v.SetFs(fs)
	rootCmd = NewRootCmd(v, fs)
	rootCmd.SetOut(colorable.NewColorableStdout())
	rootCmd.SetErr(colorable.NewColorableStderr())
	if err := rootCmd.Execute(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
