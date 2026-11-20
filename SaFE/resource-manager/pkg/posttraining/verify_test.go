/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonopsjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
)

// runVerify runs the verification script on dir and returns its result as the job would report it.
func runVerify(t *testing.T, dir string) (string, *verifyResult) {
	cmd := exec.Command("sh", "-c", runVerifyScript)
	cmd.Env = append(os.Environ(), "DEST_PATH="+dir)
	out, _ := cmd.CombinedOutput()
	job := &v1.OpsJob{Status: v1.OpsJobStatus{Outputs: []v1.Parameter{{Name: "result",
		Value: commonopsjob.FilterResultLog(out)}}}}
	return parseVerifyJob(job)
}

// The verify script finds a complete output by its weights and .done mark.
func TestRunVerifyScript(t *testing.T) {
	out, err := exec.Command("sh", "-n", "-c", runVerifyScript).CombinedOutput()
	require.NoError(t, err, string(out))
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	dir := t.TempDir()
	final := filepath.Join(dir, "final_model")
	require.NoError(t, os.MkdirAll(final, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(final, "model.safetensors"), make([]byte, 100), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "result.json"),
		[]byte(`{"stage":"export","output":"`+final+`","iteration":10}`), 0o644))

	kind, res := runVerify(t, dir)
	require.Equal(t, markerError, kind, "an output without its .done mark is incomplete")
	require.JSONEq(t, `{"checkpoints":0,"criticCheckpoints":0,"finalModel":100,"finalAdapter":0,"total":100}`,
		string(res.Disk))

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".noretry"), []byte(`{"stage":"prepare","reason":"bad data"}`), 0o644))
	kind, res = runVerify(t, dir)
	require.Equal(t, markerError, kind)
	require.Equal(t, "prepare", res.Stage)
	require.Equal(t, "bad data", res.Reason)

	require.NoError(t, os.WriteFile(filepath.Join(final, ".done"), nil, 0o644))
	kind, res = runVerify(t, dir)
	require.Equal(t, markerSuccess, kind)
	require.Equal(t, final, res.Output)
	iteration, ok := res.lastCheckpoint()
	require.True(t, ok, "the result names the last checkpoint")
	require.Equal(t, 10, iteration)

	bare := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bare, "result.json"),
		[]byte(`{"stage":"export","output":"","iteration":10}`), 0o644))
	kind, res = runVerify(t, bare)
	require.Equal(t, markerSuccess, kind, "a launcher that finished without exporting wrote a result with no output")
	require.Empty(t, res.Output)
	iteration, ok = res.lastCheckpoint()
	require.True(t, ok)
	require.Equal(t, 10, iteration)

	// A result with values that JSON does not have, or with numbers too large for one, is still read: they count
	// as none.
	for _, result := range []string{
		`{"stage":"export","output":"","iteration":NaN,"lastMetrics":{"train/loss":-Infinity}}`,
		`{"stage":"export","output":"","iteration":1e999,"lastMetrics":{"train/loss":-1e999}}`,
	} {
		odd := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(odd, "result.json"), []byte(result), 0o644))
		kind, res = runVerify(t, odd)
		require.Equal(t, markerSuccess, kind, result)
		require.Equal(t, "export", res.Stage, result)
		_, ok = res.lastCheckpoint()
		require.False(t, ok, result)
	}
}

// A verify job name is short and the same for a run every time.
func TestVerifyJobName(t *testing.T) {
	name := verifyJobName("pt-a-very-long-display-name-of-a-post-training-run-abc12")
	require.Len(t, name, len("pt-verify-")+16)
	require.Equal(t, name, verifyJobName("pt-a-very-long-display-name-of-a-post-training-run-abc12"))
}
