package spec

import (
	"encoding/json"

	"gopkg.in/yaml.v3"
)

// Responses is a container for the expected responses of an operation.
// The container maps a HTTP response code to the expected response.
// The documentation is not necessarily expected to cover all possible HTTP response codes because they may not be known in advance.
// However, documentation is expected to cover a successful operation response and any known errors.
// The default MAY be used as a default response object for all HTTP codes that are not covered individually by the Responses Object.
// The Responses Object MUST contain at least one response code, and if only one response code is provided
// it SHOULD be the response for a successful operation call.
//
// https://spec.openapis.org/oas/v3.1.0#responses-object
//
// Example:
//
//	'200':
//	  description: a pet to be returned
//	  content:
//	    application/json:
//	      schema:
//	        $ref: '#/components/schemas/Pet'
//	default:
//	  description: Unexpected error
//	  content:
//	    application/json:
//	      schema:
//	        $ref: '#/components/schemas/ErrorModel'
type Responses struct {
	// The documentation of responses other than the ones declared for specific HTTP response codes.
	// Use this field to cover undeclared responses.
	Default *RefOrSpec[Extendable[Response]] `json:"default,omitempty" yaml:"default,omitempty"`
	// Any HTTP status code can be used as the property name, but only one property per code,
	// to describe the expected response for that HTTP status code.
	// This field MUST be enclosed in quotation marks (for example, “200”) for compatibility between JSON and YAML.
	// To define a range of response codes, this field MAY contain the uppercase wildcard character X.
	// For example, 2XX represents all response codes between [200-299].
	// Only the following range definitions are allowed: 1XX, 2XX, 3XX, 4XX, and 5XX.
	// If a response is defined using an explicit code, the explicit code definition takes precedence over the range definition for that code.
	Response map[string]*RefOrSpec[Extendable[Response]] `json:"-" yaml:"-"`
}

// NewResponses creates Paths object.
func NewResponses() *Extendable[Responses] {
	return NewExtendable(&Responses{})
}

// MarshalJSON implements json.Marshaler interface.
func (o *Responses) MarshalJSON() ([]byte, error) {
	var raw map[string]json.RawMessage
	data, err := json.Marshal(&o.Response)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	if o.Default != nil {
		data, err = json.Marshal(&o.Default)
		if err != nil {
			return nil, err
		}
		raw["default"] = data
	}
	return json.Marshal(&raw)
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (o *Responses) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["default"]; ok {
		if err := json.Unmarshal(v, &o.Default); err != nil {
			return err
		}
		delete(raw, "default")
	}
	data, err := json.Marshal(&raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &o.Response)
}

// MarshalYAML implements yaml.Marshaler interface.
func (o *Responses) MarshalYAML() (any, error) {
	var raw map[string]any
	data, err := yaml.Marshal(&o.Response)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	if o.Default != nil {
		raw["default"] = o.Default
	}
	return raw, nil
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (o *Responses) UnmarshalYAML(node *yaml.Node) error {
	var raw map[string]any
	if err := node.Decode(&raw); err != nil {
		return err
	}
	if v, ok := raw["default"]; ok {
		data, err := yaml.Marshal(&v)
		if err != nil {
			return err
		}
		if err := yaml.Unmarshal(data, &o.Default); err != nil {
			return err
		}
		delete(raw, "default")
	}
	data, err := yaml.Marshal(&raw)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, &o.Response)
}
