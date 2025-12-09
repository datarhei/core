package event

import (
	"time"

	"github.com/datarhei/core/v16/global"
)

type ProcessEvent struct {
	ProcessID string
	Domain    string
	Type      string
	Line      string
	Progress  *ProcessProgress
	Timestamp time.Time
	CoreID    string
}

func (e *ProcessEvent) Clone() Event {
	evt := &ProcessEvent{
		ProcessID: e.ProcessID,
		Domain:    e.Domain,
		Type:      e.Type,
		Line:      e.Line,
		Timestamp: e.Timestamp,
		CoreID:    e.CoreID,
	}

	if e.Progress != nil {
		evt.Progress = e.Progress.Clone()
	}

	return evt
}

func NewProcessLogEvent(logline string) *ProcessEvent {
	return &ProcessEvent{
		Type:      "line",
		Line:      logline,
		Timestamp: time.Now(),
		CoreID:    global.GetCoreID(),
	}
}

func NewProcessProgressEvent(progress *ProcessProgress) *ProcessEvent {
	return &ProcessEvent{
		Type:      "progress",
		Progress:  progress,
		Timestamp: time.Now(),
		CoreID:    global.GetCoreID(),
	}
}

type ProcessProgressInput struct {
	ID       string
	URL      string
	Type     string
	Bitrate  float64
	FPS      float64
	AVstream ProcessProgressInputAVstream
}

func (p *ProcessProgressInput) Clone() ProcessProgressInput {
	c := ProcessProgressInput{
		ID:       p.ID,
		URL:      p.URL,
		Type:     p.Type,
		Bitrate:  p.Bitrate,
		FPS:      p.FPS,
		AVstream: p.AVstream.Clone(),
	}

	return c
}

type ProcessProgressInputAVstream struct {
	Enabled bool
	Looping bool
	Enc     uint64
	Drop    uint64
	Dup     uint64
	Time    uint64
}

func (p *ProcessProgressInputAVstream) Clone() ProcessProgressInputAVstream {
	c := ProcessProgressInputAVstream{
		Enabled: p.Enabled,
		Looping: p.Looping,
		Enc:     p.Enc,
		Drop:    p.Drop,
		Dup:     p.Dup,
		Time:    p.Time,
	}

	return c
}

type ProcessProgressOutput struct {
	ID      string
	URL     string
	Type    string
	Bitrate float64
	FPS     float64
}

func (p *ProcessProgressOutput) Clone() ProcessProgressOutput {
	c := ProcessProgressOutput{
		ID:      p.ID,
		URL:     p.URL,
		Type:    p.Type,
		Bitrate: p.Bitrate,
		FPS:     p.FPS,
	}

	return c
}

type ProcessProgress struct {
	Input  []ProcessProgressInput
	Output []ProcessProgressOutput
	Time   float64
}

func (p *ProcessProgress) Clone() *ProcessProgress {
	c := ProcessProgress{}

	for _, io := range p.Input {
		c.Input = append(c.Input, io.Clone())
	}

	for _, io := range p.Output {
		c.Output = append(c.Output, io.Clone())
	}

	c.Time = p.Time

	return &c
}
