package main

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestNewRootCmd(t *testing.T) {
	v := viper.New()
	fs := afero.NewMemMapFs()
	cmd := NewRootCmd(v, fs)

	assert.Equal(t, "scone", cmd.Use)
	assert.NotNil(t, cmd.Commands())
	assert.Equal(t, 3, len(cmd.Commands()))
}
