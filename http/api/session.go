package api

import (
	"maps"

	"github.com/datarhei/core/v16/encoding/json"
	mwsession "github.com/datarhei/core/v16/http/middleware/session"
	"github.com/datarhei/core/v16/session"
)

// SessionStats are the accumulated numbers for the session summary
type SessionStats struct {
	TotalSessions uint64 `json:"sessions" format:"uint64"`
	TotalRxBytes  uint64 `json:"traffic_rx_mb" format:"uint64"`
	TotalTxBytes  uint64 `json:"traffic_tx_mb" format:"uint64"`
}

// SessionPeers is for the grouping by peers in the summary
type SessionPeers struct {
	SessionStats

	Locations map[string]SessionStats `json:"local"`
}

// Session represents an active session
type Session struct {
	ID        string          `json:"id"`
	Reference string          `json:"reference"`
	CreatedAt int64           `json:"created_at" format:"int64"`
	Location  string          `json:"local"`
	Peer      string          `json:"remote"`
	Extra     map[string]any  `json:"extra"`
	RxBytes   uint64          `json:"bytes_rx" format:"uint64"`
	TxBytes   uint64          `json:"bytes_tx" format:"uint64"`
	RxBitrate json.Number     `json:"bandwidth_rx_kbit" swaggertype:"number" jsonschema:"type=number"` // kbit/s
	TxBitrate json.Number     `json:"bandwidth_tx_kbit" swaggertype:"number" jsonschema:"type=number"` // kbit/s
	HLS       *HLSSessionData `json:"hls"`
}

func (s *Session) Unmarshal(sess session.Session) {
	s.ID = sess.ID
	s.Reference = sess.Reference
	s.CreatedAt = sess.CreatedAt.Unix()
	s.Location = sess.Location
	s.Peer = sess.Peer
	s.Extra = sess.Extra
	s.RxBytes = sess.RxBytes
	s.TxBytes = sess.TxBytes
	s.RxBitrate = json.ToNumber(sess.RxBitrate / 1024)
	s.TxBitrate = json.ToNumber(sess.TxBitrate / 1024)
	s.HLS = nil

	hlsstats, ok := sess.UserData["hlsstats"]
	if !ok {
		return
	}

	data, ok := hlsstats.(*mwsession.HLSSessionData)
	if !ok {
		return
	}

	hls := &HLSSessionData{}
	hls.Segments.Requested = data.Segments.Requested
	hls.Segments.Failed = data.Segments.Failed
	hls.Segments.TooSlow = data.Segments.TooSlow
	hls.Segments.Retries = data.Segments.Retries
	hls.Segments.TooLate = data.Segments.TooLate
	hls.Segments.SequenceGaps = data.Segments.SequenceGaps
	hls.Segments.Last = data.Segments.Last.Unix()

	hls.Bandwidth.Min = data.Bandwidth.Min
	hls.Bandwidth.Max = data.Bandwidth.Max
	hls.Bandwidth.Avg = data.Bandwidth.Avg

	hls.HTTPStatus = map[int]uint64{}
	maps.Copy(hls.HTTPStatus, data.HTTPStatus)

	for path, variant := range data.Variants {
		hls.Variants = append(hls.Variants, HLSSessionVariant{
			Path:       path,
			Active:     variant.Active,
			Switches:   variant.Switches,
			Bandwidth:  variant.Bandwidth,
			Resolution: variant.Resolution,
			Codecs:     variant.Codecs,
		})
	}

	s.HLS = hls
}

type HLSSessionVariant struct {
	Path       string `json:"path"`
	Active     bool   `json:"active"`
	Switches   uint64 `json:"switches"`
	Bandwidth  uint64 `json:"bandwidth_bits"`
	Resolution string `json:"resolution"`
	Codecs     string `json:"codecs"`
}

type HLSSessionData struct {
	Variants []HLSSessionVariant `json:"hls_variants"`
	Segments struct {
		Requested    uint64 `json:"requests"`
		Failed       uint64 `json:"failed"`
		TooSlow      uint64 `json:"too_slow"`
		Retries      uint64 `json:"retries"`
		TooLate      uint64 `json:"too_late"`
		SequenceGaps uint64 `json:"sequence_gaps"`
		Last         int64  `json:"last"`
	} `json:"hls_segments"`
	HTTPStatus map[int]uint64 `json:"http_status"`
	Bandwidth  struct {
		Min float64 `json:"min"`
		Max float64 `json:"max"`
		Avg float64 `json:"avg"`
	} `json:"bandwidth_tx_bits"`
}

// SessionSummaryActive represents the currently active sessions
type SessionSummaryActive struct {
	SessionList  []Session   `json:"list"`
	Sessions     uint64      `json:"sessions" format:"uint64"`
	RxBitrate    json.Number `json:"bandwidth_rx_mbit" swaggertype:"number" jsonschema:"type=number"` // mbit/s
	TxBitrate    json.Number `json:"bandwidth_tx_mbit" swaggertype:"number" jsonschema:"type=number"` // mbit/s
	MaxSessions  uint64      `json:"max_sessions" format:"uint64"`
	MaxRxBitrate json.Number `json:"max_bandwidth_rx_mbit" swaggertype:"number" jsonschema:"type=number"` // mbit/s
	MaxTxBitrate json.Number `json:"max_bandwidth_tx_mbit" swaggertype:"number" jsonschema:"type=number"` // mbit/s
}

// SessionSummarySummary represents the summary (history) of all finished sessions
type SessionSummarySummary struct {
	Peers      map[string]SessionPeers `json:"remote"`
	Locations  map[string]SessionStats `json:"local"`
	References map[string]SessionStats `json:"reference"`
	SessionStats
}

// SessionSummary is the API representation of a session.Summary
type SessionSummary struct {
	Active SessionSummaryActive `json:"active"`

	Summary SessionSummarySummary `json:"summary"`
}

type SessionsSummary map[string]SessionSummary

// SessionsActive is the API representation of all active sessions
type SessionsActive map[string][]Session

// Unmarshal creates a new SessionSummary from a session.Summary
func (summary *SessionSummary) Unmarshal(sum session.Summary) {
	summary.Active.MaxSessions = sum.MaxSessions
	summary.Active.MaxRxBitrate = json.ToNumber(sum.MaxRxBitrate / 1024 / 1024)
	summary.Active.MaxTxBitrate = json.ToNumber(sum.MaxTxBitrate / 1024 / 1024)

	summary.Active.Sessions = sum.CurrentSessions
	summary.Active.RxBitrate = json.ToNumber(sum.CurrentRxBitrate / 1024 / 1024)
	summary.Active.TxBitrate = json.ToNumber(sum.CurrentTxBitrate / 1024 / 1024)

	summary.Active.SessionList = make([]Session, len(sum.Active))

	for i, s := range sum.Active {
		summary.Active.SessionList[i].Unmarshal(s)
	}

	summary.Summary.Peers = make(map[string]SessionPeers)

	for peer, g := range sum.Summary.Peers {
		group := SessionPeers{
			Locations: make(map[string]SessionStats),
		}

		group.TotalSessions = g.TotalSessions
		group.TotalRxBytes = g.TotalRxBytes / 1024 / 1024
		group.TotalTxBytes = g.TotalTxBytes / 1024 / 1024

		for location, s := range g.Locations {
			group.Locations[location] = SessionStats{
				TotalSessions: s.TotalSessions,
				TotalRxBytes:  s.TotalRxBytes / 1024 / 1024,
				TotalTxBytes:  s.TotalTxBytes / 1024 / 1024,
			}
		}

		summary.Summary.Peers[peer] = group
	}

	summary.Summary.Locations = make(map[string]SessionStats)

	for location, g := range sum.Summary.Locations {
		summary.Summary.Locations[location] = SessionStats{
			TotalSessions: g.TotalSessions,
			TotalRxBytes:  g.TotalRxBytes / 1024 / 1024,
			TotalTxBytes:  g.TotalTxBytes / 1024 / 1024,
		}
	}

	summary.Summary.References = make(map[string]SessionStats)

	for reference, g := range sum.Summary.References {
		summary.Summary.References[reference] = SessionStats{
			TotalSessions: g.TotalSessions,
			TotalRxBytes:  g.TotalRxBytes / 1024 / 1024,
			TotalTxBytes:  g.TotalTxBytes / 1024 / 1024,
		}
	}

	summary.Summary.TotalSessions = sum.Summary.TotalSessions
	summary.Summary.TotalRxBytes = sum.Summary.TotalRxBytes / 1024 / 1024
	summary.Summary.TotalTxBytes = sum.Summary.TotalTxBytes / 1024 / 1024
}

type SessionTokenRequest struct {
	Match  string                 `json:"match"`
	Remote []string               `json:"remote"`
	Extra  map[string]interface{} `json:"extra"`
	TTL    int64                  `json:"ttl_sec"`
	Token  string                 `json:"token,omitempty"`
}
