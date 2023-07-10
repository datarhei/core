package autocert

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
	"github.com/datarhei/core/v16/cluster/kvs"
	"github.com/datarhei/core/v16/log"
)

type storage struct {
	kvs     kvs.KVS
	prefix  string
	locks   map[string]*kvs.Lock
	muLocks sync.Mutex

	logger log.Logger
}

func NewStorage(kv kvs.KVS, prefix string, logger log.Logger) (certmagic.Storage, error) {
	s := &storage{
		kvs:    kv,
		prefix: prefix,
		locks:  map[string]*kvs.Lock{},
		logger: logger,
	}

	if s.logger == nil {
		s.logger = log.New("")
	}

	return s, nil
}

func (s *storage) prefixKey(key string) string {
	return path.Join(s.prefix, key)
}

func (s *storage) unprefixKey(key string) string {
	return strings.TrimPrefix(key, s.prefix+"/")
}

func (s *storage) Lock(ctx context.Context, name string) error {
	s.logger.Debug().WithField("name", name).Log("StorageLock")
	for {
		lock, err := s.kvs.CreateLock(s.prefixKey(name), time.Now().Add(5*time.Minute))
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

func (s *storage) Unlock(ctx context.Context, name string) error {
	s.logger.Debug().WithField("name", name).Log("StorageUnlock")
	err := s.kvs.DeleteLock(s.prefixKey(name))
	if err != nil {
		return err
	}

	s.muLocks.Lock()
	lock, ok := s.locks[name]
	if ok {
		lock.Unlock()
		delete(s.locks, name)
	}
	s.muLocks.Unlock()

	return nil
}

// Store puts value at key.
func (s *storage) Store(ctx context.Context, key string, value []byte) error {
	s.logger.Debug().WithField("key", key).Log("StorageStore")
	encodedValue := base64.StdEncoding.EncodeToString(value)
	return s.kvs.SetKV(s.prefixKey(key), encodedValue)
}

// Load retrieves the value at key.
func (s *storage) Load(ctx context.Context, key string) ([]byte, error) {
	s.logger.Debug().WithField("key", key).Log("StorageLoad")
	encodedValue, _, err := s.kvs.GetKV(s.prefixKey(key))
	if err != nil {
		s.logger.Debug().WithError(err).WithField("key", key).Log("StorageLoad")
		return nil, err
	}

	return base64.StdEncoding.DecodeString(encodedValue)
}

// Delete deletes key. An error should be
// returned only if the key still exists
// when the method returns.
func (s *storage) Delete(ctx context.Context, key string) error {
	s.logger.Debug().WithField("key", key).Log("StorageDelete")
	return s.kvs.UnsetKV(s.prefixKey(key))
}

// Exists returns true if the key exists
// and there was no error checking.
func (s *storage) Exists(ctx context.Context, key string) bool {
	s.logger.Debug().WithField("key", key).Log("StorageExits")
	_, _, err := s.kvs.GetKV(s.prefixKey(key))
	return err == nil
}

// List returns all keys that match prefix.
// If recursive is true, non-terminal keys
// will be enumerated (i.e. "directories"
// should be walked); otherwise, only keys
// prefixed exactly by prefix will be listed.
func (s *storage) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	s.logger.Debug().WithField("prefix", prefix).Log("StorageList")
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
func (s *storage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	s.logger.Debug().WithField("key", key).Log("StorageStat")
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
