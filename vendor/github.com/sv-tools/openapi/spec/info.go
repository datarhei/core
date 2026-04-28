package spec

// Info provides metadata about the API.
// The metadata MAY be used by the clients if needed, and MAY be presented in editing or documentation generation tools for convenience.
//
// https://spec.openapis.org/oas/v3.1.0#info-object
//
// Example:
//
//	title: Sample Pet Store App
//	summary: A pet store manager.
//	description: This is a sample server for a pet store.
//	termsOfService: https://example.com/terms/
//	contact:
//	  name: API Support
//	  url: https://www.example.com/support
//	  email: support@example.com
//	license:
//	  name: Apache 2.0
//	  url: https://www.apache.org/licenses/LICENSE-2.0.html
//	version: 1.0.1
type Info struct {
	// REQUIRED.
	// The title of the API.
	Title string `json:"title" yaml:"title"`
	// A short summary of the API.
	Summary string `json:"summary,omitempty" yaml:"summary,omitempty"`
	// A description of the API.
	// CommonMark syntax MAY be used for rich text representation.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// A URL to the Terms of Service for the API.
	// This MUST be in the form of a URL.
	TermsOfService string `json:"termsOfService,omitempty" yaml:"termsOfService,omitempty"`
	// The contact information for the exposed API.
	Contact *Extendable[Contact] `json:"contact,omitempty" yaml:"contact,omitempty"`
	// The license information for the exposed API.
	License *Extendable[License] `json:"license,omitempty" yaml:"license,omitempty"`
	// REQUIRED.
	// The version of the OpenAPI document (which is distinct from the OpenAPI Specification version or the API implementation version).
	Version string `json:"version" yaml:"version"`
}

// NewInfo creates Info object.
func NewInfo() *Extendable[Info] {
	return NewExtendable(&Info{})
}
