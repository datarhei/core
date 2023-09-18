package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCreateLockCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.applyCommand(Command{
		Operation: OpCreateLock,
		Data: CommandCreateLock{
			Name:       "foobar",
			ValidUntil: time.Now().Add(3 * time.Second),
		},
	})
	require.NoError(t, err)

	_, ok := s.data.Locks["foobar"]
	require.True(t, ok)
}

func TestCreateLock(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	cmd := CommandCreateLock{
		Name:       "foobar",
		ValidUntil: time.Now().Add(3 * time.Second),
	}

	err = s.createLock(cmd)
	require.NoError(t, err)

	err = s.createLock(cmd)
	require.Error(t, err)

	require.Eventually(t, func() bool {
		err = s.createLock(cmd)
		return err == nil
	}, 5*time.Second, time.Second)
}

func TestDeleteLockCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.applyCommand(Command{
		Operation: OpCreateLock,
		Data: CommandCreateLock{
			Name:       "foobar",
			ValidUntil: time.Now().Add(10 * time.Second),
		},
	})
	require.NoError(t, err)

	_, ok := s.data.Locks["foobar"]
	require.True(t, ok)

	err = s.applyCommand(Command{
		Operation: OpDeleteLock,
		Data: CommandDeleteLock{
			Name: "foobar",
		},
	})
	require.NoError(t, err)

	_, ok = s.data.Locks["foobar"]
	require.False(t, ok)
}

func TestDeleteLock(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.deleteLock(CommandDeleteLock{
		Name: "foobar",
	})
	require.NoError(t, err)

	cmd := CommandCreateLock{
		Name:       "foobar",
		ValidUntil: time.Now().Add(10 * time.Second),
	}

	err = s.createLock(cmd)
	require.NoError(t, err)

	err = s.createLock(cmd)
	require.Error(t, err)

	err = s.deleteLock(CommandDeleteLock{
		Name: "foobar",
	})
	require.NoError(t, err)

	err = s.createLock(cmd)
	require.NoError(t, err)
}

func TestClearLocksCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.applyCommand(Command{
		Operation: OpCreateLock,
		Data: CommandCreateLock{
			Name:       "foobar",
			ValidUntil: time.Now().Add(3 * time.Second),
		},
	})
	require.NoError(t, err)

	_, ok := s.data.Locks["foobar"]
	require.True(t, ok)

	err = s.applyCommand(Command{
		Operation: OpClearLocks,
		Data:      CommandClearLocks{},
	})
	require.NoError(t, err)

	_, ok = s.data.Locks["foobar"]
	require.True(t, ok)

	time.Sleep(3 * time.Second)

	err = s.applyCommand(Command{
		Operation: OpClearLocks,
		Data:      CommandClearLocks{},
	})
	require.NoError(t, err)

	_, ok = s.data.Locks["foobar"]
	require.False(t, ok)
}

func TestClearLocks(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	cmd := CommandCreateLock{
		Name:       "foobar",
		ValidUntil: time.Now().Add(3 * time.Second),
	}

	err = s.createLock(cmd)
	require.NoError(t, err)

	err = s.clearLocks(CommandClearLocks{})
	require.NoError(t, err)

	err = s.createLock(cmd)
	require.Error(t, err)

	time.Sleep(3 * time.Second)

	err = s.clearLocks(CommandClearLocks{})
	require.NoError(t, err)

	err = s.createLock(cmd)
	require.NoError(t, err)
}
