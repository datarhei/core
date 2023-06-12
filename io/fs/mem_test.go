package fs

import (
	"fmt"
	"testing"

	"github.com/datarhei/core/v16/math/rand"
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

func BenchmarkMemList(b *testing.B) {
	mem, err := NewMemFilesystem(MemConfig{})
	require.NoError(b, err)

	for i := 0; i < 1000; i++ {
		id := rand.StringAlphanumeric(8)
		path := fmt.Sprintf("/%d/%s.dat", i, id)
		mem.WriteFile(path, []byte("foobar"))
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mem.List("/", ListOptions{
			Pattern: "/5/**",
		})
	}
}

func BenchmarkMemRemoveList(b *testing.B) {
	mem, err := NewMemFilesystem(MemConfig{})
	require.NoError(b, err)

	for i := 0; i < 1000; i++ {
		id := rand.StringAlphanumeric(8)
		path := fmt.Sprintf("/%d/%s.dat", i, id)
		mem.WriteFile(path, []byte("foobar"))
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mem.RemoveList("/", ListOptions{
			Pattern: "/5/**",
		})
	}
}
