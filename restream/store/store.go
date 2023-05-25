package store

import "github.com/datarhei/core/v16/restream/app"

type Process struct {
	Process  *app.Process
	Metadata map[string]interface{}
}

type Data struct {
	Process  map[string]map[string]Process
	Metadata map[string]interface{}
}

func (d *Data) IsEmpty() bool {
	if len(d.Process) != 0 {
		return false
	}

	if len(d.Metadata) != 0 {
		return false
	}

	return true
}

type Store interface {
	// Load data from the store
	Load() (Data, error)

	// Save data to the store
	Store(Data) error
}

func NewData() Data {
	c := Data{
		Process:  make(map[string]map[string]Process),
		Metadata: make(map[string]interface{}),
	}

	return c
}
