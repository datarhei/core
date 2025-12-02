package event

import (
	"time"
)

type ProcessEvent struct {
	ProcessID string
	Domain    string
	Type      string
	Line      string
	Progress  *ProcessProgress
	Timestamp time.Time
}

func (e *ProcessEvent) Clone() Event {
	evt := &ProcessEvent{
		ProcessID: e.ProcessID,
		Domain:    e.Domain,
		Type:      e.Type,
		Line:      e.Line,
		Timestamp: e.Timestamp,
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
	}
}

func NewProcessProgressEvent(progress *ProcessProgress) *ProcessEvent {
	return &ProcessEvent{
		Type:      "progress",
		Progress:  progress,
		Timestamp: time.Now(),
	}
}

type ProcessProgressInput struct {
	Bitrate float64
	FPS     float64
	Looping bool
	Enc     uint64
	Drop    uint64
	Dup     uint64
}

func (p *ProcessProgressInput) Clone() ProcessProgressInput {
	c := ProcessProgressInput{
		Bitrate: p.Bitrate,
		FPS:     p.FPS,
		Looping: p.Looping,
		Enc:     p.Enc,
		Drop:    p.Drop,
		Dup:     p.Dup,
	}

	return c
}

type ProcessProgressOutput struct {
	Bitrate float64
	FPS     float64
}

func (p *ProcessProgressOutput) Clone() ProcessProgressOutput {
	c := ProcessProgressOutput{
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
