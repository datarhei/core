package event

import (
	"time"
)

type MediaSource interface {
	EventSource

	MediaList() []string
}

type MediaEvent struct {
	Action    string
	Name      string
	Timestamp time.Time
}

func NewMediaEvent(action string, name string) *MediaEvent {
	return &MediaEvent{
		Action:    action,
		Name:      name,
		Timestamp: time.Now(),
	}
}

func (e *MediaEvent) Clone() Event {
	return &MediaEvent{
		Action:    e.Action,
		Name:      e.Name,
		Timestamp: e.Timestamp,
	}
}
