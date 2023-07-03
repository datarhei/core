package autocert

import (
	"context"

	"github.com/caddyserver/certmagic"
)

type cryptoStorage struct {
	secret Crypto

	storage certmagic.Storage
}

func NewCryptoStorage(storage certmagic.Storage, secret Crypto) certmagic.Storage {
	s := &cryptoStorage{
		secret:  secret,
		storage: storage,
	}

	return s
}

func (s *cryptoStorage) Lock(ctx context.Context, name string) error {
	return s.storage.Lock(ctx, name)
}

func (s *cryptoStorage) Unlock(ctx context.Context, name string) error {
	return s.storage.Unlock(ctx, name)
}

func (s *cryptoStorage) Store(ctx context.Context, key string, value []byte) error {
	encryptedValue, err := s.secret.Encrypt(value)
	if err != nil {
		return err
	}

	return s.storage.Store(ctx, key, encryptedValue)
}

func (s *cryptoStorage) Load(ctx context.Context, key string) ([]byte, error) {
	encryptedValue, err := s.storage.Load(ctx, key)
	if err != nil {
		return nil, err
	}

	value, err := s.secret.Decrypt(encryptedValue)
	if err != nil {
		return nil, err
	}

	return value, nil
}

func (s *cryptoStorage) Delete(ctx context.Context, key string) error {
	return s.storage.Delete(ctx, key)
}

func (s *cryptoStorage) Exists(ctx context.Context, key string) bool {
	return s.storage.Exists(ctx, key)
}

func (s *cryptoStorage) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	return s.storage.List(ctx, prefix, recursive)
}

func (s *cryptoStorage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	keyInfo, err := s.storage.Stat(ctx, key)
	if err != nil {
		return certmagic.KeyInfo{}, err
	}

	value, err := s.Load(ctx, key)
	if err != nil {
		return certmagic.KeyInfo{}, err
	}

	keyInfo.Size = int64(len(value))

	return keyInfo, nil
}
