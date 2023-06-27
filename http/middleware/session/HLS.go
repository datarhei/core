// Package session is a HLS session middleware for Gin
package session

import (
	"net/http"
	"net/url"
	urlpath "path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/net"
	"github.com/datarhei/core/v16/session"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/lithammer/shortuuid/v4"
)

type HLSConfig struct {
	// Skipper defines a function to skip middleware.
	Skipper          middleware.Skipper
	EgressCollector  session.Collector
	IngressCollector session.Collector
}

var DefaultHLSConfig = HLSConfig{
	Skipper:          middleware.DefaultSkipper,
	EgressCollector:  session.NewNullCollector(),
	IngressCollector: session.NewNullCollector(),
}

// NewHTTP returns a new HTTP session middleware with default config
func NewHLS() echo.MiddlewareFunc {
	return NewHLSWithConfig(DefaultHLSConfig)
}

type hls struct {
	egressCollector  session.Collector
	ingressCollector session.Collector
	reSessionID      *regexp.Regexp

	rxsegments map[string]int64
	lock       sync.Mutex
}

// NewHLS returns a new HLS session middleware
func NewHLSWithConfig(config HLSConfig) echo.MiddlewareFunc {
	if config.Skipper == nil {
		config.Skipper = DefaultHLSConfig.Skipper
	}

	if config.EgressCollector == nil {
		config.EgressCollector = DefaultHLSConfig.EgressCollector
	}

	if config.IngressCollector == nil {
		config.IngressCollector = DefaultHLSConfig.IngressCollector
	}

	hls := hls{
		egressCollector:  config.EgressCollector,
		ingressCollector: config.IngressCollector,
		reSessionID:      regexp.MustCompile(`^[` + regexp.QuoteMeta(shortuuid.DefaultAlphabet) + `]{22}$`),
		rxsegments:       make(map[string]int64),
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			req := c.Request()

			if req.Method == "PUT" || req.Method == "POST" {
				return hls.handleIngress(c, next)
			} else if req.Method == "GET" || req.Method == "HEAD" {
				return hls.handleEgress(c, next)
			}

			return next(c)
		}
	}
}

func (h *hls) handleIngress(c echo.Context, next echo.HandlerFunc) error {
	req := c.Request()

	ctxuser := util.DefaultContext(c, "user", "")

	path := req.URL.Path

	if strings.HasSuffix(path, ".m3u8") {
		// Read out the path of the .ts files and look them up in the ts-map.
		// Add it as ingress for the respective "sessionId". The "sessionId" is the .m3u8 file name.
		reader := req.Body
		r := &bodyReader{
			reader: req.Body,
		}
		req.Body = r

		defer func() {
			req.Body = reader

			if r.size == 0 {
				return
			}

			if !h.ingressCollector.IsKnownSession(path) {
				ip, _ := net.AnonymizeIPString(c.RealIP())

				// Register a new session
				reference := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
				h.ingressCollector.RegisterAndActivate(path, reference, path, ip)

				h.ingressCollector.Extra(path, map[string]interface{}{
					"name":       ctxuser,
					"ip":         ip,
					"user_agent": req.Header.Get("User-Agent"),
				})
			}

			h.ingressCollector.Ingress(path, headerSize(req.Header))
			h.ingressCollector.Ingress(path, r.size)

			segments := r.getSegments(urlpath.Dir(path))

			if len(segments) != 0 {
				h.lock.Lock()
				for _, s := range segments {
					if size, ok := h.rxsegments[s]; ok {
						// Update ingress value
						h.ingressCollector.Ingress(path, size)
						delete(h.rxsegments, s)
					}
				}
				h.lock.Unlock()
			}
		}()
	} else if strings.HasSuffix(path, ".ts") {
		// Get the size of the .ts file and store it in the ts-map for later use.
		reader := req.Body
		r := &bodysizeReader{
			reader: req.Body,
		}
		req.Body = r

		defer func() {
			req.Body = reader

			if r.size != 0 {
				h.lock.Lock()
				h.rxsegments[path] = r.size + headerSize(req.Header)
				h.lock.Unlock()
			}
		}()
	}

	return next(c)
}

func (h *hls) handleEgress(c echo.Context, next echo.HandlerFunc) error {
	req := c.Request()
	res := c.Response()

	if !h.egressCollector.IsCollectableIP(c.RealIP()) {
		return next(c)
	}

	ctxuser := util.DefaultContext(c, "user", "")

	path := req.URL.Path
	sessionID := c.QueryParam("session")

	isM3U8 := strings.HasSuffix(path, ".m3u8")
	isTS := strings.HasSuffix(path, ".ts")

	rewrite := false

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

	if isM3U8 {
		if !h.egressCollector.IsKnownSession(sessionID) {
			if h.egressCollector.IsSessionsExceeded() {
				return echo.NewHTTPError(509, "Number of sessions exceeded")
			}

			streamBitrate := h.ingressCollector.SessionTopIngressBitrate(path) * 2.0 // Multiply by 2 to cover the initial peak
			maxBitrate := h.egressCollector.MaxEgressBitrate()

			if maxBitrate > 0.0 {
				currentBitrate := h.egressCollector.CompanionTopEgressBitrate() * 1.15

				// Add the new session's top bitrate to the ingress top bitrate
				resultingBitrate := currentBitrate + streamBitrate

				if resultingBitrate >= maxBitrate {
					return echo.NewHTTPError(509, "Bitrate limit exceeded")
				}
			}

			if len(sessionID) != 0 {
				if !h.reSessionID.MatchString(sessionID) {
					return echo.NewHTTPError(http.StatusForbidden)
				}

				referrer := req.Header.Get("Referer")
				if u, err := url.Parse(referrer); err == nil {
					referrer = u.Host
				}

				reference := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

				// Register a new session
				h.egressCollector.Register(sessionID, reference, path, referrer)

				ip, _ := net.AnonymizeIPString(c.RealIP())

				data["ip"] = ip
				data["user_agent"] = req.Header.Get("User-Agent")
				data["name"] = ctxuser

				h.egressCollector.Extra(sessionID, data)

				// Give the new session an initial top bitrate
				h.egressCollector.SessionSetTopEgressBitrate(sessionID, streamBitrate)
			}
		}

		rewrite = true
	}

	var rewriter *sessionRewriter

	// Keep the current writer for later
	writer := res.Writer

	if rewrite {
		// Put the session rewriter in the middle. This will collect
		// the data that we need to rewrite.
		rewriter = &sessionRewriter{
			ResponseWriter: res.Writer,
		}

		res.Writer = rewriter
	}

	if err := next(c); err != nil {
		c.Error(err)
	}

	// Restore the original writer
	res.Writer = writer

	if rewrite {
		if res.Status < 200 || res.Status >= 300 {
			res.Write(rewriter.buffer.Bytes())
			return nil
		}

		// Rewrite the data befor sending it to the client
		rewriter.rewriteHLS(sessionID, c.Request().URL)

		res.Header().Set("Cache-Control", "private")
		res.Write(rewriter.buffer.Bytes())
	}

	if isM3U8 || isTS {
		if res.Status < 200 || res.Status >= 300 {
			// Collect how many bytes we've written in this session
			h.egressCollector.Egress(sessionID, headerSize(res.Header()))
			h.egressCollector.Egress(sessionID, res.Size)

			if isTS {
				// Activate the session. If the session is already active, this is a noop
				h.egressCollector.Activate(sessionID)
			}
		}
	}

	return nil
}
