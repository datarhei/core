package spec

// OpenAPI is the root object of the OpenAPI document.
//
// https://spec.openapis.org/oas/v3.1.0#openapi-object
//
// Example:
//
//	openapi: 3.1.0
//	info:
//	  title: Minimal OpenAPI example
//	  version: 1.0.0
//	paths: { }
type OpenAPI struct {
	// An element to hold various schemas for the document.
	Components *Extendable[Components] `json:"components,omitempty" yaml:"components,omitempty"`
	// REQUIRED
	// Provides metadata about the API. The metadata MAY be used by tooling as required.
	Info *Extendable[Info] `json:"info" yaml:"info"`
	// Additional external documentation.
	ExternalDocs *Extendable[ExternalDocs] `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
	// Holds the relative paths to the individual endpoints and their operations.
	// The path is appended to the URL from the Server Object in order to construct the full URL.
	// The Paths MAY be empty, due to Access Control List (ACL) constraints.
	Paths *Extendable[Paths] `json:"paths,omitempty" yaml:"paths,omitempty"`
	// The incoming webhooks that MAY be received as part of this API and that the API consumer MAY choose to implement.
	// Closely related to the callbacks feature, this section describes requests initiated other than by an API call,
	// for example by an out of band registration.
	// The key name is a unique string to refer to each webhook, while the (optionally referenced) PathItem Object describes
	// a request that may be initiated by the API provider and the expected responses.
	WebHooks map[string]*RefOrSpec[Extendable[PathItem]] `json:"webhooks,omitempty" yaml:"webhooks,omitempty"`
	// The default value for the $schema keyword within Schema Objects contained within this OAS document.
	// This MUST be in the form of a URI.
	JsonSchemaDialect string `json:"jsonSchemaDialect,omitempty" yaml:"jsonSchemaDialect,omitempty"`
	// REQUIRED
	// This string MUST be the version number of the OpenAPI Specification that the OpenAPI document uses.
	// The openapi field SHOULD be used by tooling to interpret the OpenAPI document.
	// This is not related to the API info.version string.
	OpenAPI string `json:"openapi" yaml:"openapi"`
	// A declaration of which security mechanisms can be used across the API.
	// The list of values includes alternative security requirement objects that can be used.
	// Only one of the security requirement objects need to be satisfied to authorize a request.
	// Individual operations can override this definition.
	// To make security optional, an empty security requirement ({}) can be included in the array.
	Security []SecurityRequirement `json:"security,omitempty" yaml:"security,omitempty"`
	// A list of tags used by the document with additional metadata.
	// The order of the tags can be used to reflect on their order by the parsing tools.
	// Not all tags that are used by the Operation Object must be declared.
	// The tags that are not declared MAY be organized randomly or based on the tools’ logic.
	// Each tag name in the list MUST be unique.
	Tags []*Extendable[Tag] `json:"tags,omitempty" yaml:"tags,omitempty"`
	// An array of Server Objects, which provide connectivity information to a target server.
	// If the servers property is not provided, or is an empty array, the default value would be a Server Object with a url value of /.
	Servers []*Extendable[Server] `json:"servers,omitempty" yaml:"servers,omitempty"`
}

// NewOpenAPI creates OpenAPI object.
func NewOpenAPI() *Extendable[OpenAPI] {
	return NewExtendable(&OpenAPI{})
}
