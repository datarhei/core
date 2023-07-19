package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/cache"
	httpfs "github.com/datarhei/core/v16/http/fs"
	"github.com/datarhei/core/v16/http/handler"
	"github.com/datarhei/core/v16/http/mock"
	"github.com/datarhei/core/v16/io/fs"
	"github.com/stretchr/testify/require"

	"github.com/labstack/echo/v4"
)

func getDummyFilesystemsHandler(filesystems []httpfs.FS) (*FSHandler, error) {
	config := map[string]FSConfig{}

	for _, fs := range filesystems {
		config[fs.Name] = FSConfig{
			Type:       fs.Filesystem.Type(),
			Mountpoint: fs.Mountpoint,
			Handler:    handler.NewFS(fs),
		}
	}
	handler := NewFS(config)

	return handler, nil
}

func getDummyFilesystemsRouter(filesystems []httpfs.FS) (*echo.Echo, error) {
	router := mock.DummyEcho()

	handler, err := getDummyFilesystemsHandler(filesystems)
	if err != nil {
		return nil, err
	}

	router.GET("/:storage/*", handler.GetFile)
	router.PUT("/:storage/*", handler.PutFile)
	router.DELETE("/:storage", handler.DeleteFiles)
	router.DELETE("/:storage/*", handler.DeleteFile)
	router.GET("/:storage", handler.ListFiles)
	router.GET("/", handler.List)
	router.PUT("/", handler.FileOperation)

	return router, nil
}

func TestFilesystems(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	filesystems := []httpfs.FS{
		{
			Name:       "foo",
			Mountpoint: "/",
			AllowWrite: true,
			Filesystem: memfs,
		},
	}

	router, err := getDummyFilesystemsRouter(filesystems)
	require.NoError(t, err)

	response := mock.Request(t, http.StatusOK, router, "GET", "/", nil)

	mock.Validate(t, &[]api.FilesystemInfo{}, response.Data)

	f := []api.FilesystemInfo{}
	err = json.Unmarshal(response.Raw, &f)
	require.NoError(t, err)

	require.Equal(t, 1, len(f))

	mock.Request(t, http.StatusNotFound, router, "GET", "/bar", nil)

	response = mock.Request(t, http.StatusOK, router, "GET", "/foo", nil)

	mock.Validate(t, &[]api.FileInfo{}, response.Data)

	l := []api.FileInfo{}
	err = json.Unmarshal(response.Raw, &l)
	require.NoError(t, err)

	require.Equal(t, 0, len(l))

	mock.Request(t, http.StatusNotFound, router, "GET", "/bar/file", nil)
	mock.Request(t, http.StatusNotFound, router, "GET", "/foo/file", nil)

	data := mock.Read(t, "./fixtures/addProcess.json")
	require.NoError(t, err)

	mock.Request(t, http.StatusCreated, router, "PUT", "/foo/file", data)
	mock.Request(t, http.StatusNotFound, router, "PUT", "/bar/file", data)

	data = mock.Read(t, "./fixtures/addProcess.json")
	require.NoError(t, err)

	mock.Request(t, http.StatusNoContent, router, "PUT", "/foo/file", data)

	require.Equal(t, 1, len(memfs.List("/", fs.ListOptions{})))

	response = mock.Request(t, http.StatusOK, router, "GET", "/foo", nil)

	mock.Validate(t, &[]api.FileInfo{}, response.Data)

	l = []api.FileInfo{}
	err = json.Unmarshal(response.Raw, &l)
	require.NoError(t, err)

	require.Equal(t, 1, len(l))

	mock.Request(t, http.StatusNotFound, router, "GET", "/foo/elif", nil)

	response = mock.Request(t, http.StatusOK, router, "GET", "/foo/file", nil)

	databytes, err := io.ReadAll(mock.Read(t, "./fixtures/addProcess.json"))
	require.NoError(t, err)
	require.Equal(t, databytes, response.Raw)

	mock.Request(t, http.StatusNotFound, router, "DELETE", "/foo/elif", nil)
	mock.Request(t, http.StatusNotFound, router, "DELETE", "/bar/elif", nil)
	mock.Request(t, http.StatusOK, router, "DELETE", "/foo/file", nil)
	mock.Request(t, http.StatusNotFound, router, "GET", "/foo/file", nil)

	require.Equal(t, 0, len(memfs.List("/", fs.ListOptions{})))

	response = mock.Request(t, http.StatusOK, router, "GET", "/foo", nil)

	mock.Validate(t, &[]api.FileInfo{}, response.Data)

	l = []api.FileInfo{}
	err = json.Unmarshal(response.Raw, &l)
	require.NoError(t, err)

	require.Equal(t, 0, len(l))
}

func TestFilesystemsListSize(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	memfs.WriteFileReader("/a", strings.NewReader("a"))
	memfs.WriteFileReader("/aa", strings.NewReader("aa"))
	memfs.WriteFileReader("/aaa", strings.NewReader("aaa"))
	memfs.WriteFileReader("/aaaa", strings.NewReader("aaaa"))

	filesystems := []httpfs.FS{
		{
			Name:       "foo",
			Mountpoint: "/foo",
			AllowWrite: true,
			Filesystem: memfs,
		},
	}

	router, err := getDummyFilesystemsRouter(filesystems)
	require.NoError(t, err)

	response := mock.Request(t, http.StatusOK, router, "GET", "/foo", nil)

	f := []api.FilesystemInfo{}
	err = json.Unmarshal(response.Raw, &f)
	require.NoError(t, err)

	require.Equal(t, 4, len(f))

	getNames := func(r *mock.Response) []string {
		files := []api.FilesystemInfo{}
		err := json.Unmarshal(r.Raw, &files)
		require.NoError(t, err)

		names := []string{}
		for _, f := range files {
			names = append(names, f.Name)
		}
		return names
	}

	files := getNames(mock.Request(t, http.StatusOK, router, "GET", "/foo?size_min=1", nil))
	require.Equal(t, 4, len(files))
	require.ElementsMatch(t, []string{"/a", "/aa", "/aaa", "/aaaa"}, files)

	files = getNames(mock.Request(t, http.StatusOK, router, "GET", "/foo?size_min=2", nil))
	require.Equal(t, 3, len(files))
	require.ElementsMatch(t, []string{"/aa", "/aaa", "/aaaa"}, files)

	files = getNames(mock.Request(t, http.StatusOK, router, "GET", "/foo?size_max=4", nil))
	require.Equal(t, 4, len(files))
	require.ElementsMatch(t, []string{"/a", "/aa", "/aaa", "/aaaa"}, files)

	files = getNames(mock.Request(t, http.StatusOK, router, "GET", "/foo?size_min=2&size_max=3", nil))
	require.Equal(t, 2, len(files))
	require.ElementsMatch(t, []string{"/aa", "/aaa"}, files)
}

func TestFilesystemsListLastmod(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	memfs.WriteFileReader("/a", strings.NewReader("a"))
	time.Sleep(1 * time.Second)
	memfs.WriteFileReader("/b", strings.NewReader("b"))
	time.Sleep(1 * time.Second)
	memfs.WriteFileReader("/c", strings.NewReader("c"))
	time.Sleep(1 * time.Second)
	memfs.WriteFileReader("/d", strings.NewReader("d"))

	var a, b, c, d time.Time

	for _, f := range memfs.List("/", fs.ListOptions{}) {
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

	filesystems := []httpfs.FS{
		{
			Name:       "foo",
			Mountpoint: "/foo",
			AllowWrite: true,
			Filesystem: memfs,
		},
	}

	router, err := getDummyFilesystemsRouter(filesystems)
	require.NoError(t, err)

	getNames := func(r *mock.Response) []string {
		files := []api.FilesystemInfo{}
		err := json.Unmarshal(r.Raw, &files)
		require.NoError(t, err)

		names := []string{}
		for _, f := range files {
			names = append(names, f.Name)
		}
		return names
	}

	files := getNames(mock.Request(t, http.StatusOK, router, "GET", "/foo?lastmod_start="+strconv.FormatInt(a.Unix(), 10), nil))
	require.Equal(t, 4, len(files))
	require.ElementsMatch(t, []string{"/a", "/b", "/c", "/d"}, files)

	files = getNames(mock.Request(t, http.StatusOK, router, "GET", "/foo?lastmod_start="+strconv.FormatInt(b.Unix(), 10), nil))
	require.Equal(t, 3, len(files))
	require.ElementsMatch(t, []string{"/b", "/c", "/d"}, files)

	files = getNames(mock.Request(t, http.StatusOK, router, "GET", "/foo?lastmod_end="+strconv.FormatInt(d.Unix(), 10), nil))
	require.Equal(t, 4, len(files))
	require.ElementsMatch(t, []string{"/a", "/b", "/c", "/d"}, files)

	files = getNames(mock.Request(t, http.StatusOK, router, "GET", "/foo?lastmod_start="+strconv.FormatInt(b.Unix(), 10)+"&lastmod_end="+strconv.FormatInt(c.Unix(), 10), nil))
	require.Equal(t, 2, len(files))
	require.ElementsMatch(t, []string{"/b", "/c"}, files)
}

func TestFilesystemsDeleteFiles(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	memfs.WriteFileReader("/a", strings.NewReader("a"))
	memfs.WriteFileReader("/aa", strings.NewReader("aa"))
	memfs.WriteFileReader("/aaa", strings.NewReader("aaa"))
	memfs.WriteFileReader("/aaaa", strings.NewReader("aaaa"))

	filesystems := []httpfs.FS{
		{
			Name:       "foo",
			Mountpoint: "/foo",
			AllowWrite: true,
			Filesystem: memfs,
		},
	}

	router, err := getDummyFilesystemsRouter(filesystems)
	require.NoError(t, err)

	mock.Request(t, http.StatusBadRequest, router, "DELETE", "/foo", nil)

	getNames := func(r *mock.Response) []string {
		files := []string{}
		err := json.Unmarshal(r.Raw, &files)
		require.NoError(t, err)

		return files
	}

	files := getNames(mock.Request(t, http.StatusOK, router, "DELETE", "/foo?glob=/**", nil))
	require.Equal(t, 4, len(files))
	require.ElementsMatch(t, []string{"/a", "/aa", "/aaa", "/aaaa"}, files)

	require.Equal(t, int64(0), memfs.Files())
}

func TestFilesystemsDeleteFilesSize(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	memfs.WriteFileReader("/a", strings.NewReader("a"))
	memfs.WriteFileReader("/aa", strings.NewReader("aa"))
	memfs.WriteFileReader("/aaa", strings.NewReader("aaa"))
	memfs.WriteFileReader("/aaaa", strings.NewReader("aaaa"))

	filesystems := []httpfs.FS{
		{
			Name:       "foo",
			Mountpoint: "/foo",
			AllowWrite: true,
			Filesystem: memfs,
		},
	}

	router, err := getDummyFilesystemsRouter(filesystems)
	require.NoError(t, err)

	getNames := func(r *mock.Response) []string {
		files := []string{}
		err := json.Unmarshal(r.Raw, &files)
		require.NoError(t, err)

		return files
	}

	files := getNames(mock.Request(t, http.StatusOK, router, "DELETE", "/foo?glob=/**&size_min=2&size_max=3", nil))
	require.Equal(t, 2, len(files))
	require.ElementsMatch(t, []string{"/aa", "/aaa"}, files)

	require.Equal(t, int64(2), memfs.Files())
}

func TestFilesystemsDeleteFilesLastmod(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	memfs.WriteFileReader("/a", strings.NewReader("a"))
	time.Sleep(1 * time.Second)
	memfs.WriteFileReader("/b", strings.NewReader("b"))
	time.Sleep(1 * time.Second)
	memfs.WriteFileReader("/c", strings.NewReader("c"))
	time.Sleep(1 * time.Second)
	memfs.WriteFileReader("/d", strings.NewReader("d"))

	var b, c time.Time

	for _, f := range memfs.List("/", fs.ListOptions{}) {
		if f.Name() == "/b" {
			b = f.ModTime()
		} else if f.Name() == "/c" {
			c = f.ModTime()
		}
	}

	filesystems := []httpfs.FS{
		{
			Name:       "foo",
			Mountpoint: "/foo",
			AllowWrite: true,
			Filesystem: memfs,
		},
	}

	router, err := getDummyFilesystemsRouter(filesystems)
	require.NoError(t, err)

	getNames := func(r *mock.Response) []string {
		files := []string{}
		err := json.Unmarshal(r.Raw, &files)
		require.NoError(t, err)

		return files
	}

	files := getNames(mock.Request(t, http.StatusOK, router, "DELETE", "/foo?glob=/**&lastmod_start="+strconv.FormatInt(b.Unix(), 10)+"&lastmod_end="+strconv.FormatInt(c.Unix(), 10), nil))
	require.Equal(t, 2, len(files))
	require.ElementsMatch(t, []string{"/b", "/c"}, files)

	require.Equal(t, int64(2), memfs.Files())
}

func TestFilesystemsFileOperation(t *testing.T) {
	memfs1, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	memfs2, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	filesystems := []httpfs.FS{
		{
			Name:       "foo",
			Mountpoint: "/foo",
			AllowWrite: true,
			Filesystem: memfs1,
		},
		{
			Name:       "bar",
			Mountpoint: "/bar",
			AllowWrite: true,
			Filesystem: memfs2,
		},
	}

	router, err := getDummyFilesystemsRouter(filesystems)
	require.NoError(t, err)

	mock.Request(t, http.StatusNotFound, router, "GET", "/foo/file", nil)
	mock.Request(t, http.StatusNotFound, router, "GET", "/bar/file", nil)

	data := mock.Read(t, "./fixtures/addProcess.json")
	require.NotNil(t, data)

	mock.Request(t, http.StatusCreated, router, "PUT", "/foo/file", data)
	mock.Request(t, http.StatusOK, router, "GET", "/foo/file", nil)
	mock.Request(t, http.StatusNotFound, router, "GET", "/bar/file", nil)

	op := api.FilesystemOperation{}

	jsondata, err := json.Marshal(op)
	require.NoError(t, err)

	mock.Request(t, http.StatusBadRequest, router, "PUT", "/", bytes.NewReader(jsondata))

	op = api.FilesystemOperation{
		Operation: "copy",
	}

	jsondata, err = json.Marshal(op)
	require.NoError(t, err)

	mock.Request(t, http.StatusBadRequest, router, "PUT", "/", bytes.NewReader(jsondata))

	op = api.FilesystemOperation{
		Operation: "copy",
		Source:    "foo:/elif",
	}

	jsondata, err = json.Marshal(op)
	require.NoError(t, err)

	mock.Request(t, http.StatusBadRequest, router, "PUT", "/", bytes.NewReader(jsondata))

	op = api.FilesystemOperation{
		Operation: "copy",
		Source:    "foo:/elif",
		Target:    "/bar",
	}

	jsondata, err = json.Marshal(op)
	require.NoError(t, err)

	mock.Request(t, http.StatusBadRequest, router, "PUT", "/", bytes.NewReader(jsondata))

	op = api.FilesystemOperation{
		Operation: "copy",
		Source:    "foo:/file",
		Target:    "/bar",
	}

	jsondata, err = json.Marshal(op)
	require.NoError(t, err)

	mock.Request(t, http.StatusBadRequest, router, "PUT", "/", bytes.NewReader(jsondata))

	op = api.FilesystemOperation{
		Operation: "copy",
		Source:    "foo:file",
		Target:    "bar:/file",
	}

	jsondata, err = json.Marshal(op)
	require.NoError(t, err)

	mock.Request(t, http.StatusOK, router, "PUT", "/", bytes.NewReader(jsondata))

	mock.Request(t, http.StatusOK, router, "GET", "/foo/file", nil)
	response := mock.Request(t, http.StatusOK, router, "GET", "/bar/file", nil)

	filedata, err := io.ReadAll(mock.Read(t, "./fixtures/addProcess.json"))
	require.NoError(t, err)

	require.Equal(t, filedata, response.Raw)

	op = api.FilesystemOperation{
		Operation: "move",
		Source:    "foo:file",
		Target:    "bar:/file",
	}

	jsondata, err = json.Marshal(op)
	require.NoError(t, err)

	mock.Request(t, http.StatusOK, router, "PUT", "/", bytes.NewReader(jsondata))

	mock.Request(t, http.StatusNotFound, router, "GET", "/foo/file", nil)
	response = mock.Request(t, http.StatusOK, router, "GET", "/bar/file", nil)

	filedata, err = io.ReadAll(mock.Read(t, "./fixtures/addProcess.json"))
	require.NoError(t, err)

	require.Equal(t, filedata, response.Raw)
}

func TestFilesystemsPurgeCache(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	memfs.MkdirAll("/path", 0755)

	cache, err := cache.NewLRUCache(cache.LRUConfig{
		TTL: 5 * time.Minute,
	})
	require.NoError(t, err)

	filesystems := []httpfs.FS{
		{
			Name:        "foo",
			Mountpoint:  "/foo",
			AllowWrite:  true,
			Filesystem:  memfs,
			DefaultFile: "index.html",
			Cache:       cache,
		},
	}

	router, err := getDummyFilesystemsRouter(filesystems)
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/addProcess.json")
	require.NotNil(t, data)

	mock.Request(t, http.StatusCreated, router, "PUT", "/foo/path/index.html", data)

	cache.Put("/path/", "foobar", 42)
	cache.Put("/path/index.html", "barfoo", 42)

	mock.Request(t, http.StatusOK, router, "DELETE", "/foo/path/index.html", nil)

	f, _, err := cache.Get("/path/")
	require.Nil(t, f)
	require.NoError(t, err)

	f, _, err = cache.Get("/path/index.html")
	require.Nil(t, f)
	require.NoError(t, err)

	cache.Put("/path/", "foobar", 42)
	cache.Put("/path/index.html", "barfoo", 42)

	data = mock.Read(t, "./fixtures/addProcess.json")
	require.NotNil(t, data)

	mock.Request(t, http.StatusCreated, router, "PUT", "/foo/path/index.html", data)

	f, _, err = cache.Get("/path/")
	require.Nil(t, f)
	require.NoError(t, err)

	f, _, err = cache.Get("/path/index.html")
	require.Nil(t, f)
	require.NoError(t, err)
}
