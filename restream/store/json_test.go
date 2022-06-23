package store

import (
	"testing"

	"github.com/datarhei/core/v16/log"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	store := NewJSONStore(JSONConfig{})

	require.NotEmpty(t, store)
}

func TestLoad(t *testing.T) {
	store := &jsonStore{
		filename: "v4_empty.json",
		dir:      "./fixtures",
		logger:   log.New(""),
	}

	_, err := store.Load()
	require.Equal(t, nil, err)
}

func TestLoadFailed(t *testing.T) {
	store := &jsonStore{
		filename: "v4_invalid.json",
		dir:      "./fixtures",
		logger:   log.New(""),
	}

	_, err := store.Load()
	require.NotEqual(t, nil, err)
}

func TestIsEmpty(t *testing.T) {
	store := &jsonStore{
		filename: "v4_empty.json",
		dir:      "./fixtures",
		logger:   log.New(""),
	}

	data, _ := store.Load()
	require.Equal(t, true, data.IsEmpty())
}
