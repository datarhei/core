package api

import (
	"encoding/json"
	"time"
)

type MetricsDescription struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
}

type MetricsQueryMetric struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
}

type MetricsQuery struct {
	Timerange int64                `json:"timerange_sec"`
	Interval  int64                `json:"interval_sec"`
	Metrics   []MetricsQueryMetric `json:"metrics"`
}

type MetricsResponseMetric struct {
	Name   string                 `json:"name"`
	Labels map[string]string      `json:"labels"`
	Values []MetricsResponseValue `json:"values"`
}

type MetricsResponseValue struct {
	TS    time.Time `json:"ts"`
	Value float64   `json:"value"`
}

// MarshalJSON unmarshals a JSON to MetricsResponseValue
func (v *MetricsResponseValue) UnmarshalJSON(data []byte) error {
	x := []float64{}

	err := json.Unmarshal(data, &x)
	if err != nil {
		return err
	}

	v.TS = time.Unix(int64(x[0]), 0)
	v.Value = x[1]

	return nil
}

type MetricsResponse struct {
	Timerange int64                   `json:"timerange_sec"`
	Interval  int64                   `json:"interval_sec"`
	Metrics   []MetricsResponseMetric `json:"metrics"`
}
