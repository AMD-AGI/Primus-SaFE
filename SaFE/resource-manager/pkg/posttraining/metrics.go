/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

const (
	// SubmitterContainer is the RayJob submitter container; its log follows the launcher's output.
	SubmitterContainer = "ray-job-submitter"

	markerStage   = "[STAGE]"
	markerMetric  = "[METRIC]"
	markerSample  = "[SAMPLE]"
	markerError   = "[ERROR]"
	markerSuccess = "[SUCCESS]"

	protocolVersion = 1
	// maxReadBytes caps one log read; the rest is read on the next pass.
	maxReadBytes = 16 << 20
	// maxLineBytes is the longest line that is parsed; longer lines are skipped and counted.
	maxLineBytes     = 1 << 20
	maxMetricNameLen = 160
	sparklinePoints  = 60
	// gapGrace is how long a missing seq may stay unread before it counts as a gap; the launcher replays
	// the last two minutes of metrics every minute.
	gapGrace = 120 * time.Second
	maxGaps  = 256
	// lostAfter is how long a running launcher may go without any [STAGE], heartbeats included.
	lostAfter = 10 * time.Minute
)

var (
	ansiPattern      = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	rayPrefixPattern = regexp.MustCompile(`^\([^()]*pid=\d+[^()]*\)\s*`)
	valLossPattern   = regexp.MustCompile(`^eval/[^/]+/loss$`)
	markerKinds      = []string{markerStage, markerMetric, markerSample, markerError, markerSuccess}
)

// marker is one launcher marker line addressed to a run.
type marker struct {
	Kind string
	// Text is the marker and its JSON with color codes and the Ray prefix removed.
	Text string
	Body markerBody
}

// markerBody holds the marker fields the controller reads; unknown fields are ignored.
type markerBody struct {
	Run     string                     `json:"run"`
	Attempt string                     `json:"attempt"`
	Pv      int                        `json:"pv"`
	Stage   string                     `json:"stage"`
	State   string                     `json:"state"`
	Detail  json.RawMessage            `json:"detail"`
	Retry   *bool                      `json:"retry"`
	Seq     int64                      `json:"seq"`
	Kind    string                     `json:"kind"`
	Step    *int64                     `json:"step"`
	T       float64                    `json:"t"`
	V       map[string]json.RawMessage `json:"v"`
	Reason  string                     `json:"reason"`
	Output  string                     `json:"output"`
}

// stageDetail is the part of a [STAGE] detail the controller reads.
type stageDetail struct {
	ResumeStep    *int64 `json:"resumeStep"`
	ParseFailures int64  `json:"parseFailures"`
}

// stallDetail is the detail of a stalled [STAGE]: the last progress time in unix seconds, the last step and the
// idle minutes at which the watchdog stops the attempt.
type stallDetail struct {
	Since       float64 `json:"since"`
	LastStep    *int64  `json:"lastStep"`
	Minutes     int     `json:"minutes"`
	StopMinutes int     `json:"stopMinutes"`
}

// parseMarker strips color codes and a Ray actor prefix from line and returns the marker that starts it, when
// its JSON is valid and names runID. Forwarded child output is indented by the launcher and never matches.
func parseMarker(line, runID string) (*marker, bool) {
	text := strings.TrimRight(ansiPattern.ReplaceAllString(line, ""), "\r")
	text = rayPrefixPattern.ReplaceAllString(text, "")
	for _, kind := range markerKinds {
		if !strings.HasPrefix(text, kind+" ") {
			continue
		}
		payload := strings.TrimSpace(text[len(kind):])
		m := &marker{Kind: kind, Text: kind + " " + payload}
		if !strings.HasPrefix(payload, "{") || json.Unmarshal([]byte(payload), &m.Body) != nil ||
			m.Body.Run != runID || m.Body.Attempt == "" {
			return nil, false
		}
		return m, true
	}
	return nil, false
}

// lastResult returns the last [SUCCESS] or [ERROR] marker of runID in text, which may hold several lines.
func lastResult(text, runID string) *marker {
	var result *marker
	for _, line := range strings.Split(text, "\n") {
		if m, ok := parseMarker(line, runID); ok && (m.Kind == markerSuccess || m.Kind == markerError) {
			result = m
		}
	}
	return result
}

// attemptIndex returns N of an attempt id "aN", or 0 when the id has another form.
func attemptIndex(attempt string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(attempt, "a"))
	if err != nil || !strings.HasPrefix(attempt, "a") {
		return 0
	}
	return n
}

// logCursor is the metric_cursor column: where the submitter log was read up to, and what each attempt sent.
type logCursor struct {
	Pod string `json:"pod,omitempty"`
	// Since is the timestamp of the last line read and SinceN the number of lines read with that timestamp.
	Since   string `json:"since,omitempty"`
	SinceN  int    `json:"sinceN,omitempty"`
	Skipped int64  `json:"skipped,omitempty"`
	// Order lists the attempts in the order their first [STAGE] was read.
	Order    []string                  `json:"order,omitempty"`
	Attempts map[string]*attemptCursor `json:"attempts,omitempty"`
	// LastStage is the log time of the last [STAGE] read, in unix seconds.
	LastStage int64 `json:"lastStage,omitempty"`
}

// attemptCursor tracks the metric seqs of one attempt and where it resumed training.
type attemptCursor struct {
	MaxSeq     int64      `json:"maxSeq"`
	Gaps       []seqRange `json:"gaps,omitempty"`
	ResumeStep *int64     `json:"resumeStep,omitempty"`
	// ParseFailures is the attempt's count of unparsable metric lines from its last heartbeat.
	ParseFailures int64 `json:"parseFailures,omitempty"`
}

// seqRange is a run of missing seqs noticed at Since (unix seconds).
type seqRange struct {
	From  int64 `json:"from"`
	To    int64 `json:"to"`
	Since int64 `json:"since"`
}

// loadCursor decodes the metric_cursor column; an empty or unreadable value starts from scratch.
func loadCursor(data string) *logCursor {
	cur := &logCursor{}
	if data != "" {
		_ = json.Unmarshal([]byte(data), cur)
	}
	if cur.Attempts == nil {
		cur.Attempts = map[string]*attemptCursor{}
	}
	return cur
}

// setPod points the cursor at pod; a different pod is read from its first line.
func (c *logCursor) setPod(pod string) {
	if c.Pod != pod {
		c.Pod, c.Since, c.SinceN = pod, "", 0
	}
}

// sinceTime returns the timestamp of the last line read, or the zero time.
func (c *logCursor) sinceTime() time.Time {
	t, _ := time.Parse(time.RFC3339Nano, c.Since)
	return t
}

// register records a new attempt and reports whether it was unknown.
func (c *logCursor) register(attempt string) bool {
	if _, ok := c.Attempts[attempt]; ok {
		return false
	}
	c.Attempts[attempt] = &attemptCursor{}
	c.Order = append(c.Order, attempt)
	return true
}

// latestIndex returns the highest attempt number read so far.
func (c *logCursor) latestIndex() int {
	latest := 0
	for _, a := range c.Order {
		latest = max(latest, attemptIndex(a))
	}
	return latest
}

// resumedAfter returns the lowest resume step of the attempts registered after attempt, or nil when none resumed.
func (c *logCursor) resumedAfter(attempt string) *int64 {
	var lowest *int64
	later := false
	for _, a := range c.Order {
		if a == attempt {
			later = true
			continue
		}
		if r := c.Attempts[a].ResumeStep; later && r != nil && (lowest == nil || *r < *lowest) {
			lowest = r
		}
	}
	return lowest
}

// hasGap reports whether any attempt has had a seq missing for longer than gapGrace.
func (c *logCursor) hasGap(now time.Time) bool {
	for _, a := range c.Attempts {
		for _, g := range a.Gaps {
			if now.Sub(time.Unix(g.Since, 0)) > gapGrace {
				return true
			}
		}
	}
	return false
}

// accept records seq and reports whether it was not read before; skipped seqs become gaps until read.
func (a *attemptCursor) accept(seq int64, now time.Time) bool {
	if seq > a.MaxSeq {
		if seq > a.MaxSeq+1 && len(a.Gaps) < maxGaps {
			a.Gaps = append(a.Gaps, seqRange{From: a.MaxSeq + 1, To: seq - 1, Since: now.Unix()})
		}
		a.MaxSeq = seq
		return true
	}
	for i, g := range a.Gaps {
		if seq < g.From || seq > g.To {
			continue
		}
		var rest []seqRange
		if seq > g.From {
			rest = append(rest, seqRange{From: g.From, To: seq - 1, Since: g.Since})
		}
		if seq < g.To {
			rest = append(rest, seqRange{From: seq + 1, To: g.To, Since: g.Since})
		}
		a.Gaps = append(a.Gaps[:i], append(rest, a.Gaps[i+1:]...)...)
		return true
	}
	return false
}

// scanLog reads timestamped lines from r and calls fn with each line the cursor has not passed. Lines over
// maxLineBytes are skipped and counted; an unfinished last line is left for the next read unless it is
// already too long. It returns the number of bytes read.
func scanLog(r io.Reader, cur *logCursor, fn func(ts time.Time, line string)) (int64, error) {
	br := bufio.NewReaderSize(r, 64<<10)
	since := cur.sinceTime()
	seenAtSince := 0
	var total int64
	var buf []byte
	long := false
	consume := func(line []byte, skipped bool) {
		ts, text := splitTimestamp(line)
		if ts.IsZero() {
			ts = cur.sinceTime()
		}
		if !since.IsZero() && ts.Before(since) {
			return
		}
		if !since.IsZero() && ts.Equal(since) && seenAtSince < cur.SinceN {
			seenAtSince++
			return
		}
		if t := cur.sinceTime(); !t.IsZero() && ts.Equal(t) {
			cur.SinceN++
		} else {
			cur.Since, cur.SinceN = ts.UTC().Format(time.RFC3339Nano), 1
		}
		if skipped {
			cur.Skipped++
			return
		}
		fn(ts, text)
	}
	for {
		frag, err := br.ReadSlice('\n')
		total += int64(len(frag))
		if err == bufio.ErrBufferFull {
			if !long {
				buf = append(buf, frag...)
				long = len(buf) > maxLineBytes
			}
			continue
		}
		long = long || len(buf)+len(bytes.TrimRight(frag, "\n")) > maxLineBytes
		if err == io.EOF {
			if long {
				consume(buf, true)
			}
			return total, nil
		}
		if err != nil {
			return total, err
		}
		if long {
			consume(buf, true)
		} else {
			consume(bytes.TrimRight(append(buf, frag...), "\n"), false)
		}
		buf, long = buf[:0], false
	}
}

// splitTimestamp splits the RFC 3339 timestamp the log API puts before each line; it returns the zero time
// when the line has none.
func splitTimestamp(line []byte) (time.Time, string) {
	i := bytes.IndexByte(line, ' ')
	if i <= 0 {
		return time.Time{}, string(line)
	}
	ts, err := time.Parse(time.RFC3339Nano, string(line[:i]))
	if err != nil {
		return time.Time{}, string(line)
	}
	return ts, string(line[i+1:])
}

// metricSummary is the metric_summary column read by the run list.
type metricSummary struct {
	Points      int64         `json:"points"`
	Capped      bool          `json:"capped,omitempty"`
	Rejected    int64         `json:"rejected,omitempty"`
	Main        *mainMetric   `json:"main,omitempty"`
	BestValLoss *summaryPoint `json:"bestValLoss,omitempty"`
}

// mainMetric is the latest value and the sparkline of the train type's summary metric. The sparkline keeps
// every Stride-th point and halves itself when it grows past sparklinePoints.
type mainMetric struct {
	Name   string         `json:"name"`
	Step   int64          `json:"step"`
	Value  float64        `json:"value"`
	Spark  []summaryPoint `json:"spark,omitempty"`
	Stride int            `json:"stride,omitempty"`
	Seen   int            `json:"seen,omitempty"`
}

// summaryPoint is one (step, value) pair.
type summaryPoint struct {
	Step  int64   `json:"step"`
	Value float64 `json:"value"`
}

// loadSummary decodes the metric_summary column.
func loadSummary(data string) *metricSummary {
	s := &metricSummary{}
	if data != "" {
		_ = json.Unmarshal([]byte(data), s)
	}
	return s
}

// observe adds one finite point of the main metric to the summary.
func (m *mainMetric) observe(step int64, value float64) {
	m.Step, m.Value = step, value
	if m.Stride == 0 {
		m.Stride = 1
	}
	if m.Seen%m.Stride == 0 {
		m.Spark = append(m.Spark, summaryPoint{Step: step, Value: value})
		if len(m.Spark) > sparklinePoints {
			kept := m.Spark[:0]
			for i := 0; i < len(m.Spark); i += 2 {
				kept = append(kept, m.Spark[i])
			}
			m.Spark, m.Stride = kept, m.Stride*2
		}
	}
	m.Seen++
}

// rewind drops the summary points after step when a new attempt resumes from it.
func (s *metricSummary) rewind(step int64) {
	if m := s.Main; m != nil {
		kept := m.Spark[:0]
		for _, p := range m.Spark {
			if p.Step <= step {
				kept = append(kept, p)
			}
		}
		m.Spark = kept
	}
	if b := s.BestValLoss; b != nil && b.Step > step {
		s.BestValLoss = nil
	}
}

// metricValue decodes a point value: a number, or one of the strings NaN, Inf and -Inf.
func metricValue(raw json.RawMessage) (float64, bool) {
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f, true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, false
	}
	switch s {
	case "NaN":
		return math.NaN(), true
	case "Inf", "+Inf":
		return math.Inf(1), true
	case "-Inf":
		return math.Inf(-1), true
	}
	return 0, false
}

// pendingPoint is a metric point waiting to be saved, with the attempt that sent it.
type pendingPoint struct {
	attempt string
	point   *dbclient.PostTrainingRunMetric
}

// ingestion applies the markers of one log read to a run: it advances the stage, collects metric points and
// keeps the cursor and summary that are saved with them.
type ingestion struct {
	u          *runUpdate
	cur        *logCursor
	sum        *metricSummary
	mainName   string
	maxPoints  int64
	points     []pendingPoint
	resumeFrom *int64
	result     *marker
	newAttempt bool
	// stall is the latest stall warning not followed by progress; unstalled is set by progress after one.
	stall     *stallDetail
	unstalled bool
	sawStage  bool
	noRetry   bool
	disk      string
	// parseFailures is the growth of the launchers' unparsable metric line counts in this read.
	parseFailures int64
}

// add applies one marker.
func (in *ingestion) add(m *marker) {
	attempt := m.Body.Attempt
	if m.Kind == markerStage {
		if in.cur.register(attempt) {
			in.newAttempt = true
		}
		in.sawStage = true
		in.addStage(m)
		return
	}
	if _, known := in.cur.Attempts[attempt]; !known {
		return
	}
	switch m.Kind {
	case markerMetric:
		in.addMetric(m)
	case markerError, markerSuccess:
		if in.result == nil || attemptIndex(attempt) >= attemptIndex(in.result.Body.Attempt) {
			in.result = m
		}
	}
}

// addStage advances the stage, never backwards in the run's order, and records each attempt's resume step.
func (in *ingestion) addStage(m *marker) {
	switch m.Body.State {
	case "heartbeat":
		var detail stageDetail
		if a := in.cur.Attempts[m.Body.Attempt]; json.Unmarshal(m.Body.Detail, &detail) == nil &&
			detail.ParseFailures > a.ParseFailures {
			in.parseFailures += detail.ParseFailures - a.ParseFailures
			a.ParseFailures = detail.ParseFailures
		}
		return
	case "failed":
		in.noRetry = in.noRetry || (m.Body.Retry != nil && !*m.Body.Retry)
		return
	case "stalled":
		stall := &stallDetail{}
		if json.Unmarshal(m.Body.Detail, stall) == nil && stall.Since > 0 {
			in.stall = stall
		}
		return
	case "recovered":
		in.stall, in.unstalled = nil, true
		return
	case "disk":
		if len(m.Body.Detail) > 0 {
			in.disk = string(m.Body.Detail)
		}
		return
	}
	if launcherStage(m.Body.Stage) {
		in.u.moveTo(m.Body.Stage)
	}
	if m.Body.Stage != "train" || len(m.Body.Detail) == 0 {
		return
	}
	var detail stageDetail
	a := in.cur.Attempts[m.Body.Attempt]
	if json.Unmarshal(m.Body.Detail, &detail) != nil || detail.ResumeStep == nil || a.ResumeStep != nil {
		return
	}
	a.ResumeStep = detail.ResumeStep
	if attemptIndex(m.Body.Attempt) <= 1 && len(in.cur.Order) <= 1 {
		return
	}
	in.u.history = append(in.u.history, stageEntry{Stage: "train", Time: in.u.now.Format(time.RFC3339),
		Attempt: m.Body.Attempt, ResumeStep: detail.ResumeStep})
	in.u.historyChanged = true
	if in.resumeFrom == nil || *detail.ResumeStep < *in.resumeFrom {
		in.resumeFrom = detail.ResumeStep
		in.sum.rewind(*detail.ResumeStep)
	}
}

// addMetric validates the points of one [METRIC] and queues them; a replayed seq is ignored.
func (in *ingestion) addMetric(m *marker) {
	b := &m.Body
	if b.Kind == "link" {
		return
	}
	if b.Seq <= 0 || b.Step == nil || *b.Step < 0 {
		in.sum.Rejected += int64(max(len(b.V), 1))
		return
	}
	if !in.cur.Attempts[b.Attempt].accept(b.Seq, in.u.now) {
		return
	}
	if b.Kind == "step" || b.Kind == "rollout" || b.Kind == "eval" {
		in.stall, in.unstalled = nil, true
	}
	wall := in.u.now
	if b.T > 0 {
		wall = time.Unix(0, int64(b.T*float64(time.Second))).UTC()
	}
	for name, raw := range b.V {
		value, ok := metricValue(raw)
		if !ok || name == "" || len(name) > maxMetricNameLen {
			in.sum.Rejected++
			continue
		}
		in.points = append(in.points, pendingPoint{attempt: b.Attempt, point: &dbclient.PostTrainingRunMetric{
			RunID: in.u.run.RunID, Name: name, Step: int(*b.Step), Value: value, WallTime: wall}})
	}
}

// finish drops the points that a later attempt has replaced, applies the point cap and updates the summary.
// It returns the points to save.
func (in *ingestion) finish() []*dbclient.PostTrainingRunMetric {
	out := make([]*dbclient.PostTrainingRunMetric, 0, len(in.points))
	latest := in.cur.latestIndex()
	for _, p := range in.points {
		system := strings.HasPrefix(p.point.Name, dbclient.PostTrainingSystemMetricPrefix)
		if r := in.cur.resumedAfter(p.attempt); r != nil && !system && int64(p.point.Step) > *r {
			continue
		}
		if !system && attemptIndex(p.attempt) == latest {
			in.summarize(p.point)
		}
		if in.sum.Points >= in.maxPoints {
			in.sum.Capped = true
			continue
		}
		in.sum.Points++
		out = append(out, p.point)
	}
	return out
}

// summarize updates the main metric and the best validation loss with one point.
func (in *ingestion) summarize(p *dbclient.PostTrainingRunMetric) {
	if math.IsNaN(p.Value) || math.IsInf(p.Value, 0) {
		return
	}
	if p.Name == in.mainName && in.mainName != "" {
		if in.sum.Main == nil || in.sum.Main.Name != in.mainName {
			in.sum.Main = &mainMetric{Name: in.mainName}
		}
		in.sum.Main.observe(int64(p.Step), p.Value)
	}
	if valLossPattern.MatchString(p.Name) && (in.sum.BestValLoss == nil || p.Value < in.sum.BestValLoss.Value) {
		in.sum.BestValLoss = &summaryPoint{Step: int64(p.Step), Value: p.Value}
	}
}
