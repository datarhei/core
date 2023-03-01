package app

import (
	"time"
)

type LogLine struct {
	Timestamp time.Time
	Data      string
}

type LogEntry struct {
	CreatedAt time.Time
	Prelude   []string
	Log       []LogLine
}

type LogHistoryEntry struct {
	LogEntry

	ExitState string
	Progress  Progress
}

type Log struct {
	LogEntry
	History []LogHistoryEntry
}
