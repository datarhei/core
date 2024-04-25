package compress

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Config defines the config for compress middleware.
type Config struct {
	// Skipper defines a function to skip middleware.
	Skipper middleware.Skipper

	// Compression level.
	// Optional. Default value -1.
	Level Level

	// Length threshold before compression
	// is used. Optional. Default value 0
	MinLength int
}

type Compression interface {
	Acquire() Compressor
	Release(c Compressor)
}

type Compressor interface {
	Write(p []byte) (int, error)
	Flush() error
	Reset(w io.Writer)
	Close() error
}

type compressResponseWriter struct {
	Compressor
	http.ResponseWriter
	wroteHeader         bool
	wroteBody           bool
	minLength           int
	minLengthExceeded   bool
	buffer              *bytes.Buffer
	code                int
	headerContentLength string
	scheme              string
}

type Scheme string

func (s Scheme) String() string {
	return string(s)
}

const (
	GzipScheme   Scheme = "gzip"
	BrotliScheme Scheme = "br"
	ZstdScheme   Scheme = "zstd"
)

type Level int

const (
	DefaultCompression Level = 0
	BestCompression    Level = 1
	BestSpeed          Level = 2
)

// DefaultConfig is the default Gzip middleware config.
var DefaultConfig = Config{
	Skipper:   middleware.DefaultSkipper,
	Level:     DefaultCompression,
	MinLength: 0,
}

// ContentTypesSkipper returns a Skipper based on the list of content types
// that should be compressed. If the list is empty, all responses will be
// compressed.
func ContentTypeSkipper(contentTypes []string) middleware.Skipper {
	return func(c echo.Context) bool {
		// If no allowed content types are given, compress all
		if len(contentTypes) == 0 {
			return false
		}

		// Iterate through the allowed content types and don't skip if the content type matches
		responseContentType := c.Response().Header().Get(echo.HeaderContentType)

		for _, contentType := range contentTypes {
			if strings.Contains(responseContentType, contentType) {
				return false
			}
		}

		return true
	}
}

// New returns a middleware which compresses HTTP response using a compression
// scheme.
func New() echo.MiddlewareFunc {
	return NewWithConfig(DefaultConfig)
}

// NewWithConfig return compress middleware with config.
// See: `New()`.
func NewWithConfig(config Config) echo.MiddlewareFunc {
	// Defaults
	if config.Skipper == nil {
		config.Skipper = DefaultConfig.Skipper
	}

	if config.Level == 0 {
		config.Level = DefaultConfig.Level
	}

	if config.MinLength < 0 {
		config.MinLength = DefaultConfig.MinLength
	}

	gzipPool := NewGzip(config.Level)
	brotliPool := NewBrotli(config.Level)
	zstdPool := NewZstd(config.Level)
	bpool := bufferPool()

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			res := c.Response()
			res.Header().Add(echo.HeaderVary, echo.HeaderAcceptEncoding)
			encodings := c.Request().Header.Get(echo.HeaderAcceptEncoding)

			var pool Compression
			var scheme Scheme

			if strings.Contains(encodings, ZstdScheme.String()) {
				pool = zstdPool
				scheme = ZstdScheme
			} else if strings.Contains(encodings, BrotliScheme.String()) {
				pool = brotliPool
				scheme = BrotliScheme
			} else if strings.Contains(encodings, GzipScheme.String()) {
				pool = gzipPool
				scheme = GzipScheme
			}

			if pool != nil {
				w := pool.Acquire()
				if w == nil {
					return echo.NewHTTPError(http.StatusInternalServerError, fmt.Errorf("failed to acquire compressor for %s", scheme))
				}
				rw := res.Writer
				w.Reset(rw)

				buf := bpool.Get().(*bytes.Buffer)
				buf.Reset()

				grw := &compressResponseWriter{Compressor: w, ResponseWriter: rw, minLength: config.MinLength, buffer: buf, scheme: scheme.String()}

				defer func() {
					if !grw.wroteBody {
						if res.Header().Get(echo.HeaderContentEncoding) == scheme.String() {
							res.Header().Del(echo.HeaderContentEncoding)
						}
						// We have to reset response to it's pristine state when
						// nothing is written to body or error is returned.
						// See issue #424, #407.
						res.Writer = rw
						w.Reset(io.Discard)
					} else if !grw.minLengthExceeded {
						// If the minimum content length hasn't exceeded, write the uncompressed response
						res.Writer = rw
						if grw.wroteHeader {
							// Restore Content-Length header in case it was deleted
							if len(grw.headerContentLength) != 0 {
								grw.Header().Set(echo.HeaderContentLength, grw.headerContentLength)
							}
							grw.ResponseWriter.WriteHeader(grw.code)
						}
						grw.buffer.WriteTo(rw)
						w.Reset(io.Discard)
					}
					w.Close()
					bpool.Put(buf)
					pool.Release(w)
				}()

				res.Writer = grw
			}

			return next(c)
		}
	}
}

func (w *compressResponseWriter) WriteHeader(code int) {
	if code == http.StatusNoContent { // Issue #489
		w.ResponseWriter.Header().Del(echo.HeaderContentEncoding)
	}
	w.headerContentLength = w.Header().Get(echo.HeaderContentLength)
	w.Header().Del(echo.HeaderContentLength) // Issue #444

	w.wroteHeader = true

	// Delay writing of the header until we know if we'll actually compress the response
	w.code = code
}

func (w *compressResponseWriter) Write(b []byte) (int, error) {
	if w.Header().Get(echo.HeaderContentType) == "" {
		w.Header().Set(echo.HeaderContentType, http.DetectContentType(b))
	}

	w.wroteBody = true

	if !w.minLengthExceeded {
		n, err := w.buffer.Write(b)

		if w.buffer.Len() >= w.minLength {
			w.minLengthExceeded = true

			// The minimum length is exceeded, add Content-Encoding header and write the header
			w.Header().Set(echo.HeaderContentEncoding, w.scheme) // Issue #806
			if w.wroteHeader {
				w.ResponseWriter.WriteHeader(w.code)
			}

			return w.Compressor.Write(w.buffer.Bytes())
		} else {
			return n, err
		}
	}

	return w.Compressor.Write(b)
}

func (w *compressResponseWriter) Flush() {
	if !w.minLengthExceeded {
		// Enforce compression
		w.minLengthExceeded = true
		w.Header().Set(echo.HeaderContentEncoding, w.scheme) // Issue #806
		if w.wroteHeader {
			w.ResponseWriter.WriteHeader(w.code)
		}

		w.Compressor.Write(w.buffer.Bytes())
	}

	w.Compressor.Flush()
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *compressResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.(http.Hijacker).Hijack()
}

func (w *compressResponseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

func bufferPool() sync.Pool {
	return sync.Pool{
		New: func() interface{} {
			b := &bytes.Buffer{}
			return b
		},
	}
}
