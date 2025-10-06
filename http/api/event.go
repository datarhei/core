package api

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/datarhei/core/v16/encoding/json"
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

type EventFilters struct {
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
