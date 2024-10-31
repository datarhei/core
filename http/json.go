package http

import (
	"fmt"
	"net/http"

	"github.com/datarhei/core/v16/encoding/json"

	"github.com/labstack/echo/v4"
)

// GoJSONSerializer implements JSON encoding using encoding/json.
type GoJSONSerializer struct{}

// Serialize converts an interface into a json and writes it to the response.
// You can optionally use the indent parameter to produce pretty JSONs.
func (d GoJSONSerializer) Serialize(c echo.Context, i interface{}, _ string) error {
	enc := json.NewEncoder(c.Response())
	return enc.Encode(i)
}

// Deserialize reads a JSON from a request body and converts it into an interface.
func (d GoJSONSerializer) Deserialize(c echo.Context, i interface{}) error {
	err := json.NewDecoder(c.Request().Body).Decode(i)
	if ute, ok := err.(*json.UnmarshalTypeError); ok {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unmarshal type error: expected=%v, got=%v, field=%v, offset=%v", ute.Type, ute.Value, ute.Field, ute.Offset)).SetInternal(err)
	} else if se, ok := err.(*json.SyntaxError); ok {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Syntax error: offset=%v, error=%v", se.Offset, se.Error())).SetInternal(err)
	}
	return err
}
