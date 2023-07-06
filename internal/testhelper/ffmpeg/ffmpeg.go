package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/datarhei/core/v16/slices"
)

func main() {
	header := `ffmpeg version 4.4.1-datarhei Copyright (c) 2000-2021 the FFmpeg developers
	built with gcc 10.3.1 (Alpine 10.3.1_git20211027) 20211027
	configuration: --extra-version=datarhei --prefix=/usr --extra-libs='-lpthread -lm -lz -lsupc++ -lstdc++ -lssl -lcrypto -lz -lc -ldl' --enable-nonfree --enable-gpl --enable-version3 --enable-postproc --enable-static --enable-openssl --enable-omx --enable-omx-rpi --enable-mmal --enable-v4l2_m2m --enable-libfreetype --enable-libsrt --enable-libx264 --enable-libx265 --enable-libvpx --enable-libmp3lame --enable-libopus --enable-libvorbis --disable-ffplay --disable-debug --disable-doc --disable-shared
	libavutil      56. 70.100 / 56. 70.100
	libavcodec     58.134.100 / 58.134.100
	libavformat    58. 76.100 / 58. 76.100
	libavdevice    58. 13.100 / 58. 13.100
	libavfilter     7.110.100 /  7.110.100
	libswscale      5.  9.100 /  5.  9.100
	libswresample   3.  9.100 /  3.  9.100
	libpostproc    55.  9.100 / 55.  9.100`

	codecs := `Codecs:
 D..... = Decoding supported
 .E.... = Encoding supported
 ..V... = Video codec
 ..A... = Audio codec
 ..S... = Subtitle codec
 ..D... = Data codec
 ..T... = Attachment codec
 ...I.. = Intra frame-only codec
 ....L. = Lossy compression
 .....S = Lossless compression
 -------
 DEAIL. aac                  AAC (Advanced Audio Coding) (decoders: aac aac_fixed aac_at ) (encoders: aac aac_at )
 DEVI.S y41p                 Uncompressed YUV 4:1:1 12-bit
 DEV.LS h264                 H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10 (encoders: libx264 libx264rgb h264_videotoolbox )
 DEV.L. flv1                 FLV / Sorenson Spark / Sorenson H.263 (Flash Video) (decoders: flv ) (encoders: flv )`

	filters := `Filters:
 T.. = Timeline support
 .S. = Slice threading
 ..C = Command support
 A = Audio input/output
 V = Video input/output
 N = Dynamic number and/or type of input/output
 | = Source or sink filter
 ... afirsrc           |->A       Generate a FIR coefficients audio stream.
 ... anoisesrc         |->A       Generate a noise audio signal.
 ... anullsrc          |->A       Null audio source, return empty audio frames.
 ... hilbert           |->A       Generate a Hilbert transform FIR coefficients.
 ... sinc              |->A       Generate a sinc kaiser-windowed low-pass, high-pass, band-pass, or band-reject FIR coefficients.
 ... sine              |->A       Generate sine wave audio signal.
 ... anullsink         A->|       Do absolutely nothing with the input audio.
 ... addroi            V->V       Add region of interest to frame.
 ... alphaextract      V->N       Extract an alpha channel as a grayscale image component.
 T.. alphamerge        VV->V      Copy the luma value of the second input into the alpha channel of the first input.`

	formats := `File formats:
 D. = Demuxing supported
 .E = Muxing supported
 --
 DE mpeg            MPEG-1 Systems / MPEG program stream
  E  mpeg1video      raw MPEG-1 video
  E  mpeg2video      raw MPEG-2 video
 DE  mpegts          MPEG-TS (MPEG-2 Transport Stream)
 D   mpegtsraw       raw MPEG-TS (MPEG-2 Transport Stream)
 D   mpegvideo       raw MPEG video`

	devices := `Devices:
 D. = Demuxing supported
 .E = Muxing supported
 --
  E audiotoolbox    AudioToolbox output device
 D  avfoundation    AVFoundation input device
 D  lavfi           Libavfilter virtual input device
  E sdl,sdl2        SDL2 output device
 D  x11grab         X11 screen capture, using XCB`

	protocols := `Supported file protocols:
Input:
  async
  bluray
  cache
Output:
  crypto
  file
  ftp
  gopher`

	hwaccels := `Hardware acceleration methods:
videotoolbox`

	avfoundation := `[AVFoundation input device @ 0x7fc2db40f240] AVFoundation video devices:
[AVFoundation input device @ 0x7fc2db40f240] [0] FaceTime HD Camera (Built-in)
[AVFoundation input device @ 0x7fc2db40f240] [1] Capture screen 0
[AVFoundation input device @ 0x7fc2db40f240] [2] Capture screen 1
[AVFoundation input device @ 0x7fc2db40f240] AVFoundation audio devices:
[AVFoundation input device @ 0x7fc2db40f240] [0] Built-in Microphone
: Input/output error`

	prelude := `Input #0, lavfi, from 'testsrc=size=1280x720:rate=25':
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
[hls @ 0x7fa969803a00] Opening './data/testsrc.m3u8.tmp' for writing`

	fmt.Fprintf(os.Stderr, "%s\n", header)

	if len(os.Args) <= 1 {
		os.Exit(2)
	}

	if slices.EqualComparableElements(os.Args[1:], []string{"-f", "avfoundation", "-list_devices", "true", "-i", ""}) {
		fmt.Fprintf(os.Stderr, "%s\n", avfoundation)
		os.Exit(0)
	}

	lastArg := os.Args[len(os.Args)-1]

	if lastArg == "-version" {
		os.Exit(0)
	}

	switch lastArg {
	case "-codecs":
		fmt.Fprintf(os.Stderr, "%s\n", codecs)
		os.Exit(0)
	case "-filters":
		fmt.Fprintf(os.Stderr, "%s\n", filters)
		os.Exit(0)
	case "-formats":
		fmt.Fprintf(os.Stderr, "%s\n", formats)
		os.Exit(0)
	case "-devices":
		fmt.Fprintf(os.Stderr, "%s\n", devices)
		os.Exit(0)
	case "-protocols":
		fmt.Fprintf(os.Stderr, "%s\n", protocols)
		os.Exit(0)
	case "-hwaccels":
		fmt.Fprintf(os.Stderr, "%s\n", hwaccels)
		os.Exit(0)
	}

	if len(lastArg) > 1 && lastArg[0] == '-' {
		os.Exit(2)
	}

	fmt.Fprintf(os.Stderr, "%s\n", prelude)

	ctx, cancel := context.WithCancel(context.Background())

	go func(ctx context.Context) {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		frame := uint64(0)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				frame += 25
				fmt.Fprintf(os.Stderr, "frame=%5d fps= 25 q=-1.0 Lsize=N/A time=00:00:02.32 bitrate=N/A speed=1.0x    \r", frame)
			}
		}
	}(ctx)

	// Wait for interrupt signal to gracefully shutdown the app
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	cancel()

	fmt.Fprintf(os.Stderr, "\nExiting normally, received signal 2.\n")

	os.Exit(255)
}
