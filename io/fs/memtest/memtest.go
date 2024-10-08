package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	gorand "math/rand/v2"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/math/rand"

	"github.com/google/gops/agent"
)

func main() {
	oStorage := "mapof"
	oWriters := 500
	oReaders := 1000
	oFiles := 15
	oInterval := 1 // seconds
	oSize := 2     // megabytes
	oLimit := false

	flag.StringVar(&oStorage, "storage", "mapof", "type of mem storage implementation (mapof, map, swiss)")
	flag.IntVar(&oWriters, "writers", 500, "number of concurrent writers")
	flag.IntVar(&oReaders, "readers", 1000, "number of concurrent readers")
	flag.IntVar(&oFiles, "files", 15, "number of files to keep per writer")
	flag.IntVar(&oInterval, "interval", 1, "interval for writing files in seconds")
	flag.IntVar(&oSize, "size", 2048, "size of files to write in kilobytes")
	flag.BoolVar(&oLimit, "limit", false, "set memory limit")

	flag.Parse()

	estimatedSize := float64(oWriters*oFiles*oSize) / 1024 / 1024

	fmt.Printf("Expecting effective memory consumption of %.1fGB\n", estimatedSize)

	if oLimit {
		fmt.Printf("Setting memory limit to %.1fGB\n", estimatedSize*1.5)
		debug.SetMemoryLimit(int64(estimatedSize * 1.5))
	}

	memfs, err := fs.NewMemFilesystem(fs.MemConfig{
		Storage: oStorage,
	})

	if err != nil {
		log.Fatalf("acquiring new memfs: %s", err.Error())
	}

	err = agent.Listen(agent.Options{
		Addr:                   ":9000",
		ReuseSocketAddrAndPort: true,
	})

	if err != nil {
		log.Fatalf("starting agent: %s", err.Error())
	}

	fmt.Printf("Started agent on :9000\n")

	ctx, cancel := context.WithCancel(context.Background())

	wgWriter := sync.WaitGroup{}

	for i := 0; i < oWriters; i++ {
		fmt.Printf("%4d / %4d writer started\r", i+1, oWriters)

		wgWriter.Add(1)

		go func(ctx context.Context, memfs fs.Filesystem, index int, nfiles int64, interval time.Duration) {
			defer wgWriter.Done()

			jitter := gorand.IntN(200)
			interval += time.Duration(jitter) * time.Millisecond

			sequence := int64(0)

			buf := bytes.NewBufferString(rand.StringAlphanumeric(oSize * (1024 + jitter - 100)))
			r := bytes.NewReader(buf.Bytes())

			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					path := fmt.Sprintf("/foobar/test_%d_%06d.dat", index, sequence)

					// Write file to memfs
					r.Seek(0, io.SeekStart)
					memfs.WriteFileReader(path, r, -1)

					// Delete file from memfs
					if sequence-nfiles >= 0 {
						path = fmt.Sprintf("/foobar/test_%d_%06d.dat", index, sequence-nfiles)
						memfs.Remove(path)
					}

					path = fmt.Sprintf("/foobar/test_%d.last", index)
					memfs.WriteFile(path, []byte(strconv.FormatInt(sequence, 10)))

					sequence++
				}
			}
		}(ctx, memfs, i, int64(oFiles), time.Duration(oInterval)*time.Second)
	}

	fmt.Printf("\n")

	wgReader := sync.WaitGroup{}

	if oReaders > 0 {
		for i := 0; i < oReaders; i++ {
			fmt.Printf("%4d / %4d reader started\r", i+1, oReaders)

			wgReader.Add(1)

			go func(ctx context.Context, memfs fs.Filesystem, interval time.Duration) {
				defer wgReader.Done()

				buf := bytes.Buffer{}

				jitter := gorand.IntN(200)
				interval += time.Duration(jitter) * time.Millisecond

				ticker := time.NewTicker(interval)
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						index := gorand.IntN(oWriters)

						path := fmt.Sprintf("/foobar/test_%d.list", index)
						data, err := memfs.ReadFile(path)
						if err != nil {
							continue
						}

						sequence, err := strconv.ParseUint(string(data), 10, 64)
						if err != nil {
							continue
						}

						path = fmt.Sprintf("/foobar/test_%d_%06d.dat", index, sequence)
						file := memfs.Open(path)

						buf.ReadFrom(file)
						buf.Reset()
					}
				}
			}(ctx, memfs, time.Duration(oInterval)*time.Second)
		}

		fmt.Printf("\n")
	}

	go func(ctx context.Context, memfs fs.Filesystem) {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		nMallocs := uint64(0)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m := runtime.MemStats{}
				runtime.ReadMemStats(&m)

				size, _ := memfs.Size()
				fmt.Printf("%5.1fGB ", float64(size)/1024/1024/1024)

				listfiles := 0
				listsize := int64(0)
				files := memfs.List("/", fs.ListOptions{})
				for _, f := range files {
					listsize += f.Size()
				}
				listfiles = len(files)

				fmt.Printf("(%7d files with %5.1fGB) ", listfiles, float64(listsize)/1024/1024/1024)

				fmt.Printf("alloc=%5.1fGB (%8.1fGB) sys=%5.1fGB idle=%5.1fGB inuse=%5.1fGB mallocs=%d objects=%d\n", float64(m.HeapAlloc)/1024/1024/1024, float64(m.TotalAlloc)/1024/1024/1024, float64(m.HeapSys)/1024/1024/1024, float64(m.HeapIdle)/1024/1024/1024, float64(m.HeapInuse)/1024/1024/1024, m.Mallocs-nMallocs, m.Mallocs-m.Frees)

				nMallocs = m.Mallocs
			}
		}
	}(ctx, memfs)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	cancel()

	fmt.Printf("Waiting for readers to stop ...\n")
	wgReader.Wait()

	fmt.Printf("Waiting for writers to stop ...\n")
	wgWriter.Wait()

	fmt.Printf("Done\n")
}
