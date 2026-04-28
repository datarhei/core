package spec

import (
	"encoding/json"

	"gopkg.in/yaml.v3"
)

// SingleOrArray holds list or single value
type SingleOrArray[T any] []T

// NewSingleOrArray creates SingleOrArray object.
func NewSingleOrArray[T any](v ...T) SingleOrArray[T] {
	a := append(SingleOrArray[T]{}, v...)
	return a
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (o *SingleOrArray[T]) UnmarshalJSON(data []byte) error {
	var ret []T
	if json.Unmarshal(data, &ret) != nil {
		var s T
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		ret = []T{s}
	}
	*o = ret
	return nil
}

// MarshalJSON implements json.Marshaler interface.
func (o *SingleOrArray[T]) MarshalJSON() ([]byte, error) {
	var v any = []T(*o)
	if len(*o) == 1 {
		v = (*o)[0]
	}
	return json.Marshal(&v)
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (o *SingleOrArray[T]) UnmarshalYAML(node *yaml.Node) error {
	var ret []T
	if node.Decode(&ret) != nil {
		var s T
		if err := node.Decode(&s); err != nil {
			return err
		}
		ret = []T{s}
	}
	*o = ret
	return nil
}

// MarshalYAML implements yaml.Marshaler interface.
func (o *SingleOrArray[T]) MarshalYAML() (any, error) {
	var v any = []T(*o)
	if len(*o) == 1 {
		v = (*o)[0]
	}
	return v, nil
}
