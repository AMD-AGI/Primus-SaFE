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

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

const (
	defaultMaxPoints = 1000
	maxMaxPoints     = 20000
	valLossMetric    = "eval/val/loss"
	// checkpointsCleanedEvent is the stage_history event of a finished checkpoint cleanup; keptStep names the
	// checkpoint it kept, if any.
	checkpointsCleanedEvent = "checkpoints-cleaned"

	AxisStep    = "step"
	AxisRollout = "rollout"
	AxisTime    = "time"
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

// checkpointMark is a checkpoint position on the curves.
type checkpointMark struct {
	Step    int      `json:"step"`
	Deleted bool     `json:"deleted,omitempty"`
	ValLoss *float64 `json:"valLoss,omitempty"`
}

// markers are the checkpoints, the best checkpoint, resume points and the failure of a run.
type markers struct {
	Checkpoints    []checkpointMark `json:"checkpoints"`
	BestCheckpoint *checkpointMark  `json:"bestCheckpoint"`
	Resumes        []resumeMark     `json:"resumes"`
	Failure        *failureMark     `json:"failure"`
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
}

// GetMetrics returns a run's metric series, downsampled to maxPoints, and the markers drawn on them.
func (h *Handler) GetMetrics(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
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
	points, err := h.selectPoints(c, run, c.Query("names"))
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	resp := metricsResponse{Series: []series{}}
	for _, name := range sortedNames(points) {
		resp.Series = append(resp.Series, buildSeries(name, axisOf(name, run.TrainType), points[name], maxPoints))
	}
	resp.Markers = buildMarkers(run, points)
	c.JSON(http.StatusOK, resp)
}

// ExportMetrics writes a run's points as CSV (name, step, value, time) without downsampling.
func (h *Handler) ExportMetrics(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	run, err := h.getRun(c, v1.GetVerb)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	points, err := h.selectPoints(c, run, c.Query("names"))
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", `attachment; filename="`+run.RunID+`-metrics.csv"`)
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"name", "step", "value", "time"})
	for _, name := range sortedNames(points) {
		for _, p := range points[name] {
			_ = w.Write([]string{name, strconv.Itoa(p.Step), strconv.FormatFloat(p.Value, 'g', -1, 64),
				p.WallTime.UTC().Format(time.RFC3339)})
		}
	}
	w.Flush()
}

// selectPoints loads the points of the requested names; names is comma separated and may hold patterns where *
// matches anything but "/". An empty names selects every metric.
func (h *Handler) selectPoints(c *gin.Context, run *dbclient.PostTrainingRun,
	names string) (map[string][]*dbclient.PostTrainingRunMetric, error) {
	ctx := c.Request.Context()
	var selected []string
	if strings.TrimSpace(names) != "" {
		all, err := h.db.ListPostTrainingRunMetricNames(ctx, run.RunID)
		if err != nil {
			return nil, commonerrors.NewInternalError("failed to read metric names: " + err.Error())
		}
		patterns := strings.Split(names, ",")
		for _, n := range all {
			for _, p := range patterns {
				if MatchName(strings.TrimSpace(p), n) {
					selected = append(selected, n)
					break
				}
			}
		}
		if len(selected) == 0 {
			return map[string][]*dbclient.PostTrainingRunMetric{}, nil
		}
	}
	rows, err := h.db.ListPostTrainingRunMetrics(ctx, run.RunID, selected)
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

// axisOf is the horizontal axis of a metric: time for the launcher's system series, training steps for train/*
// and every SFT metric, and rollouts for the other RL metrics.
func axisOf(name, trainType string) string {
	switch {
	case strings.HasPrefix(name, dbclient.PostTrainingSystemMetricPrefix):
		return AxisTime
	case strings.HasPrefix(name, "train/") || trainType != registry.TrainTypeRL:
		return AxisStep
	default:
		return AxisRollout
	}
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

// buildMarkers derives the checkpoint, best checkpoint, resume and failure markers of a run from its recipe, stage
// history and points.
func buildMarkers(run *dbclient.PostTrainingRun, points map[string][]*dbclient.PostTrainingRunMetric) markers {
	m := markers{Checkpoints: []checkpointMark{}, Resumes: []resumeMark{}}
	var rec recipe.Recipe
	_ = json.Unmarshal([]byte(run.Recipe), &rec)
	main := "train/loss"
	if run.TrainType == registry.TrainTypeRL {
		main = "rollout/raw_reward"
	}
	lastStep := 0
	if rows := points[main]; len(rows) > 0 {
		lastStep = rows[len(rows)-1].Step
	}
	valLoss := map[int]float64{}
	for _, r := range points[valLossMetric] {
		if !math.IsNaN(r.Value) && !math.IsInf(r.Value, 0) {
			valLoss[r.Step] = r.Value
		}
	}
	if interval := rec.Choices.SaveInterval; interval > 0 {
		for step := interval; step <= lastStep; step += interval {
			cp := checkpointMark{Step: step}
			if v, ok := valLoss[step]; ok {
				cp.ValLoss = &v
			}
			m.Checkpoints = append(m.Checkpoints, cp)
		}
	}
	for i := range m.Checkpoints {
		if cp := m.Checkpoints[i]; cp.ValLoss != nil && (m.BestCheckpoint == nil || *cp.ValLoss < *m.BestCheckpoint.ValLoss) {
			best := cp
			m.BestCheckpoint = &best
		}
	}
	if keep := rec.Checkpoint.KeepLast; keep > 0 {
		for i := range m.Checkpoints {
			recent := i >= len(m.Checkpoints)-keep
			best := rec.Checkpoint.KeepBest && m.BestCheckpoint != nil && m.Checkpoints[i].Step == m.BestCheckpoint.Step
			m.Checkpoints[i].Deleted = !recent && !best
		}
	}
	var history []map[string]interface{}
	_ = json.Unmarshal([]byte(run.StageHistory), &history)
	for _, e := range history {
		if e["event"] == checkpointsCleanedEvent {
			kept, keptOne := e["keptStep"].(float64)
			for i := range m.Checkpoints {
				m.Checkpoints[i].Deleted = m.Checkpoints[i].Deleted || !keptOne || m.Checkpoints[i].Step != int(kept)
			}
		}
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
