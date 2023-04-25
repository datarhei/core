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
	Matches   []string
}

type LogHistoryEntry struct {
	LogEntry

	ExitedAt  time.Time
	ExitState string
	Progress  Progress
	Usage     ProcessUsage
}

type Log struct {
	LogEntry
	History []LogHistoryEntry
}

type LogHistorySearchResult struct {
	ProcessID string
	Reference string
	ExitState string
	ExitedAt  time.Time
	CreatedAt time.Time
}
