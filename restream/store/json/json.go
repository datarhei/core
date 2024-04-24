package json

import (
	"errors"
	"fmt"
	"sync"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/store"
)

type Config struct {
	Filesystem fs.Filesystem
	Filepath   string // Full path to the database file
	Logger     log.Logger
}

type jsonStore struct {
	fs       fs.Filesystem
	filepath string
	logger   log.Logger

	// Mutex to serialize access to the disk
	lock sync.RWMutex
}

func New(config Config) (store.Store, error) {
	s := &jsonStore{
		fs:       config.Filesystem,
		filepath: config.Filepath,
		logger:   config.Logger,
	}

	if len(s.filepath) == 0 {
		s.filepath = "/db.json"
	}

	if s.fs == nil {
		return nil, fmt.Errorf("no valid filesystem provided")
	}

	if s.logger == nil {
		s.logger = log.New("")
	}

	return s, nil
}

func (s *jsonStore) Load() (store.Data, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	data := store.NewData()

	d, err := s.load(s.filepath, version)
	if err != nil {
		return data, err
	}

	for _, process := range d.Process {
		if data.Process[""] == nil {
			data.Process[""] = map[string]store.Process{}
		}

		p := data.Process[""][process.ID]

		p.Process = UnmarshalProcess(process)
		p.Metadata = map[string]interface{}{}

		data.Process[""][process.ID] = p
	}

	for pid, m := range d.Metadata.Process {
		if data.Process[""] == nil {
			data.Process[""] = map[string]store.Process{}
		}

		p := data.Process[""][pid]
		p.Metadata = m
		data.Process[""][pid] = p
	}

	for k, v := range d.Metadata.System {
		data.Metadata[k] = v
	}

	for name, domain := range d.Domain {
		if data.Process[name] == nil {
			data.Process[name] = map[string]store.Process{}
		}

		for pid, process := range domain.Process {
			p := data.Process[name][pid]

			p.Process = UnmarshalProcess(process)
			p.Metadata = map[string]interface{}{}

			data.Process[name][pid] = p
		}

		for pid, m := range domain.Metadata {
			p := data.Process[name][pid]
			p.Metadata = m
			data.Process[name][pid] = p
		}
	}

	return data, nil
}

func (s *jsonStore) Store(data store.Data) error {
	r := NewData()

	for k, v := range data.Metadata {
		r.Metadata.System[k] = v
	}

	for domain, d := range data.Process {
		for pid, p := range d {
			if len(domain) == 0 {
				r.Process[pid] = MarshalProcess(p.Process)
				r.Metadata.Process[pid] = p.Metadata
			} else {
				x := r.Domain[domain]
				if x.Process == nil {
					x.Process = map[string]Process{}
				}

				x.Process[pid] = MarshalProcess(p.Process)

				if x.Metadata == nil {
					x.Metadata = map[string]map[string]interface{}{}
				}

				x.Metadata[pid] = p.Metadata

				r.Domain[domain] = x
			}
		}
	}

	s.lock.RLock()
	defer s.lock.RUnlock()

	err := s.store(s.filepath, r)
	if err != nil {
		return fmt.Errorf("failed to store data: %w", err)
	}

	return nil
}

func (s *jsonStore) store(filepath string, data Data) error {
	if data.Version != version {
		return fmt.Errorf("invalid version (have: %d, want: %d)", data.Version, version)
	}

	jsondata, err := json.MarshalIndent(&data, "", "    ")
	if err != nil {
		return err
	}

	_, _, err = s.fs.WriteFileSafe(filepath, jsondata)
	if err != nil {
		return err
	}

	s.logger.WithField("file", filepath).Debug().Log("Stored data")

	return nil
}

type storeVersion struct {
	Version uint64 `json:"version"`
}

func (s *jsonStore) load(filepath string, version uint64) (Data, error) {
	r := NewData()

	if _, err := s.fs.Stat(filepath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return r, nil
		}

		return r, err
	}

	jsondata, err := s.fs.ReadFile(filepath)
	if err != nil {
		return r, err
	}

	var db storeVersion

	if err = json.Unmarshal(jsondata, &db); err != nil {
		return r, json.FormatError(jsondata, err)
	}

	if db.Version == version {
		if err = json.Unmarshal(jsondata, &r); err != nil {
			return r, json.FormatError(jsondata, err)
		}
	} else {
		return r, fmt.Errorf("unsupported version of the DB file (want: %d, have: %d)", version, db.Version)
	}

	s.logger.WithField("file", filepath).Debug().Log("Read data")

	return r, nil
}
