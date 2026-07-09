package totp

import (
	"testing"
	"time"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
)

func TestIssueAndValidateTrust(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	store, err := NewStore(Config{Filesystem: memfs})
	require.NoError(t, err)

	setup, err := store.Setup("admin")
	require.NoError(t, err)

	code, err := totp.GenerateCode(setup.Secret, time.Now())
	require.NoError(t, err)
	require.NoError(t, store.Enable(code))

	token, err := store.IssueTrust("admin", Remember30Days)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.True(t, store.TrustValid(token, "admin"))
	require.False(t, store.TrustValid(token, "other"))
	require.False(t, store.TrustValid("invalid", "admin"))

	require.NoError(t, store.Disable(code))

	require.False(t, store.TrustValid(token, "admin"))
}
