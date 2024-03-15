package fs

import (
	"context"
	"fmt"
	gorand "math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

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

func BenchmarkMemReadFile(b *testing.B) {
	mem, err := NewMemFilesystem(MemConfig{})
	require.NoError(b, err)

	nFiles := 1000

	for i := 0; i < nFiles; i++ {
		path := fmt.Sprintf("/%d.dat", i)
		mem.WriteFile(path, []byte(rand.StringAlphanumeric(2*1024)))
	}

	r := gorand.New(gorand.NewSource(42))

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		num := r.Intn(nFiles)
		f := mem.Open("/" + strconv.Itoa(num) + ".dat")
		f.Close()
	}
}

func BenchmarkMemReadFileWhileWriting(b *testing.B) {
	mem, err := NewMemFilesystem(MemConfig{})
	require.NoError(b, err)

	nReaders := 500
	nWriters := 1000
	nFiles := 30

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writerWg := sync.WaitGroup{}

	data := []byte(rand.StringAlphanumeric(2 * 1024))

	for i := 0; i < nWriters; i++ {
		writerWg.Add(1)

		go func(ctx context.Context, from int) {
			for i := 0; i < nFiles; i++ {
				path := fmt.Sprintf("/%d.dat", i+from)
				mem.WriteFile(path, data)
			}

			ticker := time.NewTicker(40 * time.Millisecond)
			defer ticker.Stop()

			writerWg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					num := gorand.Intn(nFiles) + from
					path := fmt.Sprintf("/%d.dat", num)
					mem.WriteFile(path, data)
				}
			}
		}(ctx, i*nFiles)
	}

	// Wait for all writers to be started
	writerWg.Wait()

	b.ResetTimer()

	readerWg := sync.WaitGroup{}

	for i := 0; i < nReaders; i++ {
		readerWg.Add(1)
		go func() {
			defer readerWg.Done()

			for i := 0; i < b.N; i++ {
				num := gorand.Intn(nWriters * nFiles)
				f := mem.Open("/" + strconv.Itoa(num) + ".dat")
				f.Close()
			}
		}()
	}

	readerWg.Wait()
}
