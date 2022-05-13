package net

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPortrange(t *testing.T) {
	_, err := NewPortrange(1000, 1999)

	assert.Nil(t, err, "Valid port range not accepted: %s", err)
}

func TestInvalidPortrange(t *testing.T) {
	_, err := NewPortrange(1999, 1000)

	assert.NotNil(t, err, "Invalid port range accepted")
}

func TestGetPort(t *testing.T) {
	portrange, _ := NewPortrange(1000, 1999)

	port, err := portrange.Get()

	assert.Nil(t, err)
	assert.Equal(t, 1000, port)
}

func TestGetPutPort(t *testing.T) {
	portrange, _ := NewPortrange(1000, 1999)

	port, err := portrange.Get()
	assert.Nil(t, err)
	assert.Equal(t, 1000, port)

	port, err = portrange.Get()
	assert.Nil(t, err)
	assert.Equal(t, 1001, port)

	portrange.Put(1000)

	port, err = portrange.Get()
	assert.Nil(t, err)
	assert.Equal(t, 1000, port)
}

func TestPortUnavailable(t *testing.T) {
	portrange, _ := NewPortrange(1000, 1999)

	for i := 0; i < 1000; i++ {
		port, _ := portrange.Get()
		assert.Equal(t, 1000+i, port, "at index %d", i)
	}

	port, err := portrange.Get()
	assert.NotNil(t, err)
	assert.Less(t, port, 0)
}

func TestPutPort(t *testing.T) {
	portrange, _ := NewPortrange(1000, 1999)

	portrange.Put(999)
	portrange.Put(1000)

	portrange.Put(1999)
	portrange.Put(2000)
}

func TestClampRange(t *testing.T) {
	portrange, _ := NewPortrange(0, 70000)

	port, _ := portrange.Get()

	assert.Equal(t, 1, port)

	portrange.Put(1)

	for i := 1; i <= 65535; i++ {
		port, _ := portrange.Get()
		assert.Equal(t, i, port, "at index %d", i)
	}

	port, _ = portrange.Get()

	assert.Less(t, port, 0)
}
