package spec

// Server is an object representing a Server.
//
// https://spec.openapis.org/oas/v3.1.0#server-object
//
// Example:
//
//	servers:
//	- url: https://development.gigantic-server.com/v1
//	  description: Development server
//	- url: https://staging.gigantic-server.com/v1
//	  description: Staging server
//	- url: https://api.gigantic-server.com/v1
//	  description: Production server
type Server struct {
	// A map between a variable name and its value.
	// The value is used for substitution in the server’s URL template.
	Variables map[string]*Extendable[ServerVariable] `json:"variables,omitempty" yaml:"variables,omitempty"`
	// REQUIRED.
	// A URL to the target host.
	// This URL supports Server Variables and MAY be relative, to indicate that the host location is relative
	// to the location where the OpenAPI document is being served.
	// Variable substitutions will be made when a variable is named in {brackets}.
	URL string `json:"url" yaml:"url"`
	// An optional string describing the host designated by the URL.
	// CommonMark syntax MAY be used for rich text representation.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// NewServer creates Server object.
func NewServer() *Extendable[Server] {
	return NewExtendable(&Server{})
}
