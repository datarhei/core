package app

import (
	"time"

	"github.com/datarhei/core/v16/ffmpeg/parse"
	"github.com/datarhei/core/v16/process"
	"github.com/datarhei/core/v16/slices"
)

type LogLine struct {
	Timestamp time.Time
	Data      string
}

func (l *LogLine) UnmarshalProcess(p *process.Line) {
	l.Timestamp = p.Timestamp
	l.Data = p.Data
}

func (l *LogLine) MarshalProcess() process.Line {
	p := process.Line{
		Timestamp: l.Timestamp,
		Data:      l.Data,
	}

	return p
}

type ReportEntry struct {
	CreatedAt time.Time
	Prelude   []string
	Log       []LogLine
	Matches   []string
}

func (r *ReportEntry) UnmarshalParser(p *parse.Report) {
	r.CreatedAt = p.CreatedAt
	r.Prelude = slices.Copy(p.Prelude)
	r.Matches = slices.Copy(p.Matches)

	r.Log = make([]LogLine, len(p.Log))
	for i, line := range p.Log {
		r.Log[i].UnmarshalProcess(&line)
	}
}

func (r *ReportEntry) MarshalParser() parse.Report {
	p := parse.Report{
		CreatedAt: r.CreatedAt,
		Prelude:   slices.Copy(r.Prelude),
		Matches:   slices.Copy(r.Matches),
	}

	p.Log = make([]process.Line, len(r.Log))
	for i, line := range r.Log {
		p.Log[i] = line.MarshalProcess()
	}

	return p
}

type ReportHistoryEntry struct {
	ReportEntry

	ExitedAt  time.Time
	ExitState string
	Progress  Progress
	Usage     ProcessUsage
}

func (r *ReportHistoryEntry) UnmarshalParser(p *parse.ReportHistoryEntry) {
	r.ReportEntry.UnmarshalParser(&p.Report)

	r.ExitedAt = p.ExitedAt
	r.ExitState = p.ExitState
	r.Usage.UnmarshalParser(&p.Usage)
	r.Progress.UnmarshalParser(&p.Progress)
}

func (r *ReportHistoryEntry) MarshalParser() parse.ReportHistoryEntry {
	p := parse.ReportHistoryEntry{
		Report:    r.ReportEntry.MarshalParser(),
		ExitedAt:  r.ExitedAt,
		ExitState: r.ExitState,
		Progress:  r.Progress.MarshalParser(),
		Usage:     r.Usage.MarshalParser(),
	}

	return p
}

type Report struct {
	ReportEntry
	History []ReportHistoryEntry
}

func (r *Report) UnmarshalParser(p *parse.Report) {
	r.ReportEntry.UnmarshalParser(p)
}

func (r *Report) MarshalParser() (parse.Report, []parse.ReportHistoryEntry) {
	report := r.ReportEntry.MarshalParser()
	history := make([]parse.ReportHistoryEntry, 0, len(r.History))

	for _, h := range r.History {
		history = append(history, h.MarshalParser())
	}

	return report, history
}

type ReportHistorySearchResult struct {
	ProcessID string
	Domain    string
	Reference string
	ExitState string
	ExitedAt  time.Time
	CreatedAt time.Time
}
