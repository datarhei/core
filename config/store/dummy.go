package store

import (
	"fmt"

	"github.com/datarhei/core/v16/config"
)

type dummyStore struct {
	current *config.Config
	active  *config.Config
}

// NewDummyStore returns a store that returns the default config
func NewDummy() Store {
	s := &dummyStore{}

	cfg := config.New()

	cfg.DB.Dir = "."
	cfg.FFmpeg.Binary = "true"
	cfg.Storage.Disk.Dir = "."
	cfg.Storage.MimeTypes = ""

	s.current = cfg

	cfg = config.New()

	cfg.DB.Dir = "."
	cfg.FFmpeg.Binary = "true"
	cfg.Storage.Disk.Dir = "."
	cfg.Storage.MimeTypes = ""

	s.active = cfg

	return s
}

func (c *dummyStore) Get() *config.Config {
	return c.current.Clone()
}

func (c *dummyStore) Set(d *config.Config) error {
	d.Validate(true)

	if d.HasErrors() {
		return fmt.Errorf("configuration data has errors after validation")
	}

	c.current = d.Clone()

	return nil
}

func (c *dummyStore) GetActive() *config.Config {
	return c.active.Clone()
}

func (c *dummyStore) SetActive(d *config.Config) error {
	d.Validate(true)

	if d.HasErrors() {
		return fmt.Errorf("configuration data has errors after validation")
	}

	c.active = d.Clone()

	return nil
}

func (c *dummyStore) Reload() error {
	return nil
}
