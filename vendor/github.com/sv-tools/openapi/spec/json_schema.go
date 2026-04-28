package spec

// JsonSchemaTypeString
//
// https://json-schema.org/understanding-json-schema/reference/string.html#string
type JsonSchemaTypeString struct {
	MinLength *int   `json:"minLength,omitempty" yaml:"minLength,omitempty"`
	MaxLength *int   `json:"maxLength,omitempty" yaml:"maxLength,omitempty"`
	Pattern   string `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	Format    string `json:"format,omitempty" yaml:"format,omitempty"`
}

// JsonSchemaTypeNumber
//
// https://json-schema.org/understanding-json-schema/reference/numeric.html#numeric-types
type JsonSchemaTypeNumber struct {
	// MultipleOf restricts the numbers to a multiple of a given number, using the multipleOf keyword.
	// It may be set to any positive number.
	//
	// https://json-schema.org/understanding-json-schema/reference/numeric.html#multiples
	MultipleOf *int `json:"multipleOf,omitempty" yaml:"multipleOf,omitempty"`
	// x ≥ minimum
	Minimum *int `json:"minimum,omitempty" yaml:"minimum,omitempty"`
	// x > exclusiveMinimum
	ExclusiveMinimum *int `json:"exclusiveMinimum,omitempty" yaml:"exclusiveMinimum,omitempty"`
	// x ≤ maximum
	Maximum *int `json:"maximum,omitempty" yaml:"maximum,omitempty"`
	// x < exclusiveMaximum
	ExclusiveMaximum *int `json:"exclusiveMaximum,omitempty" yaml:"exclusiveMaximum,omitempty"`
}

// JsonSchemaTypeObject
//
// https://json-schema.org/understanding-json-schema/reference/object.html#object
type JsonSchemaTypeObject struct {
	// The properties (key-value pairs) on an object are defined using the properties keyword.
	// The value of properties is an object, where each key is the name of a property and each value is
	// a schema used to validate that property.
	// Any property that doesn't match any of the property names in the properties keyword is ignored by this keyword.
	//
	// https://json-schema.org/understanding-json-schema/reference/object.html#properties
	Properties map[string]*RefOrSpec[Schema] `json:"properties,omitempty" yaml:"properties,omitempty"`
	// Sometimes you want to say that, given a particular kind of property name, the value should match a particular schema.
	// That’s where patternProperties comes in: it maps regular expressions to schemas.
	// If a property name matches the given regular expression, the property value must validate against the corresponding schema.
	//
	// https://json-schema.org/understanding-json-schema/reference/object.html#pattern-properties
	PatternProperties map[string]*RefOrSpec[Schema] `json:"patternProperties,omitempty" yaml:"patternProperties,omitempty"`
	// The additionalProperties keyword is used to control the handling of extra stuff, that is,
	// properties whose names are not listed in the properties keyword or match any of the regular expressions
	// in the patternProperties keyword.
	// By default any additional properties are allowed.
	//
	// The value of the additionalProperties keyword is a schema that will be used to validate any properties in the instance
	// that are not matched by properties or patternProperties.
	// Setting the additionalProperties schema to false means no additional properties will be allowed.
	//
	// https://json-schema.org/understanding-json-schema/reference/object.html#additional-properties
	AdditionalProperties *BoolOrSchema `json:"additionalProperties,omitempty" yaml:"additionalProperties,omitempty"`
	// The unevaluatedProperties keyword is similar to additionalProperties except that it can recognize properties declared in subschemas.
	// So, the example from the previous section can be rewritten without the need to redeclare properties.
	//
	// https://json-schema.org/understanding-json-schema/reference/object.html#unevaluated-properties
	UnevaluatedProperties *BoolOrSchema `json:"unevaluatedProperties,omitempty" yaml:"unevaluatedProperties,omitempty"`
	// The names of properties can be validated against a schema, irrespective of their values.
	// This can be useful if you don’t want to enforce specific properties, but you want to make sure that
	// the names of those properties follow a specific convention.
	// You might, for example, want to enforce that all names are valid ASCII tokens so they can be used
	// as attributes in a particular programming language.
	//
	// https://json-schema.org/understanding-json-schema/reference/object.html#property-names
	PropertyNames *RefOrSpec[Schema] `json:"propertyNames,omitempty" yaml:"propertyNames,omitempty"`
	// The min number of properties on an object.
	//
	// https://json-schema.org/understanding-json-schema/reference/object.html#size
	MinProperties *int `json:"minProperties,omitempty" yaml:"minProperties,omitempty"`
	// The max number of properties on an object.
	//
	// https://json-schema.org/understanding-json-schema/reference/object.html#size
	MaxProperties *int `json:"maxProperties,omitempty" yaml:"maxProperties,omitempty"`
	// The required keyword takes an array of zero or more strings.
	// Each of these strings must be unique.
	//
	// https://json-schema.org/understanding-json-schema/reference/object.html#required-properties
	Required []string `json:"required,omitempty" yaml:"required,omitempty"`
}

// JsonSchemaTypeArray
//
// https://json-schema.org/understanding-json-schema/reference/array.html#array
type JsonSchemaTypeArray struct {
	// List validation is useful for arrays of arbitrary length where each item matches the same schema.
	// For this kind of array, set the items keyword to a single schema that will be used to validate all of the items in the array.
	//
	// https://json-schema.org/understanding-json-schema/reference/array.html#items
	Items *BoolOrSchema `json:"items,omitempty" yaml:"items,omitempty"`
	// https://json-schema.org/understanding-json-schema/reference/array.html#length
	MaxItems *int `json:"maxItems,omitempty" yaml:"maxItems,omitempty"`
	// The unevaluatedItems keyword is similar to unevaluatedProperties, but for items.
	//
	// https://json-schema.org/understanding-json-schema/reference/array.html#unevaluated-items
	UnevaluatedItems *BoolOrSchema `json:"unevaluatedItems,omitempty" yaml:"unevaluatedItems,omitempty"`
	// While the items schema must be valid for every item in the array, the contains schema only needs
	// to validate against one or more items in the array.
	//
	// https://json-schema.org/understanding-json-schema/reference/array.html#contains
	Contains    *RefOrSpec[Schema] `json:"contains,omitempty" yaml:"contains,omitempty"`
	MinContains *int               `json:"minContains,omitempty" yaml:"minContains,omitempty"`
	MaxContains *int               `json:"maxContains,omitempty" yaml:"maxContains,omitempty"`
	// https://json-schema.org/understanding-json-schema/reference/array.html#length
	MinItems *int `json:"minItems,omitempty" yaml:"minItems,omitempty"`
	// A schema can ensure that each of the items in an array is unique.
	// Simply set the uniqueItems keyword to true.
	//
	// https://json-schema.org/understanding-json-schema/reference/array.html#uniqueness
	UniqueItems *bool `json:"uniqueItems,omitempty" yaml:"uniqueItems,omitempty"`
	// The prefixItems is an array, where each item is a schema that corresponds to each index of the document’s array.
	// That is, an array where the first element validates the first element of the input array,
	// the second element validates the second element of the input array, etc.
	//
	// https://json-schema.org/understanding-json-schema/reference/array.html#tuple-validation
	PrefixItems []*RefOrSpec[Schema] `json:"prefixItems,omitempty" yaml:"prefixItems,omitempty"`
}

// JsonSchemaGeneric
//
// https://json-schema.org/understanding-json-schema/reference/generic.html
type JsonSchemaGeneric struct {
	Default     any    `json:"default,omitempty" yaml:"default,omitempty"`
	Title       string `json:"title,omitempty" yaml:"title,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// The const keyword is used to restrict a value to a single value.
	//
	// https://json-schema.org/understanding-json-schema/reference/generic.html#constant-values
	Const string `json:"const,omitempty" yaml:"const,omitempty"`
	// The $comment keyword is strictly intended for adding comments to a schema.
	// Its value must always be a string.
	// Unlike the annotations title, description, and examples, JSON schema implementations aren’t allowed
	// to attach any meaning or behavior to it whatsoever, and may even strip them at any time.
	// Therefore, they are useful for leaving notes to future editors of a JSON schema,
	// but should not be used to communicate to users of the schema.
	//
	// https://json-schema.org/understanding-json-schema/reference/generic.html#comments
	Comment string `json:"$comment,omitempty" yaml:"$comment,omitempty"`
	// The enum keyword is used to restrict a value to a fixed set of values.
	// It must be an array with at least one element, where each element is unique.
	//
	// https://json-schema.org/understanding-json-schema/reference/generic.html#enumerated-values
	Enum      []any `json:"enum,omitempty" yaml:"enum,omitempty"`
	Examples  []any `json:"examples,omitempty" yaml:"examples,omitempty"`
	ReadOnly  bool  `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
	WriteOnly bool  `json:"writeOnly,omitempty" yaml:"writeOnly,omitempty"`
	// The deprecated keyword is a boolean that indicates that the instance value the keyword applies to
	// should not be used and may be removed in the future.
	Deprecated bool `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
}

// JsonSchemaMedia string-encoding non-JSON data
//
// https://json-schema.org/understanding-json-schema/reference/non_json_data.html
type JsonSchemaMedia struct {
	// https://json-schema.org/understanding-json-schema/reference/non_json_data.html#contentschema
	ContentSchema *RefOrSpec[Schema] `json:"contentSchema,omitempty" yaml:"contentSchema,omitempty"`
	// The contentMediaType keyword specifies the MIME type of the contents of a string, as described in RFC 2046.
	// There is a list of MIME types officially registered by the IANA, but the set of types supported will be
	// application and operating system dependent.
	//
	// https://json-schema.org/understanding-json-schema/reference/non_json_data.html#contentmediatype
	ContentMediaType string `json:"contentMediaType,omitempty" yaml:"contentMediaType,omitempty"`
	// The contentEncoding keyword specifies the encoding used to store the contents, as specified in RFC 2054, part 6.1 and RFC 4648.
	//
	// https://json-schema.org/understanding-json-schema/reference/non_json_data.html#contentencoding
	ContentEncoding string `json:"contentEncoding,omitempty" yaml:"contentEncoding,omitempty"`
}

// JsonSchemaComposition
//
// https://json-schema.org/understanding-json-schema/reference/combining.html
type JsonSchemaComposition struct {
	// The not keyword declares that an instance validates if it doesn’t validate against the given subschema.
	//
	// https://json-schema.org/understanding-json-schema/reference/combining.html#not
	Not *RefOrSpec[Schema] `json:"not,omitempty" yaml:"not,omitempty"`
	// To validate against allOf, the given data must be valid against all of the given subschemas.
	//
	// https://json-schema.org/understanding-json-schema/reference/combining.html#allof
	AllOf []*RefOrSpec[Schema] `json:"allOf,omitempty" yaml:"allOf,omitempty"`
	// To validate against anyOf, the given data must be valid against any (one or more) of the given subschemas.
	//
	// https://json-schema.org/understanding-json-schema/reference/combining.html#anyof
	AnyOf []*RefOrSpec[Schema] `json:"anyOf,omitempty" yaml:"anyOf,omitempty"`
	// To validate against oneOf, the given data must be valid against exactly one of the given subschemas.
	//
	// https://json-schema.org/understanding-json-schema/reference/combining.html#oneof
	OneOf []*RefOrSpec[Schema] `json:"oneOf,omitempty" yaml:"oneOf,omitempty"`
}

// JsonSchemaConditionals Applying Subschemas Conditionally
//
// https://json-schema.org/understanding-json-schema/reference/conditionals.html
type JsonSchemaConditionals struct {
	// The dependentRequired keyword conditionally requires that certain properties must be present if
	// a given property is present in an object.
	// For example, suppose we have a schema representing a customer.
	// If you have their credit card number, you also want to ensure you have a billing address.
	// If you don’t have their credit card number, a billing address would not be required.
	// We represent this dependency of one property on another using the dependentRequired keyword.
	// The value of the dependentRequired keyword is an object.
	// Each entry in the object maps from the name of a property, p, to an array of strings listing properties that
	// are required if p is present.
	//
	// https://json-schema.org/understanding-json-schema/reference/conditionals.html#dependentrequired
	DependentRequired map[string][]string `json:"dependentRequired,omitempty" yaml:"dependentRequired,omitempty"`
	// The dependentSchemas keyword conditionally applies a subschema when a given property is present.
	// This schema is applied in the same way allOf applies schemas.
	// Nothing is merged or extended.
	// Both schemas apply independently.
	//
	// https://json-schema.org/understanding-json-schema/reference/conditionals.html#dependentschemas
	DependentSchemas map[string]*RefOrSpec[Schema] `json:"dependentSchemas,omitempty" yaml:"dependentSchemas,omitempty"`

	// https://json-schema.org/understanding-json-schema/reference/conditionals.html#if-then-else
	If   *RefOrSpec[Schema] `json:"if,omitempty" yaml:"if,omitempty"`
	Then *RefOrSpec[Schema] `json:"then,omitempty" yaml:"then,omitempty"`
	Else *RefOrSpec[Schema] `json:"else,omitempty" yaml:"else,omitempty"`
}

type JsonSchemaCore struct {
	// https://json-schema.org/understanding-json-schema/reference/schema.html#schema
	Schema string `json:"$schema,omitempty" yaml:"$schema,omitempty"`
	// https://json-schema.org/understanding-json-schema/structuring.html#id
	ID string `json:"$id,omitempty" yaml:"$id,omitempty"`
	// https://json-schema.org/understanding-json-schema/structuring.html#defs
	Defs          map[string]*RefOrSpec[Schema] `json:"$defs,omitempty" yaml:"$defs,omitempty"`
	DynamicRef    string                        `json:"$dynamicRef,omitempty" yaml:"$dynamicRef,omitempty"`
	Vocabulary    map[string]bool               `json:"$vocabulary,omitempty" yaml:"$vocabulary,omitempty"`
	DynamicAnchor string                        `json:"$dynamicAnchor,omitempty" yaml:"dynamicAnchor,omitempty"`
	// https://json-schema.org/understanding-json-schema/reference/type.html
	Type *SingleOrArray[string] `json:"type,omitempty" yaml:"type,omitempty"`
}

// JsonSchema fields
//
// https://json-schema.org/understanding-json-schema/index.html
//
// NOTE: all the other fields are available via Extensions property
type JsonSchema struct {
	JsonSchemaTypeNumber   `yaml:",inline"`
	JsonSchemaConditionals `yaml:",inline"`
	JsonSchemaTypeString   `yaml:",inline"`
	JsonSchemaMedia        `yaml:",inline"`
	JsonSchemaCore         `yaml:",inline"`
	JsonSchemaTypeArray    `yaml:",inline"`
	JsonSchemaTypeObject   `yaml:",inline"`
	JsonSchemaComposition  `yaml:",inline"`
	JsonSchemaGeneric      `yaml:",inline"`
}
