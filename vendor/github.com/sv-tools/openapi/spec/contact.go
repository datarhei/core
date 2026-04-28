package spec

// Contact information for the exposed API.
//
// https://spec.openapis.org/oas/v3.1.0#contact-object
//
// Example:
//
//	name: API Support
//	url: https://www.example.com/support
//	email: support@example.com
type Contact struct {
	// The identifying name of the contact person/organization.
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// The URL pointing to the contact information.
	// This MUST be in the form of a URL.
	URL string `json:"url,omitempty" yaml:"url,omitempty"`
	// The email address of the contact person/organization.
	// This MUST be in the form of an email address.
	Email string `json:"email,omitempty" yaml:"email,omitempty"`
}

// NewContact creates Contact object.
func NewContact() *Extendable[Contact] {
	return NewExtendable(&Contact{})
}
