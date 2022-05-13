package api

import (
	"net/http"
	"time"

	"github.com/datarhei/core/http/api"
	"github.com/datarhei/core/http/handler/util"
	"github.com/datarhei/core/monitor"
	"github.com/datarhei/core/monitor/metric"

	"github.com/labstack/echo/v4"
)

type MetricsConfig struct {
	Metrics monitor.HistoryReader
}

// The MetricsHandler type provides handlers for the widget API
type MetricsHandler struct {
	metrics monitor.HistoryReader
}

// NewWidget return a new Widget type
func NewMetrics(config MetricsConfig) *MetricsHandler {
	return &MetricsHandler{
		metrics: config.Metrics,
	}
}

// Query the collected metrics
// @Summary Query the collected metrics
// @Description Query the collected metrics
// @ID metrics-3-metrics
// @Accept json
// @Produce json
// @Param config body api.MetricsQuery true "Metrics query"
// @Success 200 {object} api.MetricsResponse
// @Failure 400 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/metrics [post]
func (r *MetricsHandler) Metrics(c echo.Context) error {
	var query api.MetricsQuery

	if err := util.ShouldBindJSON(c, &query); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	patterns := []metric.Pattern{}

	for _, m := range query.Metrics {
		labels := []string{}
		for k, v := range m.Labels {
			labels = append(labels, k, v)
		}

		pattern := metric.NewPattern(m.Name, labels...)

		patterns = append(patterns, pattern)
	}

	response := api.MetricsResponse{}

	var data []monitor.HistoryMetrics

	if query.Timerange == 0 {
		// current data
		data = []monitor.HistoryMetrics{
			{
				TS:      time.Now(),
				Metrics: r.metrics.Collect(patterns),
			},
		}
	} else {
		// historic data
		data = r.metrics.History(time.Second*time.Duration(query.Timerange), time.Second*time.Duration(query.Interval), patterns)
	}

	timerange, interval := r.metrics.Resolution()

	response.Unmarshal(data, timerange, interval)

	return c.JSON(http.StatusOK, response)
}
