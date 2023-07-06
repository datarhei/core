package skills

import (
	"bytes"
	"testing"

	"github.com/datarhei/core/v16/slices"
	"github.com/stretchr/testify/require"
)

func TestNewInvalidBinary(t *testing.T) {
	skills, err := New("")

	require.NotNil(t, err)

	require.Empty(t, skills.FFmpeg.Version)
	require.Empty(t, skills.FFmpeg.Compiler)
	require.Empty(t, skills.FFmpeg.Configuration)
	require.Empty(t, skills.FFmpeg.Libraries)

	require.Empty(t, skills.Filters)
	require.Empty(t, skills.HWAccels)

	require.Empty(t, skills.Codecs.Audio)
	require.Empty(t, skills.Codecs.Subtitle)
	require.Empty(t, skills.Codecs.Video)

	require.Empty(t, skills.Devices.Demuxers)
	require.Empty(t, skills.Devices.Muxers)

	require.Empty(t, skills.Formats.Demuxers)
	require.Empty(t, skills.Formats.Muxers)

	require.Empty(t, skills.Protocols.Input)
	require.Empty(t, skills.Protocols.Output)
}

func TestEqualEmptySkills(t *testing.T) {
	s := Skills{}

	ok := s.Equal(s)
	require.True(t, ok)
}

func TestEuqalSkills(t *testing.T) {
	s1 := Skills{
		FFmpeg:    parseVersion([]byte(ffmpegdata)),
		Filters:   parseFilters([]byte(filterdata)),
		HWAccels:  parseHWAccels([]byte(hwacceldata)),
		Codecs:    parseCodecs([]byte(codecdata)),
		Devices:   ffDevices{},
		Formats:   parseFormats([]byte(formatdata)),
		Protocols: parseProtocols([]byte(protocoldata)),
	}

	devices := parseV4LDevices(bytes.NewBuffer(slices.Copy([]byte(v4ldata))))

	s1.Devices.Demuxers = append(s1.Devices.Demuxers, Device{
		Id:      "v4l2",
		Name:    "webcam",
		Devices: devices,
	})
	s1.Devices.Muxers = append(s1.Devices.Muxers, Device{
		Id:      "v4l2",
		Name:    "webcam",
		Devices: devices,
	})

	ok := s1.Equal(s1)
	require.True(t, ok)

	s2 := Skills{
		FFmpeg:    parseVersion([]byte(ffmpegdata)),
		Filters:   parseFilters([]byte(filterdata)),
		HWAccels:  parseHWAccels([]byte(hwacceldata)),
		Codecs:    parseCodecs([]byte(codecdata)),
		Devices:   ffDevices{},
		Formats:   parseFormats([]byte(formatdata)),
		Protocols: parseProtocols([]byte(protocoldata)),
	}

	devices = parseV4LDevices(bytes.NewBuffer(slices.Copy([]byte(v4ldata))))

	s2.Devices.Demuxers = append(s2.Devices.Demuxers, Device{
		Id:      "v4l2",
		Name:    "webcam",
		Devices: devices,
	})
	s2.Devices.Muxers = append(s2.Devices.Muxers, Device{
		Id:      "v4l2",
		Name:    "webcam",
		Devices: devices,
	})

	ok = s1.Equal(s2)
	require.True(t, ok)

	ok = s1.Equal(Skills{})
	require.False(t, ok)
}

func TestPatchVersion(t *testing.T) {
	data := `ffmpeg version 4.3.1 Copyright (c) 2000-2020 the FFmpeg developers
 built with Apple clang version 12.0.0 (clang-1200.0.32.29)
 configuration: --enable-static --enable-debug --disable-doc --enable-libx264 --enable-gpl --enable-nonfree
 libavutil      56. 51.100 / 56. 51.100
 libavcodec     58. 91.100 / 58. 91.100
 libavformat    58. 45.100 / 58. 45.100
 libavdevice    58. 10.100 / 58. 10.100
 libavfilter     7. 85.100 /  7. 85.100
 libswscale      5.  7.100 /  5.  7.100
 libswresample   3.  7.100 /  3.  7.100
 libpostproc    55.  7.100 / 55.  7.100`

	f := parseVersion([]byte(data))

	require.Equal(t, ffmpeg{
		Version:       "4.3.1",
		Compiler:      "Apple clang version 12.0.0 (clang-1200.0.32.29)",
		Configuration: "--enable-static --enable-debug --disable-doc --enable-libx264 --enable-gpl --enable-nonfree",
		Libraries: []Library{
			{
				Name:     "libavutil",
				Compiled: "56. 51.100",
				Linked:   "56. 51.100",
			},
			{
				Name:     "libavcodec",
				Compiled: "58. 91.100",
				Linked:   "58. 91.100",
			},
			{
				Name:     "libavformat",
				Compiled: "58. 45.100",
				Linked:   "58. 45.100",
			},
			{
				Name:     "libavdevice",
				Compiled: "58. 10.100",
				Linked:   "58. 10.100",
			},
			{
				Name:     "libavfilter",
				Compiled: "7. 85.100",
				Linked:   "7. 85.100",
			},
			{
				Name:     "libswscale",
				Compiled: "5.  7.100",
				Linked:   "5.  7.100",
			},
			{
				Name:     "libswresample",
				Compiled: "3.  7.100",
				Linked:   "3.  7.100",
			},
			{
				Name:     "libpostproc",
				Compiled: "55.  7.100",
				Linked:   "55.  7.100",
			},
		},
	}, f)
}

func TestMinorVersion(t *testing.T) {
	data := `ffmpeg version 4.4 Copyright (c) 2000-2020 the FFmpeg developers
 built with Apple clang version 12.0.0 (clang-1200.0.32.29)
 configuration: --enable-static --enable-debug --disable-doc --enable-libx264 --enable-gpl --enable-nonfree
 libavutil      56. 51.100 / 56. 51.100
 libavcodec     58. 91.100 / 58. 91.100
 libavformat    58. 45.100 / 58. 45.100
 libavdevice    58. 10.100 / 58. 10.100
 libavfilter     7. 85.100 /  7. 85.100
 libswscale      5.  7.100 /  5.  7.100
 libswresample   3.  7.100 /  3.  7.100
 libpostproc    55.  7.100 / 55.  7.100`

	f := parseVersion([]byte(data))

	require.Equal(t, ffmpeg{
		Version:       "4.4.0",
		Compiler:      "Apple clang version 12.0.0 (clang-1200.0.32.29)",
		Configuration: "--enable-static --enable-debug --disable-doc --enable-libx264 --enable-gpl --enable-nonfree",
		Libraries: []Library{
			{
				Name:     "libavutil",
				Compiled: "56. 51.100",
				Linked:   "56. 51.100",
			},
			{
				Name:     "libavcodec",
				Compiled: "58. 91.100",
				Linked:   "58. 91.100",
			},
			{
				Name:     "libavformat",
				Compiled: "58. 45.100",
				Linked:   "58. 45.100",
			},
			{
				Name:     "libavdevice",
				Compiled: "58. 10.100",
				Linked:   "58. 10.100",
			},
			{
				Name:     "libavfilter",
				Compiled: "7. 85.100",
				Linked:   "7. 85.100",
			},
			{
				Name:     "libswscale",
				Compiled: "5.  7.100",
				Linked:   "5.  7.100",
			},
			{
				Name:     "libswresample",
				Compiled: "3.  7.100",
				Linked:   "3.  7.100",
			},
			{
				Name:     "libpostproc",
				Compiled: "55.  7.100",
				Linked:   "55.  7.100",
			},
		},
	}, f)
}

func TestCustomVersion(t *testing.T) {
	data := ffmpegdata

	f := parseVersion([]byte(data))

	require.Equal(t, ffmpeg{
		Version:       "4.4.1",
		Compiler:      "gcc 10.3.1 (Alpine 10.3.1_git20211027) 20211027",
		Configuration: "--extra-version=datarhei --prefix=/usr --extra-libs='-lpthread -lm -lz -lsupc++ -lstdc++ -lssl -lcrypto -lz -lc -ldl' --enable-nonfree --enable-gpl --enable-version3 --enable-postproc --enable-static --enable-openssl --enable-omx --enable-omx-rpi --enable-mmal --enable-v4l2_m2m --enable-libfreetype --enable-libsrt --enable-libx264 --enable-libx265 --enable-libvpx --enable-libmp3lame --enable-libopus --enable-libvorbis --disable-ffplay --disable-debug --disable-doc --disable-shared",
		Libraries: []Library{
			{
				Name:     "libavutil",
				Compiled: "56. 70.100",
				Linked:   "56. 70.100",
			},
			{
				Name:     "libavcodec",
				Compiled: "58.134.100",
				Linked:   "58.134.100",
			},
			{
				Name:     "libavformat",
				Compiled: "58. 76.100",
				Linked:   "58. 76.100",
			},
			{
				Name:     "libavdevice",
				Compiled: "58. 13.100",
				Linked:   "58. 13.100",
			},
			{
				Name:     "libavfilter",
				Compiled: "7.110.100",
				Linked:   "7.110.100",
			},
			{
				Name:     "libswscale",
				Compiled: "5.  9.100",
				Linked:   "5.  9.100",
			},
			{
				Name:     "libswresample",
				Compiled: "3.  9.100",
				Linked:   "3.  9.100",
			},
			{
				Name:     "libpostproc",
				Compiled: "55.  9.100",
				Linked:   "55.  9.100",
			},
		},
	}, f)
}

func TestFilters(t *testing.T) {
	data := filterdata

	f := parseFilters([]byte(data))

	require.Equal(t, []Filter{
		{
			Id:   "afirsrc",
			Name: "Generate a FIR coefficients audio stream.",
		},
		{
			Id:   "anoisesrc",
			Name: "Generate a noise audio signal.",
		},
		{
			Id:   "anullsrc",
			Name: "Null audio source, return empty audio frames.",
		},
		{
			Id:   "hilbert",
			Name: "Generate a Hilbert transform FIR coefficients.",
		},
		{
			Id:   "sinc",
			Name: "Generate a sinc kaiser-windowed low-pass, high-pass, band-pass, or band-reject FIR coefficients.",
		},
		{
			Id:   "sine",
			Name: "Generate sine wave audio signal.",
		},
		{
			Id:   "anullsink",
			Name: "Do absolutely nothing with the input audio.",
		},
		{
			Id:   "addroi",
			Name: "Add region of interest to frame.",
		},
		{
			Id:   "alphaextract",
			Name: "Extract an alpha channel as a grayscale image component.",
		},
		{
			Id:   "alphamerge",
			Name: "Copy the luma value of the second input into the alpha channel of the first input.",
		},
	}, f)
}

func TestCodecs(t *testing.T) {
	data := codecdata

	c := parseCodecs([]byte(data))

	require.Equal(t, ffCodecs{
		Audio: []Codec{
			{
				Id:   "aac",
				Name: "AAC (Advanced Audio Coding)",
				Encoders: []string{
					"aac",
					"aac_at",
				},
				Decoders: []string{
					"aac",
					"aac_fixed",
					"aac_at",
				},
			},
		},
		Video: []Codec{
			{
				Id:   "y41p",
				Name: "Uncompressed YUV 4:1:1 12-bit",
				Encoders: []string{
					"y41p",
				},
				Decoders: []string{
					"y41p",
				},
			},
			{
				Id:   "h264",
				Name: "H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10",
				Encoders: []string{
					"libx264",
					"libx264rgb",
					"h264_videotoolbox",
				},
				Decoders: []string{
					"h264",
				},
			},
			{
				Id:   "flv1",
				Name: "FLV / Sorenson Spark / Sorenson H.263 (Flash Video)",
				Encoders: []string{
					"flv",
				},
				Decoders: []string{
					"flv",
				},
			},
		},
		Subtitle: nil,
	}, c)
}

func TestFormats(t *testing.T) {
	data := formatdata

	f := parseFormats([]byte(data))

	require.Equal(t, ffFormats{
		Demuxers: []Format{
			{
				Id:   "mpeg",
				Name: "MPEG-1 Systems / MPEG program stream",
			},
			{
				Id:   "mpegts",
				Name: "MPEG-TS (MPEG-2 Transport Stream)",
			},
			{
				Id:   "mpegtsraw",
				Name: "raw MPEG-TS (MPEG-2 Transport Stream)",
			},
			{
				Id:   "mpegvideo",
				Name: "raw MPEG video",
			},
		},
		Muxers: []Format{
			{
				Id:   "mpeg",
				Name: "MPEG-1 Systems / MPEG program stream",
			},
			{
				Id:   "mpeg1video",
				Name: "raw MPEG-1 video",
			},
			{
				Id:   "mpeg2video",
				Name: "raw MPEG-2 video",
			},
			{
				Id:   "mpegts",
				Name: "MPEG-TS (MPEG-2 Transport Stream)",
			},
		},
	}, f)
}

func TestProtocols(t *testing.T) {
	data := protocoldata

	p := parseProtocols([]byte(data))

	require.Equal(t, ffProtocols{
		Input: []Protocol{
			{
				Id:   "async",
				Name: "async",
			},
			{
				Id:   "bluray",
				Name: "bluray",
			},
			{
				Id:   "cache",
				Name: "cache",
			},
		},
		Output: []Protocol{
			{
				Id:   "crypto",
				Name: "crypto",
			},
			{
				Id:   "file",
				Name: "file",
			},
			{
				Id:   "ftp",
				Name: "ftp",
			},
			{
				Id:   "gopher",
				Name: "gopher",
			},
		},
	}, p)
}

func TestHWAccels(t *testing.T) {
	data := hwacceldata

	p := parseHWAccels([]byte(data))

	require.Equal(t, []HWAccel{
		{
			Id:   "videotoolbox",
			Name: "videotoolbox",
		},
	}, p)
}
