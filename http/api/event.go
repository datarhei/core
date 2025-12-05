package api

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/event"
)

type LogEvent struct {
	Timestamp int64  `json:"ts" format:"int64"`
	Level     string `json:"level"`
	Component string `json:"event"`
	Message   string `json:"message"`
	Caller    string `json:"caller"`
	CoreID    string `json:"core_id,omitempty"`

	Data map[string]string `json:"data"`
}

func (e *LogEvent) Unmarshal(le event.Event) bool {
	evt, ok := le.(*event.LogEvent)
	if !ok {
		return false
	}

	e.Timestamp = evt.Time.Unix()
	e.Level = evt.Level
	e.Component = strings.ToLower(evt.Component)
	e.Message = evt.Message
	e.Caller = evt.Caller
	e.CoreID = evt.CoreID

	e.Data = make(map[string]string)

	for k, v := range evt.Data {
		var value string

		switch val := v.(type) {
		case string:
			value = val
		case error:
			value = val.Error()
		default:
			if s, ok := v.(fmt.Stringer); ok {
				value = s.String()
			} else {
				if jsonvalue, err := json.Marshal(v); err == nil {
					value = string(jsonvalue)
				} else {
					value = err.Error()
				}
			}
		}

		e.Data[k] = value
	}

	return true
}

func (e *LogEvent) Marshal() event.Event {
	evt := &event.LogEvent{
		Time:      time.UnixMilli(e.Timestamp),
		Level:     e.Level,
		Component: e.Component,
		Caller:    e.Caller,
		Message:   e.Message,
		CoreID:    e.CoreID,
		Data:      map[string]any{},
	}

	for k, v := range e.Data {
		evt.Data[k] = v
	}

	return evt
}

func (e *LogEvent) Filter(ef *LogEventFilter) bool {
	if ef.reMessage != nil {
		if !ef.reMessage.MatchString(e.Message) {
			return false
		}
	}

	if ef.reLevel != nil {
		if !ef.reLevel.MatchString(e.Level) {
			return false
		}
	}

	if len(e.Caller) != 0 && ef.reCaller != nil {
		if !ef.reCaller.MatchString(e.Caller) {
			return false
		}
	}

	if len(e.CoreID) != 0 && ef.reCoreID != nil {
		if !ef.reCoreID.MatchString(e.CoreID) {
			return false
		}
	}

	for k, r := range ef.reData {
		v, ok := e.Data[k]
		if !ok {
			return false
		}

		if !r.MatchString(v) {
			return false
		}
	}

	return true
}

type LogEventFilter struct {
	Component string            `json:"event"`
	Message   string            `json:"message"`
	Level     string            `json:"level"`
	Caller    string            `json:"caller"`
	CoreID    string            `json:"core_id"`
	Data      map[string]string `json:"data"`

	reMessage *regexp.Regexp
	reLevel   *regexp.Regexp
	reCaller  *regexp.Regexp
	reCoreID  *regexp.Regexp
	reData    map[string]*regexp.Regexp
}

type LogEventFilters struct {
	Filters []LogEventFilter `json:"filters"`
}

func (ef *LogEventFilter) Compile() error {
	if len(ef.Message) != 0 {
		r, err := regexp.Compile("(?i)" + ef.Message)
		if err != nil {
			return err
		}

		ef.reMessage = r
	}

	if len(ef.Level) != 0 {
		r, err := regexp.Compile("(?i)" + ef.Level)
		if err != nil {
			return err
		}

		ef.reLevel = r
	}

	if len(ef.Caller) != 0 {
		r, err := regexp.Compile("(?i)" + ef.Caller)
		if err != nil {
			return err
		}

		ef.reCaller = r
	}

	if len(ef.CoreID) != 0 {
		r, err := regexp.Compile("(?i)" + ef.CoreID)
		if err != nil {
			return err
		}

		ef.reCoreID = r
	}

	ef.reData = make(map[string]*regexp.Regexp)

	for k, v := range ef.Data {
		r, err := regexp.Compile("(?i)" + v)
		if err != nil {
			return err
		}

		ef.reData[k] = r
	}

	return nil
}

type MediaEvent struct {
	Action    string   `json:"action"`
	Name      string   `json:"name,omitempty"`
	Names     []string `json:"names,omitempty"`
	Timestamp int64    `json:"ts"`
}

func (p *MediaEvent) Unmarshal(e event.Event) bool {
	evt, ok := e.(*event.MediaEvent)
	if !ok {
		return false
	}

	p.Action = evt.Action
	p.Name = evt.Name
	p.Names = nil
	p.Timestamp = evt.Timestamp.UnixMilli()

	return true
}

type ProcessEvent struct {
	ProcessID string           `json:"pid"`
	Domain    string           `json:"domain"`
	Type      string           `json:"type"`
	Line      string           `json:"line,omitempty"`
	Progress  *ProcessProgress `json:"progress,omitempty"`
	CoreID    string           `json:"core_id,omitempty"`
	Timestamp int64            `json:"ts"`
}

type ProcessProgressInput struct {
	Bitrate  json.Number                  `json:"bitrate" swaggertype:"number" jsonschema:"type=number"`
	FPS      json.Number                  `json:"fps" swaggertype:"number" jsonschema:"type=number"`
	AVstream ProcessProgressInputAVstream `json:"avstream"`
}

func (p *ProcessProgressInput) Marshal() event.ProcessProgressInput {
	o := event.ProcessProgressInput{}

	if x, err := p.Bitrate.Float64(); err == nil {
		o.Bitrate = x
	}

	if x, err := p.FPS.Float64(); err == nil {
		o.FPS = x
	}

	o.AVstream = p.AVstream.Marshal()

	return o
}

type ProcessProgressInputAVstream struct {
	Looping bool   `json:"looping"`
	Enc     uint64 `json:"enc"`
	Drop    uint64 `json:"drop"`
	Dup     uint64 `json:"dup"`
	Time    uint64 `json:"time"`
}

func (p *ProcessProgressInputAVstream) Marshal() event.ProcessProgressInputAVstream {
	o := event.ProcessProgressInputAVstream{
		Looping: p.Looping,
		Enc:     p.Enc,
		Drop:    p.Drop,
		Dup:     p.Dup,
		Time:    p.Time,
	}

	return o
}

type ProcessProgressOutput struct {
	Bitrate json.Number `json:"bitrate" swaggertype:"number" jsonschema:"type=number"`
	FPS     json.Number `json:"fps" swaggertype:"number" jsonschema:"type=number"`
}

func (p *ProcessProgressOutput) Marshal() event.ProcessProgressOutput {
	o := event.ProcessProgressOutput{}

	if x, err := p.Bitrate.Float64(); err == nil {
		o.Bitrate = x
	}

	if x, err := p.FPS.Float64(); err == nil {
		o.FPS = x
	}

	return o
}

type ProcessProgress struct {
	Input  []ProcessProgressInput  `json:"input"`
	Output []ProcessProgressOutput `json:"output"`
	Time   json.Number             `json:"time" swaggertype:"number" jsonschema:"type=number"`
}

func (p *ProcessProgress) Unmarshal(e *event.ProcessProgress) {
	for _, io := range e.Input {
		p.Input = append(p.Input, ProcessProgressInput{
			Bitrate: json.ToNumber(io.Bitrate),
			FPS:     json.ToNumber(io.FPS),
			AVstream: ProcessProgressInputAVstream{
				Looping: io.AVstream.Looping,
				Enc:     io.AVstream.Enc,
				Drop:    io.AVstream.Drop,
				Dup:     io.AVstream.Dup,
				Time:    io.AVstream.Time,
			},
		})
	}

	for _, io := range e.Output {
		p.Output = append(p.Output, ProcessProgressOutput{
			Bitrate: json.ToNumber(io.Bitrate),
			FPS:     json.ToNumber(io.FPS),
		})
	}

	p.Time = json.ToNumber(e.Time)
}

func (p *ProcessProgress) Marshal() *event.ProcessProgress {
	e := &event.ProcessProgress{}

	if x, err := p.Time.Float64(); err == nil {
		e.Time = x
	}

	for _, input := range p.Input {
		e.Input = append(e.Input, input.Marshal())
	}

	for _, output := range p.Output {
		e.Output = append(e.Output, output.Marshal())
	}

	return e
}

func (p *ProcessEvent) Unmarshal(e event.Event) bool {
	evt, ok := e.(*event.ProcessEvent)
	if !ok {
		return false
	}

	p.ProcessID = evt.ProcessID
	p.Domain = evt.Domain
	p.Type = evt.Type
	p.Line = evt.Line
	if evt.Progress == nil {
		p.Progress = nil
	} else {
		p.Progress = &ProcessProgress{}
		p.Progress.Unmarshal(evt.Progress)
	}
	p.Timestamp = evt.Timestamp.UnixMilli()
	p.CoreID = evt.CoreID

	return true
}

func (p *ProcessEvent) Marshal() event.Event {
	evt := &event.ProcessEvent{
		ProcessID: p.ProcessID,
		Domain:    p.Domain,
		Type:      p.Type,
		Line:      p.Line,
		Progress:  nil,
		Timestamp: time.UnixMilli(p.Timestamp),
		CoreID:    p.CoreID,
	}

	if p.Progress != nil {
		evt.Progress = p.Progress.Marshal()
	}

	return evt
}

func (e *ProcessEvent) Filter(ef *ProcessEventFilter) bool {
	if ef.reProcessID != nil {
		if !ef.reProcessID.MatchString(e.ProcessID) {
			return false
		}
	}

	if ef.reDomain != nil {
		if !ef.reDomain.MatchString(e.Domain) {
			return false
		}
	}

	if ef.reType != nil {
		if !ef.reType.MatchString(e.Type) {
			return false
		}
	}

	if ef.reCoreID != nil {
		if !ef.reCoreID.MatchString(e.CoreID) {
			return false
		}
	}

	return true
}

type ProcessEventFilter struct {
	ProcessID string `json:"pid"`
	Domain    string `json:"domain"`
	Type      string `json:"type"`
	CoreID    string `json:"core_id"`

	reProcessID *regexp.Regexp
	reDomain    *regexp.Regexp
	reType      *regexp.Regexp
	reCoreID    *regexp.Regexp
}

type ProcessEventFilters struct {
	Filters []ProcessEventFilter `json:"filters"`
}

func (ef *ProcessEventFilter) Compile() error {
	if len(ef.ProcessID) != 0 {
		r, err := regexp.Compile("(?i)" + ef.ProcessID)
		if err != nil {
			return err
		}

		ef.reProcessID = r
	}

	if len(ef.Domain) != 0 {
		r, err := regexp.Compile("(?i)" + ef.Domain)
		if err != nil {
			return err
		}

		ef.reDomain = r
	}

	if len(ef.Type) != 0 {
		r, err := regexp.Compile("(?i)" + ef.Type)
		if err != nil {
			return err
		}

		ef.reType = r
	}

	if len(ef.CoreID) != 0 {
		r, err := regexp.Compile("(?i)" + ef.CoreID)
		if err != nil {
			return err
		}

		ef.reCoreID = r
	}

	return nil
}
