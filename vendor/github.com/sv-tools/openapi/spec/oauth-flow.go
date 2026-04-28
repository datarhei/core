package spec

// OAuthFlow configuration details for a supported OAuth Flow
//
// https://spec.openapis.org/oas/v3.1.0#oauth-flow-object
//
// Example:
//
//	implicit:
//	  authorizationUrl: https://example.com/api/oauth/dialog
//	  scopes:
//	    write:pets: modify pets in your account
//	    read:pets: read your pets
//	authorizationCode
//	  authorizationUrl: https://example.com/api/oauth/dialog
//	  scopes:
//	    write:pets: modify pets in your account
//	    read:pets: read your pets
type OAuthFlow struct {
	// REQUIRED.
	// The available scopes for the OAuth2 security scheme.
	// A map between the scope name and a short description for it.
	// The map MAY be empty.
	//
	// Applies To: oauth2
	Scopes map[string]string `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	// REQUIRED.
	// The authorization URL to be used for this flow.
	// This MUST be in the form of a URL.
	// The OAuth2 standard requires the use of TLS.
	//
	// Applies To:oauth2 ("implicit", "authorizationCode")
	AuthorizationURL string `json:"authorizationUrl,omitempty" yaml:"authorizationUrl,omitempty"`
	// REQUIRED.
	// The token URL to be used for this flow.
	// This MUST be in the form of a URL.
	// The OAuth2 standard requires the use of TLS.
	//
	// Applies To: oauth2 ("password", "clientCredentials", "authorizationCode")
	TokenURL string `json:"tokenUrl,omitempty" yaml:"tokenUrl,omitempty"`
	// The URL to be used for obtaining refresh tokens.
	// This MUST be in the form of a URL.
	// The OAuth2 standard requires the use of TLS.
	//
	// Applies To: oauth2
	RefreshURL string `json:"refreshUrl,omitempty" yaml:"refreshUrl,omitempty"`
}

// NewOAuthFlow creates OAuthFlow object
func NewOAuthFlow() *Extendable[OAuthFlow] {
	return NewExtendable(&OAuthFlow{})
}
