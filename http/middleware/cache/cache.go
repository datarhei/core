// Package cache is an in-memory LRU cache middleware
package cache

import (
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/datarhei/core/v16/http/cache"
	"github.com/datarhei/core/v16/mem"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Config struct {
	Skipper middleware.Skipper
	Cache   cache.Cacher
	Headers []string
	Prefix  string
}

var DefaultConfig = Config{
	Skipper: middleware.DefaultSkipper,
	Cache:   nil,
	Headers: []string{"Content-Type"},
	Prefix:  "",
}

func New() echo.MiddlewareFunc {
	return NewWithConfig(DefaultConfig)
}

func NewWithConfig(config Config) echo.MiddlewareFunc {
	// Defaults
	if config.Skipper == nil {
		config.Skipper = DefaultConfig.Skipper
	}

	if config.Headers == nil {
		config.Headers = DefaultConfig.Headers
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			if config.Cache == nil {
				return next(c)
			}

			req := c.Request()
			res := c.Response()

			if req.Method != "GET" {
				res.Header().Set("X-Cache", "SKIP ONLYGET")
				return next(c)
			}

			key := strings.TrimPrefix(req.URL.Path, config.Prefix)

			if !config.Cache.IsExtensionCacheable(path.Ext(req.URL.Path)) {
				res.Header().Set("X-Cache", "SKIP EXT")
				return next(c)
			}

			if len(req.Header.Get("Range")) != 0 {
				res.Header().Set("X-Cache", "SKIP RANGEREQ")
				return next(c)
			}

			if obj, expireIn, _ := config.Cache.Get(key); obj == nil {
				// cache miss
				writer := res.Writer

				w := &cacheWriter{
					header: writer.Header().Clone(),
					body:   mem.Get(),
					writer: writer,
					cache:  config.Cache,
				}
				res.Writer = w

				if err := next(c); err != nil {
					c.Error(err)
				}

				// Restore original writer
				res.Writer = writer

				// The response has already been sent to the client, because it will not be cached.
				if w.passthrough {
					return nil
				}

				// Copy only the headers that will be cached as well
				copyCacheHeaders(res.Header(), w.header, config.Headers)

				res.Status = w.code

				defer res.Write(w.body.Bytes())

				if res.Status != 200 {
					res.Header().Set("X-Cache", "SKIP NOTOK")
					res.Writer.WriteHeader(res.Status)
					return nil
				}

				size := uint64(w.body.Len())

				if !config.Cache.IsSizeCacheable(size) {
					res.Header().Set("X-Cache", "SKIP TOOBIG")
					res.Writer.WriteHeader(res.Status)
					return nil
				}

				o := &cacheObject{
					status: res.Status,
					body:   w.body.Bytes(),
					header: w.header.Clone(),
				}

				if err := config.Cache.Put(key, o, size); err != nil {
					res.Header().Set("X-Cache", "SKIP TOOBIG")
					res.Writer.WriteHeader(res.Status)
					return nil
				}

				res.Header().Set("Cache-Control", fmt.Sprintf("max-age=%.0f", expireIn.Seconds()))
				res.Header().Set("X-Cache", "MISS")
				res.Writer.WriteHeader(res.Status)
			} else {
				// cache hit
				o := obj.(*cacheObject)

				// overwrite the header with the cached headers
				copyCacheHeaders(res.Header(), o.header, config.Headers)

				res.Header().Set("Cache-Control", fmt.Sprintf("max-age=%.0f", expireIn.Seconds()))
				res.Header().Set("X-Cache", fmt.Sprintf("HIT %.3f", expireIn.Seconds()))

				res.WriteHeader(o.status)
				res.Write(o.body)
			}

			return nil
		}
	}
}

func copyCacheHeaders(dst, src http.Header, headers []string) {
	if len(headers) == 0 {
		return
	}

	for _, key := range headers {
		values := src.Values(key)
		if len(values) == 0 {
			continue
		}

		dst.Set(key, values[0])
		for _, v := range values[1:] {
			dst.Add(key, v)
		}
	}
}

type cacheObject struct {
	status int
	body   []byte
	header http.Header
}

// cacheWriter buffers the response as long as it can be cached. As soon as it is clear
// that the response will not be cached, the buffered data is written to the original
// writer and the rest of the response is passed through.
type cacheWriter struct {
	code   int
	header http.Header
	body   *mem.Buffer

	writer      http.ResponseWriter // The original writer
	cache       cache.Cacher
	passthrough bool
}

func (w *cacheWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}

	return w.header
}

func (w *cacheWriter) WriteHeader(code int) {
	w.code = code

	if w.passthrough {
		return
	}

	if code != http.StatusOK {
		w.startPassthrough("SKIP NOTOK")
		return
	}

	if length, err := strconv.ParseUint(w.header.Get(echo.HeaderContentLength), 10, 64); err == nil {
		if !w.cache.IsSizeCacheable(length) {
			w.startPassthrough("SKIP TOOBIG")
		}
	}
}

func (w *cacheWriter) Write(body []byte) (int, error) {
	if w.passthrough {
		return w.writer.Write(body)
	}

	n, err := w.body.Write(body)
	if err != nil {
		return n, err
	}

	if !w.cache.IsSizeCacheable(uint64(w.body.Len())) {
		if err := w.startPassthrough("SKIP TOOBIG"); err != nil {
			return 0, err
		}
	}

	return n, nil
}

// startPassthrough sends the headers and the buffered data to the original writer.
// All further data is written directly to the original writer.
func (w *cacheWriter) startPassthrough(reason string) error {
	w.passthrough = true

	header := w.writer.Header()
	clear(header)

	for key, values := range w.header {
		header[key] = values
	}

	header.Set("X-Cache", reason)

	code := w.code
	if code == 0 {
		code = http.StatusOK
	}

	w.writer.WriteHeader(code)

	body := w.body
	w.body = nil

	defer mem.Put(body)

	_, err := w.writer.Write(body.Bytes())

	return err
}
