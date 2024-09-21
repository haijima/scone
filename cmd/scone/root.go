package main

import (
	"log/slog"

	"github.com/fatih/color"
	"github.com/haijima/cobrax"
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
		// Set Log level
		lv.Set(cobrax.VerbosityLevel(v))

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
	cmd.PersistentFlags().StringP("dir", "d", ".", "The directory to analyze")
	cmd.PersistentFlags().StringP("pattern", "p", "./...", "The pattern to analyze")
	cmd.PersistentFlags().String("filter", "", "filter queries by `pattern`")
	cmd.PersistentFlags().StringSlice("analyze-funcs", []string{}, "The names of functions to analyze additionally. format: `<func pattern>@<argument index>`")
	_ = cmd.MarkFlagDirname("dir")

	cmd.AddCommand(NewCallgraphCommand(v, fs))
	cmd.AddCommand(NewQueryCommand(v, fs))
	cmd.AddCommand(NewTableCommand(v, fs))
	cmd.AddCommand(NewGenConfCmd(v, fs))
	cmd.AddCommand(NewCrudCmd(v, fs))
	cmd.AddCommand(NewLoopCmd(v, fs))

	cmd.SetGlobalNormalizationFunc(cobrax.SnakeToKebab)

	return cmd
}
