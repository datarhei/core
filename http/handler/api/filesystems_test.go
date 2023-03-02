package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/datarhei/core/v16/http/api"
	httpfs "github.com/datarhei/core/v16/http/fs"
	"github.com/datarhei/core/v16/http/handler"
	"github.com/datarhei/core/v16/http/mock"
	"github.com/datarhei/core/v16/io/fs"
	"github.com/stretchr/testify/require"

	"github.com/labstack/echo/v4"
)

func getDummyFilesystemsHandler(filesystem httpfs.FS) (*FSHandler, error) {
	handler := NewFS(map[string]FSConfig{
		filesystem.Name: {
			Type:       filesystem.Filesystem.Type(),
			Mountpoint: filesystem.Mountpoint,
			Handler:    handler.NewFS(filesystem),
		},
	})

	return handler, nil
}

func getDummyFilesystemsRouter(filesystem fs.Filesystem) (*echo.Echo, error) {
	router := mock.DummyEcho()

	fs := httpfs.FS{
		Name:               "foo",
		Mountpoint:         "/",
		AllowWrite:         true,
		EnableAuth:         false,
		Username:           "",
		Password:           "",
		DefaultFile:        "",
		DefaultContentType: "text/html",
		Gzip:               false,
		Filesystem:         filesystem,
		Cache:              nil,
	}

	handler, err := getDummyFilesystemsHandler(fs)
	if err != nil {
		return nil, err
	}

	router.GET("/:name/*", handler.GetFile)
	router.PUT("/:name/*", handler.PutFile)
	router.DELETE("/:name/*", handler.DeleteFile)
	router.GET("/:name", handler.ListFiles)
	router.GET("/", handler.List)

	return router, nil
}

func TestFilesystems(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	router, err := getDummyFilesystemsRouter(memfs)
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

	require.Equal(t, 1, len(memfs.List("/", "")))

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

	require.Equal(t, 0, len(memfs.List("/", "")))

	response = mock.Request(t, http.StatusOK, router, "GET", "/foo", nil)

	mock.Validate(t, &[]api.FileInfo{}, response.Data)

	l = []api.FileInfo{}
	err = json.Unmarshal(response.Raw, &l)
	require.NoError(t, err)

	require.Equal(t, 0, len(l))
}
