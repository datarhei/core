// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package models

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/datarhei/core/v16/http/graph/scalars"
)

type IProcessReportHistoryEntry interface {
	IsIProcessReportHistoryEntry()
	GetCreatedAt() time.Time
	GetPrelude() []string
	GetLog() []*ProcessReportLogEntry
}

type AVStream struct {
	Input       *AVStreamIo    `json:"input"`
	Output      *AVStreamIo    `json:"output"`
	Aqueue      scalars.Uint64 `json:"aqueue"`
	Queue       scalars.Uint64 `json:"queue"`
	Dup         scalars.Uint64 `json:"dup"`
	Drop        scalars.Uint64 `json:"drop"`
	Enc         scalars.Uint64 `json:"enc"`
	Looping     bool           `json:"looping"`
	Duplicating bool           `json:"duplicating"`
	Gop         string         `json:"gop"`
}

type AVStreamIo struct {
	State  string         `json:"state"`
	Packet scalars.Uint64 `json:"packet"`
	Time   scalars.Uint64 `json:"time"`
	SizeKb scalars.Uint64 `json:"size_kb"`
}

type About struct {
	App           string         `json:"app"`
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	CreatedAt     time.Time      `json:"created_at"`
	UptimeSeconds scalars.Uint64 `json:"uptime_seconds"`
	Version       *AboutVersion  `json:"version"`
}

type AboutVersion struct {
	Number           string `json:"number"`
	RepositoryCommit string `json:"repository_commit"`
	RepositoryBranch string `json:"repository_branch"`
	BuildDate        string `json:"build_date"`
	Arch             string `json:"arch"`
	Compiler         string `json:"compiler"`
}

type Metric struct {
	Name   string                          `json:"name"`
	Labels map[string]interface{}          `json:"labels"`
	Values []*scalars.MetricsResponseValue `json:"values"`
}

type MetricInput struct {
	Name   string                 `json:"name"`
	Labels map[string]interface{} `json:"labels"`
}

type Metrics struct {
	TimerangeSeconds *int      `json:"timerange_seconds"`
	IntervalSeconds  *int      `json:"interval_seconds"`
	Metrics          []*Metric `json:"metrics"`
}

type MetricsInput struct {
	TimerangeSeconds *int           `json:"timerange_seconds"`
	IntervalSeconds  *int           `json:"interval_seconds"`
	Metrics          []*MetricInput `json:"metrics"`
}

type Probe struct {
	Streams []*ProbeIo `json:"streams"`
	Log     []string   `json:"log"`
}

type ProbeIo struct {
	URL             string         `json:"url"`
	Index           scalars.Uint64 `json:"index"`
	Stream          scalars.Uint64 `json:"stream"`
	Language        string         `json:"language"`
	Type            string         `json:"type"`
	Codec           string         `json:"codec"`
	Coder           string         `json:"coder"`
	BitrateKbps     float64        `json:"bitrate_kbps"`
	DurationSeconds float64        `json:"duration_seconds"`
	Fps             float64        `json:"fps"`
	PixFmt          string         `json:"pix_fmt"`
	Width           scalars.Uint64 `json:"width"`
	Height          scalars.Uint64 `json:"height"`
	Sampling        scalars.Uint64 `json:"sampling"`
	Layout          string         `json:"layout"`
	Channels        scalars.Uint64 `json:"channels"`
}

type Process struct {
	ID        string                 `json:"id"`
	Owner     string                 `json:"owner"`
	Domain    string                 `json:"domain"`
	Type      string                 `json:"type"`
	Reference string                 `json:"reference"`
	CreatedAt time.Time              `json:"created_at"`
	Config    *ProcessConfig         `json:"config"`
	State     *ProcessState          `json:"state"`
	Report    *ProcessReport         `json:"report"`
	Metadata  map[string]interface{} `json:"metadata"`
}

type ProcessConfig struct {
	ID                    string               `json:"id"`
	Owner                 string               `json:"owner"`
	Domain                string               `json:"domain"`
	Type                  string               `json:"type"`
	Reference             string               `json:"reference"`
	Input                 []*ProcessConfigIo   `json:"input"`
	Output                []*ProcessConfigIo   `json:"output"`
	Options               []string             `json:"options"`
	Reconnect             bool                 `json:"reconnect"`
	ReconnectDelaySeconds scalars.Uint64       `json:"reconnect_delay_seconds"`
	Autostart             bool                 `json:"autostart"`
	StaleTimeoutSeconds   scalars.Uint64       `json:"stale_timeout_seconds"`
	Limits                *ProcessConfigLimits `json:"limits"`
}

type ProcessConfigIo struct {
	ID      string   `json:"id"`
	Address string   `json:"address"`
	Options []string `json:"options"`
}

type ProcessConfigLimits struct {
	CPUUsage       float64        `json:"cpu_usage"`
	MemoryBytes    scalars.Uint64 `json:"memory_bytes"`
	WaitforSeconds scalars.Uint64 `json:"waitfor_seconds"`
}

type ProcessReport struct {
	CreatedAt time.Time                    `json:"created_at"`
	Prelude   []string                     `json:"prelude"`
	Log       []*ProcessReportLogEntry     `json:"log"`
	History   []*ProcessReportHistoryEntry `json:"history"`
}

func (ProcessReport) IsIProcessReportHistoryEntry() {}
func (this ProcessReport) GetCreatedAt() time.Time  { return this.CreatedAt }
func (this ProcessReport) GetPrelude() []string {
	if this.Prelude == nil {
		return nil
	}
	interfaceSlice := make([]string, 0, len(this.Prelude))
	for _, concrete := range this.Prelude {
		interfaceSlice = append(interfaceSlice, concrete)
	}
	return interfaceSlice
}
func (this ProcessReport) GetLog() []*ProcessReportLogEntry {
	if this.Log == nil {
		return nil
	}
	interfaceSlice := make([]*ProcessReportLogEntry, 0, len(this.Log))
	for _, concrete := range this.Log {
		interfaceSlice = append(interfaceSlice, concrete)
	}
	return interfaceSlice
}

type ProcessReportHistoryEntry struct {
	CreatedAt time.Time                `json:"created_at"`
	Prelude   []string                 `json:"prelude"`
	Log       []*ProcessReportLogEntry `json:"log"`
}

func (ProcessReportHistoryEntry) IsIProcessReportHistoryEntry() {}
func (this ProcessReportHistoryEntry) GetCreatedAt() time.Time  { return this.CreatedAt }
func (this ProcessReportHistoryEntry) GetPrelude() []string {
	if this.Prelude == nil {
		return nil
	}
	interfaceSlice := make([]string, 0, len(this.Prelude))
	for _, concrete := range this.Prelude {
		interfaceSlice = append(interfaceSlice, concrete)
	}
	return interfaceSlice
}
func (this ProcessReportHistoryEntry) GetLog() []*ProcessReportLogEntry {
	if this.Log == nil {
		return nil
	}
	interfaceSlice := make([]*ProcessReportLogEntry, 0, len(this.Log))
	for _, concrete := range this.Log {
		interfaceSlice = append(interfaceSlice, concrete)
	}
	return interfaceSlice
}

type ProcessReportLogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
}

type ProcessState struct {
	Order            string         `json:"order"`
	State            string         `json:"state"`
	RuntimeSeconds   scalars.Uint64 `json:"runtime_seconds"`
	ReconnectSeconds int            `json:"reconnect_seconds"`
	LastLogline      string         `json:"last_logline"`
	Progress         *Progress      `json:"progress"`
	MemoryBytes      scalars.Uint64 `json:"memory_bytes"`
	CPUUsage         float64        `json:"cpu_usage"`
	Command          []string       `json:"command"`
}

type Progress struct {
	Input       []*ProgressIo  `json:"input"`
	Output      []*ProgressIo  `json:"output"`
	Frame       scalars.Uint64 `json:"frame"`
	Packet      scalars.Uint64 `json:"packet"`
	Fps         float64        `json:"fps"`
	Q           float64        `json:"q"`
	SizeKb      scalars.Uint64 `json:"size_kb"`
	Time        float64        `json:"time"`
	BitrateKbit float64        `json:"bitrate_kbit"`
	Speed       float64        `json:"speed"`
	Drop        scalars.Uint64 `json:"drop"`
	Dup         scalars.Uint64 `json:"dup"`
}

type ProgressIo struct {
	ID          string         `json:"id"`
	Address     string         `json:"address"`
	Index       scalars.Uint64 `json:"index"`
	Stream      scalars.Uint64 `json:"stream"`
	Format      string         `json:"format"`
	Type        string         `json:"type"`
	Codec       string         `json:"codec"`
	Coder       string         `json:"coder"`
	Frame       scalars.Uint64 `json:"frame"`
	Fps         float64        `json:"fps"`
	Packet      scalars.Uint64 `json:"packet"`
	Pps         float64        `json:"pps"`
	SizeKb      scalars.Uint64 `json:"size_kb"`
	BitrateKbit float64        `json:"bitrate_kbit"`
	Pixfmt      string         `json:"pixfmt"`
	Q           float64        `json:"q"`
	Width       scalars.Uint64 `json:"width"`
	Height      scalars.Uint64 `json:"height"`
	Sampling    scalars.Uint64 `json:"sampling"`
	Layout      string         `json:"layout"`
	Channels    scalars.Uint64 `json:"channels"`
	Avstream    *AVStream      `json:"avstream"`
}

type RawAVstream struct {
	ID          string           `json:"id"`
	URL         string           `json:"url"`
	Stream      scalars.Uint64   `json:"stream"`
	Queue       scalars.Uint64   `json:"queue"`
	Aqueue      scalars.Uint64   `json:"aqueue"`
	Dup         scalars.Uint64   `json:"dup"`
	Drop        scalars.Uint64   `json:"drop"`
	Enc         scalars.Uint64   `json:"enc"`
	Looping     bool             `json:"looping"`
	Duplicating bool             `json:"duplicating"`
	Gop         string           `json:"gop"`
	Debug       interface{}      `json:"debug"`
	Input       *RawAVstreamIo   `json:"input"`
	Output      *RawAVstreamIo   `json:"output"`
	Swap        *RawAVstreamSwap `json:"swap"`
}

type RawAVstreamIo struct {
	State  State          `json:"state"`
	Packet scalars.Uint64 `json:"packet"`
	Time   scalars.Uint64 `json:"time"`
	SizeKb scalars.Uint64 `json:"size_kb"`
}

type RawAVstreamSwap struct {
	URL       string `json:"url"`
	Status    string `json:"status"`
	Lasturl   string `json:"lasturl"`
	Lasterror string `json:"lasterror"`
}

type Command string

const (
	CommandStart   Command = "START"
	CommandStop    Command = "STOP"
	CommandRestart Command = "RESTART"
	CommandReload  Command = "RELOAD"
)

var AllCommand = []Command{
	CommandStart,
	CommandStop,
	CommandRestart,
	CommandReload,
}

func (e Command) IsValid() bool {
	switch e {
	case CommandStart, CommandStop, CommandRestart, CommandReload:
		return true
	}
	return false
}

func (e Command) String() string {
	return string(e)
}

func (e *Command) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = Command(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid Command", str)
	}
	return nil
}

func (e Command) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

type State string

const (
	StateRunning State = "RUNNING"
	StateIDLe    State = "IDLE"
)

var AllState = []State{
	StateRunning,
	StateIDLe,
}

func (e State) IsValid() bool {
	switch e {
	case StateRunning, StateIDLe:
		return true
	}
	return false
}

func (e State) String() string {
	return string(e)
}

func (e *State) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = State(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid State", str)
	}
	return nil
}

func (e State) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}
