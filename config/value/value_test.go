package value

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIntValue(t *testing.T) {
	var i int

	ivar := NewInt(&i, 11)

	assert.Equal(t, "11", ivar.String())
	assert.Equal(t, nil, ivar.Validate())
	assert.Equal(t, false, ivar.IsEmpty())

	i = 42

	assert.Equal(t, "42", ivar.String())
	assert.Equal(t, nil, ivar.Validate())
	assert.Equal(t, false, ivar.IsEmpty())

	ivar.Set("77")

	assert.Equal(t, int(77), i)
}

type testdata struct {
	value1 int
	value2 int
}

func TestCopyStruct(t *testing.T) {
	data1 := testdata{}

	NewInt(&data1.value1, 1)
	NewInt(&data1.value2, 2)

	assert.Equal(t, int(1), data1.value1)
	assert.Equal(t, int(2), data1.value2)

	data2 := testdata{}

	val21 := NewInt(&data2.value1, 3)
	val22 := NewInt(&data2.value2, 4)

	assert.Equal(t, int(3), data2.value1)
	assert.Equal(t, int(4), data2.value2)

	data2 = data1

	assert.Equal(t, int(1), data2.value1)
	assert.Equal(t, int(2), data2.value2)

	assert.Equal(t, "1", val21.String())
	assert.Equal(t, "2", val22.String())
}
