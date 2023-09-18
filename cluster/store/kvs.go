package store

import (
	"io/fs"
	"strings"
	"time"
)

func (s *store) setKV(cmd CommandSetKV) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	value := s.data.KVS[cmd.Key]

	value.Value = cmd.Value
	value.UpdatedAt = time.Now()

	s.data.KVS[cmd.Key] = value

	return nil
}

func (s *store) unsetKV(cmd CommandUnsetKV) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, ok := s.data.KVS[cmd.Key]; !ok {
		return fs.ErrNotExist
	}

	delete(s.data.KVS, cmd.Key)

	return nil
}

func (s *store) ListKVS(prefix string) map[string]Value {
	s.lock.RLock()
	defer s.lock.RUnlock()

	m := map[string]Value{}

	for key, value := range s.data.KVS {
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		m[key] = value
	}

	return m
}

func (s *store) GetFromKVS(key string) (Value, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	value, ok := s.data.KVS[key]
	if !ok {
		return Value{}, fs.ErrNotExist
	}

	return value, nil
}
