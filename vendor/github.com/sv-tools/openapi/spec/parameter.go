package spec

const (
	// InPath used together with Path Templating, where the parameter value is actually part of the operation’s URL.
	// This does not include the host or base path of the API.
	// For example, in /items/{itemId}, the path parameter is itemId.
	//
	// https://spec.openapis.org/oas/v3.1.0#parameter-locations
	InPath = "path"
	// InQuery used for parameters that are appended to the URL.
	// For example, in /items?id=###, the query parameter is id.
	//
	// https://spec.openapis.org/oas/v3.1.0#parameter-locations
	InQuery = "query"
	// InHeader used as custom headers that are expected as part of the request.
	// Note that [RFC7230] states header names are case insensitive.
	//
	// https://spec.openapis.org/oas/v3.1.0#parameter-locations
	InHeader = "header"
	// InCookie used to pass a specific cookie value to the API.
	//
	// https://spec.openapis.org/oas/v3.1.0#parameter-locations
	InCookie = "cookie"
)

// Parameter describes a single operation parameter.
// A unique parameter is defined by a combination of a name and location.
//
// https://spec.openapis.org/oas/v3.1.0#parameter-object
//
// Example:
//
//	name: pet
//	description: Pets operations
type Parameter struct {
	// Example of the parameter’s potential value.
	// The example SHOULD match the specified schema and encoding properties if present.
	// The example field is mutually exclusive of the examples field.
	// Furthermore, if referencing a schema that contains an example, the example value SHALL override the example provided by the schema.
	// To represent examples of media types that cannot naturally be represented in JSON or YAML,
	// a string value can contain the example with escaping where necessary.
	Example any `json:"example,omitempty" yaml:"example,omitempty"`
	// A map containing the representations for the parameter.
	// The key is the media type and the value describes it.
	// The map MUST only contain one entry.
	Content map[string]*Extendable[MediaType] `json:"content,omitempty" yaml:"content,omitempty"`
	// Examples of the parameter’s potential value.
	// Each example SHOULD contain a value in the correct format as specified in the parameter encoding.
	// The examples field is mutually exclusive of the example field.
	// Furthermore, if referencing a schema that contains an example, the examples value SHALL override the example provided by the schema.
	Examples map[string]*RefOrSpec[Extendable[Example]] `json:"examples,omitempty" yaml:"examples,omitempty"`
	// The schema defining the type used for the parameter.
	Schema *RefOrSpec[Schema] `json:"schema,omitempty" yaml:"schema,omitempty"`
	// REQUIRED.
	// The location of the parameter.
	// Possible values are "query", "header", "path" or "cookie".
	In string `json:"in" yaml:"in"`
	// A brief description of the parameter.
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
	// REQUIRED.
	// The name of the parameter.
	// Parameter names are case sensitive.
	// If in is "path", the name field MUST correspond to a template expression occurring within the path field in the Paths Object.
	// See Path Templating for further information.
	// If in is "header" and the name field is "Accept", "Content-Type" or "Authorization", the parameter definition SHALL be ignored.
	// For all other cases, the name corresponds to the parameter name used by the in property.
	Name string `json:"name" yaml:"name"`
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
	// Sets the ability to pass empty-valued parameters.
	// This is valid only for query parameters and allows sending a parameter with an empty value.
	// Default value is false.
	// If style is used, and if behavior is n/a (cannot be serialized), the value of allowEmptyValue SHALL be ignored.
	// Use of this property is NOT RECOMMENDED, as it is likely to be removed in a later revision.
	AllowEmptyValue bool `json:"allowEmptyValue,omitempty" yaml:"allowEmptyValue,omitempty"`
	// Specifies that a parameter is deprecated and SHOULD be transitioned out of usage.
	// Default value is false.
	Deprecated bool `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
	// Determines whether this parameter is mandatory.
	// If the parameter location is "path", this property is REQUIRED and its value MUST be true.
	// Otherwise, the property MAY be included and its default value is false.
	Required bool `json:"required,omitempty" yaml:"required,omitempty"`
}

// NewParameterSpec creates Parameter object.
func NewParameterSpec() *RefOrSpec[Extendable[Parameter]] {
	return NewRefOrSpec[Extendable[Parameter]](nil, NewExtendable(&Parameter{}))
}

// NewParameterRef creates Ref object.
func NewParameterRef(ref *Ref) *RefOrSpec[Extendable[Parameter]] {
	return NewRefOrSpec[Extendable[Parameter]](ref, nil)
}
