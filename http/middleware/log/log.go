// Package log implements a logging middleware
package log

import (
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
}

var DefaultConfig = Config{
	Skipper: middleware.DefaultSkipper,
	Logger:  log.New("HTTP"),
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
			res := c.Response()

			path := req.URL.Path
			raw := req.URL.RawQuery

			if err := next(c); err != nil {
				c.Error(err)
			}

			latency := time.Since(start)

			if raw != "" {
				path = path + "?" + raw
			}

			logger := config.Logger.WithFields(log.Fields{
				"client":      c.RealIP(),
				"method":      req.Method,
				"path":        path,
				"proto":       req.Proto,
				"status":      res.Status,
				"status_text": http.StatusText(res.Status),
				"size_bytes":  res.Size,
				"latency_ms":  latency.Milliseconds(),
				"user_agent":  req.Header.Get("User-Agent"),
			})

			if res.Status >= 400 {
				logger.Warn().Log("")
			}

			logger.Debug().Log("")

			return nil
		}
	}
}
