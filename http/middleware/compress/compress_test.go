package compress

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/datarhei/core/v16/math/rand"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type StrangeCloser interface {
	Close()
}

type Resetter interface {
	Reset(io.Reader) error
}

type ReadCloseResetter interface {
	io.Reader
	io.Closer
	Resetter
}

type nopReadCloseResetter struct {
	io.Reader
}

func (rcr *nopReadCloseResetter) Close() error {
	if closer, ok := rcr.Reader.(io.Closer); ok {
		return closer.Close()
	}

	if closer, ok := rcr.Reader.(StrangeCloser); ok {
		closer.Close()
		return nil
	}

	return nil
}

func (rcr *nopReadCloseResetter) Reset(r io.Reader) error {
	resetter, ok := rcr.Reader.(Resetter)
	if !ok {
		return nil
	}
	return resetter.Reset(r)
}

func getTestcases() map[string]func(r io.Reader) (ReadCloseResetter, error) {
	return map[string]func(r io.Reader) (ReadCloseResetter, error){
		"gzip": func(r io.Reader) (ReadCloseResetter, error) {
			return gzip.NewReader(r)
		},
		"br": func(r io.Reader) (ReadCloseResetter, error) {
			return &nopReadCloseResetter{brotli.NewReader(r)}, nil
		},
		"zstd": func(r io.Reader) (ReadCloseResetter, error) {
			reader, err := zstd.NewReader(r)
			return &nopReadCloseResetter{reader}, err
		},
	}
}

func TestCompress(t *testing.T) {
	schemes := getTestcases()

	for scheme, reader := range schemes {
		t.Run(scheme, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			ctx := e.NewContext(req, rec)

			// Skip if no Accept-Encoding header
			handler := NewWithConfig(Config{Schemes: []string{scheme}})(func(c echo.Context) error {
				c.Response().Write([]byte("test")) // For Content-Type sniffing
				return nil
			})
			handler(ctx)

			assert := assert.New(t)

			assert.Equal("test", rec.Body.String())

			// Compression
			req = httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(echo.HeaderAcceptEncoding, scheme)
			rec = httptest.NewRecorder()
			ctx = e.NewContext(req, rec)
			handler(ctx)
			assert.Equal(scheme, rec.Header().Get(echo.HeaderContentEncoding))
			assert.Contains(rec.Header().Get(echo.HeaderContentType), echo.MIMETextPlain)
			r, err := reader(rec.Body)
			if assert.NoError(err) {
				buf := &bytes.Buffer{}
				defer r.Close()
				buf.ReadFrom(r)
				assert.Equal("test", buf.String())
			}

			// Gzip chunked
			req = httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(echo.HeaderAcceptEncoding, scheme)
			rec = httptest.NewRecorder()

			ctx = e.NewContext(req, rec)
			NewWithConfig(Config{Schemes: []string{scheme}})(func(c echo.Context) error {
				c.Response().Header().Set("Content-Type", "text/event-stream")
				c.Response().Header().Set("Transfer-Encoding", "chunked")

				// Write and flush the first part of the data
				c.Response().Write([]byte("test\n"))
				c.Response().Flush()

				// Read the first part of the data
				assert.True(rec.Flushed)
				assert.Equal(scheme, rec.Header().Get(echo.HeaderContentEncoding))

				// Write and flush the second part of the data
				c.Response().Write([]byte("tost\n"))
				c.Response().Flush()

				// Write the final part of the data and return
				c.Response().Write([]byte("tast"))
				return nil
			})(ctx)

			buf := new(bytes.Buffer)
			r.Reset(rec.Body)
			defer r.Close()
			buf.ReadFrom(r)
			assert.Equal("test\ntost\ntast", buf.String())
		})
	}
}

func TestCompressWithPassthrough(t *testing.T) {
	schemes := getTestcases()

	for scheme, reader := range schemes {
		t.Run(scheme, func(t *testing.T) {
			e := echo.New()
			e.Use(NewWithConfig(Config{MinLength: 5, Schemes: []string{scheme}, ContentTypes: []string{"text/compress"}}))
			e.GET("/plain", func(c echo.Context) error {
				c.Response().Header().Set("Content-Type", "text/plain")
				c.Response().Write([]byte("testtest"))
				return nil
			})
			e.GET("/compress", func(c echo.Context) error {
				c.Response().Header().Set("Content-Type", "text/compress")
				c.Response().Write([]byte("testtest"))
				return nil
			})
			req := httptest.NewRequest(http.MethodGet, "/plain", nil)
			req.Header.Set(echo.HeaderAcceptEncoding, scheme)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			assert.Equal(t, "", rec.Header().Get(echo.HeaderContentEncoding))
			assert.Equal(t, rec.Body.String(), "testtest")

			req = httptest.NewRequest(http.MethodGet, "/compress", nil)
			req.Header.Set(echo.HeaderAcceptEncoding, scheme)
			rec = httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			assert.Equal(t, scheme, rec.Header().Get(echo.HeaderContentEncoding))
			r, err := reader(rec.Body)
			if assert.NoError(t, err) {
				buf := new(bytes.Buffer)
				defer r.Close()
				buf.ReadFrom(r)
				assert.Equal(t, "testtest", buf.String())
			}
		})
	}
}

func TestCompressWithMinLength(t *testing.T) {
	schemes := getTestcases()

	for scheme, reader := range schemes {
		t.Run(scheme, func(t *testing.T) {
			e := echo.New()
			e.Use(NewWithConfig(Config{MinLength: 5, Schemes: []string{scheme}}))
			e.GET("/", func(c echo.Context) error {
				c.Response().Write([]byte("test"))
				return nil
			})
			e.GET("/foobar", func(c echo.Context) error {
				c.Response().Write([]byte("foobar"))
				return nil
			})
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(echo.HeaderAcceptEncoding, scheme)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			assert.Equal(t, "", rec.Header().Get(echo.HeaderContentEncoding))
			assert.Equal(t, rec.Body.String(), "test")

			req = httptest.NewRequest(http.MethodGet, "/foobar", nil)
			req.Header.Set(echo.HeaderAcceptEncoding, scheme)
			rec = httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			assert.Equal(t, scheme, rec.Header().Get(echo.HeaderContentEncoding))
			r, err := reader(rec.Body)
			if assert.NoError(t, err) {
				buf := new(bytes.Buffer)
				defer r.Close()
				buf.ReadFrom(r)
				assert.Equal(t, "foobar", buf.String())
			}
		})
	}
}

func TestCompressWithAroundMinLength(t *testing.T) {
	schemes := getTestcases()
	minLength := 1000

	for scheme, reader := range schemes {
		for i := minLength - 64; i < minLength+64; i++ {
			name := fmt.Sprintf("%s-%d", scheme, i)

			t.Run(name, func(t *testing.T) {
				data := rand.Bytes(i)
				e := echo.New()
				e.Use(NewWithConfig(Config{MinLength: minLength, Schemes: []string{scheme}}))
				e.GET("/", func(c echo.Context) error {
					c.Response().Write(data[:1])
					c.Response().Write(data[1:])
					return nil
				})
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set(echo.HeaderAcceptEncoding, scheme)
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)

				if i < minLength {
					assert.Equal(t, "", rec.Header().Get(echo.HeaderContentEncoding))
					res, err := io.ReadAll(rec.Body)
					if assert.NoError(t, err) {
						assert.Equal(t, data, res)
					}
				} else {
					assert.Equal(t, scheme, rec.Header().Get(echo.HeaderContentEncoding))
					r, err := reader(rec.Body)
					if assert.NoError(t, err) {
						buf := new(bytes.Buffer)
						defer r.Close()
						buf.ReadFrom(r)
						assert.Equal(t, data, buf.Bytes())
					}
				}
			})
		}
	}
}

func TestCompressNoContent(t *testing.T) {
	schemes := getTestcases()

	for scheme := range schemes {
		t.Run(scheme, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(echo.HeaderAcceptEncoding, scheme)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			h := NewWithConfig(Config{Schemes: []string{scheme}})(func(c echo.Context) error {
				return c.NoContent(http.StatusNoContent)
			})
			if assert.NoError(t, h(c)) {
				assert.Empty(t, rec.Header().Get(echo.HeaderContentEncoding))
				assert.Empty(t, rec.Header().Get(echo.HeaderContentType))
				assert.Equal(t, 0, len(rec.Body.Bytes()))
			}
		})
	}
}

func TestCompressEmpty(t *testing.T) {
	schemes := getTestcases()

	for scheme, reader := range schemes {
		t.Run(scheme, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(echo.HeaderAcceptEncoding, scheme)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			h := NewWithConfig(Config{Schemes: []string{scheme}})(func(c echo.Context) error {
				return c.String(http.StatusOK, "")
			})
			if assert.NoError(t, h(c)) {
				assert.Equal(t, scheme, rec.Header().Get(echo.HeaderContentEncoding))
				assert.Equal(t, "text/plain; charset=UTF-8", rec.Header().Get(echo.HeaderContentType))
				r, err := reader(rec.Body)
				if assert.NoError(t, err) {
					var buf bytes.Buffer
					buf.ReadFrom(r)
					assert.Equal(t, "", buf.String())
				}
			}
		})
	}
}

func TestCompressErrorReturned(t *testing.T) {
	schemes := getTestcases()

	for scheme := range schemes {
		t.Run(scheme, func(t *testing.T) {
			e := echo.New()
			e.Use(NewWithConfig(Config{Schemes: []string{scheme}}))
			e.GET("/", func(c echo.Context) error {
				return echo.ErrNotFound
			})
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(echo.HeaderAcceptEncoding, scheme)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusNotFound, rec.Code)
			assert.Empty(t, rec.Header().Get(echo.HeaderContentEncoding))
		})
	}
}

// Issue #806
func TestCompressWithStatic(t *testing.T) {
	schemes := getTestcases()

	for scheme, reader := range schemes {
		t.Run(scheme, func(t *testing.T) {
			e := echo.New()
			e.Use(NewWithConfig(Config{Schemes: []string{scheme}}))
			e.Static("/test", "./")
			req := httptest.NewRequest(http.MethodGet, "/test/compress.go", nil)
			req.Header.Set(echo.HeaderAcceptEncoding, scheme)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code)
			// Data is written out in chunks when Content-Length == "", so only
			// validate the content length if it's not set.
			if cl := rec.Header().Get("Content-Length"); cl != "" {
				assert.Equal(t, cl, rec.Body.Len())
			}
			r, err := reader(rec.Body)
			if assert.NoError(t, err) {
				defer r.Close()
				want, err := os.ReadFile("./compress.go")
				if assert.NoError(t, err) {
					buf := new(bytes.Buffer)
					buf.ReadFrom(r)
					assert.Equal(t, want, buf.Bytes())
				}
			}
		})
	}
}

func BenchmarkCompress(b *testing.B) {
	schemes := getTestcases()

	for i := 1; i <= 18; i++ {
		datalen := 2 << i
		data := rand.Bytes(datalen)

		for scheme := range schemes {
			name := fmt.Sprintf("%s-%d", scheme, datalen)
			b.Run(name, func(b *testing.B) {
				e := echo.New()

				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set(echo.HeaderAcceptEncoding, scheme)

				h := NewWithConfig(Config{Level: BestSpeed, Schemes: []string{scheme}})(func(c echo.Context) error {
					c.Response().Write(data)
					return nil
				})

				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					rec := httptest.NewRecorder()
					c := e.NewContext(req, rec)
					h(c)
				}
			})
		}
	}
}

func BenchmarkCompressJSON(b *testing.B) {
	data, err := os.ReadFile("./fixtures/processList.json")
	require.NoError(b, err)

	schemes := getTestcases()

	for scheme := range schemes {
		b.Run(scheme, func(b *testing.B) {
			e := echo.New()

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(echo.HeaderAcceptEncoding, scheme)

			h := NewWithConfig(Config{Level: BestSpeed, Schemes: []string{scheme}})(func(c echo.Context) error {
				c.Response().Write(data)
				return nil
			})

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				rec := httptest.NewRecorder()
				c := e.NewContext(req, rec)
				h(c)
			}
		})
	}
}
