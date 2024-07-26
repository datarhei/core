package process

import (
	"time"
)

// Parser is an interface that is accepted by a process for parsing
// the process' output.
type Parser interface {
	// Parse parses the given line and returns an indicator
	// for progress (e.g. based on the contents of the line,
	// or previous line, ...)
	Parse(line []byte) uint64

	// Stop tells the parser that the process stopped and provides
	// its exit state.
	Stop(state string, usage Usage)

	// Reset resets any collected statistics or temporary data.
	// This is called before the process starts and after the
	// process stopped. The stats are meant to be collected
	// during the runtime of the process.
	ResetStats()

	// ResetLog resets any collected logs. This is called
	// before the process starts.
	ResetLog()

	// Log returns a slice of collected log lines.
	Log() []Line

	// IsRunning returns whether the process is producing output.
	IsRunning() bool
}

// Line represents a line from the output with its timestamp. The
// line doesn't include any newline character.
type Line struct {
	Timestamp time.Time
	Data      string
}

type nullParser struct{}

// NewNullParser returns a dummy parser that isn't doing anything
// and always returns progress.
func NewNullParser() Parser {
	return &nullParser{}
}

var _ Parser = &nullParser{}

func (p *nullParser) Parse(line []byte) uint64 { return 1 }
func (p *nullParser) Stop(string, Usage)       {}
func (p *nullParser) ResetStats()              {}
func (p *nullParser) ResetLog()                {}
func (p *nullParser) Log() []Line              { return []Line{} }
func (p *nullParser) IsRunning() bool          { return true }

type bufferParser struct {
	log []Line
}

// NewBufferParser returns a dummy parser that is just storing
// the lines and returns progress.
func NewBufferParser() Parser {
	return &bufferParser{}
}

var _ Parser = &bufferParser{}

func (p *bufferParser) Parse(line []byte) uint64 {
	p.log = append(p.log, Line{
		Timestamp: time.Now(),
		Data:      string(line),
	})
	return 1
}
func (p *bufferParser) Stop(string, Usage) {}
func (p *bufferParser) ResetStats()        {}
func (p *bufferParser) ResetLog() {
	p.log = []Line{}
}
func (p *bufferParser) Log() []Line     { return p.log }
func (p *bufferParser) IsRunning() bool { return true }
