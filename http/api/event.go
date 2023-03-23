package api

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/datarhei/core/v16/log"
)

type Event struct {
	Timestamp int64  `json:"ts" format:"int64"`
	Level     int    `json:"level"`
	Component string `json:"event"`
	Message   string `json:"message"`

	Data map[string]string `json:"data"`
}

func (e *Event) Marshal(le *log.Event) {
	e.Timestamp = le.Time.UnixMilli()
	e.Level = int(le.Level)
	e.Component = le.Component
	e.Message = le.Message

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
	if e.Component != ef.Component {
		return false
	}

	for k, r := range ef.filter {
		v, ok := e.Data[k]
		if !ok {
			continue
		}

		if !r.MatchString(v) {
			return false
		}
	}

	return true
}

type EventFilter struct {
	Component string            `json:"event"`
	Filter    map[string]string `json:"filter"`
	filter    map[string]*regexp.Regexp
}

func (ef *EventFilter) compile() error {
	ef.filter = make(map[string]*regexp.Regexp)

	for k, v := range ef.Filter {
		r, err := regexp.Compile(v)
		if err != nil {
			return err
		}

		ef.filter[k] = r
	}

	return nil
}
