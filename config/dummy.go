package config

import "fmt"

type dummyStore struct{}

// NewDummyStore returns a store that returns the default config
func NewDummyStore() Store {
	return &dummyStore{}
}

func (c *dummyStore) Get() *Config {
	cfg := New()

	cfg.DB.Dir = "."
	cfg.Storage.Disk.Dir = "."

	return cfg
}

func (c *dummyStore) Set(d *Config) error {
	d.Validate(true)

	if d.HasErrors() {
		return fmt.Errorf("configuration data has errors after validation")
	}

	return nil
}

func (c *dummyStore) GetActive() *Config {
	cfg := New()

	cfg.DB.Dir = "."
	cfg.Storage.Disk.Dir = "."

	return cfg
}

func (c *dummyStore) SetActive(d *Config) error {
	d.Validate(true)

	if d.HasErrors() {
		return fmt.Errorf("configuration data has errors after validation")
	}

	return nil
}

func (c *dummyStore) Reload() error {
	return nil
}
