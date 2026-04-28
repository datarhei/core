package spec

// Discriminator is used when request bodies or response payloads may be one of a number of different schemas,
// a discriminator object can be used to aid in serialization, deserialization, and validation.
// The discriminator is a specific object in a schema which is used to inform the consumer of the document of
// an alternative schema based on the value associated with it.
// When using the discriminator, inline schemas will not be considered.
//
// https://spec.openapis.org/oas/v3.1.0#discriminator-object
//
// Example:
//
//	MyResponseType:
//	  oneOf:
//	  - $ref: '#/components/schemas/Cat'
//	  - $ref: '#/components/schemas/Dog'
//	  - $ref: '#/components/schemas/Lizard'
//	  - $ref: 'https://gigantic-server.com/schemas/Monster/schema.json'
//	  discriminator:
//	    propertyName: petType
//	    mapping:
//	      dog: '#/components/schemas/Dog'
//	      monster: 'https://gigantic-server.com/schemas/Monster/schema.json'
type Discriminator struct {
	// An object to hold mappings between payload values and schema names or references.
	Mapping map[string]string `json:"mapping,omitempty" yaml:"mapping,omitempty"`
	// REQUIRED.
	// The name of the property in the payload that will hold the discriminator value.
	PropertyName string `json:"propertyName" yaml:"propertyName"`
}

// NewDiscriminator creates Discriminator object.
func NewDiscriminator() *Discriminator {
	return &Discriminator{}
}
