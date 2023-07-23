package session

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/net"
	"github.com/datarhei/core/v16/session"
	"github.com/lithammer/shortuuid/v4"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Config struct {
	Skipper             middleware.Skipper
	HTTPCollector       session.Collector
	HLSEgressCollector  session.Collector
	HLSIngressCollector session.Collector
}

var DefaultConfig = Config{
	Skipper:             middleware.DefaultSkipper,
	HTTPCollector:       session.NewNullCollector(),
	HLSEgressCollector:  session.NewNullCollector(),
	HLSIngressCollector: session.NewNullCollector(),
}

type handler struct {
	reSessionID *regexp.Regexp

	httpCollector       session.Collector
	hlsEgressCollector  session.Collector
	hlsIngressCollector session.Collector

	rxsegments map[string]int64
	lock       sync.Mutex
}

// New returns a new session middleware with default config
func New() echo.MiddlewareFunc {
	return NewWithConfig(DefaultConfig)
}

// New returns a new HLS session middleware
func NewWithConfig(config Config) echo.MiddlewareFunc {
	if config.Skipper == nil {
		config.Skipper = DefaultConfig.Skipper
	}

	if config.HTTPCollector == nil {
		config.HTTPCollector = DefaultConfig.HTTPCollector
	}

	if config.HLSEgressCollector == nil {
		config.HLSEgressCollector = DefaultConfig.HLSEgressCollector
	}

	if config.HLSIngressCollector == nil {
		config.HLSIngressCollector = DefaultConfig.HLSIngressCollector
	}

	h := handler{
		httpCollector:       config.HTTPCollector,
		hlsEgressCollector:  config.HLSEgressCollector,
		hlsIngressCollector: config.HLSIngressCollector,
		reSessionID:         regexp.MustCompile(`^[` + regexp.QuoteMeta(shortuuid.DefaultAlphabet) + `]{22}$`),
		rxsegments:          make(map[string]int64),
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			ctxuser := util.DefaultContext(c, "user", "")

			req := c.Request()

			path := req.URL.Path
			referrer := req.Header.Get("Referer")

			data, err := verifySession(util.DefaultContext[interface{}](c, "session", nil), path, referrer)
			if err != nil {
				return api.Err(http.StatusForbidden, "", "verifying session failed: %s", err.Error())
			}

			data["name"] = ctxuser
			data["method"] = req.Method
			data["user_agent"] = req.Header.Get("User-Agent")

			ip, _ := net.AnonymizeIPString(c.RealIP())

			data["ip"] = ip

			isM3U8 := strings.HasSuffix(path, ".m3u8")
			isTS := strings.HasSuffix(path, ".ts")

			if isM3U8 || isTS {
				return h.handleHLS(c, ctxuser, data, next)
			}

			return h.handleHTTP(c, ctxuser, data, next)
		}
	}
}

func verifySession(raw interface{}, path, referrer string) (map[string]interface{}, error) {
	data := map[string]interface{}{}

	if raw == nil {
		return data, nil
	}

	var ok bool
	data, ok = raw.(map[string]interface{})
	if !ok {
		return data, fmt.Errorf("invalid session data")
	}

	if match, ok := data["match"].(string); ok {
		if ok, err := glob.Match(match, path, '/'); !ok {
			if err != nil {
				return data, fmt.Errorf("no match for '%s' in %s: %s", match, path, err.Error())
			}

			return data, fmt.Errorf("no match for '%s' in %s", match, path)
		}
	}

	if u, err := url.Parse(referrer); err == nil {
		referrer = u.Host
	}

	if remote, ok := data["remote"].([]interface{}); ok && len(remote) != 0 {
		if len(referrer) == 0 {
			return data, fmt.Errorf("remote not allowed")
		}

		remotes := []string{}
		for _, r := range remote {
			v, ok := r.(string)
			if !ok {
				continue
			}

			remotes = append(remotes, v)
		}

		match := false
		for _, r := range remotes {
			if ok, _ := glob.Match(r, referrer, '.'); ok {
				match = true
				break
			}
		}

		if !match {
			return data, fmt.Errorf("remote not allowed")
		}
	}

	return data, nil
}

func headerSize(header http.Header) int64 {
	var buffer bytes.Buffer

	header.Write(&buffer)

	return int64(buffer.Len())
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
