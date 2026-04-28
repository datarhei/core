package spec

// Header Object follows the structure of the Parameter Object with the some changes.
//
// https://spec.openapis.org/oas/v3.1.0#header-object
//
// Example:
//
//	description: The number of allowed requests in the current period
//	schema:
//	  type: integer
//
// All fields are copied from Parameter Object as is, except name and in fields.
type Header struct {
	// Example of the header’s potential value.
	// The example SHOULD match the specified schema and encoding properties if present.
	// The example field is mutually exclusive of the examples field.
	// Furthermore, if referencing a schema that contains an example, the example value SHALL override the example provided by the schema.
	// To represent examples of media types that cannot naturally be represented in JSON or YAML,
	// a string value can contain the example with escaping where necessary.
	Example any `json:"example,omitempty" yaml:"example,omitempty"`
	// The schema defining the type used for the parameter.
	Schema *RefOrSpec[Schema] `json:"schema,omitempty" yaml:"schema,omitempty"`
	// Examples of the parameter’s potential value.
	// Each example SHOULD contain a value in the correct format as specified in the parameter encoding.
	// The examples field is mutually exclusive of the example field.
	// Furthermore, if referencing a schema that contains an example, the examples value SHALL override the example provided by the schema.
	Examples map[string]*RefOrSpec[Extendable[Example]] `json:"examples,omitempty" yaml:"examples,omitempty"`
	// A map containing the representations for the parameter.
	// The key is the media type and the value describes it.
	// The map MUST only contain one entry.
	Content map[string]*Extendable[MediaType] `json:"content,omitempty" yaml:"content,omitempty"`
	// A brief description of the header.
	// This could contain examples of use.
	// CommonMark syntax MAY be used for rich text representation.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Describes how the parameter value will be serialized depending on the type of the parameter value.
	// Default values (based on value of in):
	//   for query - form;
	//   for path - simple;
	//   for header - simple;
	//   for cookie - form.
	Style string `json:"style,omitempty" yaml:"style,omitempty"`
	// When this is true, parameter values of type array or object generate separate parameters
	// for each value of the array or key-value pair of the map.
	// For other types of parameters this property has no effect.
	// When style is form, the default value is true.
	// For all other styles, the default value is false.
	Explode bool `json:"explode,omitempty" yaml:"explode,omitempty"`
	// Determines whether the parameter value SHOULD allow reserved characters, as defined by [RFC3986]
	//   :/?#[]@!$&'()*+,;=
	// to be included without percent-encoding.
	// This property only applies to parameters with an in value of query.
	// The default value is false.
	AllowReserved bool `json:"allowReserved,omitempty" yaml:"allowReserved,omitempty"`
	// Determines whether this header is mandatory.
	// The property MAY be included and its default value is false.
	Required bool `json:"required,omitempty" yaml:"required,omitempty"`
	// Specifies that a header is deprecated and SHOULD be transitioned out of usage.
	// Default value is false.
	Deprecated bool `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
	// Sets the ability to pass empty-valued headers.
	// This is valid only for query parameters and allows sending a parameter with an empty value.
	// Default value is false.
	// If style is used, and if behavior is n/a (cannot be serialized), the value of allowEmptyValue SHALL be ignored.
	// Use of this property is NOT RECOMMENDED, as it is likely to be removed in a later revision.
	AllowEmptyValue bool `json:"allowEmptyValue,omitempty" yaml:"allowEmptyValue,omitempty"`
}

// NewHeaderSpec creates Header object.
func NewHeaderSpec() *RefOrSpec[Extendable[Header]] {
	return NewRefOrSpec[Extendable[Header]](nil, NewExtendable(&Header{}))
}

// NewHeaderRef creates Ref object.
func NewHeaderRef(ref *Ref) *RefOrSpec[Extendable[Header]] {
	return NewRefOrSpec[Extendable[Header]](ref, nil)
}
