package main

import (
	"context"
	"flag"
	"fmt"
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
		lv := cobrax.VerbosityLevel(v)
		l := slog.New(tint.NewHandler(rootCmd.ErrOrStderr(), &tint.Options{Level: lv, AddSource: lv < slog.LevelDebug, NoColor: color.NoColor, TimeFormat: time.Kitchen}))
		slog.SetDefault(l)
		cobrax.SetLogger(l)
	})
}

func main() {
	flag.Parse()
	flag.CommandLine.PrintDefaults()
	slog.SetDefault(slog.New(tint.NewHandler(colorable.NewColorableStderr(), &tint.Options{Level: slog.LevelError, NoColor: color.NoColor, TimeFormat: time.Kitchen})))
	v = viper.NewWithOptions(viper.WithLogger(slog.Default()))
	fs := afero.NewOsFs()
	v.SetFs(fs)
	rootCmd = NewRootCmd(v, fs)
	rootCmd.SetOut(colorable.NewColorableStdout())
	rootCmd.SetErr(colorable.NewColorableStderr())
	if err := rootCmd.Execute(); err != nil {
		if slog.Default().Enabled(context.Background(), slog.LevelInfo) {
			slog.Error(fmt.Sprintf("%+v", err))
		} else {
			slog.Error("Error: ", tint.Err(err))
		}
		os.Exit(1)
	}
}
