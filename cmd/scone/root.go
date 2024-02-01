package main

import (
	"github.com/haijima/cobrax"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewRootCmd(v *viper.Viper, fs afero.Fs) *cobra.Command {
	cmd := cobrax.NewRoot(v)
	cmd.Use = "scone"
	cmd.Short = "scone is a static analysis tool for SQL"
	cmd.Version = cobrax.VersionFunc()
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return cobrax.RootPersistentPreRunE(cmd, v, fs, args)
	}
	SetQueryOptionFlags(cmd)

	cmd.AddCommand(NewCallgraphCommand(v, fs))
	cmd.AddCommand(NewQueryCommand(v, fs))
	cmd.AddCommand(NewTableCommand(v, fs))

	cmd.SetGlobalNormalizationFunc(cobrax.SnakeToKebab)

	return cmd
}
