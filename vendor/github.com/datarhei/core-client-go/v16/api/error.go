package api

import (
	"fmt"
	"strings"
)

// Error represents an error response of the API
type Error struct {
	Code    int      `json:"code" jsonschema:"required"`
	Message string   `json:"message" jsonschema:""`
	Details []string `json:"details" jsonschema:""`
	Body    []byte   `json:"-"`
}

// Error returns the string representation of the error
func (e Error) Error() string {
	return fmt.Sprintf("code=%d, message=%s, details=%s", e.Code, e.Message, strings.Join(e.Details, " "))
}
