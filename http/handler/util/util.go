package util

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/datarhei/core/v16/encoding/json"

	"github.com/labstack/echo/v4"
)

func ShouldBindJSONValidation(c echo.Context, obj interface{}, validate bool) error {
	req := c.Request()

	if req.ContentLength == 0 {
		return fmt.Errorf("request doesn't contain any content")
	}

	ctype := req.Header.Get(echo.HeaderContentType)

	if !strings.HasPrefix(ctype, echo.MIMEApplicationJSON) {
		return fmt.Errorf("request doesn't contain JSON content")
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(body, obj); err != nil {
		return json.FormatError(body, err)
	}

	if validate {
		return c.Validate(obj)
	}

	return nil
}

// ShouldBindJSON binds the body data of the request to the given object. An error is
// returned if the body data is not valid JSON or the validation of the unmarshalled
// data failed.
func ShouldBindJSON(c echo.Context, obj interface{}) error {
	return ShouldBindJSONValidation(c, obj, true)
}

func PathWildcardParam(c echo.Context) string {
	return "/" + PathParam(c, "*")
}

// PathParam returns the decoded value of the path parameter with the given name.
// Echo matches the route on the raw path only if it is set, i.e. if it differs from
// the default encoding of the path (e.g. if it contains %2F). Otherwise the route has
// been matched on the decoded path and the parameter is already decoded.
func PathParam(c echo.Context, name string) string {
	param := c.Param(name)

	if len(c.Request().URL.RawPath) == 0 {
		return param
	}

	param, err := url.PathUnescape(param)
	if err != nil {
		return ""
	}

	return param
}

// DefaultQuery returns the value of the query parameter with the given name. The value
// is already decoded. If the value is empty, the default value is returned.
func DefaultQuery(c echo.Context, name, defValue string) string {
	param := c.QueryParam(name)

	if len(param) == 0 {
		return defValue
	}

	return param
}

func DefaultContext[T any](c echo.Context, name string, defValue T) T {
	value, ok := c.Get(name).(T)
	if !ok {
		return defValue
	}

	return value
}
