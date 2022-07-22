package config

import "fmt"

type dummyStore struct {
	current *Config
	active  *Config
}

// NewDummyStore returns a store that returns the default config
func NewDummyStore() Store {
	s := &dummyStore{}

	cfg := New()

	cfg.DB.Dir = "."
	cfg.FFmpeg.Binary = "true"
	cfg.Storage.Disk.Dir = "."
	cfg.Storage.MimeTypes = ""

	s.current = cfg

	cfg = New()

	cfg.DB.Dir = "."
	cfg.FFmpeg.Binary = "true"
	cfg.Storage.Disk.Dir = "."
	cfg.Storage.MimeTypes = ""

	s.active = cfg

	return s
}

func (c *dummyStore) Get() *Config {
	cfg := New()

	cfg.DB.Dir = "."
	cfg.FFmpeg.Binary = "true"
	cfg.Storage.Disk.Dir = "."
	cfg.Storage.MimeTypes = ""

	return cfg
}

func (c *dummyStore) Set(d *Config) error {
	d.Validate(true)

	if d.HasErrors() {
		return fmt.Errorf("configuration data has errors after validation")
	}

	c.current = NewConfigFrom(d)

	return nil
}

func (c *dummyStore) GetActive() *Config {
	cfg := New()

	cfg.DB.Dir = "."
	cfg.FFmpeg.Binary = "true"
	cfg.Storage.Disk.Dir = "."
	cfg.Storage.MimeTypes = ""

	return cfg
}

func (c *dummyStore) SetActive(d *Config) error {
	d.Validate(true)

	if d.HasErrors() {
		return fmt.Errorf("configuration data has errors after validation")
	}

	c.active = NewConfigFrom(d)

	return nil
}

func (c *dummyStore) Reload() error {
	return nil
}
