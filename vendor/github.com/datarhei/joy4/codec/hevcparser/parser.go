package hevcparser

// based on https://github.com/deepch/vdk/blob/v0.0.21/codec/h265parser/parser.go

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/datarhei/joy4/av"
	"github.com/datarhei/joy4/utils/bits"
	"github.com/datarhei/joy4/utils/bits/pio"
)

type SPSInfo struct {
	ProfileIdc                       uint // general_profile_idc
	LevelIdc                         uint // general_level_idc
	ChromaFormatIdc                  uint
	BitDepthLumaMinus8               uint
	BitDepthChromaMinus8             uint
	MaxSubLayersMinus1               uint // sps_max_sub_layers_minus1
	TemporalIdNestingFlag            uint // sps_temporal_id_nesting_flag
	CropLeft                         uint
	CropRight                        uint
	CropTop                          uint
	CropBottom                       uint
	Width                            uint
	Height                           uint
	PicWidthInLumaSamples            uint
	PicHeightInLumaSamples           uint
	generalProfileSpace              uint
	generalTierFlag                  uint
	generalProfileIDC                uint
	generalProfileCompatibilityFlags uint32
	generalConstraintIndicatorFlags  uint64
	generalLevelIDC                  uint
}

const (
	NAL_UNIT_CODED_SLICE_TRAIL_N    = 0
	NAL_UNIT_CODED_SLICE_TRAIL_R    = 1
	NAL_UNIT_CODED_SLICE_TSA_N      = 2
	NAL_UNIT_CODED_SLICE_TSA_R      = 3
	NAL_UNIT_CODED_SLICE_STSA_N     = 4
	NAL_UNIT_CODED_SLICE_STSA_R     = 5
	NAL_UNIT_CODED_SLICE_RADL_N     = 6
	NAL_UNIT_CODED_SLICE_RADL_R     = 7
	NAL_UNIT_CODED_SLICE_RASL_N     = 8
	NAL_UNIT_CODED_SLICE_RASL_R     = 9
	NAL_UNIT_RESERVED_VCL_N10       = 10
	NAL_UNIT_RESERVED_VCL_R11       = 11
	NAL_UNIT_RESERVED_VCL_N12       = 12
	NAL_UNIT_RESERVED_VCL_R13       = 13
	NAL_UNIT_RESERVED_VCL_N14       = 14
	NAL_UNIT_RESERVED_VCL_R15       = 15
	NAL_UNIT_CODED_SLICE_BLA_W_LP   = 16
	NAL_UNIT_CODED_SLICE_BLA_W_RADL = 17
	NAL_UNIT_CODED_SLICE_BLA_N_LP   = 18
	NAL_UNIT_CODED_SLICE_IDR_W_RADL = 19
	NAL_UNIT_CODED_SLICE_IDR_N_LP   = 20
	NAL_UNIT_CODED_SLICE_CRA        = 21
	NAL_UNIT_RESERVED_IRAP_VCL22    = 22
	NAL_UNIT_RESERVED_IRAP_VCL23    = 23
	NAL_UNIT_RESERVED_VCL24         = 24
	NAL_UNIT_RESERVED_VCL25         = 25
	NAL_UNIT_RESERVED_VCL26         = 26
	NAL_UNIT_RESERVED_VCL27         = 27
	NAL_UNIT_RESERVED_VCL28         = 28
	NAL_UNIT_RESERVED_VCL29         = 29
	NAL_UNIT_RESERVED_VCL30         = 30
	NAL_UNIT_RESERVED_VCL31         = 31
	NAL_UNIT_VPS                    = 32
	NAL_UNIT_SPS                    = 33
	NAL_UNIT_PPS                    = 34
	NAL_UNIT_ACCESS_UNIT_DELIMITER  = 35
	NAL_UNIT_EOS                    = 36
	NAL_UNIT_EOB                    = 37
	NAL_UNIT_FILLER_DATA            = 38
	NAL_UNIT_PREFIX_SEI             = 39
	NAL_UNIT_SUFFIX_SEI             = 40
	NAL_UNIT_RESERVED_NVCL41        = 41
	NAL_UNIT_RESERVED_NVCL42        = 42
	NAL_UNIT_RESERVED_NVCL43        = 43
	NAL_UNIT_RESERVED_NVCL44        = 44
	NAL_UNIT_RESERVED_NVCL45        = 45
	NAL_UNIT_RESERVED_NVCL46        = 46
	NAL_UNIT_RESERVED_NVCL47        = 47
	NAL_UNIT_UNSPECIFIED_48         = 48
	NAL_UNIT_UNSPECIFIED_49         = 49
	NAL_UNIT_UNSPECIFIED_50         = 50
	NAL_UNIT_UNSPECIFIED_51         = 51
	NAL_UNIT_UNSPECIFIED_52         = 52
	NAL_UNIT_UNSPECIFIED_53         = 53
	NAL_UNIT_UNSPECIFIED_54         = 54
	NAL_UNIT_UNSPECIFIED_55         = 55
	NAL_UNIT_UNSPECIFIED_56         = 56
	NAL_UNIT_UNSPECIFIED_57         = 57
	NAL_UNIT_UNSPECIFIED_58         = 58
	NAL_UNIT_UNSPECIFIED_59         = 59
	NAL_UNIT_UNSPECIFIED_60         = 60
	NAL_UNIT_UNSPECIFIED_61         = 61
	NAL_UNIT_UNSPECIFIED_62         = 62
	NAL_UNIT_UNSPECIFIED_63         = 63
	NAL_UNIT_INVALID                = 64
)

const (
	MAX_VPS_COUNT  = 16
	MAX_SUB_LAYERS = 7
	MAX_SPS_COUNT  = 32
)

var (
	ErrorHEVCIncorectUnitSize = errors.New("incorrect unit size")
	ErrorHECVIncorectUnitType = errors.New("incorrect unit type")
)

var StartCodeBytes = []byte{0, 0, 1}
var AUDBytes = []byte{0, 0, 0, 1, 0x9, 0xf0, 0, 0, 0, 1} // AUD

const (
	NALU_RAW = iota
	NALU_AVCC
	NALU_ANNEXB
)

func SplitNALUs(b []byte) (nalus [][]byte, typ int) {
	if len(b) < 4 {
		return [][]byte{b}, NALU_RAW
	}
	val3 := pio.U24BE(b)
	val4 := pio.U32BE(b)
	if val4 <= uint32(len(b)) {
		_val4 := val4
		_b := b[4:]
		nalus := [][]byte{}
		for {
			nalus = append(nalus, _b[:_val4])
			_b = _b[_val4:]
			if len(_b) < 4 {
				break
			}
			_val4 = pio.U32BE(_b)
			_b = _b[4:]
			if _val4 > uint32(len(_b)) {
				break
			}
		}
		if len(_b) == 0 {
			return nalus, NALU_AVCC
		}
	}
	if val3 == 1 || val4 == 1 {
		_val3 := val3
		_val4 := val4
		start := 0
		pos := 0
		for {
			if start != pos {
				nalus = append(nalus, b[start:pos])
			}
			if _val3 == 1 {
				pos += 3
			} else if _val4 == 1 {
				pos += 4
			}
			start = pos
			if start == len(b) {
				break
			}
			_val3 = 0
			_val4 = 0
			for pos < len(b) {
				if pos+2 < len(b) && b[pos] == 0 {
					_val3 = pio.U24BE(b[pos:])
					if _val3 == 0 {
						if pos+3 < len(b) {
							_val4 = uint32(b[pos+3])
							if _val4 == 1 {
								break
							}
						}
					} else if _val3 == 1 {
						break
					}
					pos++
				} else {
					pos++
				}
			}
		}
		typ = NALU_ANNEXB
		return
	}

	return [][]byte{b}, NALU_RAW
}

func ParseSPS(sps []byte) (ctx SPSInfo, err error) {
	if len(sps) < 2 {
		err = ErrorHEVCIncorectUnitSize
		return
	}
	rbsp := nal2rbsp(sps[2:])
	br := &bits.GolombBitReader{R: bytes.NewReader(rbsp)}

	// sps_video_parameter_set_id
	if _, err = br.ReadBits(4); err != nil {
		return
	}

	// sps_max_sub_layers_minus1
	spsMaxSubLayersMinus1, err := br.ReadBits(3)
	if err != nil {
		return
	}
	ctx.MaxSubLayersMinus1 = spsMaxSubLayersMinus1

	// sps_temporal_id_nesting_flag
	if ctx.TemporalIdNestingFlag, err = br.ReadBit(); err != nil {
		return
	}

	// profile_tier_level( 1, sps_max_sub_layers_minus1 )
	if err = parsePTL(br, &ctx, spsMaxSubLayersMinus1); err != nil {
		return
	}

	// sps_seq_parameter_set_id
	if _, err = br.ReadExponentialGolombCode(); err != nil {
		return
	}

	// chroma_format_idc
	var chroma_format_idc uint
	if chroma_format_idc, err = br.ReadExponentialGolombCode(); err != nil {
		return
	}
	ctx.ChromaFormatIdc = chroma_format_idc
	if chroma_format_idc == 3 {
		// separate_colour_plane_flag
		if _, err = br.ReadBit(); err != nil {
			return
		}
	}

	// Table 6-1, Section 6.2
	var subWidthC uint
	var subHeightC uint

	switch chroma_format_idc {
	case 0:
		subWidthC, subHeightC = 1, 1
	case 1:
		subWidthC, subHeightC = 2, 2
	case 2:
		subWidthC, subHeightC = 2, 1
	case 3:
		subWidthC, subHeightC = 1, 1
	}

	// pic_width_in_luma_samples
	if ctx.PicWidthInLumaSamples, err = br.ReadExponentialGolombCode(); err != nil {
		return
	}

	// pic_height_in_luma_samples
	if ctx.PicHeightInLumaSamples, err = br.ReadExponentialGolombCode(); err != nil {
		return
	}

	// conformance_window_flag
	conformanceWindowFlag, err := br.ReadBit()
	if err != nil {
		return
	}

	var conf_win_left_offset uint
	var conf_win_right_offset uint
	var conf_win_top_offset uint
	var conf_win_bottom_offset uint

	if conformanceWindowFlag != 0 {
		// conf_win_left_offset
		conf_win_left_offset, err = br.ReadExponentialGolombCode()
		if err != nil {
			return
		}
		ctx.CropLeft = subWidthC * conf_win_left_offset

		// conf_win_right_offset
		conf_win_right_offset, err = br.ReadExponentialGolombCode()
		if err != nil {
			return
		}
		ctx.CropRight = subWidthC * conf_win_right_offset

		// conf_win_top_offset
		conf_win_top_offset, err = br.ReadExponentialGolombCode()
		if err != nil {
			return
		}
		ctx.CropTop = subHeightC * conf_win_top_offset

		// conf_win_bottom_offset
		conf_win_bottom_offset, err = br.ReadExponentialGolombCode()
		if err != nil {
			return
		}
		ctx.CropBottom = subHeightC * conf_win_bottom_offset
	}

	ctx.Width = ctx.PicWidthInLumaSamples - ctx.CropLeft - ctx.CropRight
	ctx.Height = ctx.PicHeightInLumaSamples - ctx.CropTop - ctx.CropBottom

	// bit_depth_luma_minus8
	if ctx.BitDepthLumaMinus8, err = br.ReadExponentialGolombCode(); err != nil {
		return
	}

	// bit_depth_chroma_minus8
	if ctx.BitDepthChromaMinus8, err = br.ReadExponentialGolombCode(); err != nil {
		return
	}

	// log2_max_pic_order_cnt_lsb_minus4
	_, err = br.ReadExponentialGolombCode()
	if err != nil {
		return
	}

	// sps_sub_layer_ordering_info_present_flag
	spsSubLayerOrderingInfoPresentFlag, err := br.ReadBit()
	if err != nil {
		return
	}
	var i uint
	if spsSubLayerOrderingInfoPresentFlag != 0 {
		i = 0
	} else {
		i = spsMaxSubLayersMinus1
	}
	for ; i <= spsMaxSubLayersMinus1; i++ {
		// sps_max_dec_pic_buffering_minus1[ i ]
		if _, err = br.ReadExponentialGolombCode(); err != nil {
			return
		}
		// sps_max_num_reorder_pics[ i ]
		if _, err = br.ReadExponentialGolombCode(); err != nil {
			return
		}
		// sps_max_latency_increase_plus1[ i ]
		if _, err = br.ReadExponentialGolombCode(); err != nil {
			return
		}
	}

	// log2_min_luma_coding_block_size_minus3
	if _, err = br.ReadExponentialGolombCode(); err != nil {
		return
	}
	// log2_diff_max_min_luma_coding_block_size
	if _, err = br.ReadExponentialGolombCode(); err != nil {
		return
	}
	// log2_min_luma_transform_block_size_minus2
	if _, err = br.ReadExponentialGolombCode(); err != nil {
		return
	}
	// log2_diff_max_min_luma_transform_block_size
	if _, err = br.ReadExponentialGolombCode(); err != nil {
		return
	}
	// max_transform_hierarchy_depth_inter
	if _, err = br.ReadExponentialGolombCode(); err != nil {
		return
	}
	// max_transform_hierarchy_depth_intra
	if _, err = br.ReadExponentialGolombCode(); err != nil {
		return
	}
	return
}

func parsePTL(br *bits.GolombBitReader, ctx *SPSInfo, maxSubLayersMinus1 uint) error {
	var err error
	if ctx.generalProfileSpace, err = br.ReadBits(2); err != nil {
		return err
	}
	if ctx.generalTierFlag, err = br.ReadBit(); err != nil {
		return err
	}
	if ctx.generalProfileIDC, err = br.ReadBits(5); err != nil {
		return err
	}
	if ctx.generalProfileCompatibilityFlags, err = br.ReadBits32(32); err != nil {
		return err
	}
	if ctx.generalConstraintIndicatorFlags, err = br.ReadBits64(48); err != nil {
		return err
	}
	if ctx.generalLevelIDC, err = br.ReadBits(8); err != nil {
		return err
	}
	ctx.ProfileIdc = ctx.generalProfileIDC
	ctx.LevelIdc = ctx.generalLevelIDC
	if maxSubLayersMinus1 == 0 {
		return nil
	}
	subLayerProfilePresentFlag := make([]uint, maxSubLayersMinus1)
	subLayerLevelPresentFlag := make([]uint, maxSubLayersMinus1)
	for i := uint(0); i < maxSubLayersMinus1; i++ {
		if subLayerProfilePresentFlag[i], err = br.ReadBit(); err != nil {
			return err
		}
		if subLayerLevelPresentFlag[i], err = br.ReadBit(); err != nil {
			return err
		}
	}
	if maxSubLayersMinus1 > 0 {
		for i := maxSubLayersMinus1; i < 8; i++ {
			if _, err = br.ReadBits(2); err != nil {
				return err
			}
		}
	}
	for i := uint(0); i < maxSubLayersMinus1; i++ {
		if subLayerProfilePresentFlag[i] != 0 {
			if _, err = br.ReadBits32(32); err != nil {
				return err
			}
			if _, err = br.ReadBits32(32); err != nil {
				return err
			}
			if _, err = br.ReadBits32(24); err != nil {
				return err
			}
		}

		if subLayerLevelPresentFlag[i] != 0 {
			if _, err = br.ReadBits(8); err != nil {
				return err
			}
		}
	}
	return nil
}

func nal2rbsp(nal []byte) []byte {
	return bytes.Replace(nal, []byte{0x0, 0x0, 0x3}, []byte{0x0, 0x0}, -1)
}

type CodecData struct {
	Record     []byte
	RecordInfo HEVCDecoderConfRecord
	SPSInfo    SPSInfo
}

func (codec CodecData) Type() av.CodecType {
	return av.HEVC
}

func (codec CodecData) HEVCDecoderConfRecordBytes() []byte {
	return codec.Record
}

func (codec CodecData) SPS() []byte {
	return codec.RecordInfo.SPS[0]
}

func (codec CodecData) PPS() []byte {
	return codec.RecordInfo.PPS[0]
}

func (codec CodecData) VPS() []byte {
	return codec.RecordInfo.VPS[0]
}

func (codec CodecData) Width() int {
	return int(codec.SPSInfo.Width)
}

func (codec CodecData) Height() int {
	return int(codec.SPSInfo.Height)
}

func NewCodecDataFromHEVCDecoderConfRecord(record []byte) (self CodecData, err error) {
	self.Record = record
	if _, err = (&self.RecordInfo).Unmarshal(record); err != nil {
		return
	}
	if len(self.RecordInfo.SPS) == 0 {
		err = fmt.Errorf("hevcparser: no SPS found in HEVCDecoderConfRecord")
		return
	}
	if len(self.RecordInfo.PPS) == 0 {
		err = fmt.Errorf("hevcparser: no PPS found in HEVCDecoderConfRecord")
		return
	}
	if len(self.RecordInfo.VPS) == 0 {
		err = fmt.Errorf("hevcparser: no VPS found in HEVCDecoderConfRecord")
		return
	}
	if self.SPSInfo, err = ParseSPS(self.RecordInfo.SPS[0]); err != nil {
		err = fmt.Errorf("hevcparser: parse SPS failed(%s)", err)
		return
	}
	return
}

func NewCodecDataFromVPSAndSPSAndPPS(vps, sps, pps []byte) (self CodecData, err error) {
	if self.SPSInfo, err = ParseSPS(sps); err != nil {
		return
	}
	si := self.SPSInfo
	recordinfo := HEVCDecoderConfRecord{
		GeneralProfileSpace:              uint8(si.generalProfileSpace),
		GeneralTierFlag:                  uint8(si.generalTierFlag),
		GeneralProfileIDC:                uint8(si.generalProfileIDC),
		GeneralProfileCompatibilityFlags: si.generalProfileCompatibilityFlags,
		GeneralConstraintIndicatorFlags:  si.generalConstraintIndicatorFlags,
		GeneralLevelIDC:                  uint8(si.generalLevelIDC),
		ChromaFormatIDC:                  uint8(si.ChromaFormatIdc),
		BitDepthLumaMinus8:               uint8(si.BitDepthLumaMinus8),
		BitDepthChromaMinus8:             uint8(si.BitDepthChromaMinus8),
		NumTemporalLayers:                uint8(si.MaxSubLayersMinus1 + 1),
		TemporalIDNested:                 uint8(si.TemporalIdNestingFlag),
		LengthSizeMinusOne:               3,
		VPS:                              [][]byte{vps},
		SPS:                              [][]byte{sps},
		PPS:                              [][]byte{pps},
	}
	recordinfo.setIndications()
	buf := make([]byte, recordinfo.Len())
	recordinfo.Marshal(buf, self.SPSInfo)
	self.RecordInfo = recordinfo
	self.Record = buf
	return
}

// HEVCDecoderConfRecord is the HEVCDecoderConfigurationRecord (hvcC) of ISO/IEC 14496-15, 8.3.3.1.
type HEVCDecoderConfRecord struct {
	// HEVCProfileIndication is general_profile_space, general_tier_flag and general_profile_idc as in
	// the record (byte 1), ProfileCompatibility is the first byte of general_profile_compatibility_flags
	// and HEVCLevelIndication is general_level_idc. They are set by Unmarshal and are not used by Marshal.
	HEVCProfileIndication uint8
	ProfileCompatibility  uint8
	HEVCLevelIndication   uint8

	GeneralProfileSpace              uint8
	GeneralTierFlag                  uint8
	GeneralProfileIDC                uint8
	GeneralProfileCompatibilityFlags uint32
	GeneralConstraintIndicatorFlags  uint64 // 48 bits
	GeneralLevelIDC                  uint8
	MinSpatialSegmentationIDC        uint16
	ParallelismType                  uint8
	ChromaFormatIDC                  uint8
	BitDepthLumaMinus8               uint8
	BitDepthChromaMinus8             uint8
	AvgFrameRate                     uint16
	ConstantFrameRate                uint8
	NumTemporalLayers                uint8
	TemporalIDNested                 uint8
	LengthSizeMinusOne               uint8

	// the NAL units of the arrays of the types VPS, SPS and PPS; arrays of other types (e.g. SEI) are skipped
	VPS [][]byte
	SPS [][]byte
	PPS [][]byte
}

var ErrDecconfInvalid = fmt.Errorf("hevcparser: HEVCDecoderConfRecord invalid")

const hevcDecoderConfRecordHeaderLength = 23

func (record *HEVCDecoderConfRecord) setIndications() {
	record.HEVCProfileIndication = record.GeneralProfileSpace<<6 | record.GeneralTierFlag<<5 | record.GeneralProfileIDC
	record.ProfileCompatibility = uint8(record.GeneralProfileCompatibilityFlags >> 24)
	record.HEVCLevelIndication = record.GeneralLevelIDC
}

func (record *HEVCDecoderConfRecord) Unmarshal(b []byte) (n int, err error) {
	if len(b) < hevcDecoderConfRecordHeaderLength {
		err = ErrDecconfInvalid
		return
	}

	record.GeneralProfileSpace = b[1] >> 6
	record.GeneralTierFlag = (b[1] >> 5) & 0x01
	record.GeneralProfileIDC = b[1] & 0x1f
	record.GeneralProfileCompatibilityFlags = pio.U32BE(b[2:])
	record.GeneralConstraintIndicatorFlags = uint64(pio.U16BE(b[6:]))<<32 | uint64(pio.U32BE(b[8:]))
	record.GeneralLevelIDC = b[12]
	record.MinSpatialSegmentationIDC = pio.U16BE(b[13:]) & 0x0fff
	record.ParallelismType = b[15] & 0x03
	record.ChromaFormatIDC = b[16] & 0x03
	record.BitDepthLumaMinus8 = b[17] & 0x07
	record.BitDepthChromaMinus8 = b[18] & 0x07
	record.AvgFrameRate = pio.U16BE(b[19:])
	record.ConstantFrameRate = b[21] >> 6
	record.NumTemporalLayers = (b[21] >> 3) & 0x07
	record.TemporalIDNested = (b[21] >> 2) & 0x01
	record.LengthSizeMinusOne = b[21] & 0x03
	record.setIndications()

	record.VPS, record.SPS, record.PPS = nil, nil, nil

	numOfArrays := int(b[22])
	n = hevcDecoderConfRecordHeaderLength

	for i := 0; i < numOfArrays; i++ {
		if len(b) < n+3 {
			err = ErrDecconfInvalid
			return
		}
		naluType := b[n] & 0x3f
		numNalus := int(pio.U16BE(b[n+1:]))
		n += 3

		for j := 0; j < numNalus; j++ {
			if len(b) < n+2 {
				err = ErrDecconfInvalid
				return
			}
			naluLen := int(pio.U16BE(b[n:]))
			n += 2

			if len(b) < n+naluLen {
				err = ErrDecconfInvalid
				return
			}
			nalu := b[n : n+naluLen]
			n += naluLen

			switch naluType {
			case NAL_UNIT_VPS:
				record.VPS = append(record.VPS, nalu)
			case NAL_UNIT_SPS:
				record.SPS = append(record.SPS, nalu)
			case NAL_UNIT_PPS:
				record.PPS = append(record.PPS, nalu)
			}
		}
	}

	return
}

func (record HEVCDecoderConfRecord) arrays() [][][]byte {
	arrays := [][][]byte{}
	for _, nalus := range [][][]byte{record.VPS, record.SPS, record.PPS} {
		if len(nalus) != 0 {
			arrays = append(arrays, nalus)
		}
	}
	return arrays
}

func (record HEVCDecoderConfRecord) Len() (n int) {
	n = hevcDecoderConfRecordHeaderLength
	for _, nalus := range record.arrays() {
		n += 3
		for _, nalu := range nalus {
			n += 2 + len(nalu)
		}
	}
	return
}

// Marshal writes the record to b, which must have a length of at least Len(). The header fields are taken
// from the record, si is not used.
func (record HEVCDecoderConfRecord) Marshal(b []byte, si SPSInfo) (n int) {
	b[0] = 1 // configurationVersion
	b[1] = record.GeneralProfileSpace<<6 | (record.GeneralTierFlag&0x01)<<5 | record.GeneralProfileIDC&0x1f
	pio.PutU32BE(b[2:], record.GeneralProfileCompatibilityFlags)
	pio.PutU16BE(b[6:], uint16(record.GeneralConstraintIndicatorFlags>>32))
	pio.PutU32BE(b[8:], uint32(record.GeneralConstraintIndicatorFlags))
	b[12] = record.GeneralLevelIDC
	// the reserved bits are 1
	pio.PutU16BE(b[13:], 0xf000|record.MinSpatialSegmentationIDC&0x0fff)
	b[15] = 0xfc | record.ParallelismType&0x03
	b[16] = 0xfc | record.ChromaFormatIDC&0x03
	b[17] = 0xf8 | record.BitDepthLumaMinus8&0x07
	b[18] = 0xf8 | record.BitDepthChromaMinus8&0x07
	pio.PutU16BE(b[19:], record.AvgFrameRate)
	b[21] = record.ConstantFrameRate<<6 | (record.NumTemporalLayers&0x07)<<3 | (record.TemporalIDNested&0x01)<<2 | record.LengthSizeMinusOne&0x03

	arrays := record.arrays()
	b[22] = uint8(len(arrays))
	n = hevcDecoderConfRecordHeaderLength

	for _, nalus := range arrays {
		// array_completeness and the reserved bit are 0
		b[n] = 0
		if len(nalus[0]) != 0 {
			b[n] = (nalus[0][0] >> 1) & 0x3f
		}
		pio.PutU16BE(b[n+1:], uint16(len(nalus)))
		n += 3
		for _, nalu := range nalus {
			pio.PutU16BE(b[n:], uint16(len(nalu)))
			n += 2
			copy(b[n:], nalu)
			n += len(nalu)
		}
	}
	return
}
