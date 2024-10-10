package hlsrewrite

import (
	"bufio"
	"bytes"
	"net/http"
	"strings"

	"github.com/datarhei/core/v16/mem"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type HLSRewriteConfig struct {
	// Skipper defines a function to skip middleware.
	Skipper    middleware.Skipper
	PathPrefix string
}

var DefaultHLSRewriteConfig = HLSRewriteConfig{
	Skipper: func(c echo.Context) bool {
		req := c.Request()

		return !strings.HasSuffix(req.URL.Path, ".m3u8")
	},
	PathPrefix: "",
}

// NewHTTP returns a new HTTP session middleware with default config
func NewHLSRewrite() echo.MiddlewareFunc {
	return NewHLSRewriteWithConfig(DefaultHLSRewriteConfig)
}

type hlsrewrite struct {
	pathPrefix []byte
}

func NewHLSRewriteWithConfig(config HLSRewriteConfig) echo.MiddlewareFunc {
	if config.Skipper == nil {
		config.Skipper = DefaultHLSRewriteConfig.Skipper
	}

	pathPrefix := config.PathPrefix
	if len(pathPrefix) != 0 {
		if !strings.HasSuffix(pathPrefix, "/") {
			pathPrefix += "/"
		}
	}

	hls := hlsrewrite{
		pathPrefix: []byte(pathPrefix),
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			req := c.Request()

			if req.Method == "GET" || req.Method == "HEAD" {
				return hls.rewrite(c, next)
			}

			return next(c)
		}
	}
}

func (h *hlsrewrite) rewrite(c echo.Context, next echo.HandlerFunc) error {
	req := c.Request()
	res := c.Response()

	path := req.URL.Path

	isM3U8 := strings.HasSuffix(path, ".m3u8")

	rewrite := false

	if isM3U8 {
		rewrite = true
	}

	var rewriter *hlsRewriter

	// Keep the current writer for later
	writer := res.Writer

	if rewrite {
		// Put the session rewriter in the middle. This will collect
		// the data that we need to rewrite.
		rewriter = &hlsRewriter{
			ResponseWriter: res.Writer,
			buffer:         mem.Get(),
		}

		res.Writer = rewriter
	}

	if err := next(c); err != nil {
		c.Error(err)
	}

	// Restore the original writer
	res.Writer = writer

	if rewrite {
		if res.Status == 200 {
			// Rewrite the data befor sending it to the client
			buffer := mem.Get()
			defer mem.Put(buffer)

			rewriter.rewrite(h.pathPrefix, buffer)

			res.Header().Set("Cache-Control", "private")
			res.Write(buffer.Bytes())
		} else {
			res.Write(rewriter.buffer.Bytes())
		}

		mem.Put(rewriter.buffer)
	}

	return nil
}

type hlsRewriter struct {
	http.ResponseWriter
	buffer *bytes.Buffer
}

func (g *hlsRewriter) Write(data []byte) (int, error) {
	// Write the data into internal buffer for later rewrite
	w, err := g.buffer.Write(data)

	return w, err
}

func (g *hlsRewriter) rewrite(pathPrefix []byte, buffer *bytes.Buffer) {
	// Find all URLS in the .m3u8 and add the session ID to the query string
	scanner := bufio.NewScanner(g.buffer)
	for scanner.Scan() {
		line := scanner.Bytes()

		// Write empty lines unmodified
		if len(line) == 0 {
			buffer.Write(line)
			buffer.WriteByte('\n')
			continue
		}

		// Write comments unmodified
		if line[0] == '#' {
			buffer.Write(line)
			buffer.WriteByte('\n')
			continue
		}

		// Rewrite
		line = bytes.TrimPrefix(line, pathPrefix)
		buffer.Write(line)
		buffer.WriteByte('\n')
	}
}
