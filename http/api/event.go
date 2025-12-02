package api

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/event"
	"github.com/datarhei/core/v16/log"
)

type LogEvent struct {
	Timestamp int64  `json:"ts" format:"int64"`
	Level     int    `json:"level"`
	Component string `json:"event"`
	Message   string `json:"message"`
	Caller    string `json:"caller"`
	CoreID    string `json:"core_id,omitempty"`

	Data map[string]string `json:"data"`
}

func (e *LogEvent) Unmarshal(le *log.Event) {
	e.Timestamp = le.Time.Unix()
	e.Level = int(le.Level)
	e.Component = strings.ToLower(le.Component)
	e.Message = le.Message
	e.Caller = le.Caller

	e.Data = make(map[string]string)

	for k, v := range le.Data {
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
}

func (e *LogEvent) Filter(ef *LogEventFilter) bool {
	if ef.reMessage != nil {
		if !ef.reMessage.MatchString(e.Message) {
			return false
		}
	}

	if ef.reLevel != nil {
		level := log.Level(e.Level).String()
		if !ef.reLevel.MatchString(level) {
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
	Timestamp int64            `json:"ts"`
}

type ProcessProgressInput struct {
	Bitrate json.Number `json:"bitrate" swaggertype:"number" jsonschema:"type=number"`
	FPS     json.Number `json:"fps" swaggertype:"number" jsonschema:"type=number"`
	Looping bool        `json:"looping"`
	Enc     uint64      `json:"enc"`
	Drop    uint64      `json:"drop"`
	Dup     uint64      `json:"dup"`
}

type ProcessProgressOutput struct {
	Bitrate json.Number `json:"bitrate" swaggertype:"number" jsonschema:"type=number"`
	FPS     json.Number `json:"fps" swaggertype:"number" jsonschema:"type=number"`
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
			Looping: io.Looping,
			Enc:     io.Enc,
			Drop:    io.Drop,
			Dup:     io.Dup,
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

	return true
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

	return true
}

type ProcessEventFilter struct {
	ProcessID string `json:"pid"`
	Domain    string `json:"domain"`
	Type      string `json:"type"`

	reProcessID *regexp.Regexp
	reDomain    *regexp.Regexp
	reType      *regexp.Regexp
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

	return nil
}
