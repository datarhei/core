package fs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemFromDir(t *testing.T) {
	mem, err := NewMemFilesystemFromDir("./fixtures", MemConfig{})
	require.NoError(t, err)

	names := []string{}
	for _, f := range mem.List("/", ListOptions{Pattern: "/*.txt"}) {
		names = append(names, f.Name())
	}

	require.ElementsMatch(t, []string{
		"/a.txt",
		"/b.txt",
	}, names)
}
