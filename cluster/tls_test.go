package cluster

import (
	"context"
	"io/fs"
	"path"
	"testing"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/stretchr/testify/require"
)

func setupStorage() (certmagic.Storage, error) {
	kvs, err := NewMemoryKVS()
	if err != nil {
		return nil, err
	}

	return NewClusterStorage(kvs, "some_prefix", nil)
}

func TestStorageStore(t *testing.T) {
	cs, err := setupStorage()
	require.NoError(t, err)

	err = cs.Store(context.Background(), path.Join("acme", "example.com", "sites", "example.com", "example.com.crt"), []byte("crt data"))
	require.NoError(t, err)
}

func TestStorageExists(t *testing.T) {
	cs, err := setupStorage()
	require.NoError(t, err)

	key := path.Join("acme", "example.com", "sites", "example.com", "example.com.crt")

	err = cs.Store(context.Background(), key, []byte("crt data"))
	require.NoError(t, err)

	exists := cs.Exists(context.Background(), key)
	require.True(t, exists)
}

func TestStorageLoad(t *testing.T) {
	cs, err := setupStorage()
	require.NoError(t, err)

	key := path.Join("acme", "example.com", "sites", "example.com", "example.com.crt")
	content := []byte("crt data")

	err = cs.Store(context.Background(), key, content)
	require.NoError(t, err)

	contentLoded, err := cs.Load(context.Background(), key)
	require.NoError(t, err)

	require.Equal(t, content, contentLoded)
}

func TestStorageDelete(t *testing.T) {
	cs, err := setupStorage()
	require.NoError(t, err)

	key := path.Join("acme", "example.com", "sites", "example.com", "example.com.crt")
	content := []byte("crt data")

	err = cs.Store(context.Background(), key, content)
	require.NoError(t, err)

	err = cs.Delete(context.Background(), key)
	require.NoError(t, err)

	exists := cs.Exists(context.Background(), key)
	require.False(t, exists)

	contentLoaded, err := cs.Load(context.Background(), key)
	require.Nil(t, contentLoaded)
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestStorageStat(t *testing.T) {
	cs, err := setupStorage()
	require.NoError(t, err)

	key := path.Join("acme", "example.com", "sites", "example.com", "example.com.crt")
	content := []byte("crt data")

	err = cs.Store(context.Background(), key, content)
	require.NoError(t, err)

	info, err := cs.Stat(context.Background(), key)
	require.NoError(t, err)

	require.Equal(t, key, info.Key)
}

func TestStorageList(t *testing.T) {
	cs, err := setupStorage()
	require.NoError(t, err)

	err = cs.Store(context.Background(), path.Join("acme", "example.com", "sites", "example.com", "example.com.crt"), []byte("crt"))
	require.NoError(t, err)
	err = cs.Store(context.Background(), path.Join("acme", "example.com", "sites", "example.com", "example.com.key"), []byte("key"))
	require.NoError(t, err)
	err = cs.Store(context.Background(), path.Join("acme", "example.com", "sites", "example.com", "example.com.json"), []byte("meta"))
	require.NoError(t, err)

	keys, err := cs.List(context.Background(), path.Join("acme", "example.com", "sites", "example.com"), true)
	require.NoError(t, err)
	require.Len(t, keys, 3)
	require.Contains(t, keys, path.Join("acme", "example.com", "sites", "example.com", "example.com.crt"))
}

func TestStorageListNonRecursive(t *testing.T) {
	cs, err := setupStorage()
	require.NoError(t, err)

	err = cs.Store(context.Background(), path.Join("acme", "example.com", "sites", "example.com", "example.com.crt"), []byte("crt"))
	require.NoError(t, err)
	err = cs.Store(context.Background(), path.Join("acme", "example.com", "sites", "example.com", "example.com.key"), []byte("key"))
	require.NoError(t, err)
	err = cs.Store(context.Background(), path.Join("acme", "example.com", "sites", "example.com", "example.com.json"), []byte("meta"))
	require.NoError(t, err)

	keys, err := cs.List(context.Background(), path.Join("acme", "example.com", "sites"), false)
	require.NoError(t, err)

	require.Len(t, keys, 1)
	require.Contains(t, keys, path.Join("acme", "example.com", "sites", "example.com"))
}

func TestStorageLockUnlock(t *testing.T) {
	cs, err := setupStorage()
	require.NoError(t, err)

	lockKey := path.Join("acme", "example.com", "sites", "example.com", "lock")

	err = cs.Lock(context.Background(), lockKey)
	require.NoError(t, err)

	err = cs.Unlock(context.Background(), lockKey)
	require.NoError(t, err)
}

func TestStorageTwoLocks(t *testing.T) {
	cs, err := setupStorage()
	require.NoError(t, err)

	lockKey := path.Join("acme", "example.com", "sites", "example.com", "lock")

	err = cs.Lock(context.Background(), lockKey)
	require.NoError(t, err)

	go time.AfterFunc(5*time.Second, func() {
		err := cs.Unlock(context.Background(), lockKey)
		require.NoError(t, err)
	})

	err = cs.Lock(context.Background(), lockKey)
	require.NoError(t, err)

	err = cs.Unlock(context.Background(), lockKey)
	require.NoError(t, err)
}
