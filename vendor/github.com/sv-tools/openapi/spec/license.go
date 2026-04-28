package spec

// License information for the exposed API.
//
// https://spec.openapis.org/oas/v3.1.0#license-object
//
// Example:
//
//	name: Apache 2.0
//	identifier: Apache-2.0
type License struct {
	// REQUIRED.
	// The license name used for the API.
	Name string `json:"name" yaml:"name"`
	// An SPDX license expression for the API.
	// The identifier field is mutually exclusive of the url field.
	Identifier string `json:"identifier,omitempty" yaml:"identifier,omitempty"`
	// A URL to the license used for the API.
	// This MUST be in the form of a URL.
	// The url field is mutually exclusive of the identifier field.
	URL string `json:"url,omitempty" yaml:"url,omitempty"`
}

// NewLicense creates License object.
func NewLicense() *Extendable[License] {
	return NewExtendable(&License{})
}
