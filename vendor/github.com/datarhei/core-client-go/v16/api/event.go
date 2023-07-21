package api

type Event struct {
	Timestamp int64  `json:"ts" format:"int64"`
	Level     int    `json:"level"`
	Component string `json:"event"`
	Message   string `json:"message"`

	Data map[string]string `json:"data"`
}

type EventFilter struct {
	Component string            `json:"event"`
	Message   string            `json:"message"`
	Level     string            `json:"level"`
	Data      map[string]string `json:"data"`
}

type EventFilters struct {
	Filters []EventFilter `json:"filters"`
}
