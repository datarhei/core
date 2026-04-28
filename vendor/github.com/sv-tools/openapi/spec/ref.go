package spec

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Ref is a simple object to allow referencing other components in the OpenAPI document, internally and externally.
// The $ref string value contains a URI [RFC3986], which identifies the location of the value being referenced.
// See the rules for resolving Relative References.
//
// https://spec.openapis.org/oas/v3.1.0#reference-object
//
// Example:
//
//	$ref: '#/components/schemas/Pet'
type Ref struct {
	// REQUIRED.
	// The reference identifier.
	// This MUST be in the form of a URI.
	Ref string `json:"$ref" yaml:"$ref"`
	// A short summary which by default SHOULD override that of the referenced component.
	// If the referenced object-type does not allow a summary field, then this field has no effect.
	Summary string `json:"summary,omitempty" yaml:"summary,omitempty"`
	// A description which by default SHOULD override that of the referenced component.
	// CommonMark syntax MAY be used for rich text representation.
	// If the referenced object-type does not allow a description field, then this field has no effect.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// NewRef creates an object of Ref type.
func NewRef(ref string) *Ref {
	return &Ref{
		Ref: ref,
	}
}

// WithRef sets the Ref and returns the current object (self|this).
func (o *Ref) WithRef(v string) *Ref {
	o.Ref = v
	return o
}

// WithSummary sets the Summary and returns the current object (self|this).
func (o *Ref) WithSummary(v string) *Ref {
	o.Summary = v
	return o
}

// WithDescription sets the Description and returns the current object (self|this).
func (o *Ref) WithDescription(v string) *Ref {
	o.Description = v
	return o
}

// RefOrSpec holds either Ref or any OpenAPI spec type.
//
// NOTE: The Ref object takes precedent over Spec if using json or yaml Marshal and Unmarshal functions.
type RefOrSpec[T any] struct {
	Ref  *Ref `json:"-" yaml:"-"`
	Spec *T   `json:"-" yaml:"-"`
}

// NewRefOrSpec creates an object of RefOrSpec type for either Ref or Spec
func NewRefOrSpec[T any](ref *Ref, spec *T) *RefOrSpec[T] {
	o := RefOrSpec[T]{}
	switch {
	case ref != nil:
		o.Ref = ref
	case spec != nil:
		o.Spec = spec
	}
	return &o
}

// GetSpec return a Spec if it is set or loads it from Components in case of Ref or an error
func (o *RefOrSpec[T]) GetSpec(c *Extendable[Components]) (*T, error) {
	return o.getSpec(c, make(visitedRefs))
}

type visitedRefs map[string]bool

func (o visitedRefs) String() string {
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

func (o *RefOrSpec[T]) getSpec(c *Extendable[Components], visited visitedRefs) (*T, error) {
	// some guards
	switch {
	case o.Spec != nil:
		return o.Spec, nil
	case o.Ref == nil:
		return nil, fmt.Errorf("spect not found; all visited refs: %s", visited)
	case visited[o.Ref.Ref]:
		return nil, fmt.Errorf("cycle ref %q detected; all visited refs: %s", o.Ref.Ref, visited)
	case !strings.HasPrefix(o.Ref.Ref, "#/components/"):
		// TODO: support loading by url
		return nil, fmt.Errorf("loading outside of components is not implemented for the ref %q; all visited refs: %s", o.Ref.Ref, visited)
	case c == nil:
		return nil, fmt.Errorf("components is required, but got nil; all visited refs: %s", visited)
	}
	visited[o.Ref.Ref] = true

	parts := strings.SplitN(o.Ref.Ref[13:], "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("incorrect ref %q; all visited refs: %s", o.Ref.Ref, visited)
	}
	objName := parts[1]
	var ref any
	switch parts[0] {
	case "schemas":
		ref = c.Spec.Schemas[objName]
	case "responses":
		ref = c.Spec.Responses[objName]
	case "parameters":
		ref = c.Spec.Parameters[objName]
	case "examples":
		ref = c.Spec.Examples[objName]
	case "requestBodies":
		ref = c.Spec.RequestBodies[objName]
	case "headers":
		ref = c.Spec.Headers[objName]
	case "links":
		ref = c.Spec.Links[objName]
	case "callbacks":
		ref = c.Spec.Callbacks[objName]
	case "paths":
		ref = c.Spec.Paths[objName]
	}
	obj, ok := ref.(*RefOrSpec[T])
	if !ok {
		return nil, fmt.Errorf("expected spec of type %T, but got %T; all visited refs: %s", NewRefOrSpec[T](nil, nil), ref, visited)
	}
	if obj.Spec != nil {
		return obj.Spec, nil
	}
	return obj.getSpec(c, visited)
}

// MarshalJSON implements json.Marshaler interface.
func (o *RefOrSpec[T]) MarshalJSON() ([]byte, error) {
	var v any
	if o.Ref != nil {
		v = o.Ref
	} else {
		v = o.Spec
	}
	data, err := json.Marshal(&v)
	if err != nil {
		return nil, fmt.Errorf("%T: %w", o.Spec, err)
	}
	return data, nil
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (o *RefOrSpec[T]) UnmarshalJSON(data []byte) error {
	if json.Unmarshal(data, &o.Ref) == nil && o.Ref.Ref != "" {
		o.Spec = nil
		return nil
	}

	o.Ref = nil
	if err := json.Unmarshal(data, &o.Spec); err != nil {
		return fmt.Errorf("%T: %w", o.Spec, err)
	}
	return nil
}

// MarshalYAML implements yaml.Marshaler interface.
func (o *RefOrSpec[T]) MarshalYAML() (any, error) {
	var v any
	if o.Ref != nil {
		v = o.Ref
	} else {
		v = o.Spec
	}
	return v, nil
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (o *RefOrSpec[T]) UnmarshalYAML(node *yaml.Node) error {
	if node.Decode(&o.Ref) == nil && o.Ref.Ref != "" {
		return nil
	}

	o.Ref = nil
	if err := node.Decode(&o.Spec); err != nil {
		return fmt.Errorf("%T: %w", o.Spec, err)
	}
	return nil
}
