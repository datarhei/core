package skills

var ffmpegdata = `ffmpeg version 4.4.1-datarhei Copyright (c) 2000-2021 the FFmpeg developers
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

var filterdata = ` ... afirsrc           |->A       Generate a FIR coefficients audio stream.
... anoisesrc         |->A       Generate a noise audio signal.
... anullsrc          |->A       Null audio source, return empty audio frames.
... hilbert           |->A       Generate a Hilbert transform FIR coefficients.
... sinc              |->A       Generate a sinc kaiser-windowed low-pass, high-pass, band-pass, or band-reject FIR coefficients.
... sine              |->A       Generate sine wave audio signal.
... anullsink         A->|       Do absolutely nothing with the input audio.
... addroi            V->V       Add region of interest to frame.
... alphaextract      V->N       Extract an alpha channel as a grayscale image component.
T.. alphamerge        VV->V      Copy the luma value of the second input into the alpha channel of the first input.`

var codecdata = ` DEAIL. aac                  AAC (Advanced Audio Coding) (decoders: aac aac_fixed aac_at ) (encoders: aac aac_at )
DEVI.S y41p                 Uncompressed YUV 4:1:1 12-bit
DEV.LS h264                 H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10 (encoders: libx264 libx264rgb h264_videotoolbox )
DEV.L. flv1                 FLV / Sorenson Spark / Sorenson H.263 (Flash Video) (decoders: flv ) (encoders: flv )`

var formatdata = ` DE mpeg            MPEG-1 Systems / MPEG program stream
 E  mpeg1video      raw MPEG-1 video
 E  mpeg2video      raw MPEG-2 video
DE  mpegts          MPEG-TS (MPEG-2 Transport Stream)
D   mpegtsraw       raw MPEG-TS (MPEG-2 Transport Stream)
D   mpegvideo       raw MPEG video`

var protocoldata = `Input:
async
bluray
cache
Output:
crypto
file
ftp
gopher`

var hwacceldata = `Hardware acceleration methods:
videotoolbox`

var v4ldata = `mmal service 16.1 (platform:bcm2835-v4l2):
	/dev/video0

Webcam C170: Webcam C170 (usb-3f980000.usb-1.3):
	/dev/video1

`
