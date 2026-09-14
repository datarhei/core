package totp

import (
	"testing"
	"time"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
)

func TestStoreEnrollment(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	store, err := NewStore(Config{
		Filesystem: memfs,
		Filepath:   "/totp.json",
		Issuer:     "Test",
	})
	require.NoError(t, err)

	status := store.Status()
	require.False(t, status.Enrolled)
	require.False(t, status.Pending)

	setup, err := store.Setup("admin")
	require.NoError(t, err)
	require.NotEmpty(t, setup.Secret)
	require.NotEmpty(t, setup.URI)

	status = store.Status()
	require.False(t, status.Enrolled)
	require.True(t, status.Pending)

	code, err := totp.GenerateCode(setup.Secret, time.Now())
	require.NoError(t, err)

	require.NoError(t, store.Enable(code))

	status = store.Status()
	require.True(t, status.Enrolled)
	require.False(t, status.Pending)

	loginCode, err := totp.GenerateCode(setup.Secret, time.Now())
	require.NoError(t, err)
	require.True(t, store.Validate(loginCode))
	require.False(t, store.Validate("000000"))

	disableCode, err := totp.GenerateCode(setup.Secret, time.Now())
	require.NoError(t, err)
	require.NoError(t, store.Disable(disableCode))

	status = store.Status()
	require.False(t, status.Enrolled)
	require.False(t, status.Pending)
}

func TestStorePersists(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	store, err := NewStore(Config{
		Filesystem: memfs,
		Filepath:   "/totp.json",
	})
	require.NoError(t, err)

	setup, err := store.Setup("admin")
	require.NoError(t, err)

	code, err := totp.GenerateCode(setup.Secret, time.Now())
	require.NoError(t, err)
	require.NoError(t, store.Enable(code))

	reloaded, err := NewStore(Config{
		Filesystem: memfs,
		Filepath:   "/totp.json",
	})
	require.NoError(t, err)
	require.True(t, reloaded.Enrolled())
}
