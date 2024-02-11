package main

import (
	"context"
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
	slog.SetLogLoggerLevel(slog.LevelError)
	v = viper.NewWithOptions(viper.WithLogger(slog.Default()))
	fs := afero.NewOsFs()
	v.SetFs(fs)
	rootCmd = NewRootCmd(v, fs)
	rootCmd.SetOut(colorable.NewColorableStdout())
	rootCmd.SetErr(colorable.NewColorableStderr())
	rootCmd.SetContext(context.Background())
	if err := rootCmd.Execute(); err != nil {
		if slog.Default().Enabled(rootCmd.Context(), slog.LevelDebug) {
			slog.Error(fmt.Sprintf("%+v", err))
		} else {
			slog.Error(err.Error())
		}
		os.Exit(1)
	}
}
