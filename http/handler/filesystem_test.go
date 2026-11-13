package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpfs "github.com/datarhei/core/v16/http/fs"
	"github.com/datarhei/core/v16/http/mock"
	"github.com/datarhei/core/v16/io/fs"

	"github.com/stretchr/testify/require"
)

func TestContentDisposition(t *testing.T) {
	tests := map[string]string{
		"foo.mp4":            "foo.mp4",
		"foo bar.mp4":        "foo%20bar.mp4",
		"Grüße.mp4":          "Gr%C3%BC%C3%9Fe.mp4",
		`a"b;c'd%e*f/g\h`:    "a%22b%3Bc%27d%25e%2Af%2Fg%5Ch",
		"!#$&+-.^_`|~":       "!#$&+-.^_`|~",
		"az09AZ":             "az09AZ",
		"a\nb\x00c":          "a%0Ab%00c",
		"a\xffb":             "a%EF%BF%BDb",
		"(a)<b>@c,d:e=f?g{}": "%28a%29%3Cb%3E%40c%2Cd%3Ae%3Df%3Fg%7B%7D",
	}

	for filename, encoded := range tests {
		require.Equal(t, "attachment; filename*=UTF-8''"+encoded, contentDisposition(filename), filename)
	}
}

func TestGetFileFilename(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	_, _, err = memfs.WriteFileReader("/file.txt", strings.NewReader("0123456789"), -1)
	require.NoError(t, err)

	handler := NewFS(httpfs.FS{
		Name:       "mem",
		Mountpoint: "/",
		Filesystem: memfs,
	})

	router := mock.DummyEcho()
	router.GET("/*", handler.GetFile)
	router.HEAD("/*", handler.GetFile)

	request := func(method, target string, header http.Header) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil)
		for key, values := range header {
			req.Header[key] = values
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	rec := request(http.MethodGet, "/file.txt?filename=foo.txt", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "attachment; filename*=UTF-8''foo.txt", rec.Header().Get("Content-Disposition"))
	require.Equal(t, "0123456789", rec.Body.String())

	rec = request(http.MethodHead, "/file.txt?filename=foo.txt", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "attachment; filename*=UTF-8''foo.txt", rec.Header().Get("Content-Disposition"))

	rec = request(http.MethodGet, "/file.txt?filename=foo.txt", http.Header{"Range": []string{"bytes=2-4"}})
	require.Equal(t, http.StatusPartialContent, rec.Code)
	require.Equal(t, "attachment; filename*=UTF-8''foo.txt", rec.Header().Get("Content-Disposition"))
	require.Equal(t, "234", rec.Body.String())

	rec = request(http.MethodGet, "/file.txt?filename=Gr%C3%BC%C3%9Fe%20x%2Btxt", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "attachment; filename*=UTF-8''Gr%C3%BC%C3%9Fe%20x+txt", rec.Header().Get("Content-Disposition"))

	rec = request(http.MethodGet, "/file.txt", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, rec.Header().Values("Content-Disposition"))

	rec = request(http.MethodGet, "/file.txt?filename=", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, rec.Header().Values("Content-Disposition"))

	rec = request(http.MethodGet, "/missing.txt?filename=foo.txt", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Empty(t, rec.Header().Values("Content-Disposition"))
}
