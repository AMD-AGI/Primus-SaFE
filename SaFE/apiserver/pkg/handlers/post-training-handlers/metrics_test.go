/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

// rows builds the points of one metric from (step, value) pairs.
func rows(name string, pairs ...float64) []*dbclient.PostTrainingRunMetric {
	var out []*dbclient.PostTrainingRunMetric
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, &dbclient.PostTrainingRunMetric{Name: name, Step: int(pairs[i]), Value: pairs[i+1]})
	}
	return out
}

func TestMatchName(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"train/loss", "train/loss", true},
		{"eval/*", "eval/aime", true},
		{"eval/*", "eval/val/loss", false},
		{"eval/*/loss", "eval/val/loss", true},
		{"train/lr-pg_*", "train/lr-pg_0", true},
		{"eval/*-pass@*", "eval/aime-pass@4", true},
		{"eval/*-pass@*", "eval/aime", false},
		{"rollout/*/*", "rollout/response_len/max", true},
		{"", "train/loss", false},
	}
	for _, c := range cases {
		require.Equal(t, c.want, MatchName(c.pattern, c.name), "%s ~ %s", c.pattern, c.name)
	}
}

func TestAxisOf(t *testing.T) {
	require.Equal(t, AxisTime, axisOf("system/gpu_util_mean", "rl"))
	require.Equal(t, AxisStep, axisOf("train/pg_loss", "rl"))
	require.Equal(t, AxisRollout, axisOf("rollout/raw_reward", "rl"))
	require.Equal(t, AxisStep, axisOf("eval/val/loss", "sft"))
}

func TestBuildSeries(t *testing.T) {
	s := buildSeries("train/loss", AxisStep, rows("train/loss", 1, 2.0, 2, math.NaN(), 3, math.Inf(1), 4, 1.5), 100)
	require.Equal(t, 4, s.Count)
	require.Equal(t, [][2]float64{{1, 2.0}, {4, 1.5}}, s.Points)
	require.Equal(t, [][2]interface{}{{2, "NaN"}, {3, "Inf"}}, s.NonFinite)
	require.Equal(t, [2]float64{4, 1.5}, *s.Last)
	require.False(t, s.Downsampled)

	var pairs []float64
	for i := 0; i < 1000; i++ {
		v := 1.0
		if i == 500 {
			v = 9.0
		}
		pairs = append(pairs, float64(i), v)
	}
	d := buildSeries("train/loss", AxisStep, rows("train/loss", pairs...), 100)
	require.True(t, d.Downsampled)
	require.LessOrEqual(t, len(d.Points), 100)
	peak := false
	for _, p := range d.Points {
		if p[1] == 9.0 {
			peak = true
		}
	}
	require.True(t, peak, "the spike survives downsampling")
	require.Equal(t, 1000, d.Count)
}

func TestBuildMarkers(t *testing.T) {
	run := &dbclient.PostTrainingRun{TrainType: "sft", Status: string(v1.WorkloadFailed), FailedStage: "train",
		Message:      "no progress for 60 minutes",
		Recipe:       `{"choices":{"saveInterval":10},"checkpoint":{"keepLast":1,"keepBest":true}}`,
		StageHistory: `[{"stage":"queued"},{"stage":"train","resumeStep":10,"time":"t1"}]`}
	points := map[string][]*dbclient.PostTrainingRunMetric{
		"train/loss":    rows("train/loss", 1, 3, 10, 2, 20, 1.5, 30, 1.2, 35, 1.1),
		"eval/val/loss": rows("eval/val/loss", 10, 2.1, 20, 1.9, 30, 2.0),
	}
	m := buildMarkers(run, points)
	require.Len(t, m.Checkpoints, 3)
	require.Equal(t, 20, m.BestCheckpoint.Step)
	require.True(t, m.Checkpoints[0].Deleted, "older than keepLast and not the best")
	require.False(t, m.Checkpoints[1].Deleted, "the best is kept")
	require.False(t, m.Checkpoints[2].Deleted, "the most recent is kept")
	require.Equal(t, []resumeMark{{Step: 10, Time: "t1"}}, m.Resumes)
	require.Equal(t, &failureMark{Step: 35, Reason: "no progress for 60 minutes"}, m.Failure)

	run.Status, run.Recipe = string(v1.WorkloadRunning), `{}`
	m = buildMarkers(run, points)
	require.Empty(t, m.Checkpoints)
	require.Nil(t, m.BestCheckpoint)
	require.Nil(t, m.Failure)
}

func TestStopWorkload(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pt-x"}, Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning}}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(w).WithStatusSubresource(w).Build()
	h := NewHandler(cli, nil)
	require.NoError(t, h.stopWorkload(context.Background(), "pt-x", "stopped by alice"))
	err := cli.Get(context.Background(), ctrlclient.ObjectKey{Name: "pt-x"}, &v1.Workload{})
	require.Error(t, err, "the workload is deleted")
	require.NoError(t, h.stopWorkload(context.Background(), "missing", "x"), "a missing workload is fine")
}

func TestFrameworkOf(t *testing.T) {
	reg, err := registry.Parse([]byte(testRegistry))
	require.NoError(t, err)
	require.Equal(t, "slime", frameworkOf(reg, "").Name, "runs without a framework are slime runs")
	require.Nil(t, frameworkOf(reg, "verl"))
	require.Nil(t, frameworkOf(nil, "slime"))
}
