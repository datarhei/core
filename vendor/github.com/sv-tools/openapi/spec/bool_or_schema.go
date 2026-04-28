package spec

import (
	"encoding/json"

	"gopkg.in/yaml.v3"
)

// BoolOrSchema handles Boolean or Schema type.
//
// It MUST be used as a pointer,
// otherwise the `false` can be omitted by json or yaml encoders in case of `omitempty` tag is set.
type BoolOrSchema struct {
	Schema  *RefOrSpec[Schema]
	Allowed bool
}

// NewBoolOrSchema creates BoolOrSchema object.
func NewBoolOrSchema(allowed bool, spec *RefOrSpec[Schema]) *BoolOrSchema {
	return &BoolOrSchema{
		Allowed: allowed,
		Schema:  spec,
	}
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (o *BoolOrSchema) UnmarshalJSON(data []byte) error {
	if json.Unmarshal(data, &o.Allowed) == nil {
		o.Schema = nil
		return nil
	}
	if err := json.Unmarshal(data, &o.Schema); err != nil {
		return err
	}
	o.Allowed = true
	return nil
}

// MarshalJSON implements json.Marshaler interface.
func (o *BoolOrSchema) MarshalJSON() ([]byte, error) {
	var v any
	if o.Schema != nil {
		v = o.Schema
	} else {
		v = o.Allowed
	}
	return json.Marshal(&v)
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (o *BoolOrSchema) UnmarshalYAML(node *yaml.Node) error {
	if node.Decode(&o.Allowed) == nil {
		o.Schema = nil
		return nil
	}
	if err := node.Decode(&o.Schema); err != nil {
		return err
	}
	o.Allowed = true
	return nil
}

// MarshalYAML implements yaml.Marshaler interface.
func (o *BoolOrSchema) MarshalYAML() (any, error) {
	var v any
	if o.Schema != nil {
		v = o.Schema
	} else {
		v = o.Allowed
	}

	return v, nil
}
