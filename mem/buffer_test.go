package mem

import (
	"bytes"
	"io"
	"testing"

	"github.com/datarhei/core/v16/math/rand"

	"github.com/stretchr/testify/require"
)

func TestBufferReadChunks(t *testing.T) {
	data := []byte(rand.StringAlphanumeric(1024 * 1024))

	r := bytes.NewReader(data)
	buf := &Buffer{}

	buf.ReadFrom(r)

	res := bytes.Compare(data, buf.Bytes())
	require.Equal(t, 0, res)
}

func BenchmarkBufferReadFrom(b *testing.B) {
	data := []byte(rand.StringAlphanumeric(1024 * 1024))

	r := bytes.NewReader(data)

	for b.Loop() {
		r.Seek(0, io.SeekStart)
		buf := &Buffer{}
		buf.ReadFrom(r)
	}
}

func BenchmarkBytesBufferReadFrom(b *testing.B) {
	data := []byte(rand.StringAlphanumeric(1024 * 1024))

	r := bytes.NewReader(data)

	for b.Loop() {
		r.Seek(0, io.SeekStart)
		buf := &bytes.Buffer{}
		buf.ReadFrom(r)
	}
}
