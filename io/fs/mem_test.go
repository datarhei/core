package fs

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	mem := NewMemFilesystem(MemConfig{
		Size:  10,
		Purge: false,
	})

	cur, max := mem.Size()

	assert.Equal(t, int64(0), cur)
	assert.Equal(t, int64(10), max)

	cur = mem.Files()

	assert.Equal(t, int64(0), cur)
}

func TestSimplePutNoPurge(t *testing.T) {
	mem := NewMemFilesystem(MemConfig{
		Size:  10,
		Purge: false,
	})

	data := strings.NewReader("xxxxx")

	size, created, err := mem.Store("/foobar", data)

	assert.Nil(t, err)
	assert.Equal(t, int64(5), size)
	assert.Equal(t, true, created)

	cur, max := mem.Size()

	assert.Equal(t, int64(5), cur)
	assert.Equal(t, int64(10), max)

	cur = mem.Files()

	assert.Equal(t, int64(1), cur)
}

func TestSimpleDelete(t *testing.T) {
	mem := NewMemFilesystem(MemConfig{
		Size:  10,
		Purge: false,
	})

	size := mem.Delete("/foobar")

	assert.Equal(t, int64(-1), size)

	data := strings.NewReader("xxxxx")

	mem.Store("/foobar", data)

	size = mem.Delete("/foobar")

	assert.Equal(t, int64(5), size)

	cur, max := mem.Size()

	assert.Equal(t, int64(0), cur)
	assert.Equal(t, int64(10), max)

	cur = mem.Files()

	assert.Equal(t, int64(0), cur)
}

func TestReplaceNoPurge(t *testing.T) {
	mem := NewMemFilesystem(MemConfig{
		Size:  10,
		Purge: false,
	})

	data := strings.NewReader("xxxxx")

	size, created, err := mem.Store("/foobar", data)

	assert.Nil(t, err)
	assert.Equal(t, int64(5), size)
	assert.Equal(t, true, created)

	cur, max := mem.Size()

	assert.Equal(t, int64(5), cur)
	assert.Equal(t, int64(10), max)

	cur = mem.Files()

	assert.Equal(t, int64(1), cur)

	data = strings.NewReader("yyy")

	size, created, err = mem.Store("/foobar", data)

	assert.Nil(t, err)
	assert.Equal(t, int64(3), size)
	assert.Equal(t, false, created)

	cur, max = mem.Size()

	assert.Equal(t, int64(3), cur)
	assert.Equal(t, int64(10), max)

	cur = mem.Files()

	assert.Equal(t, int64(1), cur)
}

func TestReplacePurge(t *testing.T) {
	mem := NewMemFilesystem(MemConfig{
		Size:  10,
		Purge: true,
	})

	data1 := strings.NewReader("xxx")
	data2 := strings.NewReader("yyy")
	data3 := strings.NewReader("zzz")

	mem.Store("/foobar1", data1)
	mem.Store("/foobar2", data2)
	mem.Store("/foobar3", data3)

	cur, max := mem.Size()

	assert.Equal(t, int64(9), cur)
	assert.Equal(t, int64(10), max)

	cur = mem.Files()

	assert.Equal(t, int64(3), cur)

	data4 := strings.NewReader("zzzzz")

	size, _, _ := mem.Store("/foobar1", data4)

	assert.Equal(t, int64(5), size)

	cur, max = mem.Size()

	assert.Equal(t, int64(8), cur)
	assert.Equal(t, int64(10), max)

	cur = mem.Files()

	assert.Equal(t, int64(2), cur)
}

func TestReplaceUnlimited(t *testing.T) {
	mem := NewMemFilesystem(MemConfig{
		Size:  0,
		Purge: false,
	})

	data := strings.NewReader("xxxxx")

	size, created, err := mem.Store("/foobar", data)

	assert.Nil(t, err)
	assert.Equal(t, int64(5), size)
	assert.Equal(t, true, created)

	cur, max := mem.Size()

	assert.Equal(t, int64(5), cur)
	assert.Equal(t, int64(0), max)

	cur = mem.Files()

	assert.Equal(t, int64(1), cur)

	data = strings.NewReader("yyy")

	size, created, err = mem.Store("/foobar", data)

	assert.Nil(t, err)
	assert.Equal(t, int64(3), size)
	assert.Equal(t, false, created)

	cur, max = mem.Size()

	assert.Equal(t, int64(3), cur)
	assert.Equal(t, int64(0), max)

	cur = mem.Files()

	assert.Equal(t, int64(1), cur)
}

func TestTooBigNoPurge(t *testing.T) {
	mem := NewMemFilesystem(MemConfig{
		Size:  10,
		Purge: false,
	})

	data := strings.NewReader("xxxxxyyyyyz")

	size, _, _ := mem.Store("/foobar", data)

	assert.Equal(t, int64(-1), size)
}

func TestTooBigPurge(t *testing.T) {
	mem := NewMemFilesystem(MemConfig{
		Size:  10,
		Purge: true,
	})

	data1 := strings.NewReader("xxxxx")
	data2 := strings.NewReader("yyyyy")

	mem.Store("/foobar1", data1)
	mem.Store("/foobar2", data2)

	data := strings.NewReader("xxxxxyyyyyz")

	size, _, _ := mem.Store("/foobar", data)

	assert.Equal(t, int64(-1), size)
}

func TestFullSpaceNoPurge(t *testing.T) {
	mem := NewMemFilesystem(MemConfig{
		Size:  10,
		Purge: false,
	})

	data1 := strings.NewReader("xxxxx")
	data2 := strings.NewReader("yyyyy")

	mem.Store("/foobar1", data1)
	mem.Store("/foobar2", data2)

	cur, max := mem.Size()

	assert.Equal(t, int64(10), cur)
	assert.Equal(t, int64(10), max)

	cur = mem.Files()

	assert.Equal(t, int64(2), cur)

	data3 := strings.NewReader("zzzzz")

	size, _, _ := mem.Store("/foobar3", data3)

	assert.Equal(t, int64(-1), size)
}

func TestFullSpacePurge(t *testing.T) {
	mem := NewMemFilesystem(MemConfig{
		Size:  10,
		Purge: true,
	})

	data1 := strings.NewReader("xxxxx")
	data2 := strings.NewReader("yyyyy")

	mem.Store("/foobar1", data1)
	mem.Store("/foobar2", data2)

	cur, max := mem.Size()

	assert.Equal(t, int64(10), cur)
	assert.Equal(t, int64(10), max)

	cur = mem.Files()

	assert.Equal(t, int64(2), cur)

	data3 := strings.NewReader("zzzzz")

	size, _, _ := mem.Store("/foobar3", data3)

	assert.Equal(t, int64(5), size)

	cur, max = mem.Size()

	assert.Equal(t, int64(10), cur)
	assert.Equal(t, int64(10), max)

	cur = mem.Files()

	assert.Equal(t, int64(2), cur)
}

func TestFullSpacePurgeMulti(t *testing.T) {
	mem := NewMemFilesystem(MemConfig{
		Size:  10,
		Purge: true,
	})

	data1 := strings.NewReader("xxx")
	data2 := strings.NewReader("yyy")
	data3 := strings.NewReader("zzz")

	mem.Store("/foobar1", data1)
	mem.Store("/foobar2", data2)
	mem.Store("/foobar3", data3)

	cur, max := mem.Size()

	assert.Equal(t, int64(9), cur)
	assert.Equal(t, int64(10), max)

	cur = mem.Files()

	assert.Equal(t, int64(3), cur)

	data4 := strings.NewReader("zzzzz")

	size, _, _ := mem.Store("/foobar4", data4)

	assert.Equal(t, int64(5), size)

	cur, max = mem.Size()

	assert.Equal(t, int64(8), cur)
	assert.Equal(t, int64(10), max)

	cur = mem.Files()

	assert.Equal(t, int64(2), cur)
}

func TestPurgeOrder(t *testing.T) {
	mem := NewMemFilesystem(MemConfig{
		Size:  10,
		Purge: true,
	})

	data1 := strings.NewReader("xxxxx")
	data2 := strings.NewReader("yyyyy")
	data3 := strings.NewReader("zzzzz")

	mem.Store("/foobar1", data1)
	time.Sleep(1 * time.Second)
	mem.Store("/foobar2", data2)
	time.Sleep(1 * time.Second)
	mem.Store("/foobar3", data3)

	file := mem.Open("/foobar1")

	assert.Nil(t, file)
}

func TestList(t *testing.T) {
	mem := NewMemFilesystem(MemConfig{
		Size:  10,
		Purge: false,
	})

	data1 := strings.NewReader("a")
	data2 := strings.NewReader("bb")
	data3 := strings.NewReader("ccc")
	data4 := strings.NewReader("dddd")

	mem.Store("/foobar1", data1)
	mem.Store("/foobar2", data2)
	mem.Store("/foobar3", data3)
	mem.Store("/foobar4", data4)

	cur, max := mem.Size()

	assert.Equal(t, int64(10), cur)
	assert.Equal(t, int64(10), max)

	cur = mem.Files()

	assert.Equal(t, int64(4), cur)

	files := mem.List("")

	assert.Equal(t, 4, len(files))
}

func TestData(t *testing.T) {
	mem := NewMemFilesystem(MemConfig{
		Size:  10,
		Purge: false,
	})

	data := "gduwotoxqb"

	data1 := strings.NewReader(data)

	mem.Store("/foobar", data1)

	file := mem.Open("/foobar")

	data2 := make([]byte, len(data)+1)
	n, _ := file.Read(data2)

	assert.Equal(t, len(data), n)
	assert.Equal(t, []byte(data), data2[:n])
}
