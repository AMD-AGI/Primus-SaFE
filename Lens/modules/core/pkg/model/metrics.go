package model

type GrafanaMetricsSeries struct {
	Name   string       `json:"name"`
	Points [][2]float64 `json:"points"`
}
