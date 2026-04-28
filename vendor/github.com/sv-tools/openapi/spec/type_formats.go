package spec

const (
	// ******* Built-in OpenAPI formats *******
	//
	// https://spec.openapis.org/oas/v3.1.0#data-types

	Int32Format    = "int32"
	Int64Format    = "int64"
	FloatFormat    = "float"
	DoubleFormat   = "double"
	PasswordFormat = "password"

	// ******* Built-in JSON Schema formats *******
	//
	// https://json-schema.org/understanding-json-schema/reference/string.html#built-in-formats

	// DateTimeFormat is date and time together, for example, 2018-11-13T20:20:39+00:00.
	DateTimeFormat = "date-time"
	// TimeFormat is time, for example, 20:20:39+00:00
	TimeFormat = "time"
	// DateFormat is date, for example, 2018-11-13.
	DateFormat = "date"
	// DurationFormat is a duration as defined by the ISO 8601 ABNF for “duration”.
	// For example, P3D expresses a duration of 3 days.
	//
	// https://datatracker.ietf.org/doc/html/rfc3339#appendix-A
	DurationFormat = "duration"
	// EmailFormat is internet email address, see RFC 5321, section 4.1.2.
	//
	// https://tools.ietf.org/html/rfc5321#section-4.1.2
	EmailFormat = "email"
	// IDNEmailFormat is the internationalized form of an Internet email address, see RFC 6531.
	//
	// https://tools.ietf.org/html/rfc6531
	IDNEmailFormat = "idn-email"
	// HostnameFormat is internet host name, see RFC 1123, section 2.1.
	//
	// https://datatracker.ietf.org/doc/html/rfc1123#section-2.1
	HostnameFormat = "hostname"
	// IDNHostnameFormat is an internationalized Internet host name, see RFC5890, section 2.3.2.3.
	//
	// https://tools.ietf.org/html/rfc6531
	IDNHostnameFormat = "idn-hostname"
	// IPv4Format is IPv4 address, according to dotted-quad ABNF syntax as defined in RFC 2673, section 3.2.
	//
	// https://tools.ietf.org/html/rfc2673#section-3.2
	IPv4Format = "ipv4"
	// IPv6Format is IPv6 address, as defined in RFC 2373, section 2.2.
	//
	// https://tools.ietf.org/html/rfc2373#section-2.2
	IPv6Format = "ipv6"
	// UUIDFormat is a Universally Unique Identifier as defined by RFC 4122.
	// Example: 3e4666bf-d5e5-4aa7-b8ce-cefe41c7568a
	//
	// RFC 4122
	UUIDFormat = "uuid"
	// URIFormat is a universal resource identifier (URI), according to RFC3986.
	//
	// https://tools.ietf.org/html/rfc3986
	URIFormat = "uri"
	// URIReferenceFormat is a URI Reference (either a URI or a relative-reference), according to RFC3986, section 4.1.
	//
	// https://tools.ietf.org/html/rfc3986#section-4.1
	URIReferenceFormat = "uri-reference"
	// IRIFormat is the internationalized equivalent of a “uri”, according to RFC3987.
	//
	// https://tools.ietf.org/html/rfc3987
	IRIFormat = "iri"
	// IRIReferenceFormat is The internationalized equivalent of a “uri-reference”, according to RFC3987
	//
	// https://tools.ietf.org/html/rfc3987
	IRIReferenceFormat = "iri-reference"
	// URITemplateFormat is a URI Template (of any level) according to RFC6570.
	// If you don’t already know what a URI Template is, you probably don’t need this value.
	//
	// https://tools.ietf.org/html/rfc6570
	URITemplateFormat = "uri-template"
	// JsonPointerFormat is a JSON Pointer, according to RFC6901.
	// There is more discussion on the use of JSON Pointer within JSON Schema in Structuring a complex schema.
	// Note that this should be used only when the entire string contains only JSON Pointer content, e.g. /foo/bar.
	// JSON Pointer URI fragments, e.g. #/foo/bar/ should use "uri-reference".
	//
	// https://tools.ietf.org/html/rfc6901
	JsonPointerFormat = "json-pointer"
	// RelativeJsonPointerFormat is a relative JSON pointer.
	//
	// https://tools.ietf.org/html/draft-handrews-relative-json-pointer-01
	RelativeJsonPointerFormat = "relative-json-pointer"
	// RegexFormat is a regular expression, which should be valid according to the ECMA 262 dialect.
	//
	// https://www.ecma-international.org/publications/files/ECMA-ST/Ecma-262.pdf
	RegexFormat = "regex"
)
