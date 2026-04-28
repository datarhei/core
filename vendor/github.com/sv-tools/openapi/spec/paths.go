package spec

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Paths holds the relative paths to the individual endpoints and their operations.
// The path is appended to the URL from the Server Object in order to construct the full URL.
// The Paths MAY be empty, due to Access Control List (ACL) constraints.
//
// https://spec.openapis.org/oas/v3.1.0#paths-object
//
// Example:
//
//	/pets:
//	  get:
//	    description: Returns all pets from the system that the user has access to
//	    responses:
//	      '200':
//	        description: A list of pets.
//	        content:
//	          application/json:
//	            schema:
//	              type: array
//	              items:
//	                $ref: '#/components/schemas/pet'
type Paths struct {
	// A relative path to an individual endpoint.
	// The field name MUST begin with a forward slash (/).
	// The path is appended (no relative URL resolution) to the expanded URL
	// from the Server Object’s url field in order to construct the full URL.
	// Path templating is allowed.
	// When matching URLs, concrete (non-templated) paths would be matched before their templated counterparts.
	// Templated paths with the same hierarchy but different templated names MUST NOT exist as they are identical.
	// In case of ambiguous matching, it’s up to the tooling to decide which one to use.
	Paths map[string]*RefOrSpec[Extendable[PathItem]] `json:"-" yaml:"-"`
}

// NewPaths creates Paths object.
func NewPaths() *Extendable[Paths] {
	p := map[string]*RefOrSpec[Extendable[PathItem]]{}
	return NewExtendable(&Paths{Paths: p})
}

// WithPathItem adds the PathItem object to the Paths map.
func (o *Paths) WithPathItem(name string, v any) *Paths {
	var p *RefOrSpec[Extendable[PathItem]]
	switch spec := v.(type) {
	case *RefOrSpec[Extendable[PathItem]]:
		p = spec
	case *Extendable[PathItem]:
		p = NewRefOrSpec[Extendable[PathItem]](nil, spec)
	case *PathItem:
		p = NewRefOrSpec[Extendable[PathItem]](nil, NewExtendable(spec))
	default:
		panic(fmt.Errorf("wrong PathItem type: %T", spec))
	}
	if o.Paths == nil {
		o.Paths = make(map[string]*RefOrSpec[Extendable[PathItem]])
	}
	o.Paths[name] = p
	return o
}

// MarshalJSON implements json.Marshaler interface.
func (o *Paths) MarshalJSON() ([]byte, error) {
	return json.Marshal(&o.Paths)
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (o *Paths) UnmarshalYAML(node *yaml.Node) error {
	return node.Decode(&o.Paths)
}

// MarshalYAML implements yaml.Marshaler interface.
func (o *Paths) MarshalYAML() (any, error) {
	return o.Paths, nil
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (o *Paths) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &o.Paths)
}
