package api

// TOTPStatus is the current TOTP enrollment state.
type TOTPStatus struct {
	Enrolled bool `json:"enrolled"`
	Pending  bool `json:"pending"`
}

// TOTPSetup is returned when starting TOTP enrollment.
type TOTPSetup struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
}

// TOTPEnable enables TOTP after setup by confirming an authenticator code.
type TOTPEnable struct {
	Code string `json:"code" validate:"required" jsonschema:"minLength=1"`
}

// TOTPDisable disables TOTP enrollment.
type TOTPDisable struct {
	Code string `json:"code" validate:"required" jsonschema:"minLength=1"`
}
