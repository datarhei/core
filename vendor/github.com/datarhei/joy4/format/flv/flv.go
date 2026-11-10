package flv

import (
	"bufio"
	"bytes"
	"fmt"
	"io"

	"github.com/datarhei/joy4/av"
	"github.com/datarhei/joy4/av/avutil"
	"github.com/datarhei/joy4/codec"
	"github.com/datarhei/joy4/codec/aacparser"
	"github.com/datarhei/joy4/codec/av1parser"
	"github.com/datarhei/joy4/codec/h264parser"
	"github.com/datarhei/joy4/codec/hevcparser"
	"github.com/datarhei/joy4/codec/vp9parser"
	"github.com/datarhei/joy4/codec/vvcparser"
	"github.com/datarhei/joy4/format/flv/flvio"
	"github.com/datarhei/joy4/utils/bits/pio"
)

var MaxProbePacketCount = 20

func NewMetadataByStreams(streams []av.CodecData) (metadata flvio.AMFMap, err error) {
	metadata = flvio.AMFMap{}

	for _, _stream := range streams {
		typ := _stream.Type()
		switch {
		case typ.IsVideo():
			stream := _stream.(av.VideoCodecData)
			switch typ {
			case av.H264:
				metadata["videocodecid"] = flvio.VIDEO_H264
			case av.HEVC:
				metadata["videocodecid"] = flvio.FourCCToFloat(flvio.FOURCC_HEVC)
			case av.VP9:
				metadata["videocodecid"] = flvio.FourCCToFloat(flvio.FOURCC_VP9)
			case av.AV1:
				metadata["videocodecid"] = flvio.FourCCToFloat(flvio.FOURCC_AV1)
			case av.VVC:
				metadata["videocodecid"] = flvio.FourCCToFloat(flvio.FOURCC_VVC)

			default:
				err = fmt.Errorf("flv: metadata: unsupported video codecType=%v", stream.Type())
				return
			}

			width, height := stream.Width(), stream.Height()

			if width != 0 {
				metadata["width"] = width
				metadata["displayWidth"] = width
			}

			if height != 0 {
				metadata["height"] = height
				metadata["displayHeight"] = height
			}

		case typ.IsAudio():
			stream := _stream.(av.AudioCodecData)
			switch typ {
			case av.AAC:
				metadata["audiocodecid"] = flvio.SOUND_AAC

			case av.SPEEX:
				metadata["audiocodecid"] = flvio.SOUND_SPEEX

			case av.NELLYMOSER:
				metadata["audiocodecid"] = int(nellymoserTag(stream).SoundFormat)

			default:
				err = fmt.Errorf("flv: metadata: unsupported audio codecType=%v", stream.Type())
				return
			}

			metadata["audiosamplerate"] = stream.SampleRate()
		}
	}

	return
}

type Prober struct {
	HasAudio, HasVideo             bool
	GotAudio, GotVideo             bool
	VideoStreamIdx, AudioStreamIdx int
	PushedCount                    int
	MaxProbePacketCount            int
	Streams                        []av.CodecData
	CachedPkts                     []av.Packet

	// ColorInfo is the color info (HDR metadata, E-RTMP) that applies to the video packet returned last
	// and the following ones. It is nil if no color info has been received or it has been reset.
	ColorInfo flvio.AMFMap

	// the tags received while probing, converted to CachedPkts once probing is done
	cachedTags []cachedTag

	// the color info that has been received before the packet with the same index in CachedPkts, and
	// before the next packet after CachedPkts
	cachedColorInfo  []*colorInfoUpdate
	pendingColorInfo *colorInfoUpdate

	// the last error of PushTag
	lastErr error

	// the config record of the video stream (H.264, HEVC) that applies to the following packets, and its
	// codec data. It differs from the one in Streams after a new sequence header in the middle of the stream.
	videoRecord    []byte
	videoCodecData av.VideoCodecData

	// the parameter sets of videoRecord, length-prefixed as the NALUs in the frames. They are put in front
	// of every key frame as long as the config differs from the one in Streams. It is nil otherwise.
	videoParamSets []byte
}

type colorInfoUpdate struct {
	colorInfo flvio.AMFMap
}

// isVideoMetadata returns whether the tag is a video packet of the type PKTTYPE_METADATA (E-RTMP).
func isVideoMetadata(tag flvio.Tag) bool {
	return tag.Type == flvio.TAG_VIDEO && tag.IsExHeader && tag.PacketType == flvio.PKTTYPE_METADATA
}

type cachedTag struct {
	tag       flvio.Tag
	timestamp int64
}

func NewProber(maxProbePacketCount int) *Prober {
	prober := &Prober{
		MaxProbePacketCount: maxProbePacketCount,
	}

	return prober
}

// CacheTag caches a tag while probing. It is converted to a packet once probing is done, such that it
// gets the index of its stream even if it arrived before the sequence header of its stream.
func (prober *Prober) CacheTag(tag flvio.Tag, timestamp int64) {
	prober.cachedTags = append(prober.cachedTags, cachedTag{tag, timestamp})
}

func (prober *Prober) PushTag(tag flvio.Tag, timestamp int64) (err error) {
	defer func() {
		if err != nil {
			prober.lastErr = err
		}
	}()

	prober.PushedCount++

	if prober.MaxProbePacketCount <= 0 {
		prober.MaxProbePacketCount = MaxProbePacketCount
	}

	if prober.PushedCount > prober.MaxProbePacketCount {
		err = fmt.Errorf("flv: max probe packet count reached")
		return
	}

	switch tag.Type {
	case flvio.TAG_VIDEO:
		if isVideoMetadata(tag) {
			// it applies to the packets after it
			prober.CacheTag(tag, timestamp)
		} else if tag.IsExHeader {
			if tag.FourCC == flvio.FOURCC_HEVC {
				if tag.PacketType == flvio.PKTTYPE_SEQUENCE_START {
					if !prober.GotVideo {
						var stream hevcparser.CodecData
						//fmt.Printf("got HEVC sequence start:\n%s\n", hex.Dump(tag.Data))
						if stream, err = hevcparser.NewCodecDataFromHEVCDecoderConfRecord(tag.Data); err != nil {
							err = fmt.Errorf("flv: hevc seqhdr invalid: %s", err.Error())
							return
						}
						prober.VideoStreamIdx = len(prober.Streams)
						prober.Streams = append(prober.Streams, stream)
						prober.GotVideo = true
						prober.videoRecord = tag.Data
						prober.videoCodecData = stream
					} else {
						// a new config in the middle of the stream
						prober.CacheTag(tag, timestamp)
					}
				} else if tag.PacketType == flvio.PKTTYPE_CODED_FRAMES || tag.PacketType == flvio.PKTTYPE_CODED_FRAMESX {
					prober.CacheTag(tag, timestamp)
				}
			} else if tag.FourCC == flvio.FOURCC_VP9 {
				if tag.PacketType == flvio.PKTTYPE_SEQUENCE_START {
					if !prober.GotVideo {
						var stream vp9parser.CodecData
						//fmt.Printf("got VP9 sequence start:\n%s\n", hex.Dump(tag.Data))
						if stream, err = vp9parser.NewCodecDataFromVPDecoderConfRecord(tag.Data); err != nil {
							err = fmt.Errorf("flv: vp9 seqhdr invalid: %s", err.Error())
							return
						}
						prober.VideoStreamIdx = len(prober.Streams)
						prober.Streams = append(prober.Streams, stream)
						prober.GotVideo = true
					}
				} else if tag.PacketType == flvio.PKTTYPE_CODED_FRAMES || tag.PacketType == flvio.PKTTYPE_CODED_FRAMESX {
					prober.CacheTag(tag, timestamp)
				}
			} else if tag.FourCC == flvio.FOURCC_AV1 {
				if tag.PacketType == flvio.PKTTYPE_SEQUENCE_START || tag.PacketType == flvio.PKTTYPE_MPEG2TS_SEQUENCE_START {
					if !prober.GotVideo {
						var stream av1parser.CodecData

						if tag.PacketType == flvio.PKTTYPE_SEQUENCE_START {
							//fmt.Printf("got AV1 sequence start:\n%s\n", hex.Dump(tag.Data))
							if stream, err = av1parser.NewCodecDataFromAV1DecoderConfRecord(tag.Data); err != nil {
								err = fmt.Errorf("flv: av1 seqhdr invalid: %s", err.Error())
								return
							}
						} else {
							//fmt.Printf("got AV1 video descriptor:\n%s\n", hex.Dump(tag.Data))
							if stream, err = av1parser.NewCodecDataFromAV1VideoDescriptor(tag.Data); err != nil {
								err = fmt.Errorf("flv: av1 video descriptor invalid: %s", err.Error())
								return
							}
						}
						prober.VideoStreamIdx = len(prober.Streams)
						prober.Streams = append(prober.Streams, stream)
						prober.GotVideo = true
					}
				} else if tag.PacketType == flvio.PKTTYPE_CODED_FRAMES || tag.PacketType == flvio.PKTTYPE_CODED_FRAMESX {
					prober.CacheTag(tag, timestamp)
				}
			} else if tag.FourCC == flvio.FOURCC_VVC {
				if tag.PacketType == flvio.PKTTYPE_SEQUENCE_START {
					if !prober.GotVideo {
						var stream vvcparser.CodecData
						//fmt.Printf("got VVC sequence start:\n%s\n", hex.Dump(tag.Data))
						if stream, err = vvcparser.NewCodecDataFromVVCDecoderConfRecord(tag.Data); err != nil {
							err = fmt.Errorf("flv: vvc seqhdr invalid: %s", err.Error())
							return
						}
						prober.VideoStreamIdx = len(prober.Streams)
						prober.Streams = append(prober.Streams, stream)
						prober.GotVideo = true
					}
				} else if tag.PacketType == flvio.PKTTYPE_CODED_FRAMES || tag.PacketType == flvio.PKTTYPE_CODED_FRAMESX {
					prober.CacheTag(tag, timestamp)
				}
			}
		} else if tag.CodecID == flvio.VIDEO_H264 {
			switch tag.AVCPacketType {
			case flvio.AVC_SEQHDR:
				if !prober.GotVideo {
					var stream h264parser.CodecData
					//fmt.Printf("got H264 sequence start:\n%s\n", hex.Dump(tag.Data))
					if stream, err = h264parser.NewCodecDataFromAVCDecoderConfRecord(tag.Data); err != nil {
						err = fmt.Errorf("flv: h264 seqhdr invalid: %s", err.Error())
						return
					}
					prober.VideoStreamIdx = len(prober.Streams)
					prober.Streams = append(prober.Streams, stream)
					prober.GotVideo = true
					prober.videoRecord = tag.Data
					prober.videoCodecData = stream
				} else {
					// a new config in the middle of the stream
					prober.CacheTag(tag, timestamp)
				}

			case flvio.AVC_NALU:
				prober.CacheTag(tag, timestamp)
			}
		}

	case flvio.TAG_AUDIO:
		switch tag.SoundFormat {
		case flvio.SOUND_AAC:
			switch tag.AACPacketType {
			case flvio.AAC_SEQHDR:
				if !prober.GotAudio {
					var stream aacparser.CodecData
					if stream, err = aacparser.NewCodecDataFromMPEG4AudioConfigBytes(tag.Data); err != nil {
						err = fmt.Errorf("flv: aac seqhdr invalid")
						return
					}
					prober.AudioStreamIdx = len(prober.Streams)
					prober.Streams = append(prober.Streams, stream)
					prober.GotAudio = true
				}

			case flvio.AAC_RAW:
				prober.CacheTag(tag, timestamp)
			}

		case flvio.SOUND_SPEEX:
			if !prober.GotAudio {
				stream := codec.NewSpeexCodecData(16000, tag.ChannelLayout())
				prober.AudioStreamIdx = len(prober.Streams)
				prober.Streams = append(prober.Streams, stream)
				prober.GotAudio = true
			}
			prober.CacheTag(tag, timestamp)

		case flvio.SOUND_NELLYMOSER, flvio.SOUND_NELLYMOSER_16KHZ_MONO, flvio.SOUND_NELLYMOSER_8KHZ_MONO:
			if !prober.GotAudio {
				sampleRate, channelLayout := nellymoserFormat(tag)
				stream := codec.NewNellymoserCodecData(sampleRate, channelLayout)
				prober.AudioStreamIdx = len(prober.Streams)
				prober.Streams = append(prober.Streams, stream)
				prober.GotAudio = true
			}
			prober.CacheTag(tag, timestamp)
		}
	}

	return
}

// Err returns the last error of PushTag, or nil.
func (prober *Prober) Err() error {
	return prober.lastErr
}

// PushMetadata sets HasVideo and HasAudio from the codec ids in the metadata (onMetaData), if they are
// supported. An unsupported codec is not waited for, otherwise probing would only end after
// MaxProbePacketCount tags.
func (prober *Prober) PushMetadata(metadata flvio.AMFMap) {
	if isSupportedVideoCodecID(metadata["videocodecid"]) {
		prober.HasVideo = true
	}
	if isSupportedAudioCodecID(metadata["audiocodecid"]) {
		prober.HasAudio = true
	}
}

var supportedVideoFourCCs = [][4]byte{
	{'a', 'v', 'c', '1'},
	flvio.FOURCC_HEVC,
	flvio.FOURCC_VP9,
	flvio.FOURCC_AV1,
	flvio.FOURCC_VVC,
}

// isSupportedVideoCodecID returns whether the videocodecid of the metadata is a supported codec. It is a
// legacy codec id or a FourCC as a number (E-RTMP), or a FourCC as a string.
func isSupportedVideoCodecID(id interface{}) bool {
	switch id := id.(type) {
	case float64:
		if id == flvio.VIDEO_H264 {
			return true
		}
		for _, fourcc := range supportedVideoFourCCs {
			if id == flvio.FourCCToFloat(fourcc) {
				return true
			}
		}
	case string:
		for _, fourcc := range supportedVideoFourCCs {
			if id == string(fourcc[:]) {
				return true
			}
		}
	}
	return false
}

// isSupportedAudioCodecID returns whether the audiocodecid of the metadata is a supported codec. It is a
// legacy codec id, or "mp4a" for AAC.
func isSupportedAudioCodecID(id interface{}) bool {
	switch id := id.(type) {
	case float64:
		switch id {
		case flvio.SOUND_AAC, flvio.SOUND_SPEEX, flvio.SOUND_NELLYMOSER, flvio.SOUND_NELLYMOSER_16KHZ_MONO, flvio.SOUND_NELLYMOSER_8KHZ_MONO:
			return true
		}
	case string:
		return id == "mp4a"
	}
	return false
}

// soundRates are the sample rates of the SoundRate of an audio tag.
var soundRates = []int{5512, 11025, 22050, 44100}

// nellymoserFormat returns the sample rate and channel layout of a Nellymoser audio tag.
func nellymoserFormat(tag flvio.Tag) (sampleRate int, channelLayout av.ChannelLayout) {
	switch tag.SoundFormat {
	case flvio.SOUND_NELLYMOSER_16KHZ_MONO:
		return 16000, av.CH_MONO
	case flvio.SOUND_NELLYMOSER_8KHZ_MONO:
		return 8000, av.CH_MONO
	}
	return soundRates[tag.SoundRate&0x3], tag.ChannelLayout()
}

// nellymoserTag returns an audio tag with the SoundFormat and SoundRate for a Nellymoser stream, as FFmpeg
// writes them: 16 kHz and 8 kHz have their own SoundFormat, the other sample rates are in the SoundRate.
func nellymoserTag(stream av.AudioCodecData) flvio.Tag {
	tag := flvio.Tag{
		Type:      flvio.TAG_AUDIO,
		SoundSize: flvio.SOUND_16BIT,
	}

	switch stream.SampleRate() {
	case 16000:
		tag.SoundFormat = flvio.SOUND_NELLYMOSER_16KHZ_MONO
		return tag
	case 8000:
		tag.SoundFormat = flvio.SOUND_NELLYMOSER_8KHZ_MONO
		return tag
	}

	tag.SoundFormat = flvio.SOUND_NELLYMOSER
	for i, rate := range soundRates {
		if stream.SampleRate() == rate {
			tag.SoundRate = uint8(i)
		}
	}
	if stream.ChannelLayout().Count() == 2 {
		tag.SoundType = flvio.SOUND_STEREO
	}
	return tag
}

func (prober *Prober) Probed() (ok bool) {
	if prober.MaxProbePacketCount <= 0 {
		prober.MaxProbePacketCount = MaxProbePacketCount
	}

	if prober.HasAudio || prober.HasVideo {
		if prober.HasAudio == prober.GotAudio && prober.HasVideo == prober.GotVideo {
			return true
		}
	}

	if prober.PushedCount == prober.MaxProbePacketCount {
		return true
	}

	return false
}

// tagCodecType returns the codec type of an audio or video tag.
func tagCodecType(tag flvio.Tag) av.CodecType {
	switch tag.Type {
	case flvio.TAG_VIDEO:
		if !tag.IsExHeader {
			if tag.CodecID == flvio.VIDEO_H264 {
				return av.H264
			}
			return 0
		}
		switch tag.FourCC {
		case flvio.FOURCC_HEVC:
			return av.HEVC
		case flvio.FOURCC_VP9:
			return av.VP9
		case flvio.FOURCC_AV1:
			return av.AV1
		case flvio.FOURCC_VVC:
			return av.VVC
		}

	case flvio.TAG_AUDIO:
		switch tag.SoundFormat {
		case flvio.SOUND_AAC:
			return av.AAC
		case flvio.SOUND_SPEEX:
			return av.SPEEX
		case flvio.SOUND_NELLYMOSER, flvio.SOUND_NELLYMOSER_16KHZ_MONO, flvio.SOUND_NELLYMOSER_8KHZ_MONO:
			return av.NELLYMOSER
		}
	}

	return 0
}

// isVideoSequenceStart returns whether the tag is a video sequence header (config record).
func isVideoSequenceStart(tag flvio.Tag) bool {
	if tag.Type != flvio.TAG_VIDEO {
		return false
	}
	if tag.IsExHeader {
		return tag.PacketType == flvio.PKTTYPE_SEQUENCE_START
	}
	return tag.CodecID == flvio.VIDEO_H264 && tag.AVCPacketType == flvio.AVC_SEQHDR
}

// lengthPrefixed returns the NALUs, each prefixed with its length in lengthSize bytes, as in the frames.
func lengthPrefixed(lengthSize int, nalus ...[][]byte) []byte {
	var b []byte
	for _, list := range nalus {
		for _, nalu := range list {
			for i := lengthSize - 1; i >= 0; i-- {
				b = append(b, byte(len(nalu)>>(8*i)))
			}
			b = append(b, nalu...)
		}
	}
	return b
}

// updateVideoConfig handles a sequence header of the video stream after the first one. A re-sent,
// identical config is ignored. A new H.264 or HEVC config is put in-band: its parameter sets are put in
// front of every following key frame, such that a decoder (also one that joins later) uses them instead of
// the ones in the codec data of the stream. A change back to the config of the stream stops that.
//
// Not supported: a new config of other codecs, a change of the codec, and a different NALU length size.
// Streams() keeps the codec data of the first config (e.g. its width and height).
func (prober *Prober) updateVideoConfig(tag flvio.Tag) {
	if !prober.GotVideo || prober.VideoStreamIdx >= len(prober.Streams) {
		return
	}
	stream := prober.Streams[prober.VideoStreamIdx]
	if stream.Type() != tagCodecType(tag) || bytes.Equal(tag.Data, prober.videoRecord) {
		return
	}

	record := append([]byte(nil), tag.Data...)

	switch stream := stream.(type) {
	case h264parser.CodecData:
		codecData, err := h264parser.NewCodecDataFromAVCDecoderConfRecord(record)
		if err != nil || codecData.RecordInfo.LengthSizeMinusOne != stream.RecordInfo.LengthSizeMinusOne {
			return
		}
		prober.videoCodecData = codecData
		prober.videoParamSets = lengthPrefixed(int(stream.RecordInfo.LengthSizeMinusOne)+1, codecData.RecordInfo.SPS, codecData.RecordInfo.PPS)
		if bytes.Equal(record, stream.AVCDecoderConfRecordBytes()) {
			prober.videoParamSets = nil
		}

	case hevcparser.CodecData:
		codecData, err := hevcparser.NewCodecDataFromHEVCDecoderConfRecord(record)
		if err != nil || codecData.RecordInfo.LengthSizeMinusOne != stream.RecordInfo.LengthSizeMinusOne {
			return
		}
		prober.videoCodecData = codecData
		prober.videoParamSets = lengthPrefixed(int(stream.RecordInfo.LengthSizeMinusOne)+1, codecData.RecordInfo.VPS, codecData.RecordInfo.SPS, codecData.RecordInfo.PPS)
		if bytes.Equal(record, stream.HEVCDecoderConfRecordBytes()) {
			prober.videoParamSets = nil
		}

	default:
		return
	}

	prober.videoRecord = record
}

// TagToPacket converts an audio or video tag to a packet of the probed stream of its kind. If there's no
// such stream, or the codec of the tag doesn't match the codec of the stream, it returns false. A tag with
// color info (E-RTMP) updates ColorInfo and returns false. A video sequence header returns false, a new
// config is put in-band into the following key frames (see updateVideoConfig).
func (prober *Prober) TagToPacket(tag flvio.Tag, timestamp int64) (pkt av.Packet, ok bool) {
	if isVideoMetadata(tag) {
		if colorInfo, found := flvio.ParseColorInfo(tag.Data); found {
			prober.ColorInfo = colorInfo
		}
		return
	}

	if isVideoSequenceStart(tag) {
		prober.updateVideoConfig(tag)
		return
	}

	var idx int
	switch tag.Type {
	case flvio.TAG_VIDEO:
		if !prober.GotVideo {
			return
		}
		idx = prober.VideoStreamIdx
	case flvio.TAG_AUDIO:
		if !prober.GotAudio {
			return
		}
		idx = prober.AudioStreamIdx
	default:
		return
	}

	if idx >= len(prober.Streams) || prober.Streams[idx].Type() != tagCodecType(tag) {
		return
	}

	switch tag.Type {
	case flvio.TAG_VIDEO:
		pkt.Idx = int8(prober.VideoStreamIdx)
		// other packet types (e.g. the end of the sequence) are not passed on
		switch tag.PacketType {
		case flvio.PKTTYPE_CODED_FRAMES, flvio.PKTTYPE_CODED_FRAMESX:
			ok = true
			pkt.Data = tag.Data
			pkt.CompositionTime = flvio.TsToTime(tag.CompositionTime)
			pkt.IsKeyFrame = tag.FrameType == flvio.FRAME_KEY
			if pkt.IsKeyFrame && prober.videoParamSets != nil {
				data := make([]byte, 0, len(prober.videoParamSets)+len(tag.Data))
				data = append(data, prober.videoParamSets...)
				pkt.Data = append(data, tag.Data...)
			}
		}

	case flvio.TAG_AUDIO:
		pkt.Idx = int8(prober.AudioStreamIdx)
		switch tag.SoundFormat {
		case flvio.SOUND_AAC:
			switch tag.AACPacketType {
			case flvio.AAC_RAW:
				ok = true
				pkt.Data = tag.Data
				pkt.IsKeyFrame = true
			}

		case flvio.SOUND_SPEEX:
			ok = true
			pkt.Data = tag.Data
			pkt.IsKeyFrame = true

		case flvio.SOUND_NELLYMOSER, flvio.SOUND_NELLYMOSER_16KHZ_MONO, flvio.SOUND_NELLYMOSER_8KHZ_MONO:
			ok = true
			pkt.Data = tag.Data
			pkt.IsKeyFrame = true
		}
	}

	pkt.Time = flvio.TsToTime(timestamp)
	return
}

// convertCachedTags converts the tags cached while probing to packets, once probing is done.
func (prober *Prober) convertCachedTags() {
	if len(prober.cachedTags) == 0 || !prober.Probed() {
		return
	}

	// the color info is applied when the packet after it is popped
	update := prober.pendingColorInfo
	for _, c := range prober.cachedTags {
		if isVideoMetadata(c.tag) {
			if colorInfo, found := flvio.ParseColorInfo(c.tag.Data); found {
				update = &colorInfoUpdate{colorInfo}
			}
			continue
		}
		if pkt, ok := prober.TagToPacket(c.tag, c.timestamp); ok {
			prober.CachedPkts = append(prober.CachedPkts, pkt)
			prober.cachedColorInfo = append(prober.cachedColorInfo, update)
			update = nil
		}
	}
	prober.pendingColorInfo = update

	prober.cachedTags = nil
}

func (prober *Prober) Empty() bool {
	prober.convertCachedTags()
	if len(prober.CachedPkts) != 0 {
		return false
	}

	// the color info after the last cached packet applies to the next packet
	if prober.pendingColorInfo != nil {
		prober.ColorInfo = prober.pendingColorInfo.colorInfo
		prober.pendingColorInfo = nil
	}

	return true
}

func (prober *Prober) PopPacket() av.Packet {
	prober.convertCachedTags()
	pkt := prober.CachedPkts[0]
	prober.CachedPkts = prober.CachedPkts[1:]
	if len(prober.cachedColorInfo) != 0 {
		if update := prober.cachedColorInfo[0]; update != nil {
			prober.ColorInfo = update.colorInfo
		}
		prober.cachedColorInfo = prober.cachedColorInfo[1:]
	}
	return pkt
}

func CodecDataToTag(stream av.CodecData) (_tag flvio.Tag, ok bool, err error) {
	switch stream.Type() {
	case av.H264:
		h264 := stream.(h264parser.CodecData)
		tag := flvio.Tag{
			Type:          flvio.TAG_VIDEO,
			AVCPacketType: flvio.AVC_SEQHDR,
			CodecID:       flvio.VIDEO_H264,
			Data:          h264.AVCDecoderConfRecordBytes(),
			FrameType:     flvio.FRAME_KEY,
		}
		//fmt.Printf("set H264 sequence start:\n%v\n", hex.Dump(tag.Data))
		ok = true
		_tag = tag

	case av.HEVC:
		hevc := stream.(hevcparser.CodecData)
		tag := flvio.Tag{
			Type:       flvio.TAG_VIDEO,
			IsExHeader: true,
			PacketType: flvio.PKTTYPE_SEQUENCE_START,
			FourCC:     flvio.FOURCC_HEVC,
			Data:       hevc.HEVCDecoderConfRecordBytes(),
			FrameType:  flvio.FRAME_KEY,
		}
		//fmt.Printf("set HEVC sequence start:\n%v\n", hex.Dump(tag.Data))
		ok = true
		_tag = tag

	case av.VP9:
		vp9 := stream.(vp9parser.CodecData)
		tag := flvio.Tag{
			Type:       flvio.TAG_VIDEO,
			IsExHeader: true,
			PacketType: flvio.PKTTYPE_SEQUENCE_START,
			FourCC:     flvio.FOURCC_VP9,
			Data:       vp9.VPDecoderConfRecordBytes(),
			FrameType:  flvio.FRAME_KEY,
		}
		//fmt.Printf("set VP9 sequence start:\n%v\n", hex.Dump(tag.Data))
		ok = true
		_tag = tag

	case av.AV1:
		av1 := stream.(av1parser.CodecData)
		tag := flvio.Tag{
			Type:       flvio.TAG_VIDEO,
			IsExHeader: true,
			PacketType: flvio.PKTTYPE_SEQUENCE_START,
			FourCC:     flvio.FOURCC_AV1,
			Data:       av1.AV1DecoderConfRecordBytes(),
			FrameType:  flvio.FRAME_KEY,
		}

		if av1.IsMpeg2TS {
			tag.PacketType = flvio.PKTTYPE_MPEG2TS_SEQUENCE_START
			tag.Data = av1.AV1VideoDescriptorBytes()
		}

		//fmt.Printf("set AV1 sequence start:\n%v\n", hex.Dump(tag.Data))
		ok = true
		_tag = tag

	case av.VVC:
		vvc := stream.(vvcparser.CodecData)
		tag := flvio.Tag{
			Type:       flvio.TAG_VIDEO,
			IsExHeader: true,
			PacketType: flvio.PKTTYPE_SEQUENCE_START,
			FourCC:     flvio.FOURCC_VVC,
			Data:       vvc.VVCDecoderConfRecordBytes(),
			FrameType:  flvio.FRAME_KEY,
		}

		//fmt.Printf("set AV1 sequence start:\n%v\n", hex.Dump(tag.Data))
		ok = true
		_tag = tag

	case av.NELLYMOSER:
	case av.SPEEX:

	case av.AAC:
		aac := stream.(aacparser.CodecData)
		// the FLV spec requires 44 kHz and stereo for AAC, the actual sample rate and
		// channels are in the AudioSpecificConfig
		tag := flvio.Tag{
			Type:          flvio.TAG_AUDIO,
			SoundFormat:   flvio.SOUND_AAC,
			SoundRate:     flvio.SOUND_44Khz,
			SoundSize:     flvio.SOUND_16BIT,
			SoundType:     flvio.SOUND_STEREO,
			AACPacketType: flvio.AAC_SEQHDR,
			Data:          aac.MPEG4AudioConfigBytes(),
		}
		ok = true
		_tag = tag

	default:
		err = fmt.Errorf("flv: unspported codecType=%v", stream.Type())
		return
	}
	return
}

// ColorInfoToTag returns a video tag with the color info (HDR metadata, E-RTMP) for the stream. A nil color
// info resets it. ok is false if the codec of the stream has no FourCC, i.e. the color info is only
// defined for HEVC, AV1, VP9 and VVC.
func ColorInfoToTag(colorInfo flvio.AMFMap, stream av.CodecData) (tag flvio.Tag, ok bool) {
	var fourCC [4]byte
	switch stream.Type() {
	case av.HEVC:
		fourCC = flvio.FOURCC_HEVC
	case av.AV1:
		fourCC = flvio.FOURCC_AV1
	case av.VP9:
		fourCC = flvio.FOURCC_VP9
	case av.VVC:
		fourCC = flvio.FOURCC_VVC
	default:
		return
	}

	tag = flvio.Tag{
		Type:       flvio.TAG_VIDEO,
		IsExHeader: true,
		PacketType: flvio.PKTTYPE_METADATA,
		// the frame type is to be ignored, FFmpeg writes a command frame
		FrameType: flvio.FRAME_COMMAND,
		FourCC:    fourCC,
		Data:      flvio.ColorInfoData(colorInfo),
	}
	ok = true
	return
}

func PacketToTag(pkt av.Packet, stream av.CodecData) (tag flvio.Tag, timestamp int64) {
	switch stream.Type() {
	case av.H264:
		tag = flvio.Tag{
			Type:            flvio.TAG_VIDEO,
			AVCPacketType:   flvio.AVC_NALU,
			CodecID:         flvio.VIDEO_H264,
			Data:            pkt.Data,
			CompositionTime: flvio.TimeToTs(pkt.CompositionTime),
		}
		if pkt.IsKeyFrame {
			tag.FrameType = flvio.FRAME_KEY
		} else {
			tag.FrameType = flvio.FRAME_INTER
		}

	case av.HEVC:
		tag = flvio.Tag{
			Type:            flvio.TAG_VIDEO,
			IsExHeader:      true,
			PacketType:      flvio.PKTTYPE_CODED_FRAMES,
			CompositionTime: flvio.TimeToTs(pkt.CompositionTime),
			FourCC:          flvio.FOURCC_HEVC,
			Data:            pkt.Data,
		}

		if pkt.CompositionTime == 0 {
			tag.PacketType = flvio.PKTTYPE_CODED_FRAMESX
		}

		if pkt.IsKeyFrame {
			tag.FrameType = flvio.FRAME_KEY
		} else {
			tag.FrameType = flvio.FRAME_INTER
		}

	case av.VP9:
		tag = flvio.Tag{
			Type:            flvio.TAG_VIDEO,
			IsExHeader:      true,
			PacketType:      flvio.PKTTYPE_CODED_FRAMES,
			CompositionTime: flvio.TimeToTs(pkt.CompositionTime),
			FourCC:          flvio.FOURCC_VP9,
			Data:            pkt.Data,
		}

		if pkt.IsKeyFrame {
			tag.FrameType = flvio.FRAME_KEY
		} else {
			tag.FrameType = flvio.FRAME_INTER
		}

	case av.AV1:
		tag = flvio.Tag{
			Type:            flvio.TAG_VIDEO,
			IsExHeader:      true,
			PacketType:      flvio.PKTTYPE_CODED_FRAMES,
			CompositionTime: flvio.TimeToTs(pkt.CompositionTime),
			FourCC:          flvio.FOURCC_AV1,
			Data:            pkt.Data,
		}

		if pkt.IsKeyFrame {
			tag.FrameType = flvio.FRAME_KEY
		} else {
			tag.FrameType = flvio.FRAME_INTER
		}

	case av.VVC:
		tag = flvio.Tag{
			Type:            flvio.TAG_VIDEO,
			IsExHeader:      true,
			PacketType:      flvio.PKTTYPE_CODED_FRAMES,
			CompositionTime: flvio.TimeToTs(pkt.CompositionTime),
			FourCC:          flvio.FOURCC_VVC,
			Data:            pkt.Data,
		}

		if pkt.CompositionTime == 0 {
			tag.PacketType = flvio.PKTTYPE_CODED_FRAMESX
		}

		if pkt.IsKeyFrame {
			tag.FrameType = flvio.FRAME_KEY
		} else {
			tag.FrameType = flvio.FRAME_INTER
		}

	case av.AAC:
		// see CodecDataToTag
		tag = flvio.Tag{
			Type:          flvio.TAG_AUDIO,
			SoundFormat:   flvio.SOUND_AAC,
			SoundRate:     flvio.SOUND_44Khz,
			SoundSize:     flvio.SOUND_16BIT,
			SoundType:     flvio.SOUND_STEREO,
			AACPacketType: flvio.AAC_RAW,
			Data:          pkt.Data,
		}

	case av.SPEEX:
		tag = flvio.Tag{
			Type:        flvio.TAG_AUDIO,
			SoundFormat: flvio.SOUND_SPEEX,
			Data:        pkt.Data,
		}

	case av.NELLYMOSER:
		tag = nellymoserTag(stream.(av.AudioCodecData))
		tag.Data = pkt.Data
	}

	timestamp = flvio.TimeToTs(pkt.Time)
	return
}

type Muxer struct {
	bufw    writeFlusher
	b       []byte
	streams []av.CodecData

	colorInfo        flvio.AMFMap
	colorInfoPending bool
}

type writeFlusher interface {
	io.Writer
	Flush() error
}

func NewMuxerWriteFlusher(w writeFlusher) *Muxer {
	return &Muxer{
		bufw: w,
		b:    make([]byte, 256),
	}
}

func NewMuxer(w io.Writer) *Muxer {
	return NewMuxerWriteFlusher(bufio.NewWriterSize(w, pio.RecommendBufioSize))
}

var CodecTypes = []av.CodecType{av.H264, av.HEVC, av.VP9, av.AV1, av.VVC, av.AAC, av.SPEEX}

func (muxer *Muxer) WriteHeader(streams []av.CodecData) (err error) {
	var flags uint8
	for _, stream := range streams {
		if stream.Type().IsVideo() {
			flags |= flvio.FILE_HAS_VIDEO
		} else if stream.Type().IsAudio() {
			flags |= flvio.FILE_HAS_AUDIO
		}
	}

	n := flvio.FillFileHeader(muxer.b, flags)
	if _, err = muxer.bufw.Write(muxer.b[:n]); err != nil {
		return
	}

	for _, stream := range streams {
		var tag flvio.Tag
		var ok bool
		if tag, ok, err = CodecDataToTag(stream); err != nil {
			return
		}
		if ok {
			if err = flvio.WriteTag(muxer.bufw, tag, 0, muxer.b); err != nil {
				return
			}
		}
	}

	muxer.streams = streams
	return
}

// SetColorInfo sets the color info (HDR metadata, E-RTMP). It is written before the next video packet if
// the video codec is HEVC, AV1, VP9 or VVC, otherwise it is dropped. A nil color info resets it.
func (muxer *Muxer) SetColorInfo(colorInfo flvio.AMFMap) {
	muxer.colorInfo = colorInfo
	muxer.colorInfoPending = true
}

func (muxer *Muxer) WritePacket(pkt av.Packet) (err error) {
	stream := muxer.streams[pkt.Idx]
	tag, timestamp := PacketToTag(pkt, stream)

	if muxer.colorInfoPending && stream.Type().IsVideo() {
		muxer.colorInfoPending = false
		if colorInfoTag, ok := ColorInfoToTag(muxer.colorInfo, stream); ok {
			if err = flvio.WriteTag(muxer.bufw, colorInfoTag, timestamp, muxer.b); err != nil {
				return
			}
		}
	}

	if err = flvio.WriteTag(muxer.bufw, tag, timestamp, muxer.b); err != nil {
		return
	}
	return
}

func (muxer *Muxer) WriteTrailer() (err error) {
	if err = muxer.bufw.Flush(); err != nil {
		return
	}
	return
}

type Demuxer struct {
	prober *Prober
	bufr   *bufio.Reader
	b      []byte
	stage  int
}

func NewDemuxer(r io.Reader) *Demuxer {
	return &Demuxer{
		bufr:   bufio.NewReaderSize(r, pio.RecommendBufioSize),
		prober: &Prober{},
		b:      make([]byte, 256),
	}
}

func (demuxer *Demuxer) prepare() (err error) {
	for demuxer.stage < 2 {
		switch demuxer.stage {
		case 0:
			if _, err = io.ReadFull(demuxer.bufr, demuxer.b[:flvio.FileHeaderLength]); err != nil {
				return
			}
			var flags uint8
			var skip int
			if flags, skip, err = flvio.ParseFileHeader(demuxer.b); err != nil {
				return
			}
			if _, err = demuxer.bufr.Discard(skip); err != nil {
				return
			}
			if flags&flvio.FILE_HAS_AUDIO != 0 {
				demuxer.prober.HasAudio = true
			}
			if flags&flvio.FILE_HAS_VIDEO != 0 {
				demuxer.prober.HasVideo = true
			}
			demuxer.stage++

		case 1:
			for !demuxer.prober.Probed() {
				var tag flvio.Tag
				var timestamp int64
				if tag, timestamp, err = flvio.ReadTag(demuxer.bufr, demuxer.b); err != nil {
					return
				}
				if err = demuxer.prober.PushTag(tag, timestamp); err != nil {
					return
				}
			}
			demuxer.stage++
		}
	}
	return
}

func (demuxer *Demuxer) Streams() (streams []av.CodecData, err error) {
	if err = demuxer.prepare(); err != nil {
		return
	}
	streams = demuxer.prober.Streams
	return
}

// GetColorInfo returns the color info (HDR metadata, E-RTMP) that applies to the video packet read last and
// the following ones. It is nil if no color info has been received or it has been reset.
func (demuxer *Demuxer) GetColorInfo() flvio.AMFMap {
	return demuxer.prober.ColorInfo
}

func (demuxer *Demuxer) ReadPacket() (pkt av.Packet, err error) {
	if err = demuxer.prepare(); err != nil {
		return
	}

	if !demuxer.prober.Empty() {
		pkt = demuxer.prober.PopPacket()
		return
	}

	for {
		var tag flvio.Tag
		var timestamp int64
		if tag, timestamp, err = flvio.ReadTag(demuxer.bufr, demuxer.b); err != nil {
			return
		}

		var ok bool
		if pkt, ok = demuxer.prober.TagToPacket(tag, timestamp); ok {
			return
		}
	}
}

func Handler(h *avutil.RegisterHandler) {
	h.Probe = func(b []byte) bool {
		return b[0] == 'F' && b[1] == 'L' && b[2] == 'V'
	}

	h.Ext = ".flv"

	h.ReaderDemuxer = func(r io.Reader) av.Demuxer {
		return NewDemuxer(r)
	}

	h.WriterMuxer = func(w io.Writer) av.Muxer {
		return NewMuxer(w)
	}

	h.CodecTypes = CodecTypes
}
