package middleware

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/http/middleware/compress"
	mwsession "github.com/datarhei/core/v16/http/middleware/session"
	"github.com/datarhei/core/v16/http/mock"
	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/net"
	"github.com/datarhei/core/v16/session"
	"github.com/labstack/echo/v4"

	"github.com/stretchr/testify/require"
)

func TestContentLength(t *testing.T) {
	router := mock.DummyEcho()

	fs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	fs.WriteFile("/segment.ts", []byte("nothing"))

	router.Add("GET", "/*", func(c echo.Context) error {
		path := util.PathWildcardParam(c)

		fstat, err := fs.Stat(path)
		if err != nil {
			return fmt.Errorf("file not found")
		}

		file := fs.Open(path)
		if file == nil {
			return fmt.Errorf("file not found")
		}

		c.Response().Header().Set("Content-Length", fmt.Sprintf("%d", fstat.Size()))

		return c.Stream(http.StatusOK, "application/data", file)
	})

	response := mock.Request(t, http.StatusOK, router, "GET", "/segment.ts", nil)

	require.Equal(t, "7", response.Header.Get("Content-Length"))
}

func TestContentLengthWithCompress(t *testing.T) {
	router := mock.DummyEcho()

	mwcompress := compress.NewWithConfig(compress.Config{
		MinLength: 10,
	})

	fs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	fs.WriteFile("/segment_1.ts", []byte("nothing"))
	fs.WriteFile("/segment_2.ts", []byte("really nothing"))

	router.Add("GET", "/*", func(c echo.Context) error {
		path := util.PathWildcardParam(c)

		fstat, err := fs.Stat(path)
		if err != nil {
			return fmt.Errorf("file not found")
		}

		file := fs.Open(path)
		if file == nil {
			return fmt.Errorf("file not found")
		}

		c.Response().Header().Set("Content-Length", fmt.Sprintf("%d", fstat.Size()))

		return c.Stream(http.StatusOK, "application/data", file)
	}, mwcompress)

	response := mock.RequestEx(t, http.StatusOK, router, "GET", "/segment_1.ts", nil, nil, true)
	require.Equal(t, "7", response.Header.Get("Content-Length"))
	require.Equal(t, 7, len(response.Raw))

	response = mock.RequestEx(t, http.StatusOK, router, "GET", "/segment_2.ts", nil, nil, true)
	require.Equal(t, "14", response.Header.Get("Content-Length"))
	require.Equal(t, 14, len(response.Raw))

	header := http.Header{}
	header.Set("Accept-Encoding", "gzip")

	response = mock.RequestEx(t, http.StatusOK, router, "GET", "/segment_1.ts", header, nil, true)
	require.Equal(t, "7", response.Header.Get("Content-Length"))
	require.Equal(t, 7, len(response.Raw))
	require.Equal(t, "", response.Header.Get("Content-Encoding"))

	response = mock.RequestEx(t, http.StatusOK, router, "GET", "/segment_2.ts", header, nil, true)
	require.Equal(t, "", response.Header.Get("Content-Length"))
	require.Equal(t, "gzip", response.Header.Get("Content-Encoding"))
}

func TestContentLengthWithSession(t *testing.T) {
	router := mock.DummyEcho()

	registry, _ := session.New(session.Config{})
	registry.Register("foo", session.CollectorConfig{
		Limiter: net.NewNullIPLimiter(),
	})

	collector := registry.Collector("foo")

	mwsession := mwsession.NewWithConfig(mwsession.Config{
		HLSEgressCollector: collector,
	})

	fs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	fs.WriteFile("/foo.m3u8", []byte("foo.ts"))
	fs.WriteFile("/bar.m3u8", []byte("bar_0.m3u8"))

	router.Add("GET", "/*", func(c echo.Context) error {
		path := util.PathWildcardParam(c)

		fstat, err := fs.Stat(path)
		if err != nil {
			return fmt.Errorf("file not found")
		}

		file := fs.Open(path)
		if file == nil {
			return fmt.Errorf("file not found")
		}

		c.Response().Header().Set("Content-Length", fmt.Sprintf("%d", fstat.Size()))

		return c.Stream(http.StatusOK, "application/data", file)
	}, mwsession)

	response := mock.RequestEx(t, http.StatusOK, router, "GET", "/foo.m3u8", nil, nil, true)
	require.Equal(t, "97", response.Header.Get("Content-Length"))
	require.Equal(t, 97, len(response.Raw))

	response = mock.RequestEx(t, http.StatusOK, router, "GET", "/bar.m3u8", nil, nil, true)
	require.Equal(t, "42", response.Header.Get("Content-Length"))
	require.Equal(t, 42, len(response.Raw))
}

func TestContentLengthWithCompressAndSession(t *testing.T) {
	router := mock.DummyEcho()

	mwcompress := compress.NewWithConfig(compress.Config{
		MinLength: 50,
	})

	registry, _ := session.New(session.Config{})
	registry.Register("foo", session.CollectorConfig{
		Limiter: net.NewNullIPLimiter(),
	})

	collector := registry.Collector("foo")

	mwsession := mwsession.NewWithConfig(mwsession.Config{
		HLSEgressCollector: collector,
	})

	fs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	fs.WriteFile("/foo.m3u8", []byte("foo.ts"))
	fs.WriteFile("/bar.m3u8", []byte("bar_0.m3u8"))

	router.Add("GET", "/*", func(c echo.Context) error {
		path := util.PathWildcardParam(c)

		fstat, err := fs.Stat(path)
		if err != nil {
			return fmt.Errorf("file not found")
		}

		file := fs.Open(path)
		if file == nil {
			return fmt.Errorf("file not found")
		}

		c.Response().Header().Set("Content-Length", fmt.Sprintf("%d", fstat.Size()))

		return c.Stream(http.StatusOK, "application/data", file)
	}, mwcompress, mwsession)

	response := mock.RequestEx(t, http.StatusOK, router, "GET", "/foo.m3u8", nil, nil, true)
	require.Equal(t, "97", response.Header.Get("Content-Length"))
	require.Equal(t, 97, len(response.Raw))

	response = mock.RequestEx(t, http.StatusOK, router, "GET", "/bar.m3u8", nil, nil, true)
	require.Equal(t, "42", response.Header.Get("Content-Length"))
	require.Equal(t, 42, len(response.Raw))

	header := http.Header{}
	header.Set("Accept-Encoding", "gzip")

	response = mock.RequestEx(t, http.StatusOK, router, "GET", "/foo.m3u8", header, nil, true)
	require.Equal(t, "", response.Header.Get("Content-Length"))
	require.Equal(t, 122, len(response.Raw))
	require.Equal(t, "gzip", response.Header.Get("Content-Encoding"))

	response = mock.RequestEx(t, http.StatusOK, router, "GET", "/bar.m3u8", header, nil, true)
	require.Equal(t, "42", response.Header.Get("Content-Length"))
	require.Equal(t, 42, len(response.Raw))
	require.Equal(t, "", response.Header.Get("Content-Encoding"))
}
