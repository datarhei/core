package api

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/log"
)

type Event struct {
	Timestamp int64  `json:"ts" format:"int64"`
	Level     int    `json:"level"`
	Component string `json:"event"`
	Message   string `json:"message"`
	Caller    string `json:"caller"`

	Data map[string]string `json:"data"`
}

func (e *Event) Marshal(le *log.Event) {
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

func (e *Event) Filter(ef *EventFilter) bool {
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

	if ef.reCaller != nil {
		if !ef.reCaller.MatchString(e.Caller) {
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

type EventFilter struct {
	Component string            `json:"event"`
	Message   string            `json:"message"`
	Level     string            `json:"level"`
	Caller    string            `json:"caller"`
	Data      map[string]string `json:"data"`

	reMessage *regexp.Regexp
	reLevel   *regexp.Regexp
	reCaller  *regexp.Regexp
	reData    map[string]*regexp.Regexp
}

type EventFilters struct {
	Filters []EventFilter `json:"filters"`
}

func (ef *EventFilter) Compile() error {
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
