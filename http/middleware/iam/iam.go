package iam

import (
	"net/http"

	"github.com/datarhei/core/v16/iam"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Config struct {
	// Skipper defines a function to skip middleware.
	Skipper middleware.Skipper
	IAM     iam.IAM
}

var DefaultConfig = Config{
	Skipper: middleware.DefaultSkipper,
	IAM:     nil,
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

			if config.IAM == nil {
				return next(c)
			}

			user := c.Get("user").(string)
			if len(user) == 0 {
				user = "$anon"
			}
			domain := c.QueryParam("group")
			if len(domain) == 0 {
				domain = "$none"
			}
			resource := c.Request().URL.Path
			action := c.Request().Method

			if !config.IAM.Enforce(user, domain, resource, action) {
				return echo.NewHTTPError(http.StatusForbidden)
			}

			return next(c)
		}
	}
}
