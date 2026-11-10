// Package alias is an echo middleware that serves a resource of a filesystem under
// a different path (an alias).
//
// Unlike a rewrite before routing, the alias is applied after authorization. The IAM
// middleware has already checked the requested (alias) path. This middleware then
// replaces the path with the target path, so the handler serves the target resource.
// The requesting identity doesn't need any permissions on the target path.
//
// Only GET and HEAD requests are aliased. The alias and its target must be on the same
// filesystem mount, i.e. the request has already been routed to the handler that will
// serve the target. Register this middleware with Use() after the IAM middleware.
package alias

import (
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/datarhei/core/v16/log"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Rule describes an alias. If the request path matches Pattern, the path will be
// replaced by Target. Target can reference capture groups of Pattern with $1, ${1}, or ${name}.
type Rule struct {
	Pattern *regexp.Regexp
	Target  string
}

type Config struct {
	// Skipper defines a function to skip middleware.
	Skipper middleware.Skipper

	// Mounts is the list of the mountpoints of the filesystems, e.g. "/", "/memfs".
	Mounts []string

	// Rules is the list of aliases. The first matching rule will be applied.
	Rules []Rule

	Logger log.Logger
}

var DefaultConfig = Config{
	Skipper: middleware.DefaultSkipper,
	Mounts:  []string{},
	Rules:   []Rule{},
	Logger:  nil,
}

func New() echo.MiddlewareFunc {
	return NewWithConfig(DefaultConfig)
}

func NewWithConfig(config Config) echo.MiddlewareFunc {
	if config.Skipper == nil {
		config.Skipper = DefaultConfig.Skipper
	}

	if config.Logger == nil {
		config.Logger = log.New("")
	}

	// Normalize the mountpoints, the root mount is the empty string.
	mounts := []string{}
	for _, mount := range config.Mounts {
		mount = "/" + strings.Trim(mount, "/")
		mount = strings.TrimSuffix(mount, "/")
		mounts = append(mounts, mount)
	}

	// Sort descending by length such that the most specific mount comes first.
	slices.SortFunc(mounts, func(a, b string) int {
		return len(b) - len(a)
	})

	// The routes of the filesystems mapped to their mountpoint.
	routes := map[string]string{}
	for _, mount := range mounts {
		routes[mount+"/*"] = mount
	}

	rules := slices.Clone(config.Rules)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if len(rules) == 0 || config.Skipper(c) {
				return next(c)
			}

			req := c.Request()

			if req.Method != http.MethodGet && req.Method != http.MethodHead {
				return next(c)
			}

			mount, ok := routes[c.Path()]
			if !ok {
				return next(c)
			}

			source := req.URL.Path

			target, ok := resolve(rules, source)
			if !ok {
				return next(c)
			}

			// The target must be on the same filesystem as the request has been routed to.
			if findMount(mounts, target) != mount {
				config.Logger.Warn().WithFields(log.Fields{
					"path":   source,
					"target": target,
				}).Log("Alias target is not on the same filesystem")

				return echo.NewHTTPError(http.StatusNotFound)
			}

			index := slices.Index(c.ParamNames(), "*")
			if index == -1 {
				return next(c)
			}

			urlPath, urlRawPath := req.URL.Path, req.URL.RawPath
			params := slices.Clone(c.ParamValues())

			// Restore the original request path, such that the outer middlewares see the requested path.
			defer func() {
				req.URL.Path, req.URL.RawPath = urlPath, urlRawPath
				c.SetParamValues(params...)
			}()

			values := slices.Clone(params)

			req.URL.Path = target
			req.URL.RawPath = ""

			// The wildcard parameter is the escaped path without the mountpoint and without the leading slash.
			u := url.URL{Path: strings.TrimPrefix(target, mount+"/")}
			values[index] = u.EscapedPath()
			c.SetParamValues(values...)

			return next(c)
		}
	}
}

// resolve returns the cleaned target path of the first rule that matches path.
func resolve(rules []Rule, path string) (string, bool) {
	for _, rule := range rules {
		match := rule.Pattern.FindStringSubmatchIndex(path)
		if match == nil {
			continue
		}

		target := string(rule.Pattern.ExpandString(nil, rule.Target, path, match))

		return cleanPath(target), true
	}

	return "", false
}

// cleanPath cleans the path and keeps a trailing slash.
func cleanPath(p string) string {
	trailingSlash := strings.HasSuffix(p, "/")

	p = path.Clean("/" + p)
	if trailingSlash && p != "/" {
		p += "/"
	}

	return p
}

// findMount returns the most specific mount that contains the path. The mounts
// must be sorted by length in descending order.
func findMount(mounts []string, path string) string {
	for _, mount := range mounts {
		if strings.HasPrefix(path, mount+"/") {
			return mount
		}
	}

	return "-"
}
