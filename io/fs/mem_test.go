package fs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemFromDir(t *testing.T) {
	mem, err := NewMemFilesystemFromDir(".", MemConfig{})
	require.NoError(t, err)

	names := []string{}
	for _, f := range mem.List("/", "/*.go") {
		names = append(names, f.Name())
	}

	require.ElementsMatch(t, []string{
		"/disk.go",
		"/fs_test.go",
		"/fs.go",
		"/mem_test.go",
		"/mem.go",
		"/readonly_test.go",
		"/readonly.go",
		"/s3.go",
		"/sized_test.go",
		"/sized.go",
	}, names)
}
