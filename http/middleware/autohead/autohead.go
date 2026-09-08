package head

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Config struct {
	// Skipper defines a function to skip middleware.
	Skipper middleware.Skipper
}

var DefaultConfig = Config{
	Skipper: func(c echo.Context) bool { return false },
}

func New() echo.MiddlewareFunc {
	return NewWithConfig(DefaultConfig)
}

func NewWithConfig(config Config) echo.MiddlewareFunc {
	if config.Skipper == nil {
		config.Skipper = DefaultConfig.Skipper
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			req := c.Request()

			if req.Method != http.MethodHead {
				return next(c)
			}

			req.Method = http.MethodGet

			res := c.Response()

			// Keep the current writer for later
			writer := res.Writer

			h := &nobodyResponseWriter{
				ResponseWriter: res.Writer,
			}

			res.Writer = h

			err := next(c)

			// Restore the original writer
			res.Writer = writer
			req.Method = http.MethodHead

			return err
		}
	}
}

type nobodyResponseWriter struct {
	http.ResponseWriter
}

func (g *nobodyResponseWriter) Write(data []byte) (int, error) {
	return len(data), nil
}

func (g *nobodyResponseWriter) Flush() {}
