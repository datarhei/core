package probe

import (
	"strings"
	"testing"
)

func TestProber(t *testing.T) {
	prober := New(Config{}).(*prober)

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
Input #2, playout, from 'playout:rtmp://l5gn74l5-vpu.livespotting.com/live/0chl6hu7_360?token=m5ZuiCQYRlIon8':
  Duration: N/A, start: 0.000000, bitrate: 265 kb/s
    Stream #2:0: Video: h264 (Constrained Baseline), yuvj420p(pc, progressive), 640x360 [SAR 1:1 DAR 16:9], 265 kb/s, 10 fps, 10 tbr, 1000k tbn, 20 tbc
Input #3, mov,mp4,m4a,3gp,3g2,mj2, from 'movie.mp4':
  Metadata:
    major_brand     : isom
    minor_version   : 512
    compatible_brands: isomiso2avc1mp41
    encoder         : Lavf58.20.100
  Duration: 00:01:02.28, start: 0.000000, bitrate: 5895 kb/s
    Stream #3:0(eng): Video: h264 (Main) (avc1 / 0x31637661), yuvj420p(pc, bt709), 2560x1440 [SAR 1:1 DAR 16:9], 5894 kb/s, 23.98 fps, 25 tbr, 90k tbn, 50 tbc (default)
    Stream #3:1(por): Subtitle: subrip
Stream mapping:
  Stream #0:0 -> #0:0 (rawvideo (native) -> h264 (libx264))
  Stream #1:0 -> #0:1 (pcm_u8 (native) -> aac (native))
Press [q] to stop, [?] for help`

	data := strings.Split(rawdata, "\n")

	for _, d := range data {
		prober.Parse(d)
	}

	prober.ResetStats()

	if len(prober.inputs) != 5 {
		t.Errorf("#inputs: want=5, have=%d\n", len(prober.inputs))
		return
	}

	i := prober.inputs[0]

	if i.Address != "testsrc=size=1280x720:rate=25" {
		t.Errorf("#input0.address: want=testsrc=size=1280x720:rate=25, have=%s\n", i.Address)
	}

	if i.Format != "lavfi" {
		t.Errorf("#input0.format: want=lavfi, have=%s\n", i.Format)
	}

	if i.Index != 0 {
		t.Errorf("#input0.index: want=0, have=%d\n", i.Index)
	}

	if i.Stream != 0 {
		t.Errorf("#input0.stream: want=0, have=%d\n", i.Stream)
	}

	if i.Language != "und" {
		t.Errorf("#input0.language: want=und, have=%s\n", i.Language)
	}

	if i.Type != "video" {
		t.Errorf("#input0.type: want=video, have=%s\n", i.Type)
	}

	if i.Codec != "rawvideo" {
		t.Errorf("#input0.codec: want=rawvideo, have=%s\n", i.Codec)
	}

	if i.Bitrate != 0 {
		t.Errorf("#input0.bitrate: want=0, have=%f\n", i.Bitrate)
	}

	if i.Duration != 0 {
		t.Errorf("#input0.duration: want=0, have=%f\n", i.Duration)
	}

	if i.FPS != 0 {
		t.Errorf("#input0.fps: want=0, have=%f\n", i.FPS)
	}

	if i.Pixfmt != "rgb24" {
		t.Errorf("#input0.pixfmt: want=rgb24, have=%s\n", i.Pixfmt)
	}

	if i.Width != 1280 {
		t.Errorf("#input0.width: want=1280, have=%d\n", i.Width)
	}

	if i.Height != 720 {
		t.Errorf("#input0.height: want=720, have=%d\n", i.Height)
	}

	i = prober.inputs[1]

	if i.Address != "anullsrc=r=44100:cl=stereo" {
		t.Errorf("#input1.address: want=anullsrc=r=44100:cl=stereo, have=%s\n", i.Address)
	}

	if i.Format != "lavfi" {
		t.Errorf("#input1.format: want=lavfi, have=%s\n", i.Format)
	}

	if i.Index != 1 {
		t.Errorf("#input1.index: want=1, have=%d\n", i.Index)
	}

	if i.Stream != 0 {
		t.Errorf("#input1.stream: want=0, have=%d\n", i.Stream)
	}

	if i.Language != "und" {
		t.Errorf("#input1.language: want=und, have=%s\n", i.Language)
	}

	if i.Type != "audio" {
		t.Errorf("#input1.type: want=audio, have=%s\n", i.Type)
	}

	if i.Codec != "pcm_u8" {
		t.Errorf("#input1.codec: want=pcm_u8, have=%s\n", i.Codec)
	}

	if i.Bitrate != 705 {
		t.Errorf("#input1.bitrate: want=705, have=%f\n", i.Bitrate)
	}

	if i.Duration != 0 {
		t.Errorf("#input1.duration: want=0, have=%f\n", i.Duration)
	}

	if i.Sampling != 44100 {
		t.Errorf("#input1.sampling: want=44100, have=%d\n", i.Sampling)
	}

	if i.Layout != "stereo" {
		t.Errorf("#input1.layout: want=stereo, have=%s\n", i.Layout)
	}

	i = prober.inputs[2]

	if i.Address != "playout:rtmp://l5gn74l5-vpu.livespotting.com/live/0chl6hu7_360?token=m5ZuiCQYRlIon8" {
		t.Errorf("#input2.address: want=playout:rtmp://l5gn74l5-vpu.livespotting.com/live/0chl6hu7_360?token=m5ZuiCQYRlIon8, have=%s\n", i.Address)
	}

	if i.Format != "playout" {
		t.Errorf("#input2.format: want=playout, have=%s\n", i.Format)
	}

	if i.Index != 2 {
		t.Errorf("#input2.index: want=2, have=%d\n", i.Index)
	}

	if i.Stream != 0 {
		t.Errorf("#input2.stream: want=0, have=%d\n", i.Stream)
	}

	if i.Language != "und" {
		t.Errorf("#input2.language: want=und, have=%s\n", i.Language)
	}

	if i.Type != "video" {
		t.Errorf("#input2.type: want=video, have=%s\n", i.Type)
	}

	if i.Codec != "h264" {
		t.Errorf("#input2.codec: want=h264, have=%s\n", i.Codec)
	}

	if i.Bitrate != 265 {
		t.Errorf("#input2.bitrate: want=265, have=%f\n", i.Bitrate)
	}

	if i.Duration != 0 {
		t.Errorf("#input2.duration: want=0, have=%f\n", i.Duration)
	}

	if i.FPS != 10 {
		t.Errorf("#input2.fps: want=10, have=%f\n", i.FPS)
	}

	if i.Pixfmt != "yuvj420p" {
		t.Errorf("#input2.pixfmt: want=yuvj420p, have=%s\n", i.Pixfmt)
	}

	if i.Width != 640 {
		t.Errorf("#input2.width: want=640, have=%d\n", i.Width)
	}

	if i.Height != 360 {
		t.Errorf("#input2.height: want=360, have=%d\n", i.Height)
	}

	i = prober.inputs[3]

	if i.Address != "movie.mp4" {
		t.Errorf("#input3.address: want=movie.mp4, have=%s\n", i.Address)
	}

	if i.Format != "mov,mp4,m4a,3gp,3g2,mj2" {
		t.Errorf("#input3.format: want=mov,mp4,m4a,3gp,3g2,mj2, have=%s\n", i.Format)
	}

	if i.Index != 3 {
		t.Errorf("#input3.index: want=3, have=%d\n", i.Index)
	}

	if i.Stream != 0 {
		t.Errorf("#input3.stream: want=0, have=%d\n", i.Stream)
	}

	if i.Language != "eng" {
		t.Errorf("#input3.language: want=eng, have=%s\n", i.Language)
	}

	if i.Type != "video" {
		t.Errorf("#input3.type: want=video, have=%s\n", i.Type)
	}

	if i.Codec != "h264" {
		t.Errorf("#input3.codec: want=h264, have=%s\n", i.Codec)
	}

	if i.Bitrate != 5894 {
		t.Errorf("#input3.bitrate: want=5894, have=%f\n", i.Bitrate)
	}

	if i.Duration != 62.28 {
		t.Errorf("#input3.duration: want=62.82, have=%f\n", i.Duration)
	}

	if i.FPS != 23.98 {
		t.Errorf("#input3.fps: want=23.98, have=%f\n", i.FPS)
	}

	if i.Pixfmt != "yuvj420p" {
		t.Errorf("#input3.pixfmt: want=yuvj420p, have=%s\n", i.Pixfmt)
	}

	if i.Width != 2560 {
		t.Errorf("#input3.width: want=2560, have=%d\n", i.Width)
	}

	if i.Height != 1440 {
		t.Errorf("#input3.height: want=1440, have=%d\n", i.Height)
	}

	i = prober.inputs[4]

	if i.Language != "por" {
		t.Errorf("#input4.language: want=por, have=%s\n", i.Language)
	}

	if i.Type != "subtitle" {
		t.Errorf("#input4.type: want=subtitle, have=%s\n", i.Type)
	}

	if i.Codec != "subrip" {
		t.Errorf("#input4.codec: want=subtip, have=%s\n", i.Codec)
	}
}
