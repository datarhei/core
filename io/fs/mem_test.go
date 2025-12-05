package fs

import (
	"context"
	"fmt"
	"io"
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

func TestWriteWhileRead(t *testing.T) {
	fs, err := NewMemFilesystem(MemConfig{})
	require.NoError(t, err)

	_, _, err = fs.WriteFile("/foobar", []byte("xxxxx"))
	require.NoError(t, err)

	file := fs.Open("/foobar")
	require.NotNil(t, file)

	_, _, err = fs.WriteFile("/foobar", []byte("yyyyy"))
	require.NoError(t, err)

	data, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Equal(t, []byte("xxxxx"), data)
}

func TestCopy(t *testing.T) {
	fs, err := NewMemFilesystem(MemConfig{})
	require.NoError(t, err)

	_, _, err = fs.WriteFile("/foobar", []byte("xxxxx"))
	require.NoError(t, err)

	data, err := fs.ReadFile("/foobar")
	require.NoError(t, err)

	require.Equal(t, []byte("xxxxx"), data)

	err = fs.Copy("/foobar", "/barfoo")
	require.NoError(t, err)

	data, err = fs.ReadFile("/barfoo")
	require.NoError(t, err)

	require.Equal(t, []byte("xxxxx"), data)

	fs.Remove("/foobar")

	data, err = fs.ReadFile("/barfoo")
	require.NoError(t, err)

	require.Equal(t, []byte("xxxxx"), data)
}

func BenchmarkMemStorages(b *testing.B) {
	storages := []string{
		"map",
		"xsync",
		"swiss",
	}

	benchmarks := map[string]func(*testing.B, Filesystem){
		"list":           benchmarkMemList,
		"removeList":     benchmarkMemRemoveList,
		"readFile":       benchmarkMemReadFile,
		"writeFile":      benchmarkMemWriteFile,
		"readWhileWrite": benchmarkMemReadFileWhileWriting,
	}

	for name, fn := range benchmarks {
		for _, storage := range storages {
			mem, err := NewMemFilesystem(MemConfig{Storage: storage})
			require.NoError(b, err)

			b.Run(name+"-"+storage, func(b *testing.B) {
				fn(b, mem)
			})
		}
	}
}

func benchmarkMemList(b *testing.B, fs Filesystem) {
	for i := 0; i < 1000; i++ {
		id := rand.StringAlphanumeric(8)
		path := fmt.Sprintf("/%d/%s.dat", i, id)
		fs.WriteFile(path, []byte("foobar"))
	}

	for b.Loop() {
		fs.List("/", ListOptions{
			Pattern: "/5/**",
		})
	}
}

func benchmarkMemRemoveList(b *testing.B, fs Filesystem) {
	for i := range 1000 {
		id := rand.StringAlphanumeric(8)
		path := fmt.Sprintf("/%d/%s.dat", i, id)
		fs.WriteFile(path, []byte("foobar"))
	}

	for b.Loop() {
		fs.RemoveList("/", ListOptions{
			Pattern: "/5/**",
		})
	}
}

func benchmarkMemReadFile(b *testing.B, fs Filesystem) {
	nFiles := 1000

	for i := range nFiles {
		path := fmt.Sprintf("/%d.dat", i)
		fs.WriteFile(path, []byte(rand.StringAlphanumeric(2*1024)))
	}

	r := gorand.New(gorand.NewSource(42))

	for b.Loop() {
		num := r.Intn(nFiles)
		f := fs.Open("/" + strconv.Itoa(num) + ".dat")
		f.Close()
	}
}

func benchmarkMemWriteFile(b *testing.B, fs Filesystem) {
	nFiles := 50000

	for i := range nFiles {
		path := fmt.Sprintf("/%d.dat", i)
		fs.WriteFile(path, []byte(rand.StringAlphanumeric(1)))
	}

	for i := 0; b.Loop(); i++ {
		path := fmt.Sprintf("/%d.dat", i%nFiles)
		fs.WriteFile(path, []byte(rand.StringAlphanumeric(1)))
	}
}

func benchmarkMemReadFileWhileWriting(b *testing.B, fs Filesystem) {
	nReaders := 500
	nWriters := 1000
	nFiles := 30

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writerWg := sync.WaitGroup{}

	data := []byte(rand.StringAlphanumeric(2 * 1024))

	for i := range nWriters {
		writerWg.Add(1)

		go func(ctx context.Context, from int) {
			for i := range nFiles {
				path := fmt.Sprintf("/%d.dat", from+i)
				fs.WriteFile(path, data)
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
					fs.WriteFile(path, data)
				}
			}
		}(ctx, i*nFiles)
	}

	// Wait for all writers to be started
	writerWg.Wait()

	b.ResetTimer()

	readerWg := sync.WaitGroup{}

	for range nReaders {
		readerWg.Add(1)
		go func() {
			defer readerWg.Done()

			for b.Loop() {
				num := gorand.Intn(nWriters * nFiles)
				f := fs.Open("/" + strconv.Itoa(num) + ".dat")
				f.Close()
			}
		}()
	}

	readerWg.Wait()
}
