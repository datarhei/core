package jwt

import "errors"

// ValidationError is returned when login credentials were recognized but rejected.
type ValidationError struct {
	Detail string
	Err    error
}

func (e ValidationError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}

	return ""
}

func (e ValidationError) Unwrap() error {
	return e.Err
}

var (
	errInvalidCredentials = errors.New("invalid username or password")
	errTOTPRequired       = errors.New("TOTP code required")
	errTOTPInvalid        = errors.New("invalid TOTP code")
)
