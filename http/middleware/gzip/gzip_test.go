package gzip

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/klauspost/compress/gzip"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGzip(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Skip if no Accept-Encoding header
	h := New()(func(c echo.Context) error {
		c.Response().Write([]byte("test")) // For Content-Type sniffing
		return nil
	})
	h(c)

	assert := assert.New(t)

	assert.Equal("test", rec.Body.String())

	// Gzip
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(echo.HeaderAcceptEncoding, gzipScheme)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	h(c)
	assert.Equal(gzipScheme, rec.Header().Get(echo.HeaderContentEncoding))
	assert.Contains(rec.Header().Get(echo.HeaderContentType), echo.MIMETextPlain)
	r, err := gzip.NewReader(rec.Body)
	if assert.NoError(err) {
		buf := new(bytes.Buffer)
		defer r.Close()
		buf.ReadFrom(r)
		assert.Equal("test", buf.String())
	}

	chunkBuf := make([]byte, 5)

	// Gzip chunked
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(echo.HeaderAcceptEncoding, gzipScheme)
	rec = httptest.NewRecorder()

	c = e.NewContext(req, rec)
	New()(func(c echo.Context) error {
		c.Response().Header().Set("Content-Type", "text/event-stream")
		c.Response().Header().Set("Transfer-Encoding", "chunked")

		// Write and flush the first part of the data
		c.Response().Write([]byte("test\n"))
		c.Response().Flush()

		// Read the first part of the data
		assert.True(rec.Flushed)
		assert.Equal(gzipScheme, rec.Header().Get(echo.HeaderContentEncoding))
		r.Reset(rec.Body)

		_, err = io.ReadFull(r, chunkBuf)
		assert.NoError(err)
		assert.Equal("test\n", string(chunkBuf))

		// Write and flush the second part of the data
		c.Response().Write([]byte("test\n"))
		c.Response().Flush()

		_, err = io.ReadFull(r, chunkBuf)
		assert.NoError(err)
		assert.Equal("test\n", string(chunkBuf))

		// Write the final part of the data and return
		c.Response().Write([]byte("test"))
		return nil
	})(c)

	buf := new(bytes.Buffer)
	defer r.Close()
	buf.ReadFrom(r)
	assert.Equal("test", buf.String())
}

func TestGzipWithMinLength(t *testing.T) {
	e := echo.New()
	// Invalid level
	e.Use(NewWithConfig(Config{MinLength: 5}))
	e.GET("/", func(c echo.Context) error {
		c.Response().Write([]byte("test"))
		return nil
	})
	e.GET("/foobar", func(c echo.Context) error {
		c.Response().Write([]byte("foobar"))
		return nil
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(echo.HeaderAcceptEncoding, gzipScheme)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, "", rec.Header().Get(echo.HeaderContentEncoding))
	assert.Contains(t, rec.Body.String(), "test")

	req = httptest.NewRequest(http.MethodGet, "/foobar", nil)
	req.Header.Set(echo.HeaderAcceptEncoding, gzipScheme)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, gzipScheme, rec.Header().Get(echo.HeaderContentEncoding))
	r, err := gzip.NewReader(rec.Body)
	if assert.NoError(t, err) {
		buf := new(bytes.Buffer)
		defer r.Close()
		buf.ReadFrom(r)
		assert.Equal(t, "foobar", buf.String())
	}
}

func TestGzipNoContent(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(echo.HeaderAcceptEncoding, gzipScheme)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	h := New()(func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})
	if assert.NoError(t, h(c)) {
		assert.Empty(t, rec.Header().Get(echo.HeaderContentEncoding))
		assert.Empty(t, rec.Header().Get(echo.HeaderContentType))
		assert.Equal(t, 0, len(rec.Body.Bytes()))
	}
}

func TestGzipEmpty(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(echo.HeaderAcceptEncoding, gzipScheme)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	h := New()(func(c echo.Context) error {
		return c.String(http.StatusOK, "")
	})
	if assert.NoError(t, h(c)) {
		assert.Equal(t, gzipScheme, rec.Header().Get(echo.HeaderContentEncoding))
		assert.Equal(t, "text/plain; charset=UTF-8", rec.Header().Get(echo.HeaderContentType))
		r, err := gzip.NewReader(rec.Body)
		if assert.NoError(t, err) {
			var buf bytes.Buffer
			buf.ReadFrom(r)
			assert.Equal(t, "", buf.String())
		}
	}
}

func TestGzipErrorReturned(t *testing.T) {
	e := echo.New()
	e.Use(New())
	e.GET("/", func(c echo.Context) error {
		return echo.ErrNotFound
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(echo.HeaderAcceptEncoding, gzipScheme)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, rec.Header().Get(echo.HeaderContentEncoding))
}

func TestGzipErrorReturnedInvalidConfig(t *testing.T) {
	e := echo.New()
	// Invalid level
	e.Use(NewWithConfig(Config{Level: 12}))
	e.GET("/", func(c echo.Context) error {
		c.Response().Write([]byte("test"))
		return nil
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(echo.HeaderAcceptEncoding, gzipScheme)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "gzip")
}

// Issue #806
func TestGzipWithStatic(t *testing.T) {
	e := echo.New()
	e.Use(New())
	e.Static("/test", "./")
	req := httptest.NewRequest(http.MethodGet, "/test/gzip.go", nil)
	req.Header.Set(echo.HeaderAcceptEncoding, gzipScheme)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	// Data is written out in chunks when Content-Length == "", so only
	// validate the content length if it's not set.
	if cl := rec.Header().Get("Content-Length"); cl != "" {
		assert.Equal(t, cl, rec.Body.Len())
	}
	r, err := gzip.NewReader(rec.Body)
	if assert.NoError(t, err) {
		defer r.Close()
		want, err := os.ReadFile("./gzip.go")
		if assert.NoError(t, err) {
			buf := new(bytes.Buffer)
			buf.ReadFrom(r)
			assert.Equal(t, want, buf.Bytes())
		}
	}
}

func BenchmarkGzip(b *testing.B) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(echo.HeaderAcceptEncoding, gzipScheme)

	h := New()(func(c echo.Context) error {
		c.Response().Write([]byte("test")) // For Content-Type sniffing
		return nil
	})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Gzip
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		h(c)
	}
}

func BenchmarkGzipLarge(b *testing.B) {
	data, err := os.ReadFile("./fixtures/processList.json")
	require.NoError(b, err)

	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(echo.HeaderAcceptEncoding, gzipScheme)

	h := New()(func(c echo.Context) error {
		c.Response().Write(data) // For Content-Type sniffing
		return nil
	})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Gzip
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		h(c)
	}
}
