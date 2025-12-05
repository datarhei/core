package event

import (
	"maps"
	"time"
)

type LogEvent struct {
	Time      time.Time
	Level     string
	Component string
	Caller    string
	Message   string
	CoreID    string

	Data map[string]any
}

func (e *LogEvent) Clone() Event {
	evt := &LogEvent{
		Time:      e.Time,
		Level:     e.Level,
		Component: e.Component,
		Caller:    e.Caller,
		Message:   e.Message,
		CoreID:    e.CoreID,
		Data:      maps.Clone(e.Data),
	}

	return evt
}

func NewLogEvent(ts time.Time, level, component, caller, message string, data map[string]any) *LogEvent {
	return &LogEvent{
		Time:      ts,
		Level:     level,
		Component: component,
		Caller:    caller,
		Message:   message,
		Data:      maps.Clone(data),
	}
}
