package hlsrewrite

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewrite(t *testing.T) {
	data, err := os.ReadFile("./fixtures/data.txt")
	require.NoError(t, err)

	rewrittendata, err := os.ReadFile("./fixtures/data_rewritten.txt")
	require.NoError(t, err)

	r := &hlsRewriter{
		buffer: &bytes.Buffer{},
	}

	r.Write(data)

	buffer := &bytes.Buffer{}
	prefix := []byte("/path/to/foobar/")
	r.rewrite(prefix, buffer)

	require.Equal(t, rewrittendata, buffer.Bytes())
}

func BenchmarkRewrite(b *testing.B) {
	data, err := os.ReadFile("./fixtures/data.txt")
	require.NoError(b, err)

	r := &hlsRewriter{
		buffer: &bytes.Buffer{},
	}

	buffer := &bytes.Buffer{}
	prefix := []byte("/path/to/foobar/")

	for i := 0; i < b.N; i++ {
		r.buffer.Reset()
		r.Write(data)

		buffer.Reset()
		r.rewrite(prefix, buffer)
	}
}
