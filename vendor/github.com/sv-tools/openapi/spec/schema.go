package spec

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// The Schema Object allows the definition of input and output data types.
// These types can be objects, but also primitives and arrays.
// This object is a superset of the JSON Schema Specification Draft 2020-12.
// For more information about the properties, see JSON Schema Core and JSON Schema Validation.
// Unless stated otherwise, the property definitions follow those of JSON Schema and do not add any additional semantics.
// Where JSON Schema indicates that behavior is defined by the application (e.g. for annotations),
// OAS also defers the definition of semantics to the application consuming the OpenAPI document.
//
// https://spec.openapis.org/oas/v3.1.0#schema-object
type Schema struct {
	JsonSchema `yaml:",inline"`

	// Adds support for polymorphism.
	// The discriminator is an object name that is used to differentiate between other schemas which may satisfy the payload description.
	// See Composition and Inheritance for more details.
	Discriminator *Discriminator `json:"discriminator,omitempty" yaml:"discriminator,omitempty"`
	// Additional external documentation for this tag.
	// xml
	XML *Extendable[XML] `json:"xml,omitempty" yaml:"xml,omitempty"`
	// Additional external documentation for this schema.
	ExternalDocs *Extendable[ExternalDocs] `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
	// A free-form property to include an example of an instance for this schema.
	// To represent examples that cannot be naturally represented in JSON or YAML, a string value can be used to
	// contain the example with escaping where necessary.
	//
	// Deprecated: The example property has been deprecated in favor of the JSON Schema examples keyword.
	// Use of example is discouraged, and later versions of this specification may remove it.
	Example any `json:"example,omitempty" yaml:"example,omitempty"`

	Extensions map[string]any `json:"-" yaml:"-"`
}

// NewSchemaSpec creates Schema object.
func NewSchemaSpec() *RefOrSpec[Schema] {
	return NewRefOrSpec[Schema](nil, &Schema{})
}

// NewSchemaRef creates Ref object.
func NewSchemaRef(ref *Ref) *RefOrSpec[Schema] {
	return NewRefOrSpec[Schema](ref, nil)
}

// WithExt sets the extension and returns the current object (self|this).
// Schema does not require special `x-` prefix.
// The extension will be ignored if the name overlaps with a struct field during marshalling to JSON or YAML.
func (o *Schema) WithExt(name string, value any) *Schema {
	if o.Extensions == nil {
		o.Extensions = make(map[string]any, 1)
	}
	o.Extensions[name] = value
	return o
}

// returns the list of public fields for given tag and ignores `-` names
func getFields(t reflect.Type, tag string) map[string]struct{} {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	n := t.NumField()
	ret := make(map[string]struct{})
	for i := 0; i < n; i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if f.Anonymous {
			sub := getFields(f.Type, tag)
			for n, v := range sub {
				ret[n] = v
			}
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get(tag), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		ret[name] = struct{}{}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

type intSchema Schema // needed to avoid recursion in marshal/unmarshal

// MarshalJSON implements json.Marshaler interface.
func (o *Schema) MarshalJSON() ([]byte, error) {
	var raw map[string]json.RawMessage
	exts, err := json.Marshal(&o.Extensions)
	if err != nil {
		return nil, fmt.Errorf("%T.Extensions: %w", o, err)
	}
	if err := json.Unmarshal(exts, &raw); err != nil {
		return nil, fmt.Errorf("%T(raw extensions): %w", o, err)
	}
	s := intSchema(*o)
	fields, err := json.Marshal(&s)
	if err != nil {
		return nil, fmt.Errorf("%T: %w", o, err)
	}
	if err := json.Unmarshal(fields, &raw); err != nil {
		return nil, fmt.Errorf("%T(raw fields): %w", o, err)
	}
	data, err := json.Marshal(&raw)
	if err != nil {
		return nil, fmt.Errorf("%T(raw): %w", o, err)
	}
	return data, nil
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (o *Schema) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("%T: %w", o, err)
	}
	exts := make(map[string]any)
	keys := getFields(reflect.TypeOf(o), "json")
	for name, value := range raw {
		if _, ok := keys[name]; !ok {
			var v any
			if err := json.Unmarshal(value, &v); err != nil {
				return fmt.Errorf("%T.Extensions.%s: %w", o, name, err)
			}
			exts[name] = v
			delete(raw, name)
		}
	}
	fields, err := json.Marshal(&raw)
	if err != nil {
		return fmt.Errorf("%T(raw): %w", o, err)
	}
	var s intSchema
	if err := json.Unmarshal(fields, &s); err != nil {
		return fmt.Errorf("%T: %w", o, err)
	}
	s.Extensions = exts
	*o = Schema(s)
	return nil
}

// MarshalYAML implements yaml.Marshaler interface.
func (o *Schema) MarshalYAML() (any, error) {
	var raw map[string]any
	exts, err := yaml.Marshal(&o.Extensions)
	if err != nil {
		return nil, fmt.Errorf("%T.Extensions: %w", o, err)
	}
	if err := yaml.Unmarshal(exts, &raw); err != nil {
		return nil, fmt.Errorf("%T(raw extensions): %w", o, err)
	}
	s := intSchema(*o)
	fields, err := yaml.Marshal(&s)
	if err != nil {
		return nil, fmt.Errorf("%T: %w", o, err)
	}
	if err := yaml.Unmarshal(fields, &raw); err != nil {
		return nil, fmt.Errorf("%T(raw fields): %w", o, err)
	}
	return raw, nil
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (o *Schema) UnmarshalYAML(node *yaml.Node) error {
	var raw map[string]any
	if err := node.Decode(&raw); err != nil {
		return fmt.Errorf("%T: %w", o, err)
	}
	exts := make(map[string]any)
	keys := getFields(reflect.TypeOf(o), "json")
	for name, value := range raw {
		if _, ok := keys[name]; !ok {
			exts[name] = value
			delete(raw, name)
		}
	}
	fields, err := yaml.Marshal(&raw)
	if err != nil {
		return fmt.Errorf("%T(raw): %w", o, err)
	}
	var s intSchema
	if err := yaml.Unmarshal(fields, &s); err != nil {
		return fmt.Errorf("%T: %w", o, err)
	}
	s.Extensions = exts
	*o = Schema(s)
	return nil
}
