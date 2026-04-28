package spec

// PathItem describes the operations available on a single path.
// A Path Item MAY be empty, due to ACL constraints.
// The path itself is still exposed to the documentation viewer but they will not know which operations and parameters are available.
//
// https://spec.openapis.org/oas/v3.1.0#path-item-object
//
// Example:
//
//	get:
//	  description: Returns pets based on ID
//	  summary: Find pets by ID
//	  operationId: getPetsById
//	  responses:
//	    '200':
//	      description: pet response
//	      content:
//	        '*/*' :
//	          schema:
//	            type: array
//	            items:
//	              $ref: '#/components/schemas/Pet'
//	    default:
//	      description: error payload
//	      content:
//	        'text/html':
//	          schema:
//	            $ref: '#/components/schemas/ErrorModel'
//	parameters:
//	- name: id
//	  in: path
//	  description: ID of pet to use
//	  required: true
//	  schema:
//	    type: array
//	    items:
//	      type: string
//	  style: simple
type PathItem struct {
	// An optional, string summary, intended to apply to all operations in this path.
	Summary string `json:"summary,omitempty" yaml:"summary,omitempty"`
	// An optional, string description, intended to apply to all operations in this path.
	// CommonMark syntax MAY be used for rich text representation.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// A definition of a GET operation on this path.
	Get *Extendable[Operation] `json:"get,omitempty" yaml:"get,omitempty"`
	// A definition of a PUT operation on this path.
	Put *Extendable[Operation] `json:"put,omitempty" yaml:"put,omitempty"`
	// A definition of a POST operation on this path.
	Post *Extendable[Operation] `json:"post,omitempty" yaml:"post,omitempty"`
	// A definition of a DELETE operation on this path.
	Delete *Extendable[Operation] `json:"delete,omitempty" yaml:"delete,omitempty"`
	// A definition of a OPTIONS operation on this path.
	Options *Extendable[Operation] `json:"options,omitempty" yaml:"options,omitempty"`
	// A definition of a HEAD operation on this path.
	Head *Extendable[Operation] `json:"head,omitempty" yaml:"head,omitempty"`
	// A definition of a PATCH operation on this path.
	Patch *Extendable[Operation] `json:"patch,omitempty" yaml:"patch,omitempty"`
	// A definition of a TRACE operation on this path.
	Trace *Extendable[Operation] `json:"trace,omitempty" yaml:"trace,omitempty"`
	// An alternative server array to service all operations in this path.
	Servers []*Extendable[Server] `json:"servers,omitempty" yaml:"servers,omitempty"`
	// A list of parameters that are applicable for all the operations described under this path.
	// These parameters can be overridden at the operation level, but cannot be removed there.
	// The list MUST NOT include duplicated parameters.
	// A unique parameter is defined by a combination of a name and location.
	// The list can use the Reference Object to link to parameters that are defined at the OpenAPI Object’s components/parameters.
	Parameters []*RefOrSpec[Extendable[Parameter]] `json:"parameters,omitempty" yaml:"parameters,omitempty"`
}

// NewPathItemSpec creates PathItem object.
func NewPathItemSpec() *RefOrSpec[Extendable[PathItem]] {
	return NewRefOrSpec[Extendable[PathItem]](nil, NewExtendable(&PathItem{}))
}

// NewPathItemRef creates Ref object.
func NewPathItemRef(ref *Ref) *RefOrSpec[Extendable[PathItem]] {
	return NewRefOrSpec[Extendable[PathItem]](ref, nil)
}
