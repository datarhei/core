package main

import (
	"strings"
	"testing"

	"github.com/darkiris4/sfx-core/config/store"
	"github.com/darkiris4/sfx-core/io/fs"

	"github.com/stretchr/testify/require"
)

func TestImport(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	memfs.WriteFileReader("/mime.types", strings.NewReader("foobar"))
	memfs.WriteFileReader("/bin/ffmpeg", strings.NewReader("foobar"))

	configstore, err := store.NewJSON(memfs, "/config.json", nil)
	require.NoError(t, err)

	cfg := configstore.Get()

	err = configstore.Set(cfg)
	require.NoError(t, err)

	err = doImport(nil, memfs, configstore)
	require.NoError(t, err)
}
