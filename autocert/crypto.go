package autocert

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"

	"golang.org/x/crypto/scrypt"
)

type Crypto interface {
	// Encrypt encrypts the given data or returns error if encrypting the data is not possible.
	Encrypt(data []byte) ([]byte, error)

	// Decrypt decrypts the given data or returns error if decrypting the data is not possible.
	Decrypt(data []byte) ([]byte, error)
}

type crypto struct {
	secret []byte
}

// NewCrypto returns a new implementation of the the Crypto interface that encrypts/decrypts
// the given data with a key and salt derived from the provided secret.
// Based on https://itnext.io/encrypt-data-with-a-password-in-go-b5366384e291
func NewCrypto(secret string) Crypto {
	c := &crypto{
		secret: []byte(secret),
	}

	return c
}

func (c *crypto) Encrypt(data []byte) ([]byte, error) {
	key, salt, err := c.deriveKey(nil)
	if err != nil {
		return nil, err
	}

	blockCipher, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(blockCipher)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// The first gcm.NonceSize() are the nonce
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	// The last 32 bytes are the salt
	ciphertext = append(ciphertext, salt...)

	return ciphertext, nil
}

func (c *crypto) Decrypt(data []byte) ([]byte, error) {
	// The last 32 bytes are the salt
	salt, data := data[len(data)-32:], data[:len(data)-32]
	key, _, err := c.deriveKey(salt)
	if err != nil {
		return nil, err
	}

	blockCipher, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(blockCipher)
	if err != nil {
		return nil, err
	}

	// The first gcm.NonceSize() are the nonce
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func (c *crypto) deriveKey(salt []byte) ([]byte, []byte, error) {
	if salt == nil {
		salt = make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return nil, nil, err
		}
	}

	key, err := scrypt.Key(c.secret, salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, nil, err
	}

	return key, salt, nil
}
