package main

import (
	"log/slog"
	"time"

	"github.com/fatih/color"
	"github.com/haijima/cobrax"
	"github.com/lmittmann/tint"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewRootCmd(v *viper.Viper, fs afero.Fs) *cobra.Command {
	cmd := cobrax.NewRoot(v)
	cmd.Use = "scone"
	cmd.Short = "scone is a static analysis tool for SQL"
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Colorization settings
		color.NoColor = color.NoColor || v.GetBool("no-color")
		// Set Logger
		lv := cobrax.VerbosityLevel(v)
		l := slog.New(tint.NewHandler(cmd.ErrOrStderr(), &tint.Options{Level: lv, AddSource: lv < slog.LevelDebug, NoColor: color.NoColor, TimeFormat: time.Kitchen}))
		slog.SetDefault(l)
		cobrax.SetLogger(l)

		// Read config file
		opts := []cobrax.ConfigOption{cobrax.WithConfigFileFlag(cmd, "config"), cobrax.WithOverrideBy(cmd.Name())}
		if err := cobrax.BindConfigs(v, cmd.Root().Name(), opts...); err != nil {
			return err
		}
		// Bind flags (flags of the command to be executed)
		if err := v.BindPFlags(cmd.Flags()); err != nil {
			return err
		}
		slog.Debug("bind flags and config values")
		slog.Debug(cobrax.DebugViper(v))
		return nil
	}
	SetQueryOptionFlags(cmd)

	cmd.AddCommand(NewCallgraphCommand(v, fs))
	cmd.AddCommand(NewQueryCommand(v, fs))
	cmd.AddCommand(NewTableCommand(v, fs))

	cmd.SetGlobalNormalizationFunc(cobrax.SnakeToKebab)

	return cmd
}
