package spec

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Callback is a map of possible out-of band callbacks related to the parent operation.
// Each value in the map is a Path Item Object that describes a set of requests that may be initiated by
// the API provider and the expected responses.
// The key value used to identify the path item object is an expression, evaluated at runtime,
// that identifies a URL to use for the callback operation.
// To describe incoming requests from the API provider independent from another API call, use the webhooks field.
//
// https://spec.openapis.org/oas/v3.1.0#callback-object
//
// Example:
//
//	myCallback:
//	  '{$request.query.queryUrl}':
//	    post:
//	      requestBody:
//	        description: Callback payload
//	        content:
//	          'application/json':
//	            schema:
//	              $ref: '#/components/schemas/SomePayload'
//	      responses:
//	        '200':
//	          description: callback successfully processed
type Callback struct {
	Callback map[string]*RefOrSpec[Extendable[PathItem]]
}

// NewCallbackSpec creates Callback object.
func NewCallbackSpec() *RefOrSpec[Extendable[Callback]] {
	o := make(map[string]*RefOrSpec[Extendable[PathItem]])
	spec := NewExtendable(&Callback{
		Callback: o,
	})
	return NewRefOrSpec[Extendable[Callback]](nil, spec)
}

// NewCallbackRef creates Ref object.
func NewCallbackRef(ref *Ref) *RefOrSpec[Extendable[Callback]] {
	return NewRefOrSpec[Extendable[Callback]](ref, nil)
}

// WithPathItem adds the PathItem object to the Callback map.
func (o *Callback) WithPathItem(name string, v any) *Callback {
	var p *RefOrSpec[Extendable[PathItem]]
	switch spec := v.(type) {
	case *RefOrSpec[Extendable[PathItem]]:
		p = spec
	case *Extendable[PathItem]:
		p = NewRefOrSpec[Extendable[PathItem]](nil, spec)
	case *PathItem:
		p = NewRefOrSpec[Extendable[PathItem]](nil, NewExtendable(spec))
	default:
		panic(fmt.Errorf("wrong Callback type: %T", spec))
	}
	if o.Callback == nil {
		o.Callback = make(map[string]*RefOrSpec[Extendable[PathItem]])
	}
	o.Callback[name] = p
	return o
}

// MarshalJSON implements json.Marshaler interface.
func (o *Callback) MarshalJSON() ([]byte, error) {
	return json.Marshal(&o.Callback)
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (o *Callback) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &o.Callback)
}

// MarshalYAML implements yaml.Marshaler interface.
func (o *Callback) MarshalYAML() (any, error) {
	return o.Callback, nil
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (o *Callback) UnmarshalYAML(node *yaml.Node) error {
	return node.Decode(&o.Callback)
}
