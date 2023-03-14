package api

import (
	"strconv"

	"github.com/datarhei/core/v16/restream/app"
)

// ProcessReportEntry represents the logs of a run of a restream process
type ProcessReportEntry struct {
	CreatedAt int64       `json:"created_at" format:"int64"`
	Prelude   []string    `json:"prelude"`
	Log       [][2]string `json:"log"`
}

type ProcessReportHistoryEntry struct {
	ProcessReportEntry

	ExitedAt  int64    `json:"exited_at" format:"int64"`
	ExitState string   `json:"exit_state"`
	Progress  Progress `json:"progress"`
}

// ProcessReport represents the current log and the logs of previous runs of a restream process
type ProcessReport struct {
	ProcessReportEntry
	History []ProcessReportHistoryEntry `json:"history"`
}

// Unmarshal converts a restream log to a report
func (report *ProcessReport) Unmarshal(l *app.Log) {
	if l == nil {
		return
	}

	report.CreatedAt = l.CreatedAt.Unix()
	report.Prelude = l.Prelude
	report.Log = make([][2]string, len(l.Log))
	for i, line := range l.Log {
		report.Log[i][0] = strconv.FormatInt(line.Timestamp.Unix(), 10)
		report.Log[i][1] = line.Data
	}

	report.History = []ProcessReportHistoryEntry{}

	for _, h := range l.History {
		he := ProcessReportHistoryEntry{
			ProcessReportEntry: ProcessReportEntry{
				CreatedAt: h.CreatedAt.Unix(),
				Prelude:   h.Prelude,
				Log:       make([][2]string, len(h.Log)),
			},
			ExitedAt:  h.ExitedAt.Unix(),
			ExitState: h.ExitState,
		}

		he.Progress.Unmarshal(&h.Progress)

		for i, line := range h.Log {
			he.ProcessReportEntry.Log[i][0] = strconv.FormatInt(line.Timestamp.Unix(), 10)
			he.ProcessReportEntry.Log[i][1] = line.Data
		}

		report.History = append(report.History, he)
	}
}

type ProcessReportSearchResult struct {
	ProcessID string `json:"id"`
	Reference string `json:"reference"`
	ExitState string `json:"exit_state"`
	CreatedAt int64  `json:"created_at" format:"int64"`
	ExitedAt  int64  `json:"exited_at" format:"int64"`
}
