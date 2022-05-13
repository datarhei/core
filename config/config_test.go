package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigCopy(t *testing.T) {
	config1 := New()

	config1.Version = 42
	config1.DB.Dir = "foo"

	val1 := config1.findVariable("version")
	val2 := config1.findVariable("db.dir")
	val3 := config1.findVariable("host.name")

	assert.Equal(t, "42", val1.value.String())
	assert.Equal(t, nil, val1.value.Validate())
	assert.Equal(t, false, val1.value.IsEmpty())

	assert.Equal(t, "foo", val2.value.String())
	assert.Equal(t, "(empty)", val3.value.String())

	val3.value.Set("foo.com")

	assert.Equal(t, "foo.com", val3.value.String())

	config2 := NewConfigFrom(config1)

	assert.Equal(t, int64(42), config2.Version)
	assert.Equal(t, "foo", config2.DB.Dir)
	assert.Equal(t, []string{"foo.com"}, config2.Host.Name)

	val1.value.Set("77")

	assert.Equal(t, int64(77), config1.Version)
	assert.Equal(t, int64(42), config2.Version)

	val2.value.Set("bar")

	assert.Equal(t, "bar", config1.DB.Dir)
	assert.Equal(t, "foo", config2.DB.Dir)

	config2.DB.Dir = "baz"

	assert.Equal(t, "bar", config1.DB.Dir)
	assert.Equal(t, "baz", config2.DB.Dir)

	config1.Host.Name[0] = "bar.com"

	assert.Equal(t, []string{"bar.com"}, config1.Host.Name)
	assert.Equal(t, []string{"foo.com"}, config2.Host.Name)
}
