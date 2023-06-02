package json

import (
	"testing"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/restream/app"
	"github.com/datarhei/core/v16/restream/store"
	"github.com/stretchr/testify/require"
)

func getFS(t *testing.T) fs.Filesystem {
	fs, err := fs.NewRootedDiskFilesystem(fs.RootedDiskConfig{
		Root: ".",
	})
	require.NoError(t, err)

	info, err := fs.Stat("./fixtures/v4_empty.json")
	require.NoError(t, err)
	require.Equal(t, "/fixtures/v4_empty.json", info.Name())

	return fs
}

func TestNew(t *testing.T) {
	store, err := New(Config{
		Filesystem: getFS(t),
	})
	require.NoError(t, err)
	require.NotEmpty(t, store)
}

func TestStoreLoad(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	jsonstore, err := New(Config{
		Filesystem: memfs,
		Filepath:   "./db.json",
	})
	require.NoError(t, err)

	data := store.NewData()

	data.Process[""] = make(map[string]store.Process)
	p := store.Process{
		Process: &app.Process{
			ID:        "foobar",
			Owner:     "me",
			Domain:    "",
			Reference: "ref",
			Config: &app.Config{
				ID:        "foobar",
				Reference: "ref",
				Owner:     "me",
				Domain:    "",
				FFVersion: "5.1.3",
				Input:     []app.ConfigIO{},
				Output:    []app.ConfigIO{},
				Options: []string{
					"42",
				},
				Reconnect:      true,
				ReconnectDelay: 14,
				Autostart:      true,
				StaleTimeout:   1,
				Timeout:        6,
				Scheduler:      "10 * * * *",
				LogPatterns:    []string{"foo"},
				LimitCPU:       2,
				LimitMemory:    3,
				LimitWaitFor:   4,
			},
			CreatedAt: 0,
			UpdatedAt: 0,
			Order:     "stop",
		},
		Metadata: map[string]interface{}{
			"some": "data",
		},
	}
	data.Process[""]["foobar"] = p

	data.Process["domain"] = make(map[string]store.Process)
	p = store.Process{
		Process: &app.Process{
			ID:        "foobaz",
			Owner:     "you",
			Domain:    "domain",
			Reference: "refref",
			Config: &app.Config{
				ID:        "foobaz",
				Reference: "refref",
				Owner:     "you",
				Domain:    "domain",
				FFVersion: "^5.1.4",
				Input:     []app.ConfigIO{},
				Output:    []app.ConfigIO{},
				Options: []string{
					"47",
				},
				Reconnect:      true,
				ReconnectDelay: 24,
				Autostart:      true,
				StaleTimeout:   21,
				Timeout:        26,
				Scheduler:      "* * * * *",
				LogPatterns:    []string{"libx264"},
				LimitCPU:       22,
				LimitMemory:    23,
				LimitWaitFor:   24,
			},
			CreatedAt: 0,
			UpdatedAt: 0,
			Order:     "stop",
		},
		Metadata: map[string]interface{}{
			"some-more": "data",
		},
	}
	data.Process["domain"]["foobaz"] = p

	data.Metadata["foo"] = "bar"

	err = jsonstore.Store(data)
	require.NoError(t, err)

	d, err := jsonstore.Load()
	require.NoError(t, err)

	require.Equal(t, data, d)
}

func TestLoad(t *testing.T) {
	store, err := New(Config{
		Filesystem: getFS(t),
		Filepath:   "./fixtures/v4_empty.json",
	})
	require.NoError(t, err)

	_, err = store.Load()
	require.NoError(t, err)
}

func TestLoadFailed(t *testing.T) {
	store, err := New(Config{
		Filesystem: getFS(t),
		Filepath:   "./fixtures/v4_invalid.json",
	})
	require.NoError(t, err)

	_, err = store.Load()
	require.Error(t, err)
}

func TestIsEmpty(t *testing.T) {
	store, err := New(Config{
		Filesystem: getFS(t),
		Filepath:   "./fixtures/v4_empty.json",
	})
	require.NoError(t, err)

	data, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, true, len(data.Process) == 0)
}

func TestNotExists(t *testing.T) {
	store, err := New(Config{
		Filesystem: getFS(t),
		Filepath:   "./fixtures/v4_notexist.json",
	})
	require.NoError(t, err)

	data, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, true, len(data.Process) == 0)
}

func TestInvalidVersion(t *testing.T) {
	store, err := New(Config{
		Filesystem: getFS(t),
		Filepath:   "./fixtures/v3_empty.json",
	})
	require.NoError(t, err)

	data, err := store.Load()
	require.Error(t, err)
	require.Equal(t, true, len(data.Process) == 0)
}
