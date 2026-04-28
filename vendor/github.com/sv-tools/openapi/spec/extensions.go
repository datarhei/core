package spec

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const ExtensionPrefix = "x-"

// Extendable allows extensions to the OpenAPI Schema.
// The field name MUST begin with `x-`, for example, `x-internal-id`.
// Field names beginning `x-oai-` and `x-oas-` are reserved for uses defined by the OpenAPI Initiative.
// The value can be null, a primitive, an array or an object.
//
// https://spec.openapis.org/oas/v3.1.0#specification-extensions
//
// Example:
//
//	  openapi: 3.1.0
//	  info:
//	    title: Sample Pet Store App
//	    summary: A pet store manager.
//	    description: This is a sample server for a pet store.
//	    version: 1.0.1
//	    x-build-data: 2006-01-02T15:04:05Z07:00
//		x-build-commit-id: dac33af14d0d4a5f1c226141042ca7cefc6aeb75
type Extendable[T any] struct {
	Spec       *T             `json:"-" yaml:"-"`
	Extensions map[string]any `json:"-" yaml:"-"`
}

// WithExt sets the extension and returns the current object (self|this).
// The `x-` will be added automatically to given name.
func (o *Extendable[T]) WithExt(name string, value any) *Extendable[T] {
	if o.Extensions == nil {
		o.Extensions = make(map[string]any, 1)
	}
	if !strings.HasPrefix(name, ExtensionPrefix) {
		name = ExtensionPrefix + name
	}
	o.Extensions[name] = value
	return o
}

// NewExtendable creates new Extendable object for given spec
func NewExtendable[T any](spec *T) *Extendable[T] {
	ext := Extendable[T]{
		Spec:       spec,
		Extensions: make(map[string]any),
	}
	return &ext
}

// MarshalJSON implements json.Marshaler interface.
func (o *Extendable[T]) MarshalJSON() ([]byte, error) {
	var raw map[string]json.RawMessage
	exts, err := json.Marshal(&o.Extensions)
	if err != nil {
		return nil, fmt.Errorf("%T.Extensions: %w", o.Spec, err)
	}
	if err := json.Unmarshal(exts, &raw); err != nil {
		return nil, fmt.Errorf("%T(raw extensions): %w", o.Spec, err)
	}
	fields, err := json.Marshal(&o.Spec)
	if err != nil {
		return nil, fmt.Errorf("%T: %w", o.Spec, err)
	}
	if err := json.Unmarshal(fields, &raw); err != nil {
		return nil, fmt.Errorf("%T(raw fields): %w", o.Spec, err)
	}
	data, err := json.Marshal(&raw)
	if err != nil {
		return nil, fmt.Errorf("%T(raw): %w", o.Spec, err)
	}
	return data, nil
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (o *Extendable[T]) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("%T: %w", o.Spec, err)
	}
	o.Extensions = make(map[string]any)
	for name, value := range raw {
		if strings.HasPrefix(name, ExtensionPrefix) {
			var v any
			if err := json.Unmarshal(value, &v); err != nil {
				return fmt.Errorf("%T.Extensions.%s: %w", o.Spec, name, err)
			}
			o.Extensions[name] = v
			delete(raw, name)
		}
	}
	fields, err := json.Marshal(&raw)
	if err != nil {
		return fmt.Errorf("%T(raw): %w", o.Spec, err)
	}
	if err := json.Unmarshal(fields, &o.Spec); err != nil {
		return fmt.Errorf("%T: %w", o.Spec, err)
	}

	return nil
}

// MarshalYAML implements yaml.Marshaler interface.
func (o *Extendable[T]) MarshalYAML() (any, error) {
	var raw map[string]any
	exts, err := yaml.Marshal(&o.Extensions)
	if err != nil {
		return nil, fmt.Errorf("%T.Extensions: %w", o.Spec, err)
	}
	if err := yaml.Unmarshal(exts, &raw); err != nil {
		return nil, fmt.Errorf("%T(raw extensions): %w", o.Spec, err)
	}
	fields, err := yaml.Marshal(&o.Spec)
	if err != nil {
		return nil, fmt.Errorf("%T: %w", o.Spec, err)
	}
	if err := yaml.Unmarshal(fields, &raw); err != nil {
		return nil, fmt.Errorf("%T(raw fields): %w", o.Spec, err)
	}
	return raw, nil
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (o *Extendable[T]) UnmarshalYAML(node *yaml.Node) error {
	var raw map[string]any
	if err := node.Decode(&raw); err != nil {
		return fmt.Errorf("%T: %w", o.Spec, err)
	}
	o.Extensions = make(map[string]any)
	for name, value := range raw {
		if strings.HasPrefix(name, ExtensionPrefix) {
			o.Extensions[name] = value
			delete(raw, name)
		}
	}
	fields, err := yaml.Marshal(&raw)
	if err != nil {
		return fmt.Errorf("%T(raw): %w", o.Spec, err)
	}
	if err := yaml.Unmarshal(fields, &o.Spec); err != nil {
		return fmt.Errorf("%T: %w", o.Spec, err)
	}

	return nil
}
