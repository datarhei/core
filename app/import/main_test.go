package main

import (
	"testing"

	"github.com/datarhei/core/v16/config"
	"github.com/stretchr/testify/require"
)

func TestImport(t *testing.T) {
	configstore := config.NewDummyStore()

	cfg := configstore.Get()
	cfg.Version = 1
	cfg.Migrate()

	err := configstore.Set(cfg)
	require.NoError(t, err)

	err = doImport(nil, configstore)
	require.NoError(t, err)
}
