/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"encoding/csv"
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

const (
	defaultMaxPoints = 1000
	maxMaxPoints     = 20000
	// maxMetricRows bounds the points a curve request reads; of a run with more, each curve shows its newest.
	maxMetricRows = 200000

	AxisStep    = "step"
	AxisRollout = "rollout"
)

// series is one metric's curve.
type series struct {
	Name        string       `json:"name"`
	Axis        string       `json:"axis"`
	Points      [][2]float64 `json:"points"`
	Count       int          `json:"count"`
	Last        *[2]float64  `json:"last,omitempty"`
	Downsampled bool         `json:"downsampled,omitempty"`
	// NonFinite lists the NaN and Inf points, which are not drawn: [step, "NaN" | "Inf" | "-Inf"].
	NonFinite [][2]interface{} `json:"nonFinite,omitempty"`
}

// markers are the resume points and the failure of a run.
type markers struct {
	Resumes []resumeMark `json:"resumes"`
	Failure *failureMark `json:"failure"`
}

// resumeMark is where a new attempt resumed.
type resumeMark struct {
	Step int    `json:"step"`
	Time string `json:"time,omitempty"`
}

// failureMark is the last step of a run that failed in training.
type failureMark struct {
	Step   int    `json:"step"`
	Reason string `json:"reason"`
}

// metricsResponse is the curve data of a run.
type metricsResponse struct {
	Series  []series `json:"series"`
	Markers markers  `json:"markers"`
	// Panels and Better come from the registry's metrics section of the run's framework: the panels of its train
	// type and which direction is better for a name pattern.
	Panels []registry.Panel  `json:"panels"`
	Better map[string]string `json:"better,omitempty"`
}

// GetMetrics returns a run's metric series, downsampled to maxPoints, and the markers drawn on them.
func (h *Handler) GetMetrics(c *gin.Context) {
	reg, _, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	run, err := h.getRun(c, v1.GetVerb)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	maxPoints := defaultMaxPoints
	if v, err := strconv.Atoi(c.Query("maxPoints")); err == nil && v > 0 {
		maxPoints = min(v, maxMaxPoints)
	}
	points, err := h.selectPoints(c, run, c.Query("names"), maxMetricRows)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	resp := metricsResponse{Series: []series{}}
	for _, name := range sortedNames(points) {
		resp.Series = append(resp.Series, buildSeries(name, axisOf(name, run.TrainType), points[name], maxPoints))
	}
	resp.Markers = buildMarkers(run, points)
	resp.Panels = []registry.Panel{}
	if fw := frameworkOf(reg, run.Framework); fw != nil {
		if p := fw.Metrics.Panels[run.TrainType]; p != nil {
			resp.Panels = p
		}
		resp.Better = fw.Metrics.Better
	}
	c.JSON(http.StatusOK, resp)
}

// frameworkOf returns the registry's framework of a run, or nil when the registry or framework is missing.
func frameworkOf(reg *registry.Registry, name string) *registry.Framework {
	if reg == nil {
		return nil
	}
	if name == "" {
		name = "slime"
	}
	return reg.FrameworkByName(name)
}

// ExportMetrics writes a run's points as CSV (name, step, value, time) without downsampling. It reads and writes
// one metric at a time, of which it exports the newest maxMetricRows points, so a long run is streamed with at most
// one capped series in memory.
func (h *Handler) ExportMetrics(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	run, err := h.getRun(c, v1.GetVerb)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	names, err := h.metricNames(c, run, c.Query("names"))
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	ctx := c.Request.Context()
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", `attachment; filename="`+run.RunID+`-metrics.csv"`)
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"name", "step", "value", "time"})
	for _, name := range names {
		rows, err := h.db.ListLatestPostTrainingRunMetrics(ctx, run.RunID, []string{name}, maxMetricRows)
		if err != nil {
			// The status is sent already, so the file ends here.
			klog.ErrorS(err, "failed to export the metric points of a run", "run", run.RunID, "name", name)
			break
		}
		for _, p := range rows {
			_ = w.Write([]string{name, strconv.Itoa(p.Step), strconv.FormatFloat(p.Value, 'g', -1, 64),
				p.WallTime.UTC().Format(time.RFC3339)})
		}
		w.Flush()
	}
	w.Flush()
}

// metricNames returns, sorted, the names of a run's metrics that names selects; names is comma separated and may
// hold patterns where * matches anything but "/". An empty names selects every metric.
func (h *Handler) metricNames(c *gin.Context, run *dbclient.PostTrainingRun, names string) ([]string, error) {
	all, err := h.db.ListPostTrainingRunMetricNames(c.Request.Context(), run.RunID)
	if err != nil {
		return nil, commonerrors.NewInternalError("failed to read metric names: " + err.Error())
	}
	if strings.TrimSpace(names) == "" {
		sort.Strings(all)
		return all, nil
	}
	patterns := strings.Split(names, ",")
	selected := []string{}
	for _, n := range all {
		for _, p := range patterns {
			if MatchName(strings.TrimSpace(p), n) {
				selected = append(selected, n)
				break
			}
		}
	}
	sort.Strings(selected)
	return selected, nil
}

// selectPoints loads the points of the requested names, the newest limit of them; names is as metricNames takes
// it.
func (h *Handler) selectPoints(c *gin.Context, run *dbclient.PostTrainingRun, names string,
	limit int) (map[string][]*dbclient.PostTrainingRunMetric, error) {
	var selected []string
	if strings.TrimSpace(names) != "" {
		var err error
		if selected, err = h.metricNames(c, run, names); err != nil {
			return nil, err
		}
		if len(selected) == 0 {
			return map[string][]*dbclient.PostTrainingRunMetric{}, nil
		}
	}
	rows, err := h.db.ListLatestPostTrainingRunMetrics(c.Request.Context(), run.RunID, selected, limit)
	if err != nil {
		return nil, commonerrors.NewInternalError("failed to read metrics: " + err.Error())
	}
	out := map[string][]*dbclient.PostTrainingRunMetric{}
	for _, r := range rows {
		out[r.Name] = append(out[r.Name], r)
	}
	return out, nil
}

// MatchName matches a metric name against a pattern in which * matches any run of characters other than "/".
func MatchName(pattern, name string) bool {
	if pattern == "" {
		return false
	}
	pp, np := strings.Split(pattern, "/"), strings.Split(name, "/")
	if len(pp) != len(np) {
		return false
	}
	for i := range pp {
		if !matchSegment(pp[i], np[i]) {
			return false
		}
	}
	return true
}

// matchSegment matches one path segment against a pattern segment with * wildcards.
func matchSegment(p, s string) bool {
	parts := strings.Split(p, "*")
	if len(parts) == 1 {
		return p == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(s, parts[i])
		if idx < 0 {
			return false
		}
		s = s[idx+len(parts[i]):]
	}
	return strings.HasSuffix(s, parts[len(parts)-1])
}

// axisOf is the horizontal axis of a metric: training steps for train/* and every SFT metric, and rollouts for
// the other RL metrics.
func axisOf(name, trainType string) string {
	if strings.HasPrefix(name, "train/") || trainType != registry.TrainTypeRL {
		return AxisStep
	}
	return AxisRollout
}

// buildSeries turns a metric's points into a curve: non-finite values are listed apart, and more than maxPoints
// points are downsampled into maxPoints/2 step buckets that keep each bucket's minimum and maximum.
func buildSeries(name, axis string, rows []*dbclient.PostTrainingRunMetric, maxPoints int) series {
	s := series{Name: name, Axis: axis, Count: len(rows), Points: [][2]float64{}}
	var finite [][2]float64
	for _, r := range rows {
		if math.IsNaN(r.Value) || math.IsInf(r.Value, 0) {
			s.NonFinite = append(s.NonFinite, [2]interface{}{r.Step, nonFiniteLabel(r.Value)})
			continue
		}
		finite = append(finite, [2]float64{float64(r.Step), r.Value})
	}
	if len(rows) > 0 && !math.IsNaN(rows[len(rows)-1].Value) && !math.IsInf(rows[len(rows)-1].Value, 0) {
		last := [2]float64{float64(rows[len(rows)-1].Step), rows[len(rows)-1].Value}
		s.Last = &last
	}
	if len(finite) <= maxPoints || maxPoints < 2 {
		s.Points = append(s.Points, finite...)
		return s
	}
	s.Downsampled = true
	buckets := maxPoints / 2
	lo, span := finite[0][0], finite[len(finite)-1][0]-finite[0][0]
	bucketOf := func(x float64) int {
		if span <= 0 {
			return 0
		}
		return min(int((x-lo)/span*float64(buckets)), buckets-1)
	}
	for start := 0; start < len(finite); {
		b, minIdx, maxIdx, end := bucketOf(finite[start][0]), start, start, start
		for ; end < len(finite) && bucketOf(finite[end][0]) == b; end++ {
			if finite[end][1] < finite[minIdx][1] {
				minIdx = end
			}
			if finite[end][1] > finite[maxIdx][1] {
				maxIdx = end
			}
		}
		s.Points = append(s.Points, finite[min(minIdx, maxIdx)])
		if minIdx != maxIdx {
			s.Points = append(s.Points, finite[max(minIdx, maxIdx)])
		}
		start = end
	}
	return s
}

// nonFiniteLabel names a non-finite value.
func nonFiniteLabel(v float64) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case v > 0:
		return "Inf"
	default:
		return "-Inf"
	}
}

// sortedNames returns the metric names in order.
func sortedNames(points map[string][]*dbclient.PostTrainingRunMetric) []string {
	names := make([]string, 0, len(points))
	for n := range points {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// buildMarkers derives the resume and failure markers of a run from its stage history and points.
func buildMarkers(run *dbclient.PostTrainingRun, points map[string][]*dbclient.PostTrainingRunMetric) markers {
	m := markers{Resumes: []resumeMark{}}
	main := "train/loss"
	if run.TrainType == registry.TrainTypeRL {
		main = "rollout/raw_reward"
	}
	lastStep := 0
	if rows := points[main]; len(rows) > 0 {
		lastStep = rows[len(rows)-1].Step
	}
	var history []map[string]interface{}
	_ = json.Unmarshal([]byte(run.StageHistory), &history)
	for _, e := range history {
		if step, ok := e["resumeStep"].(float64); ok {
			t, _ := e["time"].(string)
			m.Resumes = append(m.Resumes, resumeMark{Step: int(step), Time: t})
		}
	}
	if run.Status == string(v1.WorkloadFailed) && run.FailedStage == "train" {
		m.Failure = &failureMark{Step: lastStep, Reason: run.Message}
	}
	return m
}
