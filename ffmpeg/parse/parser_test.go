package parse

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParserProgress(t *testing.T) {
	parser := New(Config{
		LogLines: 20,
	}).(*parser)

	parser.prelude.done = true
	parser.Parse("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463")

	d, _ := time.ParseDuration("3m58s440ms")
	wantP := Progress{
		Frame:     5968,
		FPS:       25,
		Quantizer: 19.4,
		Size:      453632,
		Time:      d.Seconds(),
		Bitrate:   5632,
		Speed:     0.999,
		Drop:      3522,
		Dup:       87463,
	}

	p := parser.Progress()

	require.Equal(t, wantP.Frame, p.Frame)
	require.Equal(t, wantP.Quantizer, p.Quantizer)
	require.Equal(t, wantP.Size, p.Size)
	require.Equal(t, wantP.Time, p.Time)
	require.Equal(t, wantP.Speed, p.Speed)
	require.Equal(t, wantP.Drop, p.Drop)
	require.Equal(t, wantP.Dup, p.Dup)

	parser.ResetStats()

	wantP = Progress{}

	p = parser.Progress()

	require.Equal(t, wantP.Frame, p.Frame)
	require.Equal(t, wantP.Quantizer, p.Quantizer)
	require.Equal(t, wantP.Size, p.Size)
	require.Equal(t, wantP.Time, p.Time)
	require.Equal(t, wantP.Speed, p.Speed)
	require.Equal(t, wantP.Drop, p.Drop)
	require.Equal(t, wantP.Dup, p.Dup)
}

func TestParserPrelude(t *testing.T) {
	parser := New(Config{
		LogLines:         20,
		PreludeHeadLines: 100,
		PreludeTailLines: 50,
	})

	log := parser.Prelude()

	require.Equal(t, 0, len(log))

	parser.Parse("prelude")

	log = parser.Prelude()

	require.Equal(t, 1, len(log))
}

func TestParserLongPrelude(t *testing.T) {
	parser := New(Config{
		LogLines:         20,
		PreludeHeadLines: 100,
		PreludeTailLines: 50,
	})

	log := parser.Prelude()

	require.Equal(t, 0, len(log))

	for i := 0; i < 150; i++ {
		parser.Parse(fmt.Sprintf("prelude %3d", i))
	}

	log = parser.Prelude()

	require.Equal(t, 150, len(log))
}

func TestParserVeryLongPrelude(t *testing.T) {
	parser := New(Config{
		LogLines:         20,
		PreludeHeadLines: 100,
		PreludeTailLines: 50,
	})

	log := parser.Prelude()

	require.Equal(t, 0, len(log))

	for i := 0; i < 300; i++ {
		parser.Parse(fmt.Sprintf("prelude %3d", i))
	}

	log = parser.Prelude()

	require.Equal(t, 151, len(log))
}

func TestParserLog(t *testing.T) {
	parser := New(Config{
		LogLines: 20,
	})

	log := parser.Log()

	require.Equal(t, 0, len(log))

	parser.Parse("bla")

	log = parser.Log()

	require.Equal(t, 1, len(log))
}

func TestParserLastLogLine(t *testing.T) {
	parser := New(Config{
		LogLines: 20,
	}).(*parser)

	parser.Parse("foo")

	line := parser.LastLogline()
	require.Equal(t, "foo", line)

	parser.prelude.done = true
	parser.Parse("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463")

	// progress lines are not logged
	line = parser.LastLogline()
	require.Equal(t, "foo", line)

	parser.Parse("bar")
	line = parser.LastLogline()
	require.Equal(t, "bar", line)
}

func TestParserLogHistory(t *testing.T) {
	parser := New(Config{
		LogLines:   20,
		LogHistory: 5,
	}).(*parser)

	parser.Parse("bla")

	parser.prelude.done = true
	parser.Parse("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463")

	history := parser.ReportHistory()
	require.Equal(t, 0, len(history))

	parser.Stop("finished")

	history = parser.ReportHistory()
	require.Equal(t, 1, len(history))

	require.Equal(t, "finished", history[0].ExitState)
	require.Equal(t, "bla", history[0].Log[0].Data)
	require.Equal(t, "bla", history[0].Prelude[0])

	d, _ := time.ParseDuration("3m58s440ms")
	require.Equal(t, Progress{
		Frame:     5968,
		FPS:       0, // is calculated with averager
		Quantizer: 19.4,
		Size:      453632,
		Time:      d.Seconds(),
		Bitrate:   0, // is calculated with averager
		Speed:     0.999,
		Drop:      3522,
		Dup:       87463,
	}, history[0].Progress)
}

func TestParserLogHistoryLength(t *testing.T) {
	parser := New(Config{
		LogLines:   20,
		LogHistory: 3,
	}).(*parser)

	history := parser.ReportHistory()
	require.Equal(t, 0, len(history))

	for i := 0; i < 5; i++ {
		parser.Parse("bla")

		parser.prelude.done = true
		parser.Parse("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463")

		parser.Stop("finished")
	}

	history = parser.ReportHistory()
	require.Equal(t, 3, len(history))
}

func TestParserLogMinimalHistoryLength(t *testing.T) {
	parser := New(Config{
		LogLines:          20,
		LogHistory:        3,
		LogMinimalHistory: 10,
	}).(*parser)

	history := parser.ReportHistory()
	require.Equal(t, 0, len(history))

	for i := 0; i < 15; i++ {
		parser.Parse("bla")

		parser.prelude.done = true
		parser.Parse("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463")

		parser.Stop("finished")
	}

	history = parser.ReportHistory()
	require.Equal(t, 13, len(history))

	for i := 0; i < 10; i++ {
		require.Empty(t, history[i].Log, i)
	}

	for i := 10; i < 13; i++ {
		require.NotEmpty(t, history[i].Log, i)
	}
}

func TestParserLogMinimalHistoryLengthWithoutFullHistory(t *testing.T) {
	parser := New(Config{
		LogLines:          20,
		LogHistory:        0,
		LogMinimalHistory: 10,
	}).(*parser)

	history := parser.ReportHistory()
	require.Equal(t, 0, len(history))

	for i := 0; i < 15; i++ {
		parser.Parse("bla")

		parser.prelude.done = true
		parser.Parse("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463")

		parser.Stop("finished")
	}

	history = parser.ReportHistory()
	require.Equal(t, 10, len(history))

	for i := 0; i < 10; i++ {
		require.Empty(t, history[i].Log, i)
	}
}

func TestParserLogHistorySearch(t *testing.T) {
	parser := New(Config{
		LogLines:   20,
		LogHistory: 5,
	}).(*parser)

	parser.Parse("foo")

	parser.prelude.done = true
	parser.Parse("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463")

	parser.Stop("finished")

	parser.ResetStats()

	time.Sleep(2 * time.Second)

	parser.ResetLog()

	parser.Parse("bar")

	parser.prelude.done = true
	parser.Parse("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463")

	parser.Stop("finished")

	parser.ResetStats()

	time.Sleep(2 * time.Second)

	parser.ResetLog()

	parser.Parse("foobar")

	parser.prelude.done = true
	parser.Parse("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463")

	parser.Stop("failed")

	res := parser.SearchReportHistory("", nil, nil)
	require.Equal(t, 3, len(res))

	times := []time.Time{
		time.Unix(res[0].CreatedAt.Unix(), 0),
		time.Unix(res[1].CreatedAt.Unix(), 0),
		time.Unix(res[2].CreatedAt.Unix(), 0),
	}

	res = parser.SearchReportHistory("finished", nil, nil)
	require.Equal(t, 2, len(res))

	res = parser.SearchReportHistory("failed", nil, nil)
	require.Equal(t, 1, len(res))

	res = parser.SearchReportHistory("", &times[0], nil)
	require.Equal(t, 3, len(res))

	res = parser.SearchReportHistory("", &times[1], nil)
	require.Equal(t, 2, len(res))

	res = parser.SearchReportHistory("", &times[2], nil)
	require.Equal(t, 1, len(res))

	to := times[2].Add(1 * time.Second)

	res = parser.SearchReportHistory("", nil, &to)
	require.Equal(t, 3, len(res))

	res = parser.SearchReportHistory("", nil, &times[2])
	require.Equal(t, 2, len(res))

	res = parser.SearchReportHistory("", nil, &times[1])
	require.Equal(t, 1, len(res))

	res = parser.SearchReportHistory("", nil, &times[0])
	require.Equal(t, 0, len(res))

	res = parser.SearchReportHistory("", &times[1], &times[2])
	require.Equal(t, 1, len(res))

	res = parser.SearchReportHistory("finished", &times[2], &to)
	require.Equal(t, 0, len(res))
}

func TestParserReset(t *testing.T) {
	parser := New(Config{
		LogLines:         20,
		PreludeHeadLines: 100,
		PreludeTailLines: 50,
	})

	log := parser.Log()
	prelude := parser.Prelude()

	require.Equal(t, 0, len(log))
	require.Equal(t, 0, len(prelude))

	parser.Parse("prelude")

	log = parser.Log()
	prelude = parser.Prelude()

	require.Equal(t, 1, len(log))
	require.Equal(t, 1, len(prelude))

	parser.ResetStats()

	log = parser.Log()
	prelude = parser.Prelude()

	require.NotEqual(t, 0, len(log))
	require.NotEqual(t, 0, len(prelude))

	parser.ResetLog()

	log = parser.Log()
	prelude = parser.Prelude()

	require.Equal(t, 0, len(log))
	require.Equal(t, 0, len(prelude))
}

func TestParserDefault(t *testing.T) {
	parser := New(Config{
		LogLines: 20,
	}).(*parser)

	rawdata := `ffmpeg version 4.0.2 Copyright (c) 2000-2018 the FFmpeg developers
  built with Apple LLVM version 9.1.0 (clang-902.0.39.2)
  configuration: --prefix=/usr/local/Cellar/ffmpeg/4.0.2 --enable-shared --enable-pthreads --enable-version3 --enable-hardcoded-tables --enable-avresample --cc=clang --host-cflags= --host-ldflags= --enable-gpl --enable-libmp3lame --enable-libx264 --enable-libx265 --enable-libxvid --enable-opencl --enable-videotoolbox --disable-lzma
  libavutil      56. 14.100 / 56. 14.100
  libavcodec     58. 18.100 / 58. 18.100
  libavformat    58. 12.100 / 58. 12.100
  libavdevice    58.  3.100 / 58.  3.100
  libavfilter     7. 16.100 /  7. 16.100
  libavresample   4.  0.  0 /  4.  0.  0
  libswscale      5.  1.100 /  5.  1.100
  libswresample   3.  1.100 /  3.  1.100
  libpostproc    55.  1.100 / 55.  1.100
Input #0, lavfi, from 'testsrc=size=1280x720:rate=25':
  Duration: N/A, start: 0.000000, bitrate: N/A
    Stream #0:0: Video: rawvideo (RGB[24] / 0x18424752), rgb24, 1280x720 [SAR 1:1 DAR 16:9], 25 tbr, 25 tbn, 25 tbc
Input #1, lavfi, from 'anullsrc=r=44100:cl=stereo':
  Duration: N/A, start: 0.000000, bitrate: 705 kb/s
    Stream #1:0: Audio: pcm_u8, 44100 Hz, stereo, u8, 705 kb/s
	Stream #1:1(eng): Audio: aac (LC), 48000 Hz, stereo, fltp (default)
Stream mapping:
  Stream #0:0 -> #0:0 (rawvideo (native) -> h264 (libx264))
  Stream #1:0 -> #0:1 (pcm_u8 (native) -> aac (native))
Press [q] to stop, [?] for help
[libx264 @ 0x7fa96a800600] using SAR=1/1
[libx264 @ 0x7fa96a800600] using cpu capabilities: MMX2 SSE2Fast SSSE3 SSE4.2 AVX FMA3 BMI2 AVX2
[libx264 @ 0x7fa96a800600] profile Constrained Baseline, level 3.1
[libx264 @ 0x7fa96a800600] 264 - core 152 r2854 e9a5903 - H.264/MPEG-4 AVC codec - Copyleft 2003-2017 - http://www.videolan.org/x264.html - options: cabac=0 ref=1 deblock=0:0:0 analyse=0:0 me=dia subme=0 psy=1 psy_rd=1.00:0.00 mixed_ref=0 me_range=16 chroma_me=1 trellis=0 8x8dct=0 cqm=0 deadzone=21,11 fast_pskip=1 chroma_qp_offset=0 threads=6 lookahead_threads=1 sliced_threads=0 nr=0 decimate=1 interlaced=0 bluray_compat=0 constrained_intra=0 bframes=0 weightp=0 keyint=50 keyint_min=5 scenecut=0 intra_refresh=0 rc=crf mbtree=0 crf=23.0 qcomp=0.60 qpmin=0 qpmax=69 qpstep=4 ip_ratio=1.40 aq=0
[hls @ 0x7fa969803a00] Opening './data/testsrc5375.ts.tmp' for writing
Output #0, hls, to './data/testsrc.m3u8':
  Metadata:
    encoder         : Lavf58.12.100
    Stream #0:0: Video: h264 (libx264), yuv420p(progressive), 1280x720 [SAR 1:1 DAR 16:9], q=-1--1, 25 fps, 90k tbn, 25 tbc
    Metadata:
      encoder         : Lavc58.18.100 libx264
    Side data:
      cpb: bitrate max/min/avg: 0/0/0 buffer size: 0 vbv_delay: -1
    Stream #0:1: Audio: aac (LC), 44100 Hz, stereo, fltp, 64 kb/s
    Metadata:
      encoder         : Lavc58.18.100 aac
[hls @ 0x7fa969803a00] Opening './data/testsrc5376.ts.tmp' for writing=0.872x
[hls @ 0x7fa969803a00] Opening './data/testsrc.m3u8.tmp' for writing
[hls @ 0x7fa969803a00] Opening './data/testsrc.m3u8.tmp' for writing
frame=   58 fps= 25 q=-1.0 Lsize=N/A time=00:00:02.32 bitrate=N/A speed=0.999x`

	data := strings.Split(rawdata, "\n")

	for _, d := range data {
		parser.Parse(d)
	}

	require.Equal(t, 3, len(parser.process.input), "expected 3 inputs")
	require.Equal(t, 2, len(parser.process.output), "expected 2 outputs")
}

func TestParserDefaultDelayed(t *testing.T) {
	parser := New(Config{
		LogLines: 20,
	}).(*parser)

	rawdata := `ffmpeg version 4.0.2 Copyright (c) 2000-2018 the FFmpeg developers
  built with Apple LLVM version 9.1.0 (clang-902.0.39.2)
  configuration: --prefix=/usr/local/Cellar/ffmpeg/4.0.2 --enable-shared --enable-pthreads --enable-version3 --enable-hardcoded-tables --enable-avresample --cc=clang --host-cflags= --host-ldflags= --enable-gpl --enable-libmp3lame --enable-libx264 --enable-libx265 --enable-libxvid --enable-opencl --enable-videotoolbox --disable-lzma
  libavutil      56. 14.100 / 56. 14.100
  libavcodec     58. 18.100 / 58. 18.100
  libavformat    58. 12.100 / 58. 12.100
  libavdevice    58.  3.100 / 58.  3.100
  libavfilter     7. 16.100 /  7. 16.100
  libavresample   4.  0.  0 /  4.  0.  0
  libswscale      5.  1.100 /  5.  1.100
  libswresample   3.  1.100 /  3.  1.100
  libpostproc    55.  1.100 / 55.  1.100
Input #0, lavfi, from 'testsrc=size=1280x720:rate=25':
  Duration: N/A, start: 0.000000, bitrate: N/A
    Stream #0:0: Video: rawvideo (RGB[24] / 0x18424752), rgb24, 1280x720 [SAR 1:1 DAR 16:9], 25 tbr, 25 tbn, 25 tbc
Input #1, lavfi, from 'anullsrc=r=44100:cl=stereo':
  Duration: N/A, start: 0.000000, bitrate: 705 kb/s
    Stream #1:0: Audio: pcm_u8, 44100 Hz, stereo, u8, 705 kb/s
Stream mapping:
  Stream #0:0 -> #0:0 (rawvideo (native) -> h264 (libx264))
  Stream #1:0 -> #0:1 (pcm_u8 (native) -> aac (native))
Press [q] to stop, [?] for help
frame=   29 fps= 25 q=-1.0 Lsize=N/A time=00:00:01.32 bitrate=N/A speed=0.999x
[libx264 @ 0x7fa96a800600] using SAR=1/1
[libx264 @ 0x7fa96a800600] using cpu capabilities: MMX2 SSE2Fast SSSE3 SSE4.2 AVX FMA3 BMI2 AVX2
[libx264 @ 0x7fa96a800600] profile Constrained Baseline, level 3.1
[libx264 @ 0x7fa96a800600] 264 - core 152 r2854 e9a5903 - H.264/MPEG-4 AVC codec - Copyleft 2003-2017 - http://www.videolan.org/x264.html - options: cabac=0 ref=1 deblock=0:0:0 analyse=0:0 me=dia subme=0 psy=1 psy_rd=1.00:0.00 mixed_ref=0 me_range=16 chroma_me=1 trellis=0 8x8dct=0 cqm=0 deadzone=21,11 fast_pskip=1 chroma_qp_offset=0 threads=6 lookahead_threads=1 sliced_threads=0 nr=0 decimate=1 interlaced=0 bluray_compat=0 constrained_intra=0 bframes=0 weightp=0 keyint=50 keyint_min=5 scenecut=0 intra_refresh=0 rc=crf mbtree=0 crf=23.0 qcomp=0.60 qpmin=0 qpmax=69 qpstep=4 ip_ratio=1.40 aq=0
[hls @ 0x7fa969803a00] Opening './data/testsrc5375.ts.tmp' for writing
Output #0, hls, to './data/testsrc.m3u8':
  Metadata:
    encoder         : Lavf58.12.100
    Stream #0:0: Video: h264 (libx264), yuv420p(progressive), 1280x720 [SAR 1:1 DAR 16:9], q=-1--1, 25 fps, 90k tbn, 25 tbc
    Metadata:
      encoder         : Lavc58.18.100 libx264
    Side data:
      cpb: bitrate max/min/avg: 0/0/0 buffer size: 0 vbv_delay: -1
    Stream #0:1: Audio: aac (LC), 44100 Hz, stereo, fltp, 64 kb/s
    Metadata:
      encoder         : Lavc58.18.100 aac
[hls @ 0x7fa969803a00] Opening './data/testsrc5376.ts.tmp' for writing=0.872x
[hls @ 0x7fa969803a00] Opening './data/testsrc.m3u8.tmp' for writing
[hls @ 0x7fa969803a00] Opening './data/testsrc.m3u8.tmp' for writing
frame=   58 fps= 25 q=-1.0 Lsize=N/A time=00:00:02.32 bitrate=N/A speed=0.999x`

	data := strings.Split(rawdata, "\n")

	for _, d := range data {
		parser.Parse(d)
	}

	require.Equal(t, 2, len(parser.process.input), "expected 2 inputs")
	require.Equal(t, 2, len(parser.process.output), "expected 2 outputs")
}

func TestParserJSON(t *testing.T) {
	parser := New(Config{
		LogLines: 20,
	}).(*parser)

	rawdata := `ffmpeg version 4.3.1 Copyright (c) 2000-2020 the FFmpeg developers
  built with gcc 9.3.0 (Alpine 9.3.0)
  configuration: --prefix=/usr/local --enable-nonfree --enable-gpl --enable-version3 --enable-libmp3lame --enable-libx264 --enable-openssl --enable-shared --enable-libfreetype --disable-ffplay --disable-debug --disable-doc
  libavutil      56. 51.100 / 56. 51.100
  libavcodec     58. 91.100 / 58. 91.100
  libavformat    58. 45.100 / 58. 45.100
  libavdevice    58. 10.100 / 58. 10.100
  libavfilter     7. 85.100 /  7. 85.100
  libswscale      5.  7.100 /  5.  7.100
  libswresample   3.  7.100 /  3.  7.100
  libpostproc    55.  7.100 / 55.  7.100
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-SESSION-DATA:DATA-ID="net.z60wzayk.metadata",URI="https://vpu.livespotting.com/e9slfpe3/z60wzayk_metadata.json"')
[hls @ 0x7f92a96e8100] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8' for reading
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
[hls @ 0x7f92a96e8100] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100788.ts' for reading
[hls @ 0x7f92a96e8100] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100789.ts' for reading
[https @ 0x557c840d1080] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100790.ts' for reading
[https @ 0x557c840bf480] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8' for reading
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
[https @ 0x557c840d1080] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100791.ts' for reading
[https @ 0x557c840bf480] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8' for reading
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
[https @ 0x557c840d1080] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100792.ts' for reading
Input #0, playout, from 'https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk.m3u8':
  Duration: N/A, start: 0.000000, bitrate: N/A
    Stream #0:0: Video: h264 (Baseline) ([27][0][0][0] / 0x001B), yuvj420p(pc, bt709), 1280x720 [SAR 1:1 DAR 16:9], 20.67 fps, 25 tbr, 1000k tbn, 2000k tbc
    Metadata:
      variant_bitrate : 1024000
ffmpeg.inputs:[{"url":"https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk.m3u8","format":"playout","index":0,"stream":0,"type":"video","codec":"h264","coder":"h264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":20.666666,"pix_fmt":"yuvj420p","width":1280,"height":720}]
Output #1, mp4, to '/dev/null':
  Metadata:
    encoder         : Lavf58.45.100
    Stream #1:0: Video: h264 (Baseline) (avc1 / 0x31637661), yuvj420p(pc, bt709), 1280x720 [SAR 1:1 DAR 16:9], q=2-31, 20.67 fps, 25 tbr, 1000k tbn, 1000k tbc
    Metadata:
      variant_bitrate : 1024000
Stream mapping:
  Stream #0:0 -> #0:0 (h264 (native) -> h264 (libx264))
  Stream #0:0 -> #1:0 (copy)
Press [q] to stop, [?] for help
[libx264 @ 0x557c8443e200] using SAR=1/1
[libx264 @ 0x557c8443e200] using cpu capabilities: MMX2 SSE2Fast SSSE3 SSE4.2 AVX FMA3 BMI2 AVX2
[libx264 @ 0x557c8443e200] profile Constrained Baseline, level 3.1, 4:2:0, 8-bit
[libx264 @ 0x557c8443e200] 264 - core 161 - H.264/MPEG-4 AVC codec - Copyleft 2003-2021 - http://www.videolan.org/x264.html - options: cabac=0 ref=1 deblock=0:0:0 analyse=0:0 me=dia subme=0 psy=1 psy_rd=1.00:0.00 mixed_ref=0 me_range=16 chroma_me=1 trellis=0 8x8dct=0 cqm=0 deadzone=21,11 fast_pskip=1 chroma_qp_offset=0 threads=6 lookahead_threads=1 sliced_threads=0 nr=0 decimate=1 interlaced=0 bluray_compat=0 constrained_intra=0 bframes=0 weightp=0 keyint=250 keyint_min=25 scenecut=0 intra_refresh=0 rc=crf mbtree=0 crf=23.0 qcomp=0.60 qpmin=0 qpmax=69 qpstep=4 ip_ratio=1.40 aq=0
Output #0, flv, to '/dev/null':
  Metadata:
    encoder         : Lavf58.45.100
    Stream #0:0: Video: h264 (libx264) ([7][0][0][0] / 0x0007), yuvj420p(pc), 1280x720 [SAR 1:1 DAR 16:9], q=-1--1, 25 fps, 1k tbn, 25 tbc
    Metadata:
      variant_bitrate : 1024000
      encoder         : Lavc58.91.100 libx264
    Side data:
      cpb: bitrate max/min/avg: 0/0/0 buffer size: 0 vbv_delay: N/A
ffmpeg.outputs:[{"url":"/dev/null","format":"flv","index":0,"stream":0,"type":"video","codec":"h264","coder":"libx264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":25.000000,"pix_fmt":"yuvj420p","width":1280,"height":720},{"url":"/dev/null","format":"mp4","index":1,"stream":0,"type":"video","codec":"h264","coder":"copy","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":20.666666,"pix_fmt":"yuvj420p","width":1280,"height":720}]
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":2,"packet":6,"size_kb":222}],"outputs":[{"index":0,"stream":0,"frame":2,"packet":0,"q":0.0,"size_kb":0},{"index":1,"stream":0,"frame":6,"packet":6,"q":-1.0,"size_kb":222}],"frame":2,"packet":0,"q":0.0,"size_kb":222,"time":"0h0m0.20s","speed":0.281,"dup":0,"drop":0}
[https @ 0x557c840bf480] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8' for reading
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
[https @ 0x557c840d1080] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100793.ts' for reading
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":7,"packet":11,"size_kb":226}],"outputs":[{"index":0,"stream":0,"frame":7,"packet":0,"q":0.0,"size_kb":0},{"index":1,"stream":0,"frame":11,"packet":11,"q":-1.0,"size_kb":226}],"frame":7,"packet":0,"q":0.0,"size_kb":226,"time":"0h0m0.56s","speed":0.4,"dup":0,"drop":0}
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":10,"packet":14,"size_kb":227}],"outputs":[{"index":0,"stream":0,"frame":10,"packet":3,"q":16.0,"size_kb":281},{"index":1,"stream":0,"frame":14,"packet":14,"q":-1.0,"size_kb":227}],"frame":10,"packet":3,"q":16.0,"size_kb":508,"time":"0h0m0.68s","speed":0.358,"dup":0,"drop":0}
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":13,"packet":17,"size_kb":227}],"outputs":[{"index":0,"stream":0,"frame":13,"packet":6,"q":14.0,"size_kb":350},{"index":1,"stream":0,"frame":17,"packet":17,"q":-1.0,"size_kb":227}],"frame":13,"packet":6,"q":14.0,"size_kb":577,"time":"0h0m0.80s","speed":0.333,"dup":0,"drop":0}
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":16,"packet":20,"size_kb":227}],"outputs":[{"index":0,"stream":0,"frame":16,"packet":9,"q":13.0,"size_kb":489},{"index":1,"stream":0,"frame":20,"packet":20,"q":-1.0,"size_kb":227}],"frame":16,"packet":9,"q":13.0,"size_kb":716,"time":"0h0m0.92s","speed":0.306,"dup":0,"drop":0}
[https @ 0x557c840bf480] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8' for reading
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
[https @ 0x557c840d1080] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100794.ts' for reading
[https @ 0x557c840f3180] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100795.ts' for reading
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":21,"packet":24,"size_kb":228}],"outputs":[{"index":0,"stream":0,"frame":20,"packet":20,"q":-1.0,"size_kb":562},{"index":1,"stream":0,"frame":24,"packet":24,"q":-1.0,"size_kb":228}],"frame":20,"packet":20,"q":-1.0,"size_kb":789,"time":"0h0m1.8s","speed":0.245,"dup":0,"drop":0}`

	data := strings.Split(rawdata, "\n")

	for _, d := range data {
		parser.Parse(d)
	}

	require.Equal(t, 1, len(parser.process.input), "expected 1 input")
	require.Equal(t, 2, len(parser.process.output), "expected 2 outputs")
}

func TestParserJSONDelayed(t *testing.T) {
	parser := New(Config{
		LogLines: 20,
	}).(*parser)

	rawdata := `ffmpeg version 4.3.1 Copyright (c) 2000-2020 the FFmpeg developers
  built with gcc 9.3.0 (Alpine 9.3.0)
  configuration: --prefix=/usr/local --enable-nonfree --enable-gpl --enable-version3 --enable-libmp3lame --enable-libx264 --enable-openssl --enable-shared --enable-libfreetype --disable-ffplay --disable-debug --disable-doc
  libavutil      56. 51.100 / 56. 51.100
  libavcodec     58. 91.100 / 58. 91.100
  libavformat    58. 45.100 / 58. 45.100
  libavdevice    58. 10.100 / 58. 10.100
  libavfilter     7. 85.100 /  7. 85.100
  libswscale      5.  7.100 /  5.  7.100
  libswresample   3.  7.100 /  3.  7.100
  libpostproc    55.  7.100 / 55.  7.100
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-SESSION-DATA:DATA-ID="net.z60wzayk.metadata",URI="https://vpu.livespotting.com/e9slfpe3/z60wzayk_metadata.json"')
[hls @ 0x7f92a96e8100] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8' for reading
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
[hls @ 0x7f92a96e8100] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100788.ts' for reading
[hls @ 0x7f92a96e8100] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100789.ts' for reading
[https @ 0x557c840d1080] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100790.ts' for reading
[https @ 0x557c840bf480] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8' for reading
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
[https @ 0x557c840d1080] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100791.ts' for reading
[https @ 0x557c840bf480] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8' for reading
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
[https @ 0x557c840d1080] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100792.ts' for reading
Input #0, playout, from 'https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk.m3u8':
  Duration: N/A, start: 0.000000, bitrate: N/A
    Stream #0:0: Video: h264 (Baseline) ([27][0][0][0] / 0x001B), yuvj420p(pc, bt709), 1280x720 [SAR 1:1 DAR 16:9], 20.67 fps, 25 tbr, 1000k tbn, 2000k tbc
    Metadata:
      variant_bitrate : 1024000
ffmpeg.inputs:[{"url":"https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk.m3u8","format":"playout","index":0,"stream":0,"type":"video","codec":"h264","coder":"h264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":20.666666,"pix_fmt":"yuvj420p","width":1280,"height":720}]
Output #1, mp4, to '/dev/null':
  Metadata:
    encoder         : Lavf58.45.100
    Stream #1:0: Video: h264 (Baseline) (avc1 / 0x31637661), yuvj420p(pc, bt709), 1280x720 [SAR 1:1 DAR 16:9], q=2-31, 20.67 fps, 25 tbr, 1000k tbn, 1000k tbc
    Metadata:
      variant_bitrate : 1024000
Stream mapping:
  Stream #0:0 -> #0:0 (h264 (native) -> h264 (libx264))
  Stream #0:0 -> #1:0 (copy)
Press [q] to stop, [?] for help
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":2,"packet":6,"size_kb":222}],"outputs":[{"index":0,"stream":0,"frame":2,"packet":0,"q":0.0,"size_kb":0},{"index":1,"stream":0,"frame":6,"packet":6,"q":-1.0,"size_kb":222}],"frame":2,"packet":0,"q":0.0,"size_kb":222,"time":"0h0m0.20s","speed":0.281,"dup":0,"drop":0}
[libx264 @ 0x557c8443e200] using SAR=1/1
[libx264 @ 0x557c8443e200] using cpu capabilities: MMX2 SSE2Fast SSSE3 SSE4.2 AVX FMA3 BMI2 AVX2
[libx264 @ 0x557c8443e200] profile Constrained Baseline, level 3.1, 4:2:0, 8-bit
[libx264 @ 0x557c8443e200] 264 - core 161 - H.264/MPEG-4 AVC codec - Copyleft 2003-2021 - http://www.videolan.org/x264.html - options: cabac=0 ref=1 deblock=0:0:0 analyse=0:0 me=dia subme=0 psy=1 psy_rd=1.00:0.00 mixed_ref=0 me_range=16 chroma_me=1 trellis=0 8x8dct=0 cqm=0 deadzone=21,11 fast_pskip=1 chroma_qp_offset=0 threads=6 lookahead_threads=1 sliced_threads=0 nr=0 decimate=1 interlaced=0 bluray_compat=0 constrained_intra=0 bframes=0 weightp=0 keyint=250 keyint_min=25 scenecut=0 intra_refresh=0 rc=crf mbtree=0 crf=23.0 qcomp=0.60 qpmin=0 qpmax=69 qpstep=4 ip_ratio=1.40 aq=0
Output #0, flv, to '/dev/null':
  Metadata:
    encoder         : Lavf58.45.100
    Stream #0:0: Video: h264 (libx264) ([7][0][0][0] / 0x0007), yuvj420p(pc), 1280x720 [SAR 1:1 DAR 16:9], q=-1--1, 25 fps, 1k tbn, 25 tbc
    Metadata:
      variant_bitrate : 1024000
      encoder         : Lavc58.91.100 libx264
    Side data:
      cpb: bitrate max/min/avg: 0/0/0 buffer size: 0 vbv_delay: N/A
ffmpeg.outputs:[{"url":"/dev/null","format":"flv","index":0,"stream":0,"type":"video","codec":"h264","coder":"libx264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":25.000000,"pix_fmt":"yuvj420p","width":1280,"height":720},{"url":"/dev/null","format":"mp4","index":1,"stream":0,"type":"video","codec":"h264","coder":"copy","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":20.666666,"pix_fmt":"yuvj420p","width":1280,"height":720}]
[https @ 0x557c840bf480] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8' for reading
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
[https @ 0x557c840d1080] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100793.ts' for reading
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":7,"packet":11,"size_kb":226}],"outputs":[{"index":0,"stream":0,"frame":7,"packet":0,"q":0.0,"size_kb":0},{"index":1,"stream":0,"frame":11,"packet":11,"q":-1.0,"size_kb":226}],"frame":7,"packet":0,"q":0.0,"size_kb":226,"time":"0h0m0.56s","speed":0.4,"dup":0,"drop":0}
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":10,"packet":14,"size_kb":227}],"outputs":[{"index":0,"stream":0,"frame":10,"packet":3,"q":16.0,"size_kb":281},{"index":1,"stream":0,"frame":14,"packet":14,"q":-1.0,"size_kb":227}],"frame":10,"packet":3,"q":16.0,"size_kb":508,"time":"0h0m0.68s","speed":0.358,"dup":0,"drop":0}
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":13,"packet":17,"size_kb":227}],"outputs":[{"index":0,"stream":0,"frame":13,"packet":6,"q":14.0,"size_kb":350},{"index":1,"stream":0,"frame":17,"packet":17,"q":-1.0,"size_kb":227}],"frame":13,"packet":6,"q":14.0,"size_kb":577,"time":"0h0m0.80s","speed":0.333,"dup":0,"drop":0}
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":16,"packet":20,"size_kb":227}],"outputs":[{"index":0,"stream":0,"frame":16,"packet":9,"q":13.0,"size_kb":489},{"index":1,"stream":0,"frame":20,"packet":20,"q":-1.0,"size_kb":227}],"frame":16,"packet":9,"q":13.0,"size_kb":716,"time":"0h0m0.92s","speed":0.306,"dup":0,"drop":0}
[https @ 0x557c840bf480] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8' for reading
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
[https @ 0x557c840d1080] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100794.ts' for reading
[https @ 0x557c840f3180] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100795.ts' for reading
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":21,"packet":24,"size_kb":228}],"outputs":[{"index":0,"stream":0,"frame":20,"packet":20,"q":-1.0,"size_kb":562},{"index":1,"stream":0,"frame":24,"packet":24,"q":-1.0,"size_kb":228}],"frame":20,"packet":20,"q":-1.0,"size_kb":789,"time":"0h0m1.8s","speed":0.245,"dup":0,"drop":0}`

	data := strings.Split(rawdata, "\n")

	for _, d := range data {
		parser.Parse(d)
	}

	require.Equal(t, 1, len(parser.process.input), "expected 1 input")
	require.Equal(t, 2, len(parser.process.output), "expected 2 outputs")
}

func TestParserJSONDelayedPlayout(t *testing.T) {
	parser := New(Config{
		LogLines: 20,
	}).(*parser)

	rawdata := `ffmpeg version 4.3.1 Copyright (c) 2000-2020 the FFmpeg developers
  built with gcc 9.3.0 (Alpine 9.3.0)
  configuration: --prefix=/usr/local --enable-nonfree --enable-gpl --enable-version3 --enable-libmp3lame --enable-libx264 --enable-openssl --enable-shared --enable-libfreetype --disable-ffplay --disable-debug --disable-doc
  libavutil      56. 51.100 / 56. 51.100
  libavcodec     58. 91.100 / 58. 91.100
  libavformat    58. 45.100 / 58. 45.100
  libavdevice    58. 10.100 / 58. 10.100
  libavfilter     7. 85.100 /  7. 85.100
  libswscale      5.  7.100 /  5.  7.100
  libswresample   3.  7.100 /  3.  7.100
  libpostproc    55.  7.100 / 55.  7.100
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-SESSION-DATA:DATA-ID="net.z60wzayk.metadata",URI="https://vpu.livespotting.com/e9slfpe3/z60wzayk_metadata.json"')
[hls @ 0x7f92a96e8100] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8' for reading
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
[hls @ 0x7f92a96e8100] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100788.ts' for reading
[hls @ 0x7f92a96e8100] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100789.ts' for reading
[https @ 0x557c840d1080] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100790.ts' for reading
[https @ 0x557c840bf480] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8' for reading
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
[https @ 0x557c840d1080] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100791.ts' for reading
[https @ 0x557c840bf480] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8' for reading
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
[https @ 0x557c840d1080] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100792.ts' for reading
Input #0, playout, from 'https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk.m3u8':
  Duration: N/A, start: 0.000000, bitrate: N/A
    Stream #0:0: Video: h264 (Baseline) ([27][0][0][0] / 0x001B), yuvj420p(pc, bt709), 1280x720 [SAR 1:1 DAR 16:9], 20.67 fps, 25 tbr, 1000k tbn, 2000k tbc
    Metadata:
      variant_bitrate : 1024000
ffmpeg.inputs:[{"url":"https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk.m3u8","format":"playout","index":0,"stream":0,"type":"video","codec":"h264","coder":"h264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":20.666666,"pix_fmt":"yuvj420p","width":1280,"height":720}]
Output #1, mp4, to '/dev/null':
  Metadata:
    encoder         : Lavf58.45.100
    Stream #1:0: Video: h264 (Baseline) (avc1 / 0x31637661), yuvj420p(pc, bt709), 1280x720 [SAR 1:1 DAR 16:9], q=2-31, 20.67 fps, 25 tbr, 1000k tbn, 1000k tbc
    Metadata:
      variant_bitrate : 1024000
Stream mapping:
  Stream #0:0 -> #0:0 (h264 (native) -> h264 (libx264))
  Stream #0:0 -> #1:0 (copy)
Press [q] to stop, [?] for help
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":2,"packet":6,"size_kb":222}],"outputs":[{"index":0,"stream":0,"frame":2,"packet":0,"q":0.0,"size_kb":0},{"index":1,"stream":0,"frame":6,"packet":6,"q":-1.0,"size_kb":222}],"frame":2,"packet":0,"q":0.0,"size_kb":222,"time":"0h0m0.20s","speed":0.281,"dup":0,"drop":0}
[libx264 @ 0x557c8443e200] using SAR=1/1
[libx264 @ 0x557c8443e200] using cpu capabilities: MMX2 SSE2Fast SSSE3 SSE4.2 AVX FMA3 BMI2 AVX2
[libx264 @ 0x557c8443e200] profile Constrained Baseline, level 3.1, 4:2:0, 8-bit
[libx264 @ 0x557c8443e200] 264 - core 161 - H.264/MPEG-4 AVC codec - Copyleft 2003-2021 - http://www.videolan.org/x264.html - options: cabac=0 ref=1 deblock=0:0:0 analyse=0:0 me=dia subme=0 psy=1 psy_rd=1.00:0.00 mixed_ref=0 me_range=16 chroma_me=1 trellis=0 8x8dct=0 cqm=0 deadzone=21,11 fast_pskip=1 chroma_qp_offset=0 threads=6 lookahead_threads=1 sliced_threads=0 nr=0 decimate=1 interlaced=0 bluray_compat=0 constrained_intra=0 bframes=0 weightp=0 keyint=250 keyint_min=25 scenecut=0 intra_refresh=0 rc=crf mbtree=0 crf=23.0 qcomp=0.60 qpmin=0 qpmax=69 qpstep=4 ip_ratio=1.40 aq=0
Output #0, flv, to '/dev/null':
  Metadata:
    encoder         : Lavf58.45.100
    Stream #0:0: Video: h264 (libx264) ([7][0][0][0] / 0x0007), yuvj420p(pc), 1280x720 [SAR 1:1 DAR 16:9], q=-1--1, 25 fps, 1k tbn, 25 tbc
    Metadata:
      variant_bitrate : 1024000
      encoder         : Lavc58.91.100 libx264
    Side data:
      cpb: bitrate max/min/avg: 0/0/0 buffer size: 0 vbv_delay: N/A
ffmpeg.outputs:[{"url":"/dev/null","format":"flv","index":0,"stream":0,"type":"video","codec":"h264","coder":"libx264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":25.000000,"pix_fmt":"yuvj420p","width":1280,"height":720},{"url":"/dev/null","format":"mp4","index":1,"stream":0,"type":"video","codec":"h264","coder":"copy","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":20.666666,"pix_fmt":"yuvj420p","width":1280,"height":720}]
[https @ 0x557c840bf480] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8' for reading
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
avstream.progress:{"id":"playout:https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk.m3u8","url":"https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk.m3u8","stream":0,"queue":140,"aqueue":0,"dup":0,"drop":0,"enc":0,"looping":false,"duplicating":false,"gop":"none","input":{"state":"running","packet":148,"size_kb":1529,"time":5},"output":{"state":"running","packet":8,"size_kb":128,"time":1},"swap":{"url":"","status":"waiting","lasturl":"","lasterror":""}}
[https @ 0x557c840d1080] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100793.ts' for reading
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":7,"packet":11,"size_kb":226}],"outputs":[{"index":0,"stream":0,"frame":7,"packet":0,"q":0.0,"size_kb":0},{"index":1,"stream":0,"frame":11,"packet":11,"q":-1.0,"size_kb":226}],"frame":7,"packet":0,"q":0.0,"size_kb":226,"time":"0h0m0.56s","speed":0.4,"dup":0,"drop":0}
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":10,"packet":14,"size_kb":227}],"outputs":[{"index":0,"stream":0,"frame":10,"packet":3,"q":16.0,"size_kb":281},{"index":1,"stream":0,"frame":14,"packet":14,"q":-1.0,"size_kb":227}],"frame":10,"packet":3,"q":16.0,"size_kb":508,"time":"0h0m0.68s","speed":0.358,"dup":0,"drop":0}
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":13,"packet":17,"size_kb":227}],"outputs":[{"index":0,"stream":0,"frame":13,"packet":6,"q":14.0,"size_kb":350},{"index":1,"stream":0,"frame":17,"packet":17,"q":-1.0,"size_kb":227}],"frame":13,"packet":6,"q":14.0,"size_kb":577,"time":"0h0m0.80s","speed":0.333,"dup":0,"drop":0}
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":16,"packet":20,"size_kb":227}],"outputs":[{"index":0,"stream":0,"frame":16,"packet":9,"q":13.0,"size_kb":489},{"index":1,"stream":0,"frame":20,"packet":20,"q":-1.0,"size_kb":227}],"frame":16,"packet":9,"q":13.0,"size_kb":716,"time":"0h0m0.92s","speed":0.306,"dup":0,"drop":0}
[https @ 0x557c840bf480] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8' for reading
[hls @ 0x7f92a96e8100] Skip ('#EXT-X-VERSION:3')
[https @ 0x557c840d1080] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100794.ts' for reading
[https @ 0x557c840f3180] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100795.ts' for reading
ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":21,"packet":24,"size_kb":228}],"outputs":[{"index":0,"stream":0,"frame":20,"packet":20,"q":-1.0,"size_kb":562},{"index":1,"stream":0,"frame":24,"packet":24,"q":-1.0,"size_kb":228}],"frame":20,"packet":20,"q":-1.0,"size_kb":789,"time":"0h0m1.8s","speed":0.245,"dup":0,"drop":0}`

	data := strings.Split(rawdata, "\n")

	for _, d := range data {
		parser.Parse(d)
	}

	require.Equal(t, 1, len(parser.process.input), "expected 1 input")
	require.Equal(t, 2, len(parser.process.output), "expected 2 outputs")
}

func TestParserProgressPlayout(t *testing.T) {
	parser := New(Config{
		LogLines: 20,
	}).(*parser)

	parser.Parse(`ffmpeg.inputs:[{"url":"playout:https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk.m3u8","format":"playout","index":0,"stream":0,"type":"video","codec":"h264","coder":"h264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":20.666666,"pix_fmt":"yuvj420p","width":1280,"height":720}]`)
	parser.Parse(`ffmpeg.outputs:[{"url":"/dev/null","format":"flv","index":0,"stream":0,"type":"video","codec":"h264","coder":"libx264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":25.000000,"pix_fmt":"yuvj420p","width":1280,"height":720},{"url":"/dev/null","format":"mp4","index":1,"stream":0,"type":"video","codec":"h264","coder":"copy","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":20.666666,"pix_fmt":"yuvj420p","width":1280,"height":720}]`)
	parser.Parse(`ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":7,"keyframe":1,"packet":11,"size_kb":226,"size_bytes":42}],"outputs":[{"index":0,"stream":0,"frame":7,"keyframe":1,"packet":0,"q":0.0,"size_kb":0,"size_bytes":5,"extradata_size_bytes":32},{"index":1,"stream":0,"frame":11,"packet":11,"q":-1.0,"size_kb":226}],"frame":7,"packet":0,"q":0.0,"size_kb":226,"time":"0h0m0.56s","speed":0.4,"dup":0,"drop":0}`)
	parser.Parse(`avstream.progress:{"id":"playout:https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk.m3u8","url":"https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk.m3u8","stream":0,"queue":140,"aqueue":0,"dup":0,"drop":0,"enc":0,"looping":false,"duplicating":false,"gop":"none","input":{"state":"running","packet":148,"size_kb":1529,"time":5},"output":{"state":"running","packet":8,"size_kb":128,"time":1},"swap":{"url":"","status":"waiting","lasturl":"","lasterror":""}}`)

	progress := parser.Progress()

	require.Equal(t, Progress{
		Input: []ProgressIO{
			{
				Address:   "playout:https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk.m3u8",
				Index:     0,
				Stream:    0,
				Format:    "playout",
				Type:      "video",
				Codec:     "h264",
				Coder:     "h264",
				Frame:     7,
				Keyframe:  1,
				FPS:       0,
				Packet:    11,
				PPS:       0,
				Size:      42,
				Bitrate:   0,
				Pixfmt:    "yuvj420p",
				Quantizer: 0,
				Width:     1280,
				Height:    720,
				Sampling:  0,
				Layout:    "",
				Channels:  0,
				AVstream: &AVstream{
					Input: AVstreamIO{
						State:  "running",
						Packet: 148,
						Time:   5,
						Size:   1565696,
					},
					Output: AVstreamIO{
						State:  "running",
						Packet: 8,
						Time:   1,
						Size:   131072,
					},
					Aqueue:      0,
					Queue:       140,
					Dup:         0,
					Drop:        0,
					Enc:         0,
					Looping:     false,
					Duplicating: false,
					GOP:         "none",
				},
			},
		},
		Output: []ProgressIO{
			{
				Address:   "/dev/null",
				Index:     0,
				Stream:    0,
				Format:    "flv",
				Type:      "video",
				Codec:     "h264",
				Coder:     "libx264",
				Frame:     7,
				Keyframe:  1,
				FPS:       0,
				Packet:    0,
				PPS:       0,
				Size:      5,
				Bitrate:   0,
				Extradata: 32,
				Pixfmt:    "yuvj420p",
				Quantizer: 0,
				Width:     1280,
				Height:    720,
				Sampling:  0,
				Layout:    "",
				Channels:  0,
				AVstream:  nil,
			},
			{
				Address:   "/dev/null",
				Index:     1,
				Stream:    0,
				Format:    "mp4",
				Type:      "video",
				Codec:     "h264",
				Coder:     "copy",
				Frame:     11,
				FPS:       0,
				Packet:    11,
				PPS:       0,
				Size:      231424,
				Bitrate:   0,
				Pixfmt:    "yuvj420p",
				Quantizer: -1,
				Width:     1280,
				Height:    720,
				Sampling:  0,
				Layout:    "",
				Channels:  0,
				AVstream:  nil,
			},
		},
		Frame:     7,
		Packet:    0,
		FPS:       0,
		PPS:       0,
		Quantizer: 0,
		Size:      231424,
		Time:      0.56,
		Bitrate:   0,
		Speed:     0.4,
		Drop:      0,
		Dup:       0,
	}, progress)
}

func TestParserPatterns(t *testing.T) {
	p := New(Config{
		LogHistory: 3,
		Patterns: []string{
			"^foobar",
			"foobar$",
		},
	})

	p.Parse("some foobar more")
	require.Empty(t, p.Report().Matches)

	p.Parse("foobar some more")
	require.Equal(t, 1, len(p.Report().Matches))
	require.Equal(t, "foobar some more", p.Report().Matches[0])

	p.Parse("some more foobar")
	require.Equal(t, 2, len(p.Report().Matches))
	require.Equal(t, "some more foobar", p.Report().Matches[1])

	pp, ok := p.(*parser)
	require.True(t, ok)

	pp.storeReportHistory("something")

	report := p.ReportHistory()
	require.Equal(t, 1, len(report))
	require.Equal(t, 2, len(report[0].Matches))
	require.Equal(t, "foobar some more", report[0].Matches[0])
	require.Equal(t, "some more foobar", report[0].Matches[1])
}

func TestParserPatternsError(t *testing.T) {
	parser := New(Config{
		Patterns: []string{
			"^foobar",
			"foo(bar$",
		},
	})

	require.Equal(t, 1, len(parser.Report().Matches))
}
