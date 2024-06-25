package autocert

import (
	"context"
	"io/fs"
	"os"
	"testing"

	"github.com/caddyserver/certmagic"
	"github.com/stretchr/testify/require"
)

func getCryptoStorage(t *testing.T) certmagic.Storage {
	s := &certmagic.FileStorage{
		Path: "./testing",
	}
	c := NewCrypto("secret")

	sc := NewCryptoStorage(s, c)

	t.Cleanup(func() {
		os.RemoveAll("./testing/")
	})

	return sc
}

func TestFileStorageStoreLoad(t *testing.T) {
	s := getCryptoStorage(t)

	data := []byte("some data")
	ctx := context.Background()

	err := s.Store(ctx, "foo", data)
	require.NoError(t, err)

	loadedData, err := s.Load(ctx, "foo")
	require.NoError(t, err)
	require.Equal(t, data, loadedData)
}

func TestFileStorageDelete(t *testing.T) {
	s := getCryptoStorage(t)

	data := []byte("some data")
	ctx := context.Background()

	err := s.Delete(ctx, "foo")
	require.NoError(t, err)

	err = s.Store(ctx, "foo", data)
	require.NoError(t, err)

	err = s.Delete(ctx, "foo")
	require.NoError(t, err)

	_, err = s.Load(ctx, "foo")
	require.Error(t, err, fs.ErrNotExist)
}

func TestFileStorageExists(t *testing.T) {
	s := getCryptoStorage(t)

	data := []byte("some data")
	ctx := context.Background()

	b := s.Exists(ctx, "foo")
	require.False(t, b)

	err := s.Store(ctx, "foo", data)
	require.NoError(t, err)

	b = s.Exists(ctx, "foo")
	require.True(t, b)

	err = s.Delete(ctx, "foo")
	require.NoError(t, err)

	b = s.Exists(ctx, "foo")
	require.False(t, b)
}

func TestFileStorageStat(t *testing.T) {
	s := getCryptoStorage(t)

	data := []byte("some data")
	ctx := context.Background()

	err := s.Store(ctx, "foo", data)
	require.NoError(t, err)

	info, err := s.Stat(ctx, "foo")
	require.NoError(t, err)

	require.Equal(t, "foo", info.Key)
	require.Equal(t, int64(len(data)), info.Size)
	require.Equal(t, true, info.IsTerminal)
}
