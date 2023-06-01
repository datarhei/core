package api

// SkillsFilter represents an ffmpeg filter
type SkillsFilter struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SkillsHWAccel represents an ffmpeg HW accelerator
type SkillsHWAccel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SkillsCodec represents an ffmpeg codec
type SkillsCodec struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Encoders []string `json:"encoders"`
	Decoders []string `json:"decoders"`
}

// SkillsHWDevice represents an ffmpeg hardware device
type SkillsHWDevice struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Extra string `json:"extra"`
	Media string `json:"media"`
}

// SkillsDevice represents a group of ffmpeg hardware devices
type SkillsDevice struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Devices []SkillsHWDevice `json:"devices"`
}

// SkillsFormat represents an ffmpeg format
type SkillsFormat struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SkillsProtocol represents an ffmpeg protocol
type SkillsProtocol struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SkillsLibrary represents an avlib ffmpeg is compiled and linked with
type SkillsLibrary struct {
	Name     string `json:"name"`
	Compiled string `json:"compiled"`
	Linked   string `json:"linked"`
}

// Skills represents a set of ffmpeg capabilities
type Skills struct {
	FFmpeg struct {
		Version       string          `json:"version"`
		Compiler      string          `json:"compiler"`
		Configuration string          `json:"configuration"`
		Libraries     []SkillsLibrary `json:"libraries"`
	} `json:"ffmpeg"`

	Filters  []SkillsFilter  `json:"filter"`
	HWAccels []SkillsHWAccel `json:"hwaccels"`

	Codecs struct {
		Audio    []SkillsCodec `json:"audio"`
		Video    []SkillsCodec `json:"video"`
		Subtitle []SkillsCodec `json:"subtitle"`
	} `json:"codecs"`

	Devices struct {
		Demuxers []SkillsDevice `json:"demuxers"`
		Muxers   []SkillsDevice `json:"muxers"`
	} `json:"devices"`

	Formats struct {
		Demuxers []SkillsFormat `json:"demuxers"`
		Muxers   []SkillsFormat `json:"muxers"`
	} `json:"formats"`

	Protocols struct {
		Input  []SkillsProtocol `json:"input"`
		Output []SkillsProtocol `json:"output"`
	} `json:"protocols"`
}
