package api

// Login are the requires login credentials
type Login struct {
	Username         string `json:"username" validate:"required" jsonschema:"minLength=1"`
	Password         string `json:"password" validate:"required" jsonschema:"minLength=1"`
	TOTPCode         string `json:"totp_code,omitempty"`
	DeviceTrustToken string `json:"device_trust_token,omitempty"`
	RememberDevice   string `json:"remember_device,omitempty" jsonschema:"enum=30d,enum=1y"`
}
