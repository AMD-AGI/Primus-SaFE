/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

const testRun = "pt-qwen-abc12"

// stageLine builds a [STAGE] marker of testRun.
func stageLine(attempt, stage, state, detail string) string {
	body := fmt.Sprintf(`{"run":%q,"attempt":%q,"pv":1,"stage":%q,"state":%q`, testRun, attempt, stage, state)
	if detail != "" {
		body += `,"detail":` + detail
	}
	return "[STAGE] " + body + "}"
}

// metricLine builds a [METRIC] marker of testRun.
func metricLine(attempt string, seq, step int, values string) string {
	return fmt.Sprintf(`[METRIC] {"run":%q,"attempt":%q,"pv":1,"seq":%d,"kind":"step","axis":"train","step":%d,`+
		`"t":1762164000.5,"v":%s}`, testRun, attempt, seq, step, values)
}

// newTestIngestion returns an ingestion over a fresh run of testRun.
func newTestIngestion(maxPoints int64) *ingestion {
	run := &dbclient.PostTrainingRun{RunID: testRun, Status: "Running", Stage: "queued", TrainType: "sft"}
	return &ingestion{u: newRunUpdate(run), cur: loadCursor(""), sum: loadSummary(""), mainName: "train/loss",
		maxPoints: maxPoints}
}

// feed applies marker lines to in.
func feed(in *ingestion, lines ...string) {
	for _, line := range lines {
		if m, ok := parseMarker(line, testRun); ok {
			in.add(m)
		}
	}
}

// pointKeys renders points as name@step=value, sorted.
func pointKeys(points []*dbclient.PostTrainingRunMetric) []string {
	out := make([]string, 0, len(points))
	for _, p := range points {
		out = append(out, fmt.Sprintf("%s@%d=%g", p.Name, p.Step, p.Value))
	}
	sort.Strings(out)
	return out
}

func TestParseMarker(t *testing.T) {
	actor := "\x1b[36m(MegatronTrainRayActor pid=2414, ip=10.245.153.145)\x1b[0m " + metricLine("a1", 3, 2, `{"train/loss":0.5}`)
	m, ok := parseMarker(actor, testRun)
	require.True(t, ok, "an actor's marker is read behind its color codes and Ray prefix")
	require.Equal(t, markerMetric, m.Kind)
	require.Equal(t, int64(3), m.Body.Seq)
	require.True(t, strings.HasPrefix(m.Text, "[METRIC] {"))

	for _, line := range []string{
		"  " + metricLine("a1", 3, 2, `{"train/loss":0.5}`),
		strings.Replace(metricLine("a1", 3, 2, `{}`), testRun, "pt-other", 1),
		"[STAGE] {broken",
		`[STAGE] {"run":"pt-qwen-abc12","stage":"train"}`,
		"step 3: " + metricLine("a1", 3, 2, `{}`),
	} {
		_, ok := parseMarker(line, testRun)
		require.False(t, ok, line)
	}
}

func TestScanLogCursor(t *testing.T) {
	cur := loadCursor("")
	var got []string
	collect := func(_ time.Time, line string) { got = append(got, line) }
	log := "2026-11-03T10:00:00.000000001Z a\n2026-11-03T10:00:00.000000001Z b\n2026-11-03T10:00:01.5Z c\n" +
		"2026-11-03T10:00:02Z unfinish"
	_, err := scanLog(strings.NewReader(log), cur, collect)
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b", "c"}, got, "an unfinished last line waits for the next read")
	require.Equal(t, "2026-11-03T10:00:01.5Z", cur.Since)

	got = nil
	_, err = scanLog(strings.NewReader("2026-11-03T10:00:01.5Z c\n2026-11-03T10:00:02Z unfinished\n"), cur, collect)
	require.NoError(t, err)
	require.Equal(t, []string{"unfinished"}, got, "the overlap of a second-precision SinceTime is skipped")

	cut := &logCursor{Since: "2026-11-03T10:00:00.000000001Z", SinceN: 1}
	got = nil
	_, err = scanLog(strings.NewReader(log[:strings.Index(log, "c\n")]), cut, collect)
	require.NoError(t, err)
	require.Equal(t, []string{"b"}, got, "a read cut between lines with one timestamp resumes after the first")
}

func TestScanLogSkipsLongLines(t *testing.T) {
	long := "2026-11-03T10:00:00Z " + strings.Repeat("x", maxLineBytes+10) + "\n"
	marker := "2026-11-03T10:00:01Z " + stageLine("a1", "train", "running", "") + "\n"
	unfinished := "2026-11-03T10:00:02Z " + strings.Repeat("y", maxLineBytes+100)
	cur := loadCursor("")
	var got []string
	_, err := scanLog(strings.NewReader(long+marker+unfinished), cur, func(_ time.Time, line string) {
		got = append(got, line)
	})
	require.NoError(t, err)
	require.Len(t, got, 1, "the marker after a long line is still read")
	require.Equal(t, int64(2), cur.Skipped, "an unfinished line that is already too long is skipped too")
}

func TestAttemptCursorGaps(t *testing.T) {
	now := time.Unix(1000, 0)
	a := &attemptCursor{}
	require.True(t, a.accept(1, 10, now))
	require.True(t, a.accept(5, 50, now))
	require.False(t, a.accept(5, 50, now), "a replayed seq is ignored")
	require.True(t, a.accept(3, 30, now))
	require.Equal(t, []seqRange{{From: 2, To: 2, Since: 1000, TimeFrom: 10, TimeTo: 30},
		{From: 4, To: 4, Since: 1000, TimeFrom: 30, TimeTo: 50}}, a.Gaps, "each gap keeps the times around it")
	cur := &logCursor{Attempts: map[string]*attemptCursor{"a1": a}}
	require.False(t, cur.hasGap(now.Add(gapGrace)))
	require.True(t, cur.hasGap(now.Add(gapGrace+time.Second)))
	from, to, ok := cur.gapWindow(now.Add(gapGrace + time.Second))
	require.True(t, ok)
	require.Equal(t, []int64{10, 50}, []int64{from.Unix(), to.Unix()})
	a.accept(2, 20, now)
	a.accept(4, 40, now)
	require.Empty(t, a.Gaps)
	require.False(t, cur.hasGap(now.Add(time.Hour)))
}

func TestIngestionResumeDropsReplacedPoints(t *testing.T) {
	in := newTestIngestion(100)
	feed(in,
		stageLine("a1", "prepare", "running", ""),
		stageLine("a1", "train", "running", `{"resumeStep":0}`),
		metricLine("a1", 1, 9, `{"train/loss":1.0}`),
		metricLine("a1", 2, 10, `{"train/loss":0.9,"system/gpu_util":50}`),
		stageLine("a2", "prepare", "skipped", ""),
		stageLine("a2", "train", "running", `{"resumeStep":9}`),
		metricLine("a1", 3, 11, `{"train/loss":0.8}`),
		metricLine("a2", 1, 10, `{"train/loss":0.7}`),
	)
	points := in.finish()
	require.Equal(t, []string{"system/gpu_util@10=50", "train/loss@10=0.7", "train/loss@9=1"}, pointKeys(points))
	require.Equal(t, int64(9), *in.resumeFrom)
	require.Equal(t, "train", in.u.stage)
	require.Equal(t, int64(10), in.sum.Main.Step)
	require.Equal(t, 0.7, in.sum.Main.Value)
	var resumes []string
	for _, e := range in.u.history {
		if e.ResumeStep != nil {
			resumes = append(resumes, fmt.Sprintf("%s@%d", e.Attempt, *e.ResumeStep))
		}
	}
	require.Equal(t, []string{"a2@9"}, resumes, "the first attempt starting at step 0 is not a resume")
}

func TestIngestionStageOrderAndResult(t *testing.T) {
	in := newTestIngestion(100)
	feed(in,
		metricLine("a1", 1, 1, `{"train/loss":1}`),
		stageLine("a1", "train", "running", ""),
		stageLine("a1", "check", "running", ""),
		stageLine("a1", "train", "heartbeat", ""),
		`[SUCCESS] {"run":"pt-qwen-abc12","attempt":"a1","pv":1,"stage":"export","output":"/w/run/final_model"}`,
	)
	require.Empty(t, in.finish(), "metrics of an attempt with no [STAGE] yet are ignored")
	require.Equal(t, "train", in.u.stage, "the stage never moves back")
	require.Equal(t, markerSuccess, in.result.Kind)
}

func TestIngestionValidationCapAndSummary(t *testing.T) {
	in := newTestIngestion(3)
	feed(in, stageLine("a1", "train", "running", ""),
		metricLine("a1", 1, 0, `{"train/loss":"NaN","eval/val/loss":2,"bad":"x","`+strings.Repeat("n", 161)+`":1}`),
		metricLine("a1", 2, 1, `{"eval/val/loss":1.5,"train/loss":3}`),
		fmt.Sprintf(`[METRIC] {"run":%q,"attempt":"a1","seq":3,"step":-1,"v":{"train/loss":1}}`, testRun),
	)
	points := in.finish()
	require.Len(t, points, 3)
	require.True(t, math.IsNaN(points[0].Value) || math.IsNaN(points[1].Value), "NaN is stored")
	require.True(t, in.sum.Capped)
	require.Equal(t, int64(3), in.sum.Rejected)
	require.Equal(t, summaryPoint{Step: 1, Value: 1.5}, *in.sum.BestValLoss)
	require.Equal(t, 3.0, in.sum.Main.Value, "the summary keeps updating past the point cap")
}

func TestSparklineStaysBounded(t *testing.T) {
	m := &mainMetric{Name: "train/loss"}
	for i := 0; i < 1000; i++ {
		m.observe(int64(i), float64(i))
	}
	require.LessOrEqual(t, len(m.Spark), sparklinePoints)
	require.Greater(t, len(m.Spark), sparklinePoints/4)
	require.Equal(t, int64(0), m.Spark[0].Step)
	require.Equal(t, int64(999), m.Step)
}

func TestIngestionStallDiskAndNoRetry(t *testing.T) {
	in := newTestIngestion(100)
	feed(in, stageLine("a1", "train", "running", ""),
		stageLine("a1", "train", "stalled", `{"since":1762160000.5,"lastStep":340,"minutes":31,"stopMinutes":60}`),
		stageLine("a1", "train", "disk", `{"checkpoints":1024,"total":1024}`))
	require.Equal(t, 340, int(*in.stall.LastStep))
	require.Equal(t, `{"checkpoints":1024,"total":1024}`, in.disk)

	u := in.u
	syncStall(u, in, true)
	require.Equal(t, time.Unix(1762160000, 500000000).UTC(), u.fields["stalled_since"])
	require.Equal(t, "No training progress for 31 minutes (last step 340); the attempt is stopped and retried at "+
		"60 minutes", u.fields["message"])

	feed(in, metricLine("a1", 1, 341, `{"train/loss":1}`),
		stageLine("a1", "train", "failed", `{"stage":"train","reason":"disk full"}`))
	require.Nil(t, in.stall, "a new training step clears the stall")
	require.True(t, in.unstalled)
	require.False(t, in.noRetry, "a failed stage without retry false keeps failover")
	feed(in, strings.Replace(stageLine("a1", "train", "failed", ""), `"state"`, `"retry":false,"state"`, 1))
	require.True(t, in.noRetry)
}

func TestLostLauncherIsMarkedAndCleared(t *testing.T) {
	in := newTestIngestion(100)
	in.cur.LastStage = time.Now().Add(-11 * time.Minute).Unix()
	syncStall(in.u, in, true)
	require.Contains(t, in.u.fields["message"], lostMessage)
	require.NotNil(t, in.u.fields["stalled_since"])

	in = newTestIngestion(100)
	in.u.run.StalledSince.Valid, in.u.run.Message = true, lostMessage+" for 11 minutes"
	feed(in, stageLine("a1", "train", "heartbeat", ""))
	syncStall(in.u, in, true)
	require.Contains(t, in.u.fields, "stalled_since")
	require.Nil(t, in.u.fields["stalled_since"], "any [STAGE] from a lost launcher clears the mark")
	require.Equal(t, "", in.u.fields["message"])
}

func TestLinkMarkers(t *testing.T) {
	in := newTestIngestion(100)
	link := func(v string) string {
		return fmt.Sprintf(`[METRIC] {"run":%q,"attempt":"a1","pv":1,"kind":"link","t":1762164000.5,"v":%s}`, testRun, v)
	}
	feed(in, stageLine("a1", "train", "running", ""), link(`{"wandb":"https://wandb.ai/team/pt/runs/abc"}`),
		metricLine("a1", 1, 1, `{"train/loss":2}`))
	require.Equal(t, map[string]string{"wandb": "https://wandb.ai/team/pt/runs/abc"}, in.sum.Links)
	require.True(t, in.linked)
	require.Len(t, in.finish(), 1, "a link is not a point")
	require.Empty(t, in.cur.Attempts["a1"].Gaps, "a link has no seq and leaves no gap")

	again := newTestIngestion(100)
	again.sum.Links = map[string]string{"wandb": "https://wandb.ai/team/pt/runs/abc"}
	feed(again, stageLine("a1", "train", "running", ""),
		link(`{"wandb":"https://wandb.ai/team/pt/runs/abc","x":"javascript:alert(1)","y":"ftp://h/f","z":3}`))
	require.False(t, again.linked, "an unchanged link and links that are not http(s) URLs change nothing")
	require.Len(t, again.sum.Links, 1)
}

func TestSampleMarkers(t *testing.T) {
	in := newTestIngestion(100)
	sample := func(attempt string, step int, kind string) string {
		return fmt.Sprintf(`[SAMPLE] {"run":%q,"attempt":%q,"pv":1,"kind":%q,"step":%d,"dataset":"","group":2,"index":1,`+
			`"prompt":"2+2?","response":"4","label":"4","reward":1.0,"status":"completed","tokens":1,"promptChars":4,`+
			`"responseChars":1}`, testRun, attempt, kind, step)
	}
	feed(in, stageLine("a1", "train", "running", ""), sample("a1", 5, "rollout"), sample("a1", 9, "rollout"),
		sample("a1", 3, "other"), stageLine("a2", "train", "running", `{"resumeStep":6}`), sample("a2", 7, "eval"))
	rows := in.finishSamples()
	require.Len(t, rows, 2, "a1's step 9 is past a2's resume and the unknown kind is dropped")
	require.Equal(t, 5, rows[0].Step)
	require.Equal(t, "eval", rows[1].Kind)
	require.Equal(t, 2, rows[0].GroupIndex)
	require.True(t, rows[0].Reward.Valid)
	require.Equal(t, "4", rows[0].Label.String)
}
