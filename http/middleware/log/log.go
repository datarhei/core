// Package log implements a logging middleware
package log

import (
	"io"
	"net/http"
	"time"

	"github.com/datarhei/core/v16/log"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Config struct {
	// Skipper defines a function to skip middleware.
	Skipper middleware.Skipper
	Logger  log.Logger
	Status  func(code int, method, path string, size int64, ttfb time.Duration)
}

var DefaultConfig = Config{
	Skipper: middleware.DefaultSkipper,
	Logger:  log.New(""),
	Status:  nil,
}

func New() echo.MiddlewareFunc {
	return NewWithConfig(DefaultConfig)
}

// New returns a middleware for logging HTTP requests
func NewWithConfig(config Config) echo.MiddlewareFunc {
	if config.Skipper == nil {
		config.Skipper = DefaultConfig.Skipper
	}

	if config.Logger == nil {
		config.Logger = DefaultConfig.Logger
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			start := time.Now()

			req := c.Request()

			var reader io.ReadCloser
			r := &sizeReadCloser{}

			if req.Body != nil {
				reader = req.Body
				r.ReadCloser = req.Body
				req.Body = r
			}

			res := c.Response()

			writer := res.Writer
			w := &sizeWriter{
				ResponseWriter: res.Writer,
			}
			res.Writer = w

			path := req.URL.Path
			raw := req.URL.RawQuery

			if err := next(c); err != nil {
				c.Error(err)
			}

			res.Writer = writer
			req.Body = reader

			if w.ttfb.IsZero() {
				w.ttfb = start
			}

			latency := time.Since(start)
			ttfb := time.Since(w.ttfb)

			if config.Status != nil {
				config.Status(res.Status, req.Method, c.Path(), w.size, ttfb)
			}

			if raw != "" {
				path = path + "?" + raw
			}

			logger := config.Logger.WithFields(log.Fields{
				"client":          c.RealIP(),
				"method":          req.Method,
				"path":            path,
				"proto":           req.Proto,
				"status":          res.Status,
				"status_text":     http.StatusText(res.Status),
				"tx_size_bytes":   w.size,
				"rx_size_bytes":   r.size,
				"latency_ms":      latency.Milliseconds(),
				"latency_ttfb_ms": ttfb.Milliseconds(),
				"user_agent":      req.Header.Get("User-Agent"),
			})

			logger.Debug().Log("")

			return nil
		}
	}
}

type sizeWriter struct {
	http.ResponseWriter

	size int64
	ttfb time.Time
}

func (w *sizeWriter) Write(body []byte) (int, error) {
	n, err := w.ResponseWriter.Write(body)

	w.size += int64(n)
	if w.ttfb.IsZero() {
		w.ttfb = time.Now()
	}

	return n, err
}

func (w *sizeWriter) Flush() {
	flusher, ok := w.ResponseWriter.(http.Flusher)
	if ok {
		flusher.Flush()
	}
}

type sizeReadCloser struct {
	io.ReadCloser

	size int64
}

func (r *sizeReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)

	r.size += int64(n)

	return n, err
}

func (r *sizeReadCloser) Close() error {
	err := r.ReadCloser.Close()

	return err
}
