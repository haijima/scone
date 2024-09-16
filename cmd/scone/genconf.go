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
		cmd.Flag("dir").Hidden = true
		cmd.Flag("pattern").Hidden = true
		cmd.Flag("filter").Hidden = true
		cmd.Flag("analyze-funcs").Hidden = true
		cmd.Flag("config").Hidden = true
		cmd.Flag("no-color").Hidden = true
		cmd.Root().HelpFunc()(cmd, args)
	})
	return genConfCmd
}
