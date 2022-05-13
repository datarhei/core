package store

import (
	gojson "encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sync"

	"github.com/datarhei/core/encoding/json"
	"github.com/datarhei/core/io/file"
	"github.com/datarhei/core/log"
)

type JSONConfig struct {
	Dir    string
	Logger log.Logger
}

type jsonStore struct {
	filename string
	dir      string
	logger   log.Logger

	// Mutex to serialize access to the backend
	lock sync.RWMutex
}

var version uint64 = 4

func NewJSONStore(config JSONConfig) Store {
	s := &jsonStore{
		filename: "db.json",
		dir:      config.Dir,
		logger:   config.Logger,
	}

	if s.logger == nil {
		s.logger = log.New("JSONStore")
	}

	return s
}

func (s *jsonStore) Load() (StoreData, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	data, err := s.load(version)
	if err != nil {
		return NewStoreData(), err
	}

	data.sanitize()

	return data, nil
}

func (s *jsonStore) Store(data StoreData) error {
	if data.Version != version {
		return fmt.Errorf("invalid version (have: %d, want: %d)", data.Version, version)
	}

	s.lock.RLock()
	defer s.lock.RUnlock()

	err := s.store(data)
	if err != nil {
		return fmt.Errorf("failed to store data: %w", err)
	}

	return nil
}

func (s *jsonStore) store(data StoreData) error {
	jsondata, err := gojson.MarshalIndent(&data, "", "    ")
	if err != nil {
		return err
	}

	tmpfile, err := ioutil.TempFile(s.dir, s.filename)
	if err != nil {
		return err
	}

	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write(jsondata); err != nil {
		return err
	}

	if err := tmpfile.Close(); err != nil {
		return err
	}

	filename := s.dir + "/" + s.filename

	if err := file.Rename(tmpfile.Name(), filename); err != nil {
		return err
	}

	s.logger.WithField("file", filename).Debug().Log("Stored data")

	return nil
}

type storeVersion struct {
	Version uint64 `json:"version"`
}

func (s *jsonStore) load(version uint64) (StoreData, error) {
	r := NewStoreData()

	filename := s.dir + "/" + s.filename

	_, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}

		return r, err
	}

	jsondata, err := ioutil.ReadFile(filename)
	if err != nil {
		return r, err
	}

	var db storeVersion

	if err = gojson.Unmarshal(jsondata, &db); err != nil {
		return r, json.FormatError(jsondata, err)
	}

	if db.Version != version {
		return r, fmt.Errorf("unsupported version of the DB file (want: %d, have: %d)", version, db.Version)
	}

	if err = gojson.Unmarshal(jsondata, &r); err != nil {
		return r, json.FormatError(jsondata, err)
	}

	s.logger.WithField("file", filename).Debug().Log("Read data")

	return r, nil
}
