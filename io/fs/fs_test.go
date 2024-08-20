package fs

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var ErrNoMinio = errors.New("minio binary not found")

func startMinio(t *testing.T, path string) (*exec.Cmd, error) {
	err := os.MkdirAll(path, 0755)
	require.NoError(t, err)

	minio, err := exec.LookPath("minio")
	if err != nil {
		return nil, ErrNoMinio
	}

	proc := exec.Command(minio, "server", path, "--address", "127.0.0.1:9000")
	proc.Stderr = os.Stderr
	proc.Stdout = os.Stdout
	err = proc.Start()
	require.NoError(t, err)

	time.Sleep(5 * time.Second)

	return proc, nil
}

func stopMinio(t *testing.T, proc *exec.Cmd) {
	err := proc.Process.Signal(os.Interrupt)
	require.NoError(t, err)

	proc.Wait()
}

func TestFilesystem(t *testing.T) {
	miniopath, err := filepath.Abs("./minio")
	require.NoError(t, err)

	err = os.RemoveAll(miniopath)
	require.NoError(t, err)

	minio, err := startMinio(t, miniopath)
	if err != nil {
		if err != ErrNoMinio {
			require.NoError(t, err)
		}
	}

	os.RemoveAll("./testing/")

	filesystems := map[string]func(string) (Filesystem, error){
		"memfs-map": func(name string) (Filesystem, error) {
			return NewMemFilesystem(MemConfig{Storage: "map"})
		},
		"memfs-xsync": func(name string) (Filesystem, error) {
			return NewMemFilesystem(MemConfig{Storage: "xsync"})
		},
		"memfs-swiss": func(name string) (Filesystem, error) {
			return NewMemFilesystem(MemConfig{Storage: "swiss"})
		},
		"diskfs": func(name string) (Filesystem, error) {
			return NewRootedDiskFilesystem(RootedDiskConfig{
				Root: "./testing/" + name,
			})
		},
		"s3fs": func(name string) (Filesystem, error) {
			return NewS3Filesystem(S3Config{
				Name:            name,
				Endpoint:        "127.0.0.1:9000",
				AccessKeyID:     "minioadmin",
				SecretAccessKey: "minioadmin",
				Region:          "",
				Bucket:          strings.ToLower(name),
				UseSSL:          false,
				Logger:          nil,
			})
		},
	}

	tests := map[string]func(*testing.T, Filesystem){
		"new":             testNew,
		"metadata":        testMetadata,
		"writeFile":       testWriteFile,
		"writeFileSafe":   testWriteFileSafe,
		"writeFileReader": testWriteFileReader,
		"writeFileDir":    testWriteFileDir,
		"remove":          testRemove,
		"files":           testFiles,
		"replace":         testReplace,
		"list":            testList,
		"listGlob":        testListGlob,
		"listSize":        testListSize,
		"listModified":    testListModified,
		"removeAll":       testRemoveAll,
		"removeList":      testRemoveList,
		"data":            testData,
		"statDir":         testStatDir,
		"mkdirAll":        testMkdirAll,
		"rename":          testRename,
		"renameOverwrite": testRenameOverwrite,
		"copy":            testCopy,
		"symlink":         testSymlink,
		"stat":            testStat,
		"copyOverwrite":   testCopyOverwrite,
		"symlinkErrors":   testSymlinkErrors,
		"symlinkOpenStat": testSymlinkOpenStat,
		"open":            testOpen,
	}

	for fsname, fs := range filesystems {
		for name, test := range tests {
			t.Run(fsname+"-"+name, func(t *testing.T) {
				if fsname == "s3fs" && minio == nil {
					t.Skip("minio server not available")
				}
				filesystem, err := fs(name)
				require.NoError(t, err)
				test(t, filesystem)
			})
		}
	}

	os.RemoveAll("./testing/")

	if minio != nil {
		stopMinio(t, minio)
	}

	os.RemoveAll(miniopath)
}

func testNew(t *testing.T, fs Filesystem) {
	cur, max := fs.Size()

	require.Equal(t, int64(0), cur, "current size")
	require.Equal(t, int64(-1), max, "max size")

	cur = fs.Files()

	require.Equal(t, int64(0), cur, "number of files")
}

func testMetadata(t *testing.T, fs Filesystem) {
	fs.SetMetadata("foo", "bar")
	require.Equal(t, "bar", fs.Metadata("foo"))
}

func testWriteFile(t *testing.T, fs Filesystem) {
	size, created, err := fs.WriteFile("/foobar", []byte("xxxxx"))

	require.Nil(t, err)
	require.Equal(t, int64(5), size)
	require.Equal(t, true, created)

	cur, max := fs.Size()

	require.Equal(t, int64(5), cur)
	require.Equal(t, int64(-1), max)

	cur = fs.Files()

	require.Equal(t, int64(1), cur)
}

func testWriteFileSafe(t *testing.T, fs Filesystem) {
	size, created, err := fs.WriteFileSafe("/foobar", []byte("xxxxx"))

	require.Nil(t, err)
	require.Equal(t, int64(5), size)
	require.Equal(t, true, created)

	cur, max := fs.Size()

	require.Equal(t, int64(5), cur)
	require.Equal(t, int64(-1), max)

	cur = fs.Files()

	require.Equal(t, int64(1), cur)
}

func testWriteFileReader(t *testing.T, fs Filesystem) {
	data := strings.NewReader("xxxxx")

	size, created, err := fs.WriteFileReader("/foobar", data, -1)

	require.Nil(t, err)
	require.Equal(t, int64(5), size)
	require.Equal(t, true, created)

	cur, max := fs.Size()

	require.Equal(t, int64(5), cur)
	require.Equal(t, int64(-1), max)

	cur = fs.Files()

	require.Equal(t, int64(1), cur)
}

func testWriteFileDir(t *testing.T, fs Filesystem) {
	_, _, err := fs.WriteFile("/", []byte("xxxxx"))
	require.Error(t, err)
}

func testOpen(t *testing.T, fs Filesystem) {
	file := fs.Open("/foobar")
	require.Nil(t, file)

	_, _, err := fs.WriteFileReader("/foobar", strings.NewReader("xxxxx"), -1)
	require.NoError(t, err)

	file = fs.Open("/foobar")
	require.NotNil(t, file)
	require.Equal(t, "/foobar", file.Name())

	stat, err := file.Stat()
	require.NoError(t, err)
	require.Equal(t, "/foobar", stat.Name())
	require.Equal(t, int64(5), stat.Size())
	require.Equal(t, false, stat.IsDir())
}

func testRemove(t *testing.T, fs Filesystem) {
	size := fs.Remove("/foobar")

	require.Equal(t, int64(-1), size)

	data := strings.NewReader("xxxxx")

	fs.WriteFileReader("/foobar", data, -1)

	size = fs.Remove("/foobar")

	require.Equal(t, int64(5), size)

	cur, max := fs.Size()

	require.Equal(t, int64(0), cur)
	require.Equal(t, int64(-1), max)

	cur = fs.Files()

	require.Equal(t, int64(0), cur)
}

func testFiles(t *testing.T, fs Filesystem) {
	require.Equal(t, int64(0), fs.Files())

	fs.WriteFileReader("/foobar.txt", strings.NewReader("bar"), -1)

	require.Equal(t, int64(1), fs.Files())

	fs.MkdirAll("/path/to/foo", 0755)

	require.Equal(t, int64(1), fs.Files())

	fs.Remove("/foobar.txt")

	require.Equal(t, int64(0), fs.Files())
}

func testReplace(t *testing.T, fs Filesystem) {
	data := strings.NewReader("xxxxx")

	size, created, err := fs.WriteFileReader("/foobar", data, -1)

	require.Nil(t, err)
	require.Equal(t, int64(5), size)
	require.Equal(t, true, created)

	cur, max := fs.Size()

	require.Equal(t, int64(5), cur)
	require.Equal(t, int64(-1), max)

	cur = fs.Files()

	require.Equal(t, int64(1), cur)

	data = strings.NewReader("yyy")

	size, created, err = fs.WriteFileReader("/foobar", data, -1)

	require.Nil(t, err)
	require.Equal(t, int64(3), size)
	require.Equal(t, false, created)

	cur, max = fs.Size()

	require.Equal(t, int64(3), cur)
	require.Equal(t, int64(-1), max)

	cur = fs.Files()

	require.Equal(t, int64(1), cur)
}

func testList(t *testing.T, fs Filesystem) {
	fs.WriteFileReader("/foobar1", strings.NewReader("a"), -1)
	fs.WriteFileReader("/foobar2", strings.NewReader("bb"), -1)
	fs.WriteFileReader("/foobar3", strings.NewReader("ccc"), -1)
	fs.WriteFileReader("/foobar4", strings.NewReader("dddd"), -1)
	fs.WriteFileReader("/path/foobar3", strings.NewReader("ccc"), -1)
	fs.WriteFileReader("/path/to/foobar4", strings.NewReader("dddd"), -1)

	cur, max := fs.Size()

	require.Equal(t, int64(17), cur)
	require.Equal(t, int64(-1), max)

	cur = fs.Files()

	require.Equal(t, int64(6), cur)

	getNames := func(files []FileInfo) []string {
		names := []string{}
		for _, f := range files {
			names = append(names, f.Name())
		}
		return names
	}

	files := fs.List("/", ListOptions{})

	require.Equal(t, 6, len(files))
	require.ElementsMatch(t, []string{"/foobar1", "/foobar2", "/foobar3", "/foobar4", "/path/foobar3", "/path/to/foobar4"}, getNames(files))

	files = fs.List("/path", ListOptions{})

	require.Equal(t, 2, len(files))
	require.ElementsMatch(t, []string{"/path/foobar3", "/path/to/foobar4"}, getNames(files))
}

func testListGlob(t *testing.T, fs Filesystem) {
	fs.WriteFileReader("/foobar1", strings.NewReader("a"), -1)
	fs.WriteFileReader("/path/foobar2", strings.NewReader("a"), -1)
	fs.WriteFileReader("/path/to/foobar3", strings.NewReader("a"), -1)
	fs.WriteFileReader("/foobar4", strings.NewReader("a"), -1)

	cur := fs.Files()

	require.Equal(t, int64(4), cur)

	getNames := func(files []FileInfo) []string {
		names := []string{}
		for _, f := range files {
			names = append(names, f.Name())
		}
		return names
	}

	files := getNames(fs.List("/", ListOptions{Pattern: "/foo*"}))
	require.Equal(t, 2, len(files))
	require.ElementsMatch(t, []string{"/foobar1", "/foobar4"}, files)

	files = getNames(fs.List("/", ListOptions{Pattern: "/*bar?"}))
	require.Equal(t, 2, len(files))
	require.ElementsMatch(t, []string{"/foobar1", "/foobar4"}, files)

	files = getNames(fs.List("/", ListOptions{Pattern: "/path/*"}))
	require.Equal(t, 1, len(files))
	require.ElementsMatch(t, []string{"/path/foobar2"}, files)

	files = getNames(fs.List("/", ListOptions{Pattern: "/path/**"}))
	require.Equal(t, 2, len(files))
	require.ElementsMatch(t, []string{"/path/foobar2", "/path/to/foobar3"}, files)

	files = getNames(fs.List("/path", ListOptions{Pattern: "/**"}))
	require.Equal(t, 2, len(files))
	require.ElementsMatch(t, []string{"/path/foobar2", "/path/to/foobar3"}, files)
}

func testListSize(t *testing.T, fs Filesystem) {
	fs.WriteFileReader("/a", strings.NewReader("a"), -1)
	fs.WriteFileReader("/aa", strings.NewReader("aa"), -1)
	fs.WriteFileReader("/aaa", strings.NewReader("aaa"), -1)
	fs.WriteFileReader("/aaaa", strings.NewReader("aaaa"), -1)

	cur := fs.Files()

	require.Equal(t, int64(4), cur)

	getNames := func(files []FileInfo) []string {
		names := []string{}
		for _, f := range files {
			names = append(names, f.Name())
		}
		return names
	}

	files := getNames(fs.List("/", ListOptions{SizeMin: 1}))
	require.Equal(t, 4, len(files))
	require.ElementsMatch(t, []string{"/a", "/aa", "/aaa", "/aaaa"}, files)

	files = getNames(fs.List("/", ListOptions{SizeMin: 2}))
	require.Equal(t, 3, len(files))
	require.ElementsMatch(t, []string{"/aa", "/aaa", "/aaaa"}, files)

	files = getNames(fs.List("/", ListOptions{SizeMax: 4}))
	require.Equal(t, 4, len(files))
	require.ElementsMatch(t, []string{"/a", "/aa", "/aaa", "/aaaa"}, files)

	files = getNames(fs.List("/", ListOptions{SizeMin: 2, SizeMax: 3}))
	require.Equal(t, 2, len(files))
	require.ElementsMatch(t, []string{"/aa", "/aaa"}, files)
}

func testListModified(t *testing.T, fs Filesystem) {
	fs.WriteFileReader("/a", strings.NewReader("a"), -1)
	time.Sleep(500 * time.Millisecond)
	fs.WriteFileReader("/b", strings.NewReader("b"), -1)
	time.Sleep(500 * time.Millisecond)
	fs.WriteFileReader("/c", strings.NewReader("c"), -1)
	time.Sleep(500 * time.Millisecond)
	fs.WriteFileReader("/d", strings.NewReader("d"), -1)

	cur := fs.Files()

	require.Equal(t, int64(4), cur)

	getNames := func(files []FileInfo) []string {
		names := []string{}
		for _, f := range files {
			names = append(names, f.Name())
		}
		return names
	}

	var a, b, c, d time.Time

	for _, f := range fs.List("/", ListOptions{}) {
		if f.Name() == "/a" {
			a = f.ModTime()
		} else if f.Name() == "/b" {
			b = f.ModTime()
		} else if f.Name() == "/c" {
			c = f.ModTime()
		} else if f.Name() == "/d" {
			d = f.ModTime()
		}
	}

	files := getNames(fs.List("/", ListOptions{ModifiedStart: &a}))
	require.Equal(t, 4, len(files))
	require.ElementsMatch(t, []string{"/a", "/b", "/c", "/d"}, files)

	files = getNames(fs.List("/", ListOptions{ModifiedStart: &b}))
	require.Equal(t, 3, len(files))
	require.ElementsMatch(t, []string{"/b", "/c", "/d"}, files)

	files = getNames(fs.List("/", ListOptions{ModifiedEnd: &d}))
	require.Equal(t, 4, len(files))
	require.ElementsMatch(t, []string{"/a", "/b", "/c", "/d"}, files)

	files = getNames(fs.List("/", ListOptions{ModifiedStart: &b, ModifiedEnd: &c}))
	require.Equal(t, 2, len(files))
	require.ElementsMatch(t, []string{"/b", "/c"}, files)
}

func testRemoveAll(t *testing.T, fs Filesystem) {
	fs.WriteFileReader("/foobar1", strings.NewReader("abc"), -1)
	fs.WriteFileReader("/path/foobar2", strings.NewReader("abc"), -1)
	fs.WriteFileReader("/path/to/foobar3", strings.NewReader("abc"), -1)
	fs.WriteFileReader("/foobar4", strings.NewReader("abc"), -1)

	cur := fs.Files()

	require.Equal(t, int64(4), cur)

	_, size := fs.RemoveList("/", ListOptions{
		Pattern: "",
	})
	require.Equal(t, int64(12), size)

	cur = fs.Files()

	require.Equal(t, int64(0), cur)
}

func testRemoveList(t *testing.T, fs Filesystem) {
	fs.WriteFileReader("/foobar1", strings.NewReader("abc"), -1)
	fs.WriteFileReader("/path/foobar2", strings.NewReader("abc"), -1)
	fs.WriteFileReader("/path/to/foobar3", strings.NewReader("abc"), -1)
	fs.WriteFileReader("/foobar4", strings.NewReader("abc"), -1)

	cur := fs.Files()

	require.Equal(t, int64(4), cur)

	_, size := fs.RemoveList("/", ListOptions{
		Pattern: "/path/**",
	})
	require.Equal(t, int64(6), size)

	cur = fs.Files()

	require.Equal(t, int64(2), cur)
}

func testData(t *testing.T, fs Filesystem) {
	file := fs.Open("/foobar")
	require.Nil(t, file)

	_, err := fs.ReadFile("/foobar")
	require.Error(t, err)

	data := "gduwotoxqb"

	data1 := strings.NewReader(data)

	_, _, err = fs.WriteFileReader("/foobar", data1, -1)
	require.NoError(t, err)

	file = fs.Open("/foobar")
	require.NotNil(t, file)

	data2 := make([]byte, len(data)+1)
	n, err := file.Read(data2)
	if err != nil {
		if err != io.EOF {
			require.NoError(t, err)
		}
	}

	require.Equal(t, len(data), n)
	require.Equal(t, []byte(data), data2[:n])

	data3, err := fs.ReadFile("/foobar")

	require.NoError(t, err)
	require.Equal(t, []byte(data), data3)
}

func testStatDir(t *testing.T, fs Filesystem) {
	info, err := fs.Stat("/")
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, true, info.IsDir())

	fs.WriteFileReader("/these/are/some/directories/foobar", strings.NewReader("gduwotoxqb"), -1)

	info, err = fs.Stat("/foobar")
	require.Error(t, err)
	require.Nil(t, info)

	info, err = fs.Stat("/these/are/some/directories/foobar")
	require.NoError(t, err)
	require.Equal(t, "/these/are/some/directories/foobar", info.Name())
	require.Equal(t, int64(10), info.Size())
	require.Equal(t, false, info.IsDir())

	info, err = fs.Stat("/these")
	require.NoError(t, err)
	require.Equal(t, "/these", info.Name())
	require.Equal(t, int64(0), info.Size())
	require.Equal(t, true, info.IsDir())

	info, err = fs.Stat("/these/are/")
	require.NoError(t, err)
	require.Equal(t, "/these/are", info.Name())
	require.Equal(t, int64(0), info.Size())
	require.Equal(t, true, info.IsDir())

	info, err = fs.Stat("/these/are/some")
	require.NoError(t, err)
	require.Equal(t, "/these/are/some", info.Name())
	require.Equal(t, int64(0), info.Size())
	require.Equal(t, true, info.IsDir())

	info, err = fs.Stat("/these/are/some/directories")
	require.NoError(t, err)
	require.Equal(t, "/these/are/some/directories", info.Name())
	require.Equal(t, int64(0), info.Size())
	require.Equal(t, true, info.IsDir())
}

func testMkdirAll(t *testing.T, fs Filesystem) {
	info, err := fs.Stat("/foo/bar/dir")
	require.Error(t, err)
	require.Nil(t, info)

	err = fs.MkdirAll("/foo/bar/dir", 0755)
	require.NoError(t, err)

	err = fs.MkdirAll("/foo/bar", 0755)
	require.NoError(t, err)

	info, err = fs.Stat("/foo/bar/dir")
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, int64(0), info.Size())
	require.Equal(t, true, info.IsDir())

	info, err = fs.Stat("/")
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, int64(0), info.Size())
	require.Equal(t, true, info.IsDir())

	info, err = fs.Stat("/foo")
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, int64(0), info.Size())
	require.Equal(t, true, info.IsDir())

	info, err = fs.Stat("/foo/bar")
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, int64(0), info.Size())
	require.Equal(t, true, info.IsDir())

	_, _, err = fs.WriteFileReader("/foobar", strings.NewReader("gduwotoxqb"), -1)
	require.NoError(t, err)

	err = fs.MkdirAll("/foobar", 0755)
	require.Error(t, err)
}

func testRename(t *testing.T, fs Filesystem) {
	err := fs.Rename("/foobar", "/foobaz")
	require.Error(t, err)

	_, err = fs.Stat("/foobar")
	require.Error(t, err)

	_, err = fs.Stat("/foobaz")
	require.Error(t, err)

	_, _, err = fs.WriteFileReader("/foobar", strings.NewReader("gduwotoxqb"), -1)
	require.NoError(t, err)

	_, err = fs.Stat("/foobar")
	require.NoError(t, err)

	err = fs.Rename("/foobar", "/foobaz")
	require.NoError(t, err)

	_, err = fs.Stat("/foobar")
	require.Error(t, err)

	_, err = fs.Stat("/foobaz")
	require.NoError(t, err)
}

func testRenameOverwrite(t *testing.T, fs Filesystem) {
	_, err := fs.Stat("/foobar")
	require.Error(t, err)

	_, err = fs.Stat("/foobaz")
	require.Error(t, err)

	_, _, err = fs.WriteFileReader("/foobar", strings.NewReader("foobar"), -1)
	require.NoError(t, err)

	_, _, err = fs.WriteFileReader("/foobaz", strings.NewReader("foobaz"), -1)
	require.NoError(t, err)

	_, err = fs.Stat("/foobar")
	require.NoError(t, err)

	_, err = fs.Stat("/foobaz")
	require.NoError(t, err)

	err = fs.Rename("/foobar", "/foobaz")
	require.NoError(t, err)

	_, err = fs.Stat("/foobar")
	require.Error(t, err)

	_, err = fs.Stat("/foobaz")
	require.NoError(t, err)

	data, err := fs.ReadFile("/foobaz")
	require.NoError(t, err)
	require.Equal(t, "foobar", string(data))
}

func testSymlink(t *testing.T, fs Filesystem) {
	if _, ok := fs.(*s3Filesystem); ok {
		return
	}

	err := fs.Symlink("/foobar", "/foobaz")
	require.Error(t, err)

	_, _, err = fs.WriteFileReader("/foobar", strings.NewReader("foobar"), -1)
	require.NoError(t, err)

	err = fs.Symlink("/foobar", "/foobaz")
	require.NoError(t, err)

	file := fs.Open("/foobaz")
	require.NotNil(t, file)
	require.Equal(t, "/foobaz", file.Name())

	data := make([]byte, 10)
	n, err := file.Read(data)
	if err != nil {
		if err != io.EOF {
			require.NoError(t, err)
		}
	}
	require.NoError(t, err)
	require.Equal(t, 6, n)
	require.Equal(t, "foobar", string(data[:n]))

	stat, err := fs.Stat("/foobaz")
	require.NoError(t, err)
	require.Equal(t, "/foobaz", stat.Name())
	require.Equal(t, int64(6), stat.Size())
	require.NotEqual(t, 0, int(stat.Mode()&os.ModeSymlink))

	link, ok := stat.IsLink()
	require.Equal(t, "/foobar", link)
	require.Equal(t, true, ok)

	data, err = fs.ReadFile("/foobaz")
	require.NoError(t, err)
	require.Equal(t, "foobar", string(data))
}

func testSymlinkOpenStat(t *testing.T, fs Filesystem) {
	if _, ok := fs.(*s3Filesystem); ok {
		return
	}

	_, _, err := fs.WriteFileReader("/foobar", strings.NewReader("foobar"), -1)
	require.NoError(t, err)

	err = fs.Symlink("/foobar", "/foobaz")
	require.NoError(t, err)

	file := fs.Open("/foobaz")
	require.NotNil(t, file)
	require.Equal(t, "/foobaz", file.Name())

	fstat, err := file.Stat()
	require.NoError(t, err)

	stat, err := fs.Stat("/foobaz")
	require.NoError(t, err)

	require.Equal(t, "/foobaz", fstat.Name())
	require.Equal(t, fstat.Name(), stat.Name())

	require.Equal(t, int64(6), fstat.Size())
	require.Equal(t, fstat.Size(), stat.Size())

	require.NotEqual(t, 0, int(fstat.Mode()&os.ModeSymlink))
	require.Equal(t, fstat.Mode(), stat.Mode())
}

func testStat(t *testing.T, fs Filesystem) {
	_, _, err := fs.WriteFileReader("/foobar", strings.NewReader("foobar"), -1)
	require.NoError(t, err)

	file := fs.Open("/foobar")
	require.NotNil(t, file)

	stat1, err := fs.Stat("/foobar")
	require.NoError(t, err)

	stat2, err := file.Stat()
	require.NoError(t, err)

	require.Equal(t, stat1, stat2)
}

func testCopy(t *testing.T, fs Filesystem) {
	err := fs.Rename("/foobar", "/foobaz")
	require.Error(t, err)

	_, err = fs.Stat("/foobar")
	require.Error(t, err)

	_, err = fs.Stat("/foobaz")
	require.Error(t, err)

	_, _, err = fs.WriteFileReader("/foobar", strings.NewReader("gduwotoxqb"), -1)
	require.NoError(t, err)

	_, err = fs.Stat("/foobar")
	require.NoError(t, err)

	err = fs.Copy("/foobar", "/foobaz")
	require.NoError(t, err)

	_, err = fs.Stat("/foobar")
	require.NoError(t, err)

	_, err = fs.Stat("/foobaz")
	require.NoError(t, err)
}

func testCopyOverwrite(t *testing.T, fs Filesystem) {
	_, err := fs.Stat("/foobar")
	require.Error(t, err)

	_, err = fs.Stat("/foobaz")
	require.Error(t, err)

	_, _, err = fs.WriteFileReader("/foobar", strings.NewReader("foobar"), -1)
	require.NoError(t, err)

	_, _, err = fs.WriteFileReader("/foobaz", strings.NewReader("foobaz"), -1)
	require.NoError(t, err)

	_, err = fs.Stat("/foobar")
	require.NoError(t, err)

	_, err = fs.Stat("/foobaz")
	require.NoError(t, err)

	err = fs.Copy("/foobar", "/foobaz")
	require.NoError(t, err)

	_, err = fs.Stat("/foobar")
	require.NoError(t, err)

	_, err = fs.Stat("/foobaz")
	require.NoError(t, err)

	data, err := fs.ReadFile("/foobaz")
	require.NoError(t, err)
	require.Equal(t, "foobar", string(data))
}

func testSymlinkErrors(t *testing.T, fs Filesystem) {
	if _, ok := fs.(*s3Filesystem); ok {
		return
	}

	err := fs.Symlink("/foobar", "/foobaz")
	require.Error(t, err)

	_, _, err = fs.WriteFileReader("/foobar", strings.NewReader("foobar"), -1)
	require.NoError(t, err)

	_, _, err = fs.WriteFileReader("/foobaz", strings.NewReader("foobaz"), -1)
	require.NoError(t, err)

	err = fs.Symlink("/foobar", "/foobaz")
	require.Error(t, err)

	err = fs.Symlink("/foobar", "/bazfoo")
	require.NoError(t, err)

	err = fs.Symlink("/bazfoo", "/barfoo")
	require.Error(t, err)
}
