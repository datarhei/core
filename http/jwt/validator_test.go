package jwt

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authtotp "github.com/datarhei/core/v16/auth/totp"
	"github.com/datarhei/core/v16/http/api"
	httpvalidator "github.com/datarhei/core/v16/http/validator"
	"github.com/datarhei/core/v16/io/fs"
	otplib "github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"

	"github.com/labstack/echo/v4"
)

type totpStub struct {
	enrolled bool
	secret   string
}

func (s *totpStub) Enrolled() bool {
	return s.enrolled
}

func (s *totpStub) Validate(code string) bool {
	return otplib.Validate(code, s.secret)
}

func (s *totpStub) TrustValid(token, username string) bool {
	return false
}

func TestLocalValidatorTOTPRequired(t *testing.T) {
	validator, err := NewLocalValidator("admin", "secret", &totpStub{enrolled: true, secret: "JBSWY3DPEHPK3PXP"})
	require.NoError(t, err)

	e := echo.New()
	e.Validator = httpvalidator.New()
	body, err := json.Marshal(api.Login{Username: "admin", Password: "secret"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	ok, subject, err := validator.Validate(c)
	require.True(t, ok)
	require.Empty(t, subject)
	require.Error(t, err)

	var verr ValidationError
	require.ErrorAs(t, err, &verr)
	require.Equal(t, "totp_required", verr.Detail)
}

func TestLocalValidatorTOTPValid(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	store, err := authtotp.NewStore(authtotp.Config{Filesystem: memfs})
	require.NoError(t, err)

	setup, err := store.Setup("admin")
	require.NoError(t, err)

	code, err := otplib.GenerateCode(setup.Secret, time.Now())
	require.NoError(t, err)
	require.NoError(t, store.Enable(code))

	loginCode, err := otplib.GenerateCode(setup.Secret, time.Now())
	require.NoError(t, err)

	validator, err := NewLocalValidator("admin", "secret", store)
	require.NoError(t, err)

	e := echo.New()
	e.Validator = httpvalidator.New()
	body, err := json.Marshal(api.Login{
		Username: "admin",
		Password: "secret",
		TOTPCode: loginCode,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	ok, subject, err := validator.Validate(c)
	require.True(t, ok)
	require.Equal(t, "admin", subject)
	require.NoError(t, err)
}
