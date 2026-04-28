package spec

// RequestBody describes a single request body.
//
// https://spec.openapis.org/oas/v3.1.0#request-body-object
//
// Example:
//
//	description: user to add to the system
//	content:
//	  'application/json':
//	    schema:
//	      $ref: '#/components/schemas/User'
//	    examples:
//	      user:
//	        summary: User Example
//	        externalValue: 'https://foo.bar/examples/user-example.json'
//	  'application/xml':
//	    schema:
//	      $ref: '#/components/schemas/User'
//	    examples:
//	      user:
//	        summary: User example in XML
//	        externalValue: 'https://foo.bar/examples/user-example.xml'
//	  'text/plain':
//	    examples:
//	      user:
//	        summary: User example in Plain text
//	        externalValue: 'https://foo.bar/examples/user-example.txt'
//	  '*/*':
//	    examples:
//	      user:
//	        summary: User example in other format
//	        externalValue: 'https://foo.bar/examples/user-example.whatever'
type RequestBody struct {
	// REQUIRED.
	// The content of the request body.
	// The key is a media type or [media type range](appendix-D) and the value describes it.
	// For requests that match multiple keys, only the most specific key is applicable. e.g. text/plain overrides text/*
	Content map[string]*Extendable[MediaType] `json:"content,omitempty" yaml:"content,omitempty"`
	// A brief description of the request body.
	// This could contain examples of use.
	// CommonMark syntax MAY be used for rich text representation.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Determines if the request body is required in the request.
	// Defaults to false.
	Required bool `json:"required,omitempty" yaml:"required,omitempty"`
}

// NewRequestBodySpec creates RequestBody object.
func NewRequestBodySpec() *RefOrSpec[Extendable[RequestBody]] {
	return NewRefOrSpec[Extendable[RequestBody]](nil, NewExtendable(&RequestBody{}))
}

// NewRequestBodyRef creates Ref object.
func NewRequestBodyRef(ref *Ref) *RefOrSpec[Extendable[RequestBody]] {
	return NewRefOrSpec[Extendable[RequestBody]](ref, nil)
}
