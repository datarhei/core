package session

import (
	"bytes"
	"io"
	"net/url"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/datarhei/core/v16/mem"
	"github.com/stretchr/testify/require"
)

func TestHLSRewriteMaster(t *testing.T) {
	data, err := os.ReadFile("./fixtures/master.txt")
	require.NoError(t, err)

	br := &sessionRewriter{
		buffer: &mem.Buffer{},
	}

	_, err = br.Write(data)
	require.NoError(t, err)

	u, err := url.Parse("http://example.com/test.m3u8")
	require.NoError(t, err)

	buffer := &mem.Buffer{}

	br.rewriteHLS("8pfhCNqTnxSsbdoA3jnWxs", u, buffer)

	data, err = os.ReadFile("./fixtures/master_with_session.txt")
	require.NoError(t, err)

	require.Equal(t, data, buffer.Bytes())
}

func TestHLSRewriteMasterWithoutSession(t *testing.T) {
	data, err := os.ReadFile("./fixtures/master.txt")
	require.NoError(t, err)

	br := &sessionRewriter{
		buffer: &mem.Buffer{},
	}

	_, err = br.Write(data)
	require.NoError(t, err)

	u, err := url.Parse("http://example.com/test.m3u8")
	require.NoError(t, err)

	buffer := &mem.Buffer{}

	br.rewriteHLS("", u, buffer)

	re := regexp.MustCompile(`session=([0-9A-Za-z]+)`)

	matches := re.FindAllStringSubmatch(buffer.String(), -1)

	require.Equal(t, 2, len(matches))
	require.Equal(t, matches[0][1], matches[1][1])
}

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

func TestParseKeyValueBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:  "standard example with quoted commas",
			input: `BANDWIDTH=243648,AVERAGE-BANDWIDTH=115196,RESOLUTION=1280x720,CODECS="avc1.42c01f,mp4a.40.2"`,
			expected: map[string]string{
				"BANDWIDTH":         "243648",
				"AVERAGE-BANDWIDTH": "115196",
				"RESOLUTION":        "1280x720",
				"CODECS":            "avc1.42c01f,mp4a.40.2",
			},
		},
		{
			name:  "spaces around separators and quotes",
			input: ` FOO = bar , BAZ = " hello, world " `,
			expected: map[string]string{
				"FOO": "bar",
				"BAZ": " hello, world ",
			},
		},
		{
			name:  "empty values and empty quotes",
			input: `KEY1=,KEY2="",KEY3="value"`,
			expected: map[string]string{
				"KEY1": "",
				"KEY2": "",
				"KEY3": "value",
			},
		},
		{
			name:     "empty byte array input",
			input:    "",
			expected: map[string]string{},
		},
		{
			name:  "single key value pair",
			input: `NAME="John Doe"`,
			expected: map[string]string{
				"NAME": "John Doe",
			},
		},
		{
			name:  "ignore invalid entries without equals sign",
			input: `VALID=123,INVALID_ENTRY,ANOTHER=456`,
			expected: map[string]string{
				"VALID":   "123",
				"ANOTHER": "456",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseKeyValueBytes([]byte(tt.input))
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestParseVariants(t *testing.T) {
	buffer := mem.Get()

	buffer.Write([]byte(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-STREAM-INF:BANDWIDTH=243648,AVERAGE-BANDWIDTH=115196,RESOLUTION=1280x720,CODECS="avc1.42c01f,mp4a.40.2"
source_beep_0.m3u8?session=Ur5iBrHhuPdAdKgH7yvDrt

#EXT-X-STREAM-INF:BANDWIDTH=215072,AVERAGE-BANDWIDTH=104560,RESOLUTION=640x360,CODECS="avc1.42c01e,mp4a.40.2"
source_beep_1.m3u8?session=Ur5iBrHhuPdAdKgH7yvDrt`))

	variants := parseVariants("/", buffer)

	require.Equal(t, []variant{
		{
			file:       "/source_beep_0.m3u8",
			bandwidth:  243648,
			resolution: "1280x720",
			codecs:     "avc1.42c01f,mp4a.40.2",
		}, {
			file:       "/source_beep_1.m3u8",
			bandwidth:  215072,
			resolution: "640x360",
			codecs:     "avc1.42c01e,mp4a.40.2",
		},
	}, variants)
}

func TestParseSegments(t *testing.T) {
	buffer := mem.Get()

	buffer.Write([]byte(`#EXTM3U
#EXT-X-VERSION:6
#EXT-X-TARGETDURATION:2
#EXT-X-MEDIA-SEQUENCE:2279
#EXT-X-INDEPENDENT-SEGMENTS
#EXTINF:2.000000,
#EXT-X-PROGRAM-DATE-TIME:2026-09-01T11:53:23.409+0200
source_beep_0_2279.ts
#EXTINF:2.000000,
#EXT-X-PROGRAM-DATE-TIME:2026-09-01T11:53:25.409+0200
source_beep_0_2280.ts
#EXTINF:2.000000,
#EXT-X-PROGRAM-DATE-TIME:2026-09-01T11:53:27.409+0200
source_beep_0_2281.ts
#EXTINF:2.000000,
#EXT-X-PROGRAM-DATE-TIME:2026-09-01T11:53:29.409+0200
source_beep_0_2282.ts
#EXTINF:2.000000,
#EXT-X-PROGRAM-DATE-TIME:2026-09-01T11:53:31.409+0200
source_beep_0_2283.ts
#EXTINF:2.000000,
#EXT-X-PROGRAM-DATE-TIME:2026-09-01T11:53:33.409+0200
source_beep_0_2284.ts`))

	segments := parseSegments(buffer)

	require.Equal(t, map[string]hlsSegment{
		"source_beep_0_2279.ts": {
			duration:  time.Duration(2 * time.Second),
			sequence:  2279,
			requested: false,
		},
		"source_beep_0_2280.ts": {
			duration:  time.Duration(2 * time.Second),
			sequence:  2280,
			requested: false,
		},
		"source_beep_0_2281.ts": {
			duration:  time.Duration(2 * time.Second),
			sequence:  2281,
			requested: false,
		},
		"source_beep_0_2282.ts": {
			duration:  time.Duration(2 * time.Second),
			sequence:  2282,
			requested: false,
		},
		"source_beep_0_2283.ts": {
			duration:  time.Duration(2 * time.Second),
			sequence:  2283,
			requested: false,
		},
		"source_beep_0_2284.ts": {
			duration:  time.Duration(2 * time.Second),
			sequence:  2284,
			requested: false,
		},
	}, segments)
}
