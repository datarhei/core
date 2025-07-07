package api

import (
	"strconv"
	"time"

	"github.com/datarhei/core/v16/restream/app"
)

// ProcessReportEntry represents the logs of a run of a core process
type ProcessReportEntry struct {
	CreatedAt int64       `json:"created_at" format:"int64"`
	Prelude   []string    `json:"prelude,omitempty"`
	Log       [][2]string `json:"log,omitempty"`
	Matches   []string    `json:"matches,omitempty"`
}

func (r *ProcessReportEntry) Unmarshal(p *app.ReportEntry) {
	r.CreatedAt = p.CreatedAt.Unix()
	r.Prelude = p.Prelude
	r.Log = make([][2]string, len(p.Log))
	for i, line := range p.Log {
		r.Log[i][0] = strconv.FormatInt(line.Timestamp.Unix(), 10)
		r.Log[i][1] = line.Data
	}
	r.Matches = p.Matches
}

func (r *ProcessReportEntry) Marshal() app.ReportEntry {
	p := app.ReportEntry{
		CreatedAt: time.Unix(r.CreatedAt, 0),
		Prelude:   r.Prelude,
		Log:       make([]app.LogLine, 0, len(r.Log)),
		Matches:   r.Matches,
	}

	for _, l := range r.Log {
		ts, _ := strconv.ParseInt(l[0], 10, 64)
		p.Log = append(p.Log, app.LogLine{
			Timestamp: time.Unix(ts, 0),
			Data:      l[1],
		})
	}

	return p
}

type ProcessReportHistoryEntry struct {
	ProcessReportEntry

	ExitedAt  int64         `json:"exited_at,omitempty" format:"int64"`
	ExitState string        `json:"exit_state,omitempty"`
	Progress  *Progress     `json:"progress,omitempty"`
	Resources *ProcessUsage `json:"resources,omitempty"`
}

func (r *ProcessReportHistoryEntry) Unmarshal(p *app.ReportHistoryEntry) {
	r.ProcessReportEntry.Unmarshal(&p.ReportEntry)

	r.ExitedAt = p.ExitedAt.Unix()
	r.ExitState = p.ExitState

	r.Resources = &ProcessUsage{}
	r.Resources.Unmarshal(&p.Usage)

	r.Progress = &Progress{}
	r.Progress.Unmarshal(&p.Progress)
}

func (r *ProcessReportHistoryEntry) Marshal() app.ReportHistoryEntry {
	p := app.ReportHistoryEntry{
		ReportEntry: r.ProcessReportEntry.Marshal(),
		ExitedAt:    time.Unix(r.ExitedAt, 0),
		ExitState:   r.ExitState,
		Progress:    app.Progress{},
		Usage:       app.ProcessUsage{},
	}

	if r.Progress != nil {
		p.Progress = r.Progress.Marshal()
	}

	if r.Resources != nil {
		p.Usage = r.Resources.Marshal()
	}

	return p
}

// ProcessReport represents the current log and the logs of previous runs of a restream process
type ProcessReport struct {
	ProcessReportEntry
	History []ProcessReportHistoryEntry `json:"history"`
}

// Unmarshal converts a core report to a report
func (r *ProcessReport) Unmarshal(l *app.Report) {
	if l == nil {
		return
	}

	r.ProcessReportEntry.Unmarshal(&l.ReportEntry)
	r.History = make([]ProcessReportHistoryEntry, len(l.History))

	for i, h := range l.History {
		r.History[i].Unmarshal(&h)
	}
}

func (r *ProcessReport) Marshal() app.Report {
	p := app.Report{
		ReportEntry: r.ProcessReportEntry.Marshal(),
		History:     make([]app.ReportHistoryEntry, 0, len(r.History)),
	}

	for _, h := range r.History {
		p.History = append(p.History, h.Marshal())
	}

	return p
}

type ProcessReportSearchResult struct {
	ProcessID string `json:"id"`
	Domain    string `json:"domain"`
	Reference string `json:"reference"`
	ExitState string `json:"exit_state"`
	CreatedAt int64  `json:"created_at" format:"int64"`
	ExitedAt  int64  `json:"exited_at" format:"int64"`
}
