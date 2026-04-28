package spec

import "fmt"

// Components holds a set of reusable objects for different aspects of the OAS.
// All objects defined within the components object will have no effect on the API unless they are explicitly referenced
// from properties outside the components object.
//
// https://spec.openapis.org/oas/v3.1.0#components-object
//
// Example:
//
//	components:
//	  schemas:
//	    GeneralError:
//	      type: object
//	      properties:
//	        code:
//	          type: integer
//	          format: int32
//	        message:
//	          type: string
//	    Category:
//	      type: object
//	      properties:
//	        id:
//	          type: integer
//	          format: int64
//	        name:
//	          type: string
//	    Tag:
//	      type: object
//	      properties:
//	        id:
//	          type: integer
//	          format: int64
//	        name:
//	          type: string
//	  parameters:
//	    skipParam:
//	      name: skip
//	      in: query
//	      description: number of items to skip
//	      required: true
//	      schema:
//	        type: integer
//	        format: int32
//	    limitParam:
//	      name: limit
//	      in: query
//	      description: max records to return
//	      required: true
//	      schema:
//	        type: integer
//	        format: int32
//	  responses:
//	    NotFound:
//	      description: Entity not found.
//	    IllegalInput:
//	      description: Illegal input for operation.
//	    GeneralError:
//	      description: General Error
//	      content:
//	        application/json:
//	          schema:
//	            $ref: '#/components/schemas/GeneralError'
//	  securitySchemes:
//	    api_key:
//	      type: apiKey
//	      name: api_key
//	      in: header
//	    petstore_auth:
//	      type: oauth2
//	      flows:
//	        implicit:
//	          authorizationUrl: https://example.org/api/oauth/dialog
//	          scopes:
//	            write:pets: modify pets in your account
//	            read:pets: read your pets
type Components struct {
	// An object to hold reusable Schema Objects.
	Schemas map[string]*RefOrSpec[Schema] `json:"schemas,omitempty" yaml:"schemas,omitempty"`
	// An object to hold reusable Response Objects.
	Responses map[string]*RefOrSpec[Extendable[Response]] `json:"responses,omitempty" yaml:"responses,omitempty"`
	// An object to hold reusable Parameter Objects.
	Parameters map[string]*RefOrSpec[Extendable[Parameter]] `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	// An object to hold reusable Example Objects.
	Examples map[string]*RefOrSpec[Extendable[Example]] `json:"examples,omitempty" yaml:"examples,omitempty"`
	// An object to hold reusable Request Body Objects.
	RequestBodies map[string]*RefOrSpec[Extendable[RequestBody]] `json:"requestBodies,omitempty" yaml:"requestBodies,omitempty"`
	// An object to hold reusable Header Objects.
	Headers map[string]*RefOrSpec[Extendable[Header]] `json:"headers,omitempty" yaml:"headers,omitempty"`
	// An object to hold reusable Security Scheme Objects.
	SecuritySchemes map[string]*RefOrSpec[Extendable[SecurityScheme]] `json:"securitySchemes,omitempty" yaml:"securitySchemes,omitempty"`
	// An object to hold reusable Link Objects.
	Links map[string]*RefOrSpec[Extendable[Link]] `json:"links,omitempty" yaml:"links,omitempty"`
	// An object to hold reusable Callback Objects.
	Callbacks map[string]*RefOrSpec[Extendable[Callback]] `json:"callbacks,omitempty" yaml:"callbacks,omitempty"`
	// An object to hold reusable Path Item Object.
	Paths map[string]*RefOrSpec[Extendable[PathItem]] `json:"paths,omitempty" yaml:"paths,omitempty"`
}

// NewComponents creates new Components object.
func NewComponents() *Extendable[Components] {
	return NewExtendable(&Components{})
}

// WithRefOrSpec adds the given object to the appropriate list based on a type and returns the current object (self|this).
func (o *Components) WithRefOrSpec(name string, v any) *Components {
	switch spec := v.(type) {
	case *RefOrSpec[Schema]:
		if o.Schemas == nil {
			o.Schemas = make(map[string]*RefOrSpec[Schema], 1)
		}
		o.Schemas[name] = spec
	case *Schema:
		if o.Schemas == nil {
			o.Schemas = make(map[string]*RefOrSpec[Schema], 1)
		}
		o.Schemas[name] = NewRefOrSpec(nil, spec)
	case *RefOrSpec[Extendable[Response]]:
		if o.Responses == nil {
			o.Responses = make(map[string]*RefOrSpec[Extendable[Response]], 1)
		}
		o.Responses[name] = spec
	case *Extendable[Response]:
		if o.Responses == nil {
			o.Responses = make(map[string]*RefOrSpec[Extendable[Response]], 1)
		}
		o.Responses[name] = NewRefOrSpec(nil, spec)
	case *Response:
		if o.Responses == nil {
			o.Responses = make(map[string]*RefOrSpec[Extendable[Response]], 1)
		}
		o.Responses[name] = NewRefOrSpec(nil, NewExtendable(spec))
	case *RefOrSpec[Extendable[Parameter]]:
		if o.Parameters == nil {
			o.Parameters = make(map[string]*RefOrSpec[Extendable[Parameter]], 1)
		}
		o.Parameters[name] = spec
	case *Extendable[Parameter]:
		if o.Parameters == nil {
			o.Parameters = make(map[string]*RefOrSpec[Extendable[Parameter]], 1)
		}
		o.Parameters[name] = NewRefOrSpec(nil, spec)
	case *Parameter:
		if o.Parameters == nil {
			o.Parameters = make(map[string]*RefOrSpec[Extendable[Parameter]], 1)
		}
		o.Parameters[name] = NewRefOrSpec(nil, NewExtendable(spec))
	case *RefOrSpec[Extendable[Example]]:
		if o.Examples == nil {
			o.Examples = make(map[string]*RefOrSpec[Extendable[Example]], 1)
		}
		o.Examples[name] = spec
	case *Extendable[Example]:
		if o.Examples == nil {
			o.Examples = make(map[string]*RefOrSpec[Extendable[Example]], 1)
		}
		o.Examples[name] = NewRefOrSpec(nil, spec)
	case *Example:
		if o.Examples == nil {
			o.Examples = make(map[string]*RefOrSpec[Extendable[Example]], 1)
		}
		o.Examples[name] = NewRefOrSpec(nil, NewExtendable(spec))
	case *RefOrSpec[Extendable[RequestBody]]:
		if o.RequestBodies == nil {
			o.RequestBodies = make(map[string]*RefOrSpec[Extendable[RequestBody]], 1)
		}
		o.RequestBodies[name] = spec
	case *Extendable[RequestBody]:
		if o.RequestBodies == nil {
			o.RequestBodies = make(map[string]*RefOrSpec[Extendable[RequestBody]], 1)
		}
		o.RequestBodies[name] = NewRefOrSpec(nil, spec)
	case *RequestBody:
		if o.RequestBodies == nil {
			o.RequestBodies = make(map[string]*RefOrSpec[Extendable[RequestBody]], 1)
		}
		o.RequestBodies[name] = NewRefOrSpec(nil, NewExtendable(spec))
	case *RefOrSpec[Extendable[Header]]:
		if o.Headers == nil {
			o.Headers = make(map[string]*RefOrSpec[Extendable[Header]], 1)
		}
		o.Headers[name] = spec
	case *Extendable[Header]:
		if o.Headers == nil {
			o.Headers = make(map[string]*RefOrSpec[Extendable[Header]], 1)
		}
		o.Headers[name] = NewRefOrSpec(nil, spec)
	case *Header:
		if o.Headers == nil {
			o.Headers = make(map[string]*RefOrSpec[Extendable[Header]], 1)
		}
		o.Headers[name] = NewRefOrSpec(nil, NewExtendable(spec))
	case *RefOrSpec[Extendable[SecurityScheme]]:
		if o.SecuritySchemes == nil {
			o.SecuritySchemes = make(map[string]*RefOrSpec[Extendable[SecurityScheme]], 1)
		}
		o.SecuritySchemes[name] = spec
	case *Extendable[SecurityScheme]:
		if o.SecuritySchemes == nil {
			o.SecuritySchemes = make(map[string]*RefOrSpec[Extendable[SecurityScheme]], 1)
		}
		o.SecuritySchemes[name] = NewRefOrSpec(nil, spec)
	case *SecurityScheme:
		if o.SecuritySchemes == nil {
			o.SecuritySchemes = make(map[string]*RefOrSpec[Extendable[SecurityScheme]], 1)
		}
		o.SecuritySchemes[name] = NewRefOrSpec(nil, NewExtendable(spec))
	case *RefOrSpec[Extendable[Link]]:
		if o.Links == nil {
			o.Links = make(map[string]*RefOrSpec[Extendable[Link]], 1)
		}
		o.Links[name] = spec
	case *Extendable[Link]:
		if o.Links == nil {
			o.Links = make(map[string]*RefOrSpec[Extendable[Link]], 1)
		}
		o.Links[name] = NewRefOrSpec(nil, spec)
	case *Link:
		if o.Links == nil {
			o.Links = make(map[string]*RefOrSpec[Extendable[Link]], 1)
		}
		o.Links[name] = NewRefOrSpec(nil, NewExtendable(spec))
	case *RefOrSpec[Extendable[Callback]]:
		if o.Callbacks == nil {
			o.Callbacks = make(map[string]*RefOrSpec[Extendable[Callback]], 1)
		}
		o.Callbacks[name] = spec
	case *Extendable[Callback]:
		if o.Callbacks == nil {
			o.Callbacks = make(map[string]*RefOrSpec[Extendable[Callback]], 1)
		}
		o.Callbacks[name] = NewRefOrSpec(nil, spec)
	case *Callback:
		if o.Callbacks == nil {
			o.Callbacks = make(map[string]*RefOrSpec[Extendable[Callback]], 1)
		}
		o.Callbacks[name] = NewRefOrSpec(nil, NewExtendable(spec))
	case *RefOrSpec[Extendable[PathItem]]:
		if o.Paths == nil {
			o.Paths = make(map[string]*RefOrSpec[Extendable[PathItem]], 1)
		}
		o.Paths[name] = spec
	case *Extendable[PathItem]:
		if o.Paths == nil {
			o.Paths = make(map[string]*RefOrSpec[Extendable[PathItem]], 1)
		}
		o.Paths[name] = NewRefOrSpec(nil, spec)
	case *PathItem:
		if o.Paths == nil {
			o.Paths = make(map[string]*RefOrSpec[Extendable[PathItem]], 1)
		}
		o.Paths[name] = NewRefOrSpec(nil, NewExtendable(spec))
	default:
		panic(fmt.Errorf("wrong component type: %T", spec))
	}
	return o
}
