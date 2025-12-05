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

func TestHLSSegmentReaderTS(t *testing.T) {
	data, err := os.ReadFile("./fixtures/segments_v6.txt")
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

func TestHLSSegmentReaderMP4(t *testing.T) {
	data, err := os.ReadFile("./fixtures/segments_v7.txt")
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
		"/foobar/test_output_0_0067.mp4",
		"/foobar/test_output_0_0068.mp4",
		"/foobar/test_output_0_0069.mp4",
		"/foobar/test_output_0_0070.mp4",
		"/foobar/test_output_0_0071.mp4",
		"/foobar/test_output_0_0072.mp4",
	}, segments)
}

func BenchmarkHLSSegmentReader(b *testing.B) {
	data, err := os.ReadFile("./fixtures/segments_v6.txt")
	require.NoError(b, err)

	rd := bytes.NewReader(data)
	r := io.NopCloser(rd)

	for b.Loop() {
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

func TestHLSRewriteTS(t *testing.T) {
	data, err := os.ReadFile("./fixtures/segments_v6.txt")
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

	data, err = os.ReadFile("./fixtures/segments_v6_with_session.txt")
	require.NoError(t, err)

	require.Equal(t, data, buffer.Bytes())
}

func TestHLSRewriteMP4(t *testing.T) {
	data, err := os.ReadFile("./fixtures/segments_v7.txt")
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

	data, err = os.ReadFile("./fixtures/segments_v7_with_session.txt")
	require.NoError(t, err)

	require.Equal(t, data, buffer.Bytes())
}

func BenchmarkHLSRewrite(b *testing.B) {
	data, err := os.ReadFile("./fixtures/segments_v6.txt")
	require.NoError(b, err)

	u, err := url.Parse("http://example.com/test.m3u8")
	require.NoError(b, err)

	for b.Loop() {
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
