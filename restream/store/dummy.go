package store

import (
	"github.com/datarhei/core/v16/log"
)

type DummyConfig struct {
	Logger log.Logger
}

type dummyStore struct {
	logger log.Logger
}

func NewDummyStore(config DummyConfig) Store {
	s := &dummyStore{
		logger: config.Logger,
	}

	if s.logger == nil {
		s.logger = log.New("")
	}

	return s
}

func (sb *dummyStore) Store(data StoreData) error {
	sb.logger.Debug().Log("Data stored")

	return nil
}

func (sb *dummyStore) Load() (StoreData, error) {
	sb.logger.Debug().Log("Data loaded")

	return NewStoreData(), nil
}
