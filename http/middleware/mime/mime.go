// mime-type middleware
package mime

import (
	"bufio"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Config defines the config for Mime middleware.
type Config struct {
	// Skipper defines a function to skip middleware.
	Skipper middleware.Skipper

	MimeTypesFile      string
	MimeTypes          map[string]string
	DefaultContentType string
}

// DefaultConfig is the default Gzip middleware config.
var DefaultConfig = Config{
	Skipper:            middleware.DefaultSkipper,
	MimeTypesFile:      "",
	MimeTypes:          map[string]string{},
	DefaultContentType: "application/data",
}

func New() echo.MiddlewareFunc {
	return NewWithConfig(DefaultConfig)
}

func NewWithConfig(config Config) echo.MiddlewareFunc {
	if config.Skipper == nil {
		config.Skipper = DefaultConfig.Skipper
	}

	mimeTypes := map[string]string{}

	if len(config.MimeTypesFile) != 0 {
		mimeTypes = loadMimeFile(config.MimeTypesFile)
	} else {
		mimeTypes = maps.Clone(config.MimeTypes)
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			ext := filepath.Ext(c.Request().URL.Path)
			mimeType := mimeTypes[ext]

			if mimeType == "" {
				mimeType = config.DefaultContentType
			}

			if mimeType != "" {
				c.Response().Header().Set(echo.HeaderContentType, mimeType)
			}

			return next(c)
		}
	}
}

func loadMimeFile(filename string) map[string]string {
	mimeTypes := make(map[string]string)

	f, err := os.Open(filename)
	if err != nil {
		return mimeTypes
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) <= 1 || fields[0][0] == '#' {
			continue
		}
		mimeType := fields[0]

		for _, ext := range fields[1:] {
			if ext[0] == '#' {
				break
			}

			mimeTypes[ext] = mimeType
		}
	}

	if err := scanner.Err(); err != nil {
		return mimeTypes
	}

	return mimeTypes
}
