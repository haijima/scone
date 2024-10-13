package main

import (
	"github.com/haijima/cobrax"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewGenConfCmd(_ *viper.Viper, _ afero.Fs) *cobra.Command {
	genConfCmd := cobrax.PrintConfigCmd("genconf")
	genConfCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		for _, flag := range []string{"dir", "pattern", "filter", "analyze-funcs", "config", "no-color"} {
			cmd.Flag(flag).Hidden = true
		}
		cmd.Root().HelpFunc()(cmd, args)
	})
	return genConfCmd
}
