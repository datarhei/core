package main

import (
	"testing"

	"github.com/datarhei/core/v16/config/store"
	"github.com/stretchr/testify/require"
)

func TestImport(t *testing.T) {
	configstore := store.NewDummy()

	cfg := configstore.Get()

	err := configstore.Set(cfg)
	require.NoError(t, err)

	err = doImport(nil, configstore)
	require.NoError(t, err)
}
