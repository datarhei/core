package spec

// ServerVariable is an object representing a Server Variable for server URL template substitution.
//
// https://spec.openapis.org/oas/v3.1.0#server-variable-object
type ServerVariable struct {
	// REQUIRED.
	// The default value to use for substitution, which SHALL be sent if an alternate value is not supplied.
	// Note this behavior is different than the Schema Object’s treatment of default values,
	// because in those cases parameter values are optional.
	// If the enum is defined, the value MUST exist in the enum’s values.
	Default string `json:"default" yaml:"default"`
	// An optional description for the server variable.
	// CommonMark syntax MAY be used for rich text representation.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// An enumeration of string values to be used if the substitution options are from a limited set.
	// The array MUST NOT be empty.
	Enum []string `json:"enum,omitempty" yaml:"enum,omitempty"`
}

// NewServerVariable creates ServerVariable object.
func NewServerVariable() *Extendable[ServerVariable] {
	return NewExtendable(&ServerVariable{})
}
