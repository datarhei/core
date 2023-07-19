package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"
)

type Adapter interface {
	LoadIdentities() ([]User, error)
	SaveIdentities(user []User) error
}

type fileAdapter struct {
	fs       fs.Filesystem
	filePath string
	logger   log.Logger
	lock     sync.Mutex
}

func NewJSONAdapter(fs fs.Filesystem, filePath string, logger log.Logger) (Adapter, error) {
	a := &fileAdapter{
		fs:       fs,
		filePath: filePath,
		logger:   logger,
	}

	if a.fs == nil {
		return nil, fmt.Errorf("a filesystem must be provided")
	}

	if a.filePath == "" {
		return nil, fmt.Errorf("invalid file path, file path cannot be empty")
	}

	if a.logger == nil {
		a.logger = log.New("")
	}

	return a, nil
}

func (a *fileAdapter) LoadIdentities() ([]User, error) {
	a.lock.Lock()
	defer a.lock.Unlock()

	if _, err := a.fs.Stat(a.filePath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	data, err := a.fs.ReadFile(a.filePath)
	if err != nil {
		return nil, err
	}

	users := []User{}

	err = json.Unmarshal(data, &users)
	if err != nil {
		return nil, err
	}

	a.logger.Debug().WithField("path", a.filePath).Log("Identity file loaded")

	return users, nil
}

func (a *fileAdapter) SaveIdentities(user []User) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	jsondata, err := json.MarshalIndent(user, "", "    ")
	if err != nil {
		return err
	}

	_, _, err = a.fs.WriteFileSafe(a.filePath, jsondata)
	if err != nil {
		return err
	}

	a.logger.Debug().WithField("path", a.filePath).Log("Identity file save")

	return nil
}
