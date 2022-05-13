// Package bodysize is an echo middleware that fixes the final number of body bytes sent on the wire
package bodysize

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Config struct {
	Skipper middleware.Skipper
}

var DefaultConfig = Config{
	Skipper: middleware.DefaultSkipper,
}

func New() echo.MiddlewareFunc {
	return NewWithConfig(DefaultConfig)
}

// New return a new bodysize middleware handler
func NewWithConfig(config Config) echo.MiddlewareFunc {
	if config.Skipper == nil {
		config.Skipper = DefaultConfig.Skipper
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			res := c.Response()

			writer := res.Writer
			w := &fakeWriter{
				ResponseWriter: res.Writer,
			}
			res.Writer = w

			defer func() {
				res.Writer = writer
				res.Size = w.size
			}()

			return next(c)
		}
	}
}

type fakeWriter struct {
	http.ResponseWriter

	size int64
}

func (w *fakeWriter) Write(body []byte) (int, error) {
	n, err := w.ResponseWriter.Write(body)

	w.size += int64(n)

	return n, err
}
