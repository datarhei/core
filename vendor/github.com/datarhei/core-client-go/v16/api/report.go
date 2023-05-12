package api

// ProcessReportEntry represents the logs of a run of a restream process
type ProcessReportEntry struct {
	CreatedAt int64         `json:"created_at" format:"int64"`
	Prelude   []string      `json:"prelude,omitempty"`
	Log       [][2]string   `json:"log,omitempty"`
	Matches   []string      `json:"matches,omitempty"`
	ExitedAt  int64         `json:"exited_at,omitempty" format:"int64"`
	ExitState string        `json:"exit_state,omitempty"`
	Progress  *Progress     `json:"progress,omitempty"`
	Resources *ProcessUsage `json:"resources,omitempty"`
}

type ProcessReportHistoryEntry struct {
	ProcessReportEntry
}

// ProcessReport represents the current log and the logs of previous runs of a restream process
type ProcessReport struct {
	ProcessReportEntry
	History []ProcessReportEntry `json:"history"`
}

type ProcessReportSearchResult struct {
	ProcessID string `json:"id"`
	Reference string `json:"reference"`
	ExitState string `json:"exit_state"`
	CreatedAt int64  `json:"created_at" format:"int64"`
	ExitedAt  int64  `json:"exited_at" format:"int64"`
}
