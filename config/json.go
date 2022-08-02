package config

import (
	gojson "encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/io/file"
)

type jsonStore struct {
	path string

	data map[string]*Config

	reloadFn func()
}

// NewJSONStore will read a JSON config file from the given path. After successfully reading it in, it will be written
// back to the path. The returned error will be nil if everything went fine.
// If the path doesn't exist, a default JSON config file will be written to that path.
// The returned ConfigStore can be used to retrieve or write the config.
func NewJSONStore(path string, reloadFn func()) (Store, error) {
	c := &jsonStore{
		path:     path,
		data:     make(map[string]*Config),
		reloadFn: reloadFn,
	}

	c.data["base"] = New()

	if err := c.load(c.data["base"]); err != nil {
		return nil, fmt.Errorf("failed to read JSON from '%s': %w", path, err)
	}

	if err := c.store(c.data["base"]); err != nil {
		return nil, fmt.Errorf("failed to write JSON to '%s': %w", path, err)
	}

	return c, nil
}

func (c *jsonStore) Get() *Config {
	return NewConfigFrom(c.data["base"])
}

func (c *jsonStore) Set(d *Config) error {
	if d.HasErrors() {
		return fmt.Errorf("configuration data has errors after validation")
	}

	data := NewConfigFrom(d)

	data.CreatedAt = time.Now()

	if err := c.store(data); err != nil {
		return fmt.Errorf("failed to write JSON to '%s': %w", c.path, err)
	}

	data.UpdatedAt = time.Now()

	c.data["base"] = data

	return nil
}

func (c *jsonStore) GetActive() *Config {
	if x, ok := c.data["merged"]; ok {
		return NewConfigFrom(x)
	}

	if x, ok := c.data["base"]; ok {
		return NewConfigFrom(x)
	}

	return nil
}

func (c *jsonStore) SetActive(d *Config) error {
	d.Validate(true)

	if d.HasErrors() {
		return fmt.Errorf("configuration data has errors after validation")
	}

	c.data["merged"] = NewConfigFrom(d)

	return nil
}

func (c *jsonStore) Reload() error {
	if c.reloadFn == nil {
		return nil
	}

	c.reloadFn()

	return nil
}

func (c *jsonStore) load(config *Config) error {
	if len(c.path) == 0 {
		return nil
	}

	if _, err := os.Stat(c.path); os.IsNotExist(err) {
		return nil
	}

	jsondata, err := os.ReadFile(c.path)
	if err != nil {
		return err
	}

	dataV3 := &Data{}

	version := DataVersion{}

	if err = gojson.Unmarshal(jsondata, &version); err != nil {
		return json.FormatError(jsondata, err)
	}

	if version.Version == 1 {
		dataV1 := &dataV1{}

		if err = gojson.Unmarshal(jsondata, dataV1); err != nil {
			return json.FormatError(jsondata, err)
		}

		dataV2, err := NewV2FromV1(dataV1)
		if err != nil {
			return err
		}

		dataV3, err = NewV3FromV2(dataV2)
		if err != nil {
			return err
		}
	} else if version.Version == 2 {
		dataV2 := &dataV2{}

		if err = gojson.Unmarshal(jsondata, dataV2); err != nil {
			return json.FormatError(jsondata, err)
		}

		dataV3, err = NewV3FromV2(dataV2)
		if err != nil {
			return err
		}
	} else if version.Version == 3 {
		if err = gojson.Unmarshal(jsondata, dataV3); err != nil {
			return json.FormatError(jsondata, err)
		}
	}

	config.Data = *dataV3

	config.LoadedAt = time.Now()
	config.UpdatedAt = config.LoadedAt

	return nil
}

func (c *jsonStore) store(data *Config) error {
	data.CreatedAt = time.Now()

	if len(c.path) == 0 {
		return nil
	}

	jsondata, err := gojson.MarshalIndent(data, "", "    ")
	if err != nil {
		return err
	}

	dir, filename := filepath.Split(c.path)

	tmpfile, err := os.CreateTemp(dir, filename)
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

	if err := file.Rename(tmpfile.Name(), c.path); err != nil {
		return err
	}

	return nil
}
