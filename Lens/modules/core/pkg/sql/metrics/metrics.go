package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	customSlowQueryHistogram *prometheus.HistogramVec
	customSlowQuerySummary   *prometheus.SummaryVec
	sqlErrorCounter          *prometheus.CounterVec
	queryTimeUseSummary      *prometheus.SummaryVec
	queryTimeUseHistogram    *prometheus.HistogramVec
)

func init() {
	customSlowQueryHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "sql_slow_query_duration_seconds",
		Help: "SQL slow query duration in seconds.",
	}, []string{"caller"})
	prometheus.MustRegister(customSlowQueryHistogram)
	customSlowQuerySummary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "sql_slow_query_duration_seconds_summary",
		Help: "SQL slow query duration in seconds summary.",
	}, []string{"caller"})
	prometheus.MustRegister(customSlowQuerySummary)
	sqlErrorCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sql_error_total",
		Help: "SQL error count.",
	}, []string{"caller", "table", "error"})
	prometheus.MustRegister(sqlErrorCounter)
	queryTimeUseSummary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "sql_query_time_use_seconds_summary",
		Help: "SQL query time use in seconds summary.",
	}, []string{"caller"})
	prometheus.MustRegister(queryTimeUseSummary)
	queryTimeUseHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "sql_query_time_use_seconds",
		Help: "SQL query time use in seconds.",
	}, []string{"caller"})
	prometheus.MustRegister(queryTimeUseHistogram)
}

// RecordSlowQueryDuration records duration for slow query.
func RecordSlowQueryDuration(caller string, durationSeconds float64) {
	customSlowQueryHistogram.WithLabelValues(caller).Observe(durationSeconds)
	customSlowQuerySummary.WithLabelValues(caller).Observe(durationSeconds)
}

// RecordSQLError records error for sql.
func RecordSQLError(caller, table, error string) {
	sqlErrorCounter.WithLabelValues(caller, table, error).Inc()
}

// RecordQueryTimeUse records time use for query.
func RecordQueryTimeUse(caller string, timeUseSeconds float64) {
	queryTimeUseSummary.WithLabelValues(caller).Observe(timeUseSeconds)
	queryTimeUseHistogram.WithLabelValues(caller).Observe(timeUseSeconds)
}
