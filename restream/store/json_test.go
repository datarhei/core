package store

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	store := NewJSONStore(JSONConfig{})

	require.NotEmpty(t, store)
}

func TestLoad(t *testing.T) {
	store := NewJSONStore(JSONConfig{
		Filepath: "./fixtures/v4_empty.json",
	})

	_, err := store.Load()
	require.Equal(t, nil, err)
}

func TestLoadFailed(t *testing.T) {
	store := NewJSONStore(JSONConfig{
		Filepath: "./fixtures/v4_invalid.json",
	})

	_, err := store.Load()
	require.NotEqual(t, nil, err)
}

func TestIsEmpty(t *testing.T) {
	store := NewJSONStore(JSONConfig{
		Filepath: "./fixtures/v4_empty.json",
	})

	data, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, true, data.IsEmpty())
}

func TestNotExists(t *testing.T) {
	store := NewJSONStore(JSONConfig{
		Filepath: "./fixtures/v4_notexist.json",
	})

	data, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, true, data.IsEmpty())
}

func TestStore(t *testing.T) {
	os.Remove("./fixtures/v4_store.json")

	store := NewJSONStore(JSONConfig{
		Filepath: "./fixtures/v4_store.json",
	})

	data, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, true, data.IsEmpty())

	data.Metadata.System["somedata"] = "foobar"

	store.Store(data)

	data2, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, data, data2)

	os.Remove("./fixtures/v4_store.json")
}

func TestInvalidVersion(t *testing.T) {
	store := NewJSONStore(JSONConfig{
		Filepath: "./fixtures/v3_empty.json",
	})

	data, err := store.Load()
	require.Error(t, err)
	require.Equal(t, true, data.IsEmpty())
}
