package cluster

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/caddyserver/certmagic"
)

type clusterStorage struct {
	kvs     KVS
	prefix  string
	locks   map[string]*Lock
	muLocks sync.Mutex
}

func NewClusterStorage(kvs KVS, prefix string) (certmagic.Storage, error) {
	s := &clusterStorage{
		kvs:    kvs,
		prefix: prefix,
		locks:  map[string]*Lock{},
	}

	return s, nil
}

func (s *clusterStorage) prefixKey(key string) string {
	return path.Join(s.prefix, key)
}

func (s *clusterStorage) unprefixKey(key string) string {
	return strings.TrimPrefix(key, s.prefix+"/")
}

func (s *clusterStorage) Lock(ctx context.Context, name string) error {
	for {
		lock, err := s.kvs.CreateLock(s.prefixKey(name), time.Now().Add(time.Minute))
		if err == nil {
			go func() {
				<-lock.Expired()
				s.Unlock(context.Background(), name)
			}()
			s.muLocks.Lock()
			s.locks[name] = lock
			s.muLocks.Unlock()

			return nil
		}

		select {
		case <-time.After(time.Second):
			continue
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Minute):
			return fmt.Errorf("wasn't able to acquire lock")
		}
	}
}

func (s *clusterStorage) Unlock(ctx context.Context, name string) error {
	err := s.kvs.DeleteLock(s.prefixKey(name))
	if err != nil {
		return err
	}

	s.muLocks.Lock()
	delete(s.locks, name)
	s.muLocks.Unlock()

	return nil
}

// Store puts value at key.
func (s *clusterStorage) Store(ctx context.Context, key string, value []byte) error {
	encodedValue := base64.StdEncoding.EncodeToString(value)
	return s.kvs.SetKV(s.prefixKey(key), encodedValue)
}

// Load retrieves the value at key.
func (s *clusterStorage) Load(ctx context.Context, key string) ([]byte, error) {
	encodedValue, _, err := s.kvs.GetKV(s.prefixKey(key))
	if err != nil {
		return nil, err
	}

	return base64.StdEncoding.DecodeString(encodedValue)
}

// Delete deletes key. An error should be
// returned only if the key still exists
// when the method returns.
func (s *clusterStorage) Delete(ctx context.Context, key string) error {
	return s.kvs.UnsetKV(s.prefixKey(key))
}

// Exists returns true if the key exists
// and there was no error checking.
func (s *clusterStorage) Exists(ctx context.Context, key string) bool {
	_, _, err := s.kvs.GetKV(s.prefixKey(key))
	return err == nil
}

// List returns all keys that match prefix.
// If recursive is true, non-terminal keys
// will be enumerated (i.e. "directories"
// should be walked); otherwise, only keys
// prefixed exactly by prefix will be listed.
func (s *clusterStorage) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	values := s.kvs.ListKV(s.prefixKey(prefix))

	keys := []string{}

	for key := range values {
		keys = append(keys, s.unprefixKey(key))
	}

	if len(keys) == 0 {
		return nil, fs.ErrNotExist
	}

	if recursive {
		return keys, nil
	}

	prefix = strings.TrimSuffix(prefix, "/")

	keyMap := map[string]struct{}{}
	for _, key := range keys {
		elms := strings.Split(strings.TrimPrefix(key, prefix+"/"), "/")
		keyMap[elms[0]] = struct{}{}
	}

	keys = []string{}
	for key := range keyMap {
		keys = append(keys, path.Join(prefix, key))
	}

	return keys, nil
}

// Stat returns information about key.
func (s *clusterStorage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	encodedValue, lastModified, err := s.kvs.GetKV(s.prefixKey(key))
	if err != nil {
		return certmagic.KeyInfo{}, err
	}

	value, err := base64.StdEncoding.DecodeString(encodedValue)
	if err != nil {
		return certmagic.KeyInfo{}, err
	}

	info := certmagic.KeyInfo{
		Key:        key,
		Modified:   lastModified,
		Size:       int64(len(value)),
		IsTerminal: false,
	}

	return info, nil
}
