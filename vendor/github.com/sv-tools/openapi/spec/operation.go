package spec

// Operation Describes a single API operation on a path.
//
// https://spec.openapis.org/oas/v3.1.0#operation-object
//
// Example:
//
//	tags:
//	- pet
//	summary: Updates a pet in the store with form data
//	operationId: updatePetWithForm
//	parameters:
//	- name: petId
//	  in: path
//	  description: ID of pet that needs to be updated
//	  required: true
//	  schema:
//	    type: string
//	requestBody:
//	  content:
//	    'application/x-www-form-urlencoded':
//	      schema:
//	       type: object
//	       properties:
//	          name:
//	            description: Updated name of the pet
//	            type: string
//	          status:
//	            description: Updated status of the pet
//	            type: string
//	       required:
//	         - status
//	responses:
//	  '200':
//	    description: Pet updated.
//	    content:
//	      'application/json': {}
//	      'application/xml': {}
//	  '405':
//	    description: Method Not Allowed
//	    content:
//	      'application/json': {}
//	      'application/xml': {}
//	security:
//	- petstore_auth:
//	  - write:pets
//	  - read:pets
type Operation struct {
	// The request body applicable for this operation.
	// The requestBody is fully supported in HTTP methods where the HTTP 1.1 specification [RFC7231] has
	// explicitly defined semantics for request bodies.
	// In other cases where the HTTP spec is vague (such as [GET](section-4.3.1), [HEAD](section-4.3.2) and
	// [DELETE](section-4.3.5)), requestBody is permitted but does not have well-defined semantics and SHOULD be avoided if possible.
	RequestBody *RefOrSpec[Extendable[RequestBody]] `json:"requestBody,omitempty" yaml:"requestBody,omitempty"`
	// The list of possible responses as they are returned from executing this operation.
	Responses *Extendable[Responses] `json:"responses,omitempty" yaml:"responses,omitempty"`
	// A map of possible out-of band callbacks related to the parent operation.
	// The key is a unique identifier for the Callback Object.
	// Each value in the map is a Callback Object that describes a request that may be initiated by the API provider and the expected responses.
	Callbacks map[string]*RefOrSpec[Extendable[Callback]] `json:"callbacks,omitempty" yaml:"callbacks,omitempty"`
	// Additional external documentation for this operation.
	ExternalDocs *Extendable[ExternalDocs] `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
	// Unique string used to identify the operation.
	// The id MUST be unique among all operations described in the API.
	// The operationId value is case-sensitive.
	// Tools and libraries MAY use the operationId to uniquely identify an operation, therefore,
	// it is RECOMMENDED to follow common programming naming conventions.
	OperationID string `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	// A short summary of what the operation does.
	Summary string `json:"summary,omitempty" yaml:"summary,omitempty"`
	// A verbose explanation of the operation behavior.
	// CommonMark syntax MAY be used for rich text representation.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// A list of parameters that are applicable for this operation.
	// If a parameter is already defined at the Path Item, the new definition will override it but can never remove it.
	// The list MUST NOT include duplicated parameters.
	// A unique parameter is defined by a combination of a name and location.
	// The list can use the Reference Object to link to parameters that are defined at the OpenAPI Object’s components/parameters.
	Parameters []*RefOrSpec[Extendable[Parameter]] `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	// A list of tags for API documentation control.
	// Tags can be used for logical grouping of operations by resources or any other qualifier.
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	// A declaration of which security mechanisms can be used for this operation.
	// The list of values includes alternative security requirement objects that can be used.
	// Only one of the security requirement objects need to be satisfied to authorize a request.
	// To make security optional, an empty security requirement ({}) can be included in the array.
	// This definition overrides any declared top-level security.
	// To remove a top-level security declaration, an empty array can be used.
	Security []SecurityRequirement `json:"security,omitempty" yaml:"security,omitempty"`
	// An alternative server array to service this operation.
	// If an alternative server object is specified at the Path Item Object or Root level, it will be overridden by this value.
	Servers []*Extendable[Server] `json:"servers,omitempty" yaml:"servers,omitempty"`
	// Declares this operation to be deprecated.
	// Consumers SHOULD refrain from usage of the declared operation.
	// Default value is false.
	Deprecated bool `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
}

// NewOperation creates Operation object.
func NewOperation() *Extendable[Operation] {
	return NewExtendable(&Operation{})
}
