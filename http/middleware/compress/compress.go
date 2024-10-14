package compress

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/datarhei/core/v16/mem"
	"github.com/datarhei/core/v16/slices"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Config defines the config for compress middleware.
type Config struct {
	// Skipper defines a function to skip middleware.
	Skipper middleware.Skipper

	// Compression level.
	// Optional. Default value 0.
	Level Level

	// Length threshold before compression
	// is used. Optional. Default value 0
	MinLength int

	// Schemes is a list of enabled compressiond. Optional. Default [gzip]
	Schemes []string

	// List of content type to compress. If empty, everything will be compressed
	ContentTypes []string
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
	hasHeader           bool
	wroteHeader         bool
	wroteBody           bool
	minLength           int
	minLengthExceeded   bool
	buffer              *mem.Buffer
	code                int
	headerContentLength string
	scheme              string
	contentTypes        []string
	passThrough         bool
}

type Level int

const (
	DefaultCompression Level = 0
	BestCompression    Level = 1
	BestSpeed          Level = 2
)

// DefaultConfig is the default Gzip middleware config.
var DefaultConfig = Config{
	Skipper:      middleware.DefaultSkipper,
	Level:        DefaultCompression,
	MinLength:    0,
	Schemes:      []string{"gzip"},
	ContentTypes: []string{},
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

	if len(config.Schemes) == 0 {
		config.Schemes = DefaultConfig.Schemes
	}

	contentTypes := slices.Copy(config.ContentTypes)

	gzipEnable := false
	brotliEnable := false
	zstdEnable := false

	for _, s := range config.Schemes {
		switch s {
		case "gzip":
			gzipEnable = true
		case "br":
			brotliEnable = true
		case "zstd":
			zstdEnable = true
		}
	}

	var gzipCompressor Compression
	var brotliCompressor Compression
	var zstdCompressor Compression

	if gzipEnable {
		gzipCompressor = NewGzip(config.Level)
	}

	if brotliEnable {
		brotliCompressor = NewBrotli(config.Level)
	}

	if zstdEnable {
		zstdCompressor = NewZstd(config.Level)
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			res := c.Response()
			encodings := c.Request().Header.Get(echo.HeaderAcceptEncoding)

			var compress Compression
			var scheme string

			if zstdEnable && strings.Contains(encodings, "zstd") {
				compress = zstdCompressor
				scheme = "zstd"
			} else if brotliEnable && strings.Contains(encodings, "br") {
				compress = brotliCompressor
				scheme = "br"
			} else if gzipEnable && strings.Contains(encodings, "gzip") {
				compress = gzipCompressor
				scheme = "gzip"
			}

			if compress != nil {
				compressor := compress.Acquire()
				if compressor == nil {
					return echo.NewHTTPError(http.StatusInternalServerError, fmt.Errorf("failed to acquire compressor for %s", scheme))
				}
				rw := res.Writer
				compressor.Reset(rw)

				buffer := mem.Get()

				grw := &compressResponseWriter{
					Compressor:     compressor,
					ResponseWriter: rw,
					minLength:      config.MinLength,
					buffer:         buffer,
					scheme:         scheme,
					contentTypes:   contentTypes,
				}

				defer func() {
					if !grw.passThrough {
						if !grw.wroteBody {
							if res.Header().Get(echo.HeaderContentEncoding) == scheme {
								res.Header().Del(echo.HeaderContentEncoding)
							}
							// We have to reset response to it's pristine state when
							// nothing is written to body or error is returned.
							// See issue #424, #407.
							res.Writer = rw
							compressor.Reset(io.Discard)
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
							compressor.Reset(io.Discard)
						}
					}
					compressor.Close()
					mem.Put(buffer)
					compress.Release(compressor)
				}()

				res.Writer = grw
			}

			return next(c)
		}
	}
}

func (w *compressResponseWriter) WriteHeader(code int) {
	if code == http.StatusNoContent { // Issue #489
		w.Header().Del(echo.HeaderContentEncoding)
	}
	w.headerContentLength = w.Header().Get(echo.HeaderContentLength)
	w.Header().Del(echo.HeaderContentLength) // Issue #444

	if !w.canCompress(w.Header().Get(echo.HeaderContentType)) {
		w.passThrough = true
	}

	w.hasHeader = true

	// Delay writing of the header until we know if we'll actually compress the response
	w.code = code
}

func (w *compressResponseWriter) canCompress(responseContentType string) bool {
	// If no content types are given, compress all
	if len(w.contentTypes) == 0 {
		return true
	}

	// Iterate through the allowed content types and don't skip if the content type matches
	for _, contentType := range w.contentTypes {
		if strings.Contains(responseContentType, contentType) {
			return true
		}
	}

	return false
}

func (w *compressResponseWriter) Write(b []byte) (int, error) {
	if w.Header().Get(echo.HeaderContentType) == "" {
		w.Header().Set(echo.HeaderContentType, http.DetectContentType(b))
	}

	w.wroteBody = true

	if !w.hasHeader {
		w.WriteHeader(http.StatusOK)
	}

	if w.passThrough {
		if !w.wroteHeader {
			w.ResponseWriter.WriteHeader(w.code)
			w.wroteHeader = true
		}
		return w.ResponseWriter.Write(b)
	}

	if !w.minLengthExceeded {
		n, err := w.buffer.Write(b)

		if w.buffer.Len() >= w.minLength {
			w.minLengthExceeded = true

			// The minimum length is exceeded, add Content-Encoding header and write the header
			w.Header().Set(echo.HeaderContentEncoding, w.scheme) // Issue #806
			w.Header().Add(echo.HeaderVary, echo.HeaderAcceptEncoding)
			if w.hasHeader {
				w.ResponseWriter.WriteHeader(w.code)
				w.wroteHeader = true
			}

			return w.Compressor.Write(w.buffer.Bytes())
		} else {
			return n, err
		}
	}

	return w.Compressor.Write(b)
}

func (w *compressResponseWriter) Flush() {
	if !w.hasHeader {
		w.WriteHeader(http.StatusOK)
	}

	if w.passThrough {
		if !w.wroteHeader {
			w.ResponseWriter.WriteHeader(w.code)
			w.wroteHeader = true
		}

		if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
			flusher.Flush()
		}

		return
	}

	if !w.minLengthExceeded {
		// Enforce compression
		w.minLengthExceeded = true
		w.Header().Set(echo.HeaderContentEncoding, w.scheme) // Issue #806
		w.Header().Add(echo.HeaderVary, echo.HeaderAcceptEncoding)
		if w.hasHeader {
			w.ResponseWriter.WriteHeader(w.code)
			w.wroteHeader = true
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
