package session

import (
	"bytes"
	"io"
	"net/url"
	"os"
	"testing"

	"github.com/datarhei/core/v16/mem"
	"github.com/stretchr/testify/require"
)

func TestHLSSegmentReader(t *testing.T) {
	data, err := os.ReadFile("./fixtures/segments.txt")
	require.NoError(t, err)

	r := bytes.NewReader(data)

	br := &segmentReader{
		reader: io.NopCloser(r),
		buffer: &mem.Buffer{},
	}

	_, err = io.ReadAll(br)
	require.NoError(t, err)

	segments := br.getSegments("/foobar")
	require.Equal(t, []string{
		"/foobar/test_0_0_0303.ts",
		"/foobar/test_0_0_0304.ts",
		"/foobar/test_0_0_0305.ts",
		"/foobar/test_0_0_0306.ts",
		"/foobar/test_0_0_0307.ts",
		"/foobar/test_0_0_0308.ts",
		"/foobar/test_0_0_0309.ts",
		"/foobar/test_0_0_0310.ts",
	}, segments)
}

func BenchmarkHLSSegmentReader(b *testing.B) {
	data, err := os.ReadFile("./fixtures/segments.txt")
	require.NoError(b, err)

	rd := bytes.NewReader(data)
	r := io.NopCloser(rd)

	for i := 0; i < b.N; i++ {
		rd.Reset(data)
		br := &segmentReader{
			reader: io.NopCloser(r),
			buffer: mem.Get(),
		}

		_, err := io.ReadAll(br)
		require.NoError(b, err)

		mem.Put(br.buffer)
	}
}

func TestHLSRewrite(t *testing.T) {
	data, err := os.ReadFile("./fixtures/segments.txt")
	require.NoError(t, err)

	br := &sessionRewriter{
		buffer: &mem.Buffer{},
	}

	_, err = br.Write(data)
	require.NoError(t, err)

	u, err := url.Parse("http://example.com/test.m3u8")
	require.NoError(t, err)

	buffer := &mem.Buffer{}

	br.rewriteHLS("oT5GV8eWBbRAh4aib5egoK", u, buffer)

	data, err = os.ReadFile("./fixtures/segments_with_session.txt")
	require.NoError(t, err)

	require.Equal(t, data, buffer.Bytes())
}

func BenchmarkHLSRewrite(b *testing.B) {
	data, err := os.ReadFile("./fixtures/segments.txt")
	require.NoError(b, err)

	u, err := url.Parse("http://example.com/test.m3u8")
	require.NoError(b, err)

	for i := 0; i < b.N; i++ {
		br := &sessionRewriter{
			buffer: mem.Get(),
		}

		_, err = br.Write(data)
		require.NoError(b, err)

		buffer := mem.Get()

		br.rewriteHLS("oT5GV8eWBbRAh4aib5egoK", u, buffer)

		mem.Put(br.buffer)
		mem.Put(buffer)
	}
}
