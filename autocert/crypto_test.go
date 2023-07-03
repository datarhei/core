package autocert

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncryptDecrypt(t *testing.T) {
	c := NewCrypto("foobar")

	data := "top secret"

	encryptedData, err := c.Encrypt([]byte(data))
	require.NoError(t, err)
	require.NotEqual(t, []byte(data), encryptedData)

	decryptedData, err := c.Decrypt(encryptedData)
	require.NoError(t, err)
	require.Equal(t, []byte(data), decryptedData)
}

func TestEncryptDecryptWrongSecret(t *testing.T) {
	c1 := NewCrypto("foobar")
	c2 := NewCrypto("foobaz")

	data := "top secret"

	encryptedData, err := c1.Encrypt([]byte(data))
	require.NoError(t, err)
	require.NotEqual(t, []byte(data), encryptedData)

	_, err = c2.Decrypt(encryptedData)
	require.Error(t, err)

	decryptedData, err := c1.Decrypt(encryptedData)
	require.NoError(t, err)
	require.Equal(t, []byte(data), decryptedData)
}
