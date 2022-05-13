package api

type WidgetProcess struct {
	CurrentSessions uint64 `json:"current_sessions"`
	TotalSessions   uint64 `json:"total_sessions"`
	Uptime          int64  `json:"uptime"`
}
