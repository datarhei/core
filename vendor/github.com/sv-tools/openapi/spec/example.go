package spec

// Example is expected to be compatible with the type schema of its associated value.
// Tooling implementations MAY choose to validate compatibility automatically, and reject the example value(s) if incompatible.
//
// https://spec.openapis.org/oas/v3.1.0#example-object
//
// Example:
//
//	requestBody:
//	  content:
//	    'application/json':
//	      schema:
//	        $ref: '#/components/schemas/Address'
//	      examples:
//	        foo:
//	          summary: A foo example
//	          value: {"foo": "bar"}
//	        bar:
//	          summary: A bar example
//	          value: {"bar": "baz"}
//	    'application/xml':
//	      examples:
//	        xmlExample:
//	          summary: This is an example in XML
//	          externalValue: 'https://example.org/examples/address-example.xml'
//	    'text/plain':
//	      examples:
//	        textExample:
//	          summary: This is a text example
//	          externalValue: 'https://foo.bar/examples/address-example.txt'
type Example struct {
	// Short description for the example.
	Summary string `json:"summary,omitempty" yaml:"summary,omitempty"`
	// Long description for the example.
	// CommonMark syntax MAY be used for rich text representation.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Embedded literal example.
	// The value field and externalValue field are mutually exclusive.
	// To represent examples of media types that cannot naturally represented in JSON or YAML,
	// use a string value to contain the example, escaping where necessary.
	Value any `json:"value,omitempty" yaml:"value,omitempty"`
	// A URI that points to the literal example.
	// This provides the capability to reference examples that cannot easily be included in JSON or YAML documents.
	// The value field and externalValue field are mutually exclusive.
	// See the rules for resolving Relative References.
	ExternalValue string `json:"externalValue,omitempty" yaml:"externalValue,omitempty"`
}

// NewExampleSpec creates Example object.
func NewExampleSpec() *RefOrSpec[Extendable[Example]] {
	return NewRefOrSpec[Extendable[Example]](nil, NewExtendable(&Example{}))
}

// NewExampleRef creates Ref object.
func NewExampleRef(ref *Ref) *RefOrSpec[Extendable[Example]] {
	return NewRefOrSpec[Extendable[Example]](ref, nil)
}
