package session

import (
	"testing"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/stretchr/testify/require"
)

func TestHistorySource(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	s, err := NewHistorySource(memfs, "/foobar.json")
	require.NoError(t, err)
	require.Nil(t, s)
}

func TestHistorySourceDisk(t *testing.T) {
	diskfs, err := fs.NewDiskFilesystem(fs.DiskConfig{})
	require.NoError(t, err)

	s, err := NewHistorySource(diskfs, "./foobar.json")
	require.NoError(t, err)
	require.Nil(t, s)
}
