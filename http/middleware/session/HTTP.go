package session

import (
	"io"
	"net"
	"net/http"
	"net/url"

	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/session"
	"github.com/lithammer/shortuuid/v4"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type HTTPConfig struct {
	// Skipper defines a function to skip middleware.
	Skipper   middleware.Skipper
	Collector session.Collector
}

var DefaultHTTPConfig = HTTPConfig{
	Skipper:   middleware.DefaultSkipper,
	Collector: session.NewNullCollector(),
}

// NewHTTP returns a new HTTP session middleware with default config
func NewHTTP() echo.MiddlewareFunc {
	return NewHTTPWithConfig(DefaultHTTPConfig)
}

// NewHTTPWithConfig returns a new HTTP session middleware
func NewHTTPWithConfig(config HTTPConfig) echo.MiddlewareFunc {
	if config.Skipper == nil {
		config.Skipper = DefaultHTTPConfig.Skipper
	}

	if config.Collector == nil {
		config.Collector = DefaultHTTPConfig.Collector
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			if !config.Collector.IsCollectableIP(c.RealIP()) {
				return next(c)
			}

			ctxuser := util.DefaultContext(c, "user", "")

			req := c.Request()
			res := c.Response()

			host, _, err := net.SplitHostPort(req.RemoteAddr)
			if err != nil {
				host = ""
			}

			path := req.URL.Path

			location := path + req.URL.RawQuery
			remote := req.RemoteAddr

			id := shortuuid.New()

			data := map[string]interface{}{}

			e := util.DefaultContext[interface{}](c, "session", nil)
			if e != nil {
				var ok bool
				data, ok = e.(map[string]interface{})
				if !ok {
					return api.Err(http.StatusForbidden, "", "invalid session data, cast")
				}

				if match, ok := data["match"].(string); ok {
					if ok, err := glob.Match(match, path, '/'); !ok {
						if err != nil {
							return api.Err(http.StatusForbidden, "", "no match for '%s' in %s: %s", match, path, err.Error())
						}

						return api.Err(http.StatusForbidden, "", "no match for '%s' in %s", match, path)
					}
				}

				referrer := req.Header.Get("Referer")
				if u, err := url.Parse(referrer); err == nil {
					referrer = u.Host
				}

				if remote, ok := data["remote"].([]string); ok && len(remote) != 0 {
					match := false
					for _, r := range remote {
						if ok, _ := glob.Match(r, referrer, '.'); ok {
							match = true
							break
						}
					}

					if !match {
						return api.Err(http.StatusForbidden, "", "remote not allowed")
					}
				}
			}

			data["name"] = ctxuser
			data["method"] = req.Method
			data["code"] = 0

			reader := req.Body
			r := &fakeReader{
				reader: req.Body,
			}
			req.Body = r

			writer := res.Writer
			w := &fakeWriter{
				ResponseWriter: res.Writer,
			}
			res.Writer = w

			if config.Collector.IsCollectableIP(host) {
				config.Collector.RegisterAndActivate(id, "", location, remote)
				config.Collector.Extra(id, data)
			}

			defer config.Collector.Close(id)

			defer func() {
				req.Body = reader

				if config.Collector.IsCollectableIP(host) {
					config.Collector.Ingress(id, r.size+headerSize(req.Header))
				}
			}()

			defer func() {
				res.Writer = writer

				if config.Collector.IsCollectableIP(host) {
					config.Collector.Egress(id, w.size+headerSize(res.Header()))
					data["code"] = res.Status
					config.Collector.Extra(id, data)
				}
			}()

			return next(c)
		}
	}
}

type fakeReader struct {
	reader io.ReadCloser
	size   int64
}

func (r *fakeReader) Read(b []byte) (int, error) {
	n, err := r.reader.Read(b)
	r.size += int64(n)

	return n, err
}

func (r *fakeReader) Close() error {
	return r.reader.Close()
}

type fakeWriter struct {
	http.ResponseWriter
	size int64
	code int
}

func (w *fakeWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)

	w.code = statusCode
}

func (w *fakeWriter) Write(body []byte) (int, error) {
	n, err := w.ResponseWriter.Write(body)

	w.size += int64(n)

	return n, err
}

func (w *fakeWriter) Flush() {
	flusher, ok := w.ResponseWriter.(http.Flusher)
	if ok {
		flusher.Flush()
	}
}
