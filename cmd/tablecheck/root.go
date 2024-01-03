package main

import (
	"github.com/haijima/cobrax"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewRootCmd(v *viper.Viper, fs afero.Fs) *cobra.Command {
	cmd := cobrax.NewRoot(v)
	cmd.Use = "tablecheck"
	cmd.Short = "tablecheck is a static analysis tool for SQL"
	cmd.Version = cobrax.VersionFunc()
	cmd.SetGlobalNormalizationFunc(cobrax.SnakeToKebab)
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return cobrax.RootPersistentPreRunE(cmd, v, fs, args)
	}

	cmd.AddCommand(NewCallGraphCmd(v, fs))

	return cmd
}
