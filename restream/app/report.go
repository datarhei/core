package app

import (
	"time"
)

type LogLine struct {
	Timestamp time.Time
	Data      string
}

type ReportEntry struct {
	CreatedAt time.Time
	Prelude   []string
	Log       []LogLine
	Matches   []string
}

type ReportHistoryEntry struct {
	ReportEntry

	ExitedAt  time.Time
	ExitState string
	Progress  Progress
	Usage     ProcessUsage
}

type Report struct {
	ReportEntry
	History []ReportHistoryEntry
}

type ReportHistorySearchResult struct {
	ProcessID string
	Reference string
	ExitState string
	ExitedAt  time.Time
	CreatedAt time.Time
}
