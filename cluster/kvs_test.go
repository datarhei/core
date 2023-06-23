package cluster

import (
	"io/fs"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSetGetUnsetValue(t *testing.T) {
	kvs, err := NewMemoryKVS()
	require.NoError(t, err)

	_, _, err = kvs.GetKV("foo")
	require.Error(t, err)

	err = kvs.SetKV("foo", "bar")
	require.NoError(t, err)

	value, _, err := kvs.GetKV("foo")
	require.NoError(t, err)
	require.Equal(t, "bar", value)

	err = kvs.UnsetKV("foo")
	require.NoError(t, err)

	_, _, err = kvs.GetKV("foo")
	require.Error(t, err)
}

func TestKeyNotFound(t *testing.T) {
	kvs, err := NewMemoryKVS()
	require.NoError(t, err)

	_, _, err = kvs.GetKV("foo")
	require.ErrorIs(t, err, fs.ErrNotExist)

	err = kvs.UnsetKV("foo")
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestListKV(t *testing.T) {
	kvs, err := NewMemoryKVS()
	require.NoError(t, err)

	err = kvs.SetKV("foo", "bar")
	require.NoError(t, err)

	err = kvs.SetKV("foz", "baz")
	require.NoError(t, err)

	err = kvs.SetKV("bar", "foo")
	require.NoError(t, err)

	list := kvs.ListKV("")
	require.Equal(t, 3, len(list))

	list = kvs.ListKV("f")
	require.Equal(t, 2, len(list))

	list = kvs.ListKV("b")
	require.Equal(t, 1, len(list))

	list = kvs.ListKV("fo")
	require.Equal(t, 2, len(list))

	list = kvs.ListKV("foo")
	require.Equal(t, 1, len(list))
}

func TestLock(t *testing.T) {
	kvs, err := NewMemoryKVS()
	require.NoError(t, err)

	until := time.Now().Add(5 * time.Second)

	lock, err := kvs.CreateLock("foobar", until)
	require.NoError(t, err)
	require.Equal(t, until, lock.ValidUntil)

	require.Eventually(t, func() bool {
		select {
		case <-lock.Expired():
			return true
		case <-time.After(10 * time.Millisecond):
			return false
		}
	}, 10*time.Second, time.Second)
}

func TestLockCreate(t *testing.T) {
	kvs, err := NewMemoryKVS()
	require.NoError(t, err)

	until := time.Now().Add(5 * time.Second)
	lock, err := kvs.CreateLock("foobar", until)
	require.NoError(t, err)
	require.Equal(t, until, lock.ValidUntil)

	_, err = kvs.CreateLock("foobar", until)
	require.Error(t, err)

	err = kvs.DeleteLock("foobar")
	require.NoError(t, err)
}

func TestLockDelete(t *testing.T) {
	kvs, err := NewMemoryKVS()
	require.NoError(t, err)

	err = kvs.DeleteLock("foobar")
	require.Error(t, err)

	until := time.Now().Add(5 * time.Second)
	_, err = kvs.CreateLock("foobar", until)
	require.NoError(t, err)

	err = kvs.DeleteLock("foobar")
	require.NoError(t, err)
}

func TestLocksList(t *testing.T) {
	kvs, err := NewMemoryKVS()
	require.NoError(t, err)

	list := kvs.ListLocks()
	require.Empty(t, list)

	until := time.Now().Add(5 * time.Second)
	_, err = kvs.CreateLock("foobar", until)
	require.NoError(t, err)

	list = kvs.ListLocks()
	require.NotEmpty(t, list)
	require.Equal(t, list["foobar"], until)

	err = kvs.DeleteLock("foobar")
	require.NoError(t, err)

	list = kvs.ListLocks()
	require.Empty(t, list)
}
