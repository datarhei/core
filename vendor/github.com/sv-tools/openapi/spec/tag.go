package spec

// Tag adds metadata to a single tag that is used by the Operation Object.
// It is not mandatory to have a Tag Object per tag defined in the Operation Object instances.
//
// https://spec.openapis.org/oas/v3.1.0#tag-object
//
// Example:
//
//	name: pet
//	description: Pets operations
type Tag struct {
	// Additional external documentation for this tag.
	ExternalDocs *Extendable[ExternalDocs] `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
	// REQUIRED.
	// The name of the tag.
	Name string `json:"name" yaml:"name"`
	// A description for the tag.
	// CommonMark syntax MAY be used for rich text representation.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// NewTag creates Tag object.
func NewTag() *Extendable[Tag] {
	return NewExtendable(&Tag{})
}
