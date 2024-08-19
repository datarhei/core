package parse

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/datarhei/core/v16/process"
	"github.com/stretchr/testify/require"
)

func TestParserProgress(t *testing.T) {
	parser := New(Config{
		LogLines: 20,
	}).(*parser)

	parser.prelude.done = true
	parser.Parse([]byte("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463"))

	d, _ := time.ParseDuration("3m58s440ms")
	wantP := Progress{
		Frame:     5968,
		Quantizer: 19.4,
		Size:      453632,
		Time:      d.Seconds(),
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

	parser.Parse([]byte("prelude"))

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
		parser.Parse([]byte(fmt.Sprintf("prelude %3d", i)))
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
		parser.Parse([]byte(fmt.Sprintf("prelude %3d", i)))
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

	parser.Parse([]byte("bla"))

	log = parser.Log()

	require.Equal(t, 1, len(log))
}

func TestParserLastLogLine(t *testing.T) {
	parser := New(Config{
		LogLines: 20,
	}).(*parser)

	parser.Parse([]byte("foo"))

	line := parser.LastLogline()
	require.Equal(t, "foo", line)

	parser.prelude.done = true
	parser.Parse([]byte("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463"))

	// progress lines are not logged
	line = parser.LastLogline()
	require.Equal(t, "foo", line)

	parser.Parse([]byte("bar"))
	line = parser.LastLogline()
	require.Equal(t, "bar", line)
}

func TestParserLogHistory(t *testing.T) {
	parser := New(Config{
		LogLines:   20,
		LogHistory: 5,
	}).(*parser)

	for i := 0; i < 7; i++ {
		parser.Parse([]byte("bla"))

		parser.prelude.done = true
		parser.Parse([]byte("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463"))

		history := parser.ReportHistory()
		require.Equal(t, int(math.Min(float64(i), 5)), len(history))

		parser.Stop("finished", process.Usage{})
		parser.ResetStats()

		time.Sleep(time.Second)
	}

	history := parser.ReportHistory()
	require.Equal(t, 5, len(history))

	for i := 0; i < 5; i++ {
		require.Equal(t, "finished", history[i].ExitState)
		require.Equal(t, "bla", history[i].Log[0].Data)
		require.Equal(t, "bla", history[i].Prelude[0])

		d, _ := time.ParseDuration("3m58s440ms")
		require.Equal(t, Progress{
			Started:   true,
			Frame:     5968,
			FPS:       0, // is calculated with averager
			Quantizer: 19.4,
			Size:      453632,
			Time:      d.Seconds(),
			Bitrate:   0, // is calculated with averager
			Speed:     0.999,
			Drop:      3522,
			Dup:       87463,
		}, history[i].Progress)

		if i != 0 {
			require.Greater(t, history[i].CreatedAt, history[i-1].ExitedAt)
		}
	}
}

func TestParserImportLogHistory(t *testing.T) {
	parser := New(Config{
		LogLines:   20,
		LogHistory: 5,
	}).(*parser)

	for i := 0; i < 7; i++ {
		parser.Parse([]byte("bla"))

		parser.prelude.done = true
		parser.Parse([]byte("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463"))

		history := parser.ReportHistory()
		require.Equal(t, int(math.Min(float64(i), 5)), len(history))

		parser.Stop("finished", process.Usage{})
		parser.ResetStats()

		time.Sleep(time.Second)
	}

	history := parser.ReportHistory()

	for i, h := range history {
		h.Prelude[0] = "blubb"
		h.ExitState = "nothing"
		h.Progress.Frame = 42

		history[i] = h
	}

	parser.ImportReportHistory(history[:3])

	history = parser.ReportHistory()
	require.Equal(t, 3, len(history))

	for i := 0; i < 3; i++ {
		require.Equal(t, "nothing", history[i].ExitState)
		require.Equal(t, "bla", history[i].Log[0].Data)
		require.Equal(t, "blubb", history[i].Prelude[0])

		d, _ := time.ParseDuration("3m58s440ms")
		require.Equal(t, Progress{
			Started:   true,
			Frame:     42,
			FPS:       0, // is calculated with averager
			Quantizer: 19.4,
			Size:      453632,
			Time:      d.Seconds(),
			Bitrate:   0, // is calculated with averager
			Speed:     0.999,
			Drop:      3522,
			Dup:       87463,
		}, history[i].Progress)

		if i != 0 {
			require.Greater(t, history[i].CreatedAt, history[i-1].ExitedAt)
		}
	}
}

func TestParserLogHistoryLength(t *testing.T) {
	parser := New(Config{
		LogLines:   20,
		LogHistory: 3,
	}).(*parser)

	history := parser.ReportHistory()
	require.Equal(t, 0, len(history))

	for i := 0; i < 5; i++ {
		parser.Parse([]byte("bla"))

		parser.prelude.done = true
		parser.Parse([]byte("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463"))

		parser.Stop("finished", process.Usage{})
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

	for i := 0; i < 42; i++ {
		parser.Parse([]byte("bla"))

		parser.prelude.done = true
		parser.Parse([]byte("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463"))

		parser.Stop("finished", process.Usage{})
	}

	history = parser.ReportHistory()
	require.Equal(t, 13, len(history))

	for i := 0; i < 10; i++ {
		require.Empty(t, history[i].Log, i)

		d, _ := time.ParseDuration("3m58s440ms")
		require.Equal(t, Progress{
			Started:   true,
			Frame:     5968,
			FPS:       0, // is calculated with averager
			Quantizer: 19.4,
			Size:      453632,
			Time:      d.Seconds(),
			Bitrate:   0, // is calculated with averager
			Speed:     0.999,
			Drop:      3522,
			Dup:       87463,
		}, history[i].Progress)
	}

	for i := 10; i < 13; i++ {
		require.NotEmpty(t, history[i].Log, i)

		d, _ := time.ParseDuration("3m58s440ms")
		require.Equal(t, Progress{
			Started:   true,
			Frame:     5968,
			FPS:       0, // is calculated with averager
			Quantizer: 19.4,
			Size:      453632,
			Time:      d.Seconds(),
			Bitrate:   0, // is calculated with averager
			Speed:     0.999,
			Drop:      3522,
			Dup:       87463,
		}, history[i].Progress)
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
		parser.Parse([]byte("bla"))

		parser.prelude.done = true
		parser.Parse([]byte("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463"))

		parser.Stop("finished", process.Usage{})
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

	parser.Parse([]byte("foo"))

	parser.prelude.done = true
	parser.Parse([]byte("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463"))

	parser.Stop("finished", process.Usage{})

	parser.ResetStats()

	time.Sleep(2 * time.Second)

	parser.ResetLog()

	parser.Parse([]byte("bar"))

	parser.prelude.done = true
	parser.Parse([]byte("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463"))

	parser.Stop("finished", process.Usage{})

	parser.ResetStats()

	time.Sleep(2 * time.Second)

	parser.ResetLog()

	parser.Parse([]byte("foobar"))

	parser.prelude.done = true
	parser.Parse([]byte("frame= 5968 fps= 25 q=19.4 size=443kB time=00:03:58.44 bitrate=5632kbits/s speed=0.999x skip=9733 drop=3522 dup=87463"))

	parser.Stop("failed", process.Usage{})

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

	parser.Parse([]byte("prelude"))

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
		parser.Parse([]byte(d))
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
		parser.Parse([]byte(d))
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
		parser.Parse([]byte(d))
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
		parser.Parse([]byte(d))
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
		parser.Parse([]byte(d))
	}

	require.Equal(t, 1, len(parser.process.input), "expected 1 input")
	require.Equal(t, 2, len(parser.process.output), "expected 2 outputs")
}

func TestParserProgressPlayout(t *testing.T) {
	parser := New(Config{
		LogLines: 20,
	}).(*parser)

	parser.Parse([]byte(`ffmpeg.inputs:[{"url":"playout:https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk.m3u8","format":"playout","index":0,"stream":0,"type":"video","codec":"h264","coder":"h264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":20.666666,"pix_fmt":"yuvj420p","width":1280,"height":720}]`))
	parser.Parse([]byte(`ffmpeg.outputs:[{"url":"/dev/null","format":"flv","index":0,"stream":0,"type":"video","codec":"h264","coder":"libx264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":25.000000,"pix_fmt":"yuvj420p","width":1280,"height":720},{"url":"/dev/null","format":"mp4","index":1,"stream":0,"type":"video","codec":"h264","coder":"copy","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":20.666666,"pix_fmt":"yuvj420p","width":1280,"height":720}]`))
	parser.Parse([]byte(`ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"frame":7,"keyframe":1,"packet":11,"size_kb":226,"size_bytes":42}],"outputs":[{"index":0,"stream":0,"frame":7,"keyframe":1,"packet":0,"q":0.0,"size_kb":0,"size_bytes":5,"extradata_size_bytes":32},{"index":1,"stream":0,"frame":11,"packet":11,"q":-1.0,"size_kb":226}],"frame":7,"packet":0,"q":0.0,"size_kb":226,"time":"0h0m0.56s","speed":0.4,"dup":0,"drop":0}`))
	parser.Parse([]byte(`avstream.progress:{"id":"playout:https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk.m3u8","url":"https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk.m3u8","stream":0,"queue":140,"aqueue":42,"dup":5,"drop":8,"enc":7,"looping":true,"duplicating":true,"gop":"key","mode":"live","input":{"state":"running","packet":148,"size_kb":1529,"time":5},"output":{"state":"running","packet":8,"size_kb":128,"time":1},"swap":{"url":"","status":"waiting","lasturl":"","lasterror":""}}`))

	progress := parser.Progress()

	require.Equal(t, Progress{
		Started: true,
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
					Aqueue:      42,
					Queue:       140,
					Dup:         5,
					Drop:        8,
					Enc:         7,
					Looping:     true,
					Duplicating: true,
					GOP:         "key",
					Mode:        "live",
					Swap: AVStreamSwap{
						URL:       "",
						Status:    "waiting",
						LastURL:   "",
						LastError: "",
					},
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

func TestParserStreamMapping(t *testing.T) {
	parser := New(Config{
		LogLines: 20,
	}).(*parser)

	parser.Parse([]byte(`ffmpeg.inputs:[{"url":"https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8","format":"hls","index":0,"stream":0,"type":"video","codec":"h264","coder":"h264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":25.000000,"pix_fmt":"yuvj420p","width":1280,"height":720},{"url":"https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk_1440.m3u8","format":"hls","index":1,"stream":0,"type":"video","codec":"h264","coder":"h264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":25.000000,"pix_fmt":"yuvj420p","width":2560,"height":1440},{"url":"anullsrc=r=44100:cl=mono","format":"lavfi","index":2,"stream":0,"type":"audio","codec":"pcm_u8","coder":"pcm_u8","bitrate_kbps":352,"duration_sec":0.000000,"language":"und","sampling_hz":44100,"layout":"mono","channels":1}]`))
	parser.Parse([]byte(`hls.streammap:{"address":"http://127.0.0.1:8080/memfs/live/%v.m3u8","variants":[{"variant":0,"address":"http://127.0.0.1:8080/memfs/live/0.m3u8","streams":[0,2]},{"variant":1,"address":"http://127.0.0.1:8080/memfs/live/1.m3u8","streams":[1,3]}]}`))
	parser.Parse([]byte(`ffmpeg.outputs:[{"url":"http://127.0.0.1:8080/memfs/live/%v.m3u8","format":"hls","index":0,"stream":0,"type":"video","codec":"h264","coder":"copy","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":25.000000,"pix_fmt":"yuvj420p","width":1280,"height":720},{"url":"http://127.0.0.1:8080/memfs/live/%v.m3u8","format":"hls","index":0,"stream":1,"type":"video","codec":"h264","coder":"copy","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":25.000000,"pix_fmt":"yuvj420p","width":2560,"height":1440},{"url":"http://127.0.0.1:8080/memfs/live/%v.m3u8","format":"hls","index":0,"stream":2,"type":"audio","codec":"aac","coder":"aac","bitrate_kbps":69,"duration_sec":0.000000,"language":"und","sampling_hz":44100,"layout":"mono","channels":1},{"url":"http://127.0.0.1:8080/memfs/live/%v.m3u8","format":"hls","index":0,"stream":3,"type":"audio","codec":"aac","coder":"aac","bitrate_kbps":69,"duration_sec":0.000000,"language":"und","sampling_hz":44100,"layout":"mono","channels":1}]`))
	parser.Parse([]byte(`ffmpeg.mapping:{"graphs":[{"index":0,"graph":[{"src_name":"Parsed_null_0","src_filter":"null","dst_name":"format","dst_filter":"format","inpad":"default","outpad":"default","timebase": "1/90000","type":"video","format":"yuvj420p","width":1280,"height":720},{"src_name":"graph 0 input from stream 0:0","src_filter":"buffer","dst_name":"Parsed_null_0","dst_filter":"null","inpad":"default","outpad":"default","timebase": "1/90000","type":"video","format":"yuvj420p","width":1280,"height":720},{"src_name":"format","src_filter":"format","dst_name":"out_0_0","dst_filter":"buffersink","inpad":"default","outpad":"default","timebase": "1/90000","type":"video","format":"yuvj420p","width":1280,"height":720}]},{"index":1,"graph":[{"src_name":"Parsed_anull_0","src_filter":"anull","dst_name":"auto_aresample_0","dst_filter":"aresample","inpad":"default","outpad":"default","timebase": "1/44100","type":"audio","format":"u8","sampling_hz":44100,"layout":"mono"},{"src_name":"graph_1_in_2_0","src_filter":"abuffer","dst_name":"Parsed_anull_0","dst_filter":"anull","inpad":"default","outpad":"default","timebase": "1/44100","type":"audio","format":"u8","sampling_hz":44100,"layout":"mono"},{"src_name":"format_out_0_2","src_filter":"aformat","dst_name":"out_0_2","dst_filter":"abuffersink","inpad":"default","outpad":"default","timebase": "1/44100","type":"audio","format":"fltp","sampling_hz":44100,"layout":"mono"},{"src_name":"auto_aresample_0","src_filter":"aresample","dst_name":"format_out_0_2","dst_filter":"aformat","inpad":"default","outpad":"default","timebase": "1/44100","type":"audio","format":"fltp","sampling_hz":44100,"layout":"mono"}]},{"index":2,"graph":[{"src_name":"Parsed_anull_0","src_filter":"anull","dst_name":"auto_aresample_0","dst_filter":"aresample","inpad":"default","outpad":"default","timebase": "1/44100","type":"audio","format":"u8","sampling_hz":44100,"layout":"mono"},{"src_name":"graph_2_in_2_0","src_filter":"abuffer","dst_name":"Parsed_anull_0","dst_filter":"anull","inpad":"default","outpad":"default","timebase": "1/44100","type":"audio","format":"u8","sampling_hz":44100,"layout":"mono"},{"src_name":"format_out_0_3","src_filter":"aformat","dst_name":"out_0_3","dst_filter":"abuffersink","inpad":"default","outpad":"default","timebase": "1/44100","type":"audio","format":"fltp","sampling_hz":44100,"layout":"mono"},{"src_name":"auto_aresample_0","src_filter":"aresample","dst_name":"format_out_0_3","dst_filter":"aformat","inpad":"default","outpad":"default","timebase": "1/44100","type":"audio","format":"fltp","sampling_hz":44100,"layout":"mono"}]}],"mapping":[{"input":{"index":0,"stream":0},"graph":{"index":0,"name":"graph 0 input from stream 0:0"},"output":null},{"input":{"index":2,"stream":0},"graph":{"index":1,"name":"graph_1_in_2_0"},"output":null},{"input":{"index":2,"stream":0},"graph":{"index":2,"name":"graph_2_in_2_0"},"output":null},{"input":null,"graph":{"index":0,"name":"out_0_0"},"output":{"index":0,"stream":0}},{"input":{"index":1,"stream":0},"output":{"index":0,"stream":1},"copy":true},{"input":null,"graph":{"index":1,"name":"out_0_2"},"output":{"index":0,"stream":2}},{"input":null,"graph":{"index":2,"name":"out_0_3"},"output":{"index":0,"stream":3}}]}`))
	parser.Parse([]byte(`ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"framerate":{"min":24.975,"max":24.975,"avg":24.975},"frame":149,"keyframe":3,"packet":149,"size_kb":1467,"size_bytes":1501854},{"index":1,"stream":0,"framerate":{"min":24.975,"max":24.975,"avg":24.975},"frame":149,"keyframe":3,"packet":149,"size_kb":4428,"size_bytes":4534541},{"index":2,"stream":0,"framerate":{"min":43.066,"max":43.068,"avg":43.066},"frame":257,"keyframe":257,"packet":257,"size_kb":257,"size_bytes":263168}],"outputs":[{"index":0,"stream":0,"frame":149,"keyframe":3,"packet":149,"q":-1.0,"size_kb":1467,"size_bytes":1501923,"extradata_size_bytes":69},{"index":0,"stream":1,"frame":149,"keyframe":3,"packet":149,"q":-1.0,"size_kb":4428,"size_bytes":4534612,"extradata_size_bytes":71},{"index":0,"stream":2,"frame":257,"keyframe":256,"packet":256,"size_kb":1,"size_bytes":1046,"extradata_size_bytes":5},{"index":0,"stream":3,"frame":257,"keyframe":256,"packet":256,"size_kb":1,"size_bytes":1046,"extradata_size_bytes":5}],"frame":149,"packet":149,"q":-1.0,"size_kb":5897,"size_bytes":6038627,"time":"0h0m5.96s","speed":4.79,"dup":0,"drop":0}`))

	progress := parser.Progress()

	require.Equal(t, 3, len(progress.Input))
	require.Equal(t, 4, len(progress.Output))

	require.Equal(t, StreamMapping{
		Graphs: []GraphElement{
			{Index: 0, Name: "Parsed_null_0", Filter: "null", DstName: "format", DstFilter: "format", Inpad: "default", Outpad: "default", Timebase: "1/90000", Type: "video", Format: "yuvj420p", Sampling: 0, Layout: "", Width: 1280, Height: 720},
			{Index: 0, Name: "graph 0 input from stream 0:0", Filter: "buffer", DstName: "Parsed_null_0", DstFilter: "null", Inpad: "default", Outpad: "default", Timebase: "1/90000", Type: "video", Format: "yuvj420p", Sampling: 0, Layout: "", Width: 1280, Height: 720},
			{Index: 0, Name: "format", Filter: "format", DstName: "out_0_0", DstFilter: "buffersink", Inpad: "default", Outpad: "default", Timebase: "1/90000", Type: "video", Format: "yuvj420p", Sampling: 0, Layout: "", Width: 1280, Height: 720},
			{Index: 1, Name: "Parsed_anull_0", Filter: "anull", DstName: "auto_aresample_0", DstFilter: "aresample", Inpad: "default", Outpad: "default", Timebase: "1/44100", Type: "audio", Format: "u8", Sampling: 44100, Layout: "mono", Width: 0, Height: 0},
			{Index: 1, Name: "graph_1_in_2_0", Filter: "abuffer", DstName: "Parsed_anull_0", DstFilter: "anull", Inpad: "default", Outpad: "default", Timebase: "1/44100", Type: "audio", Format: "u8", Sampling: 44100, Layout: "mono", Width: 0, Height: 0},
			{Index: 1, Name: "format_out_0_2", Filter: "aformat", DstName: "out_0_2", DstFilter: "abuffersink", Inpad: "default", Outpad: "default", Timebase: "1/44100", Type: "audio", Format: "fltp", Sampling: 44100, Layout: "mono", Width: 0, Height: 0},
			{Index: 1, Name: "auto_aresample_0", Filter: "aresample", DstName: "format_out_0_2", DstFilter: "aformat", Inpad: "default", Outpad: "default", Timebase: "1/44100", Type: "audio", Format: "fltp", Sampling: 44100, Layout: "mono", Width: 0, Height: 0},
			{Index: 2, Name: "Parsed_anull_0", Filter: "anull", DstName: "auto_aresample_0", DstFilter: "aresample", Inpad: "default", Outpad: "default", Timebase: "1/44100", Type: "audio", Format: "u8", Sampling: 44100, Layout: "mono", Width: 0, Height: 0},
			{Index: 2, Name: "graph_2_in_2_0", Filter: "abuffer", DstName: "Parsed_anull_0", DstFilter: "anull", Inpad: "default", Outpad: "default", Timebase: "1/44100", Type: "audio", Format: "u8", Sampling: 44100, Layout: "mono", Width: 0, Height: 0},
			{Index: 2, Name: "format_out_0_3", Filter: "aformat", DstName: "out_0_3", DstFilter: "abuffersink", Inpad: "default", Outpad: "default", Timebase: "1/44100", Type: "audio", Format: "fltp", Sampling: 44100, Layout: "mono", Width: 0, Height: 0},
			{Index: 2, Name: "auto_aresample_0", Filter: "aresample", DstName: "format_out_0_3", DstFilter: "aformat", Inpad: "default", Outpad: "default", Timebase: "1/44100", Type: "audio", Format: "fltp", Sampling: 44100, Layout: "mono", Width: 0, Height: 0},
		},
		Mapping: []GraphMapping{
			{
				Input:  0,
				Output: -1,
				Index:  0,
				Name:   "graph 0 input from stream 0:0",
				Copy:   false,
			},
			{
				Input:  2,
				Output: -1,
				Index:  1,
				Name:   "graph_1_in_2_0",
				Copy:   false,
			},
			{
				Input:  2,
				Output: -1,
				Index:  2,
				Name:   "graph_2_in_2_0",
				Copy:   false,
			},
			{
				Input:  -1,
				Output: 0,
				Index:  0,
				Name:   "out_0_0",
				Copy:   false,
			},
			{
				Input:  1,
				Output: 1,
				Index:  -1,
				Name:   "",
				Copy:   true,
			},
			{
				Input:  -1,
				Output: 2,
				Index:  1,
				Name:   "out_0_2",
				Copy:   false,
			},
			{
				Input:  -1,
				Output: 3,
				Index:  2,
				Name:   "out_0_3",
				Copy:   false,
			},
		},
	}, progress.Mapping)

	require.Equal(t, "http://127.0.0.1:8080/memfs/live/0.m3u8", progress.Output[0].Address)
	require.Equal(t, uint64(0), progress.Output[0].Index)
	require.Equal(t, uint64(0), progress.Output[0].Stream)

	require.Equal(t, "http://127.0.0.1:8080/memfs/live/1.m3u8", progress.Output[1].Address)
	require.Equal(t, uint64(1), progress.Output[1].Index)
	require.Equal(t, uint64(0), progress.Output[1].Stream)

	require.Equal(t, "http://127.0.0.1:8080/memfs/live/0.m3u8", progress.Output[2].Address)
	require.Equal(t, uint64(0), progress.Output[2].Index)
	require.Equal(t, uint64(1), progress.Output[2].Stream)

	require.Equal(t, "http://127.0.0.1:8080/memfs/live/1.m3u8", progress.Output[3].Address)
	require.Equal(t, uint64(1), progress.Output[3].Index)
	require.Equal(t, uint64(1), progress.Output[3].Stream)
}

func TestParserHLSMapping(t *testing.T) {
	parser := New(Config{
		LogLines: 20,
	}).(*parser)

	parser.Parse([]byte(`ffmpeg.inputs:[{"url":"https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8","format":"hls","index":0,"stream":0,"type":"video","codec":"h264","coder":"h264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":25.000000,"pix_fmt":"yuvj420p","width":1280,"height":720},{"url":"https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk_1440.m3u8","format":"hls","index":1,"stream":0,"type":"video","codec":"h264","coder":"h264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":25.000000,"pix_fmt":"yuvj420p","width":2560,"height":1440},{"url":"anullsrc=r=44100:cl=mono","format":"lavfi","index":2,"stream":0,"type":"audio","codec":"pcm_u8","coder":"pcm_u8","bitrate_kbps":352,"duration_sec":0.000000,"language":"und","sampling_hz":44100,"layout":"mono","channels":1}]`))
	parser.Parse([]byte(`hls.streammap:{"address":"http://127.0.0.1:8080/memfs/live/%v.m3u8","variants":[{"variant":0,"address":"http://127.0.0.1:8080/memfs/live/0.m3u8","streams":[0,2]},{"variant":1,"address":"http://127.0.0.1:8080/memfs/live/1.m3u8","streams":[1,3]}]}`))
	parser.Parse([]byte(`ffmpeg.outputs:[{"url":"http://127.0.0.1:8080/memfs/live/%v.m3u8","format":"hls","index":0,"stream":0,"type":"video","codec":"h264","coder":"copy","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":25.000000,"pix_fmt":"yuvj420p","width":1280,"height":720},{"url":"http://127.0.0.1:8080/memfs/live/%v.m3u8","format":"hls","index":0,"stream":1,"type":"video","codec":"h264","coder":"copy","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":25.000000,"pix_fmt":"yuvj420p","width":2560,"height":1440},{"url":"http://127.0.0.1:8080/memfs/live/%v.m3u8","format":"hls","index":0,"stream":2,"type":"audio","codec":"aac","coder":"aac","bitrate_kbps":69,"duration_sec":0.000000,"language":"und","sampling_hz":44100,"layout":"mono","channels":1},{"url":"http://127.0.0.1:8080/memfs/live/%v.m3u8","format":"hls","index":0,"stream":3,"type":"audio","codec":"aac","coder":"aac","bitrate_kbps":69,"duration_sec":0.000000,"language":"und","sampling_hz":44100,"layout":"mono","channels":1}]`))
	parser.Parse([]byte(`ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"framerate":{"min":24.975,"max":24.975,"avg":24.975},"frame":149,"keyframe":3,"packet":149,"size_kb":1467,"size_bytes":1501854},{"index":1,"stream":0,"framerate":{"min":24.975,"max":24.975,"avg":24.975},"frame":149,"keyframe":3,"packet":149,"size_kb":4428,"size_bytes":4534541},{"index":2,"stream":0,"framerate":{"min":43.066,"max":43.068,"avg":43.066},"frame":257,"keyframe":257,"packet":257,"size_kb":257,"size_bytes":263168}],"outputs":[{"index":0,"stream":0,"frame":149,"keyframe":3,"packet":149,"q":-1.0,"size_kb":1467,"size_bytes":1501923,"extradata_size_bytes":69},{"index":0,"stream":1,"frame":149,"keyframe":3,"packet":149,"q":-1.0,"size_kb":4428,"size_bytes":4534612,"extradata_size_bytes":71},{"index":0,"stream":2,"frame":257,"keyframe":256,"packet":256,"size_kb":1,"size_bytes":1046,"extradata_size_bytes":5},{"index":0,"stream":3,"frame":257,"keyframe":256,"packet":256,"size_kb":1,"size_bytes":1046,"extradata_size_bytes":5}],"frame":149,"packet":149,"q":-1.0,"size_kb":5897,"size_bytes":6038627,"time":"0h0m5.96s","speed":4.79,"dup":0,"drop":0}`))

	progress := parser.Progress()

	require.Equal(t, 3, len(progress.Input))
	require.Equal(t, 4, len(progress.Output))

	require.Equal(t, "http://127.0.0.1:8080/memfs/live/0.m3u8", progress.Output[0].Address)
	require.Equal(t, uint64(0), progress.Output[0].Index)
	require.Equal(t, uint64(0), progress.Output[0].Stream)

	require.Equal(t, "http://127.0.0.1:8080/memfs/live/1.m3u8", progress.Output[1].Address)
	require.Equal(t, uint64(1), progress.Output[1].Index)
	require.Equal(t, uint64(0), progress.Output[1].Stream)

	require.Equal(t, "http://127.0.0.1:8080/memfs/live/0.m3u8", progress.Output[2].Address)
	require.Equal(t, uint64(0), progress.Output[2].Index)
	require.Equal(t, uint64(1), progress.Output[2].Stream)

	require.Equal(t, "http://127.0.0.1:8080/memfs/live/1.m3u8", progress.Output[3].Address)
	require.Equal(t, uint64(1), progress.Output[3].Index)
	require.Equal(t, uint64(1), progress.Output[3].Stream)
}

func TestParserPatterns(t *testing.T) {
	p := New(Config{
		LogHistory: 3,
		Patterns: []string{
			"^foobar",
			"foobar$",
		},
	})

	p.Parse([]byte("some foobar more"))
	require.Empty(t, p.Report().Matches)

	p.Parse([]byte("foobar some more"))
	require.Equal(t, 1, len(p.Report().Matches))
	require.Equal(t, "foobar some more", p.Report().Matches[0])

	p.Parse([]byte("some more foobar"))
	require.Equal(t, 2, len(p.Report().Matches))
	require.Equal(t, "some more foobar", p.Report().Matches[1])

	pp, ok := p.(*parser)
	require.True(t, ok)

	pp.storeReportHistory("something", Usage{})

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

func BenchmarkParserString(b *testing.B) {
	parser := New(Config{
		LogLines: 100,
	})

	parser.Parse([]byte(`ffmpeg.inputs:[{"url":"https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk_720.m3u8","format":"hls","index":0,"stream":0,"type":"video","codec":"h264","coder":"h264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":25.000000,"pix_fmt":"yuvj420p","width":1280,"height":720},{"url":"https://cdn.livespotting.com/vpu/e9slfpe3/z60wzayk_1440.m3u8","format":"hls","index":1,"stream":0,"type":"video","codec":"h264","coder":"h264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":25.000000,"pix_fmt":"yuvj420p","width":2560,"height":1440},{"url":"anullsrc=r=44100:cl=mono","format":"lavfi","index":2,"stream":0,"type":"audio","codec":"pcm_u8","coder":"pcm_u8","bitrate_kbps":352,"duration_sec":0.000000,"language":"und","sampling_hz":44100,"layout":"mono","channels":1}]`))
	parser.Parse([]byte(`hls.streammap:{"address":"http://127.0.0.1:8080/memfs/live/%v.m3u8","variants":[{"variant":0,"address":"http://127.0.0.1:8080/memfs/live/0.m3u8","streams":[0,2]},{"variant":1,"address":"http://127.0.0.1:8080/memfs/live/1.m3u8","streams":[1,3]}]}`))
	parser.Parse([]byte(`ffmpeg.outputs:[{"url":"http://127.0.0.1:8080/memfs/live/%v.m3u8","format":"hls","index":0,"stream":0,"type":"video","codec":"h264","coder":"copy","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":25.000000,"pix_fmt":"yuvj420p","width":1280,"height":720},{"url":"http://127.0.0.1:8080/memfs/live/%v.m3u8","format":"hls","index":0,"stream":1,"type":"video","codec":"h264","coder":"copy","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","fps":25.000000,"pix_fmt":"yuvj420p","width":2560,"height":1440},{"url":"http://127.0.0.1:8080/memfs/live/%v.m3u8","format":"hls","index":0,"stream":2,"type":"audio","codec":"aac","coder":"aac","bitrate_kbps":69,"duration_sec":0.000000,"language":"und","sampling_hz":44100,"layout":"mono","channels":1},{"url":"http://127.0.0.1:8080/memfs/live/%v.m3u8","format":"hls","index":0,"stream":3,"type":"audio","codec":"aac","coder":"aac","bitrate_kbps":69,"duration_sec":0.000000,"language":"und","sampling_hz":44100,"layout":"mono","channels":1}]`))

	data := [][]byte{
		[]byte(`ffmpeg.progress:{"inputs":[{"index":0,"stream":0,"framerate":{"min":24.975,"max":24.975,"avg":24.975},"frame":149,"keyframe":3,"packet":149,"size_kb":1467,"size_bytes":1501854},{"index":1,"stream":0,"framerate":{"min":24.975,"max":24.975,"avg":24.975},"frame":149,"keyframe":3,"packet":149,"size_kb":4428,"size_bytes":4534541},{"index":2,"stream":0,"framerate":{"min":43.066,"max":43.068,"avg":43.066},"frame":257,"keyframe":257,"packet":257,"size_kb":257,"size_bytes":263168}],"outputs":[{"index":0,"stream":0,"frame":149,"keyframe":3,"packet":149,"q":-1.0,"size_kb":1467,"size_bytes":1501923,"extradata_size_bytes":69},{"index":0,"stream":1,"frame":149,"keyframe":3,"packet":149,"q":-1.0,"size_kb":4428,"size_bytes":4534612,"extradata_size_bytes":71},{"index":0,"stream":2,"frame":257,"keyframe":256,"packet":256,"size_kb":1,"size_bytes":1046,"extradata_size_bytes":5},{"index":0,"stream":3,"frame":257,"keyframe":256,"packet":256,"size_kb":1,"size_bytes":1046,"extradata_size_bytes":5}],"frame":149,"packet":149,"q":-1.0,"size_kb":5897,"size_bytes":6038627,"time":"0h0m5.96s","speed":4.79,"dup":0,"drop":0}`),
		[]byte(`[https @ 0x557c840d1080] Opening 'https://ch-fra-n16.livespotting.com/vpu/e9slfpe3/z60wzayk_720_100794.ts' for reading`),
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parser.Parse(data[0])
		parser.Parse(data[1])
	}
}
