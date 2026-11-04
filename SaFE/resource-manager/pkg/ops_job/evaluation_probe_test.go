/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testProbes = `[{"prompt":"What is the capital of France? Answer with one word.","expect":"exact","answer":"Paris"},` +
	`{"prompt":"Write a Triton kernel that adds two vectors.","expect":"pythonCode"}]`

// probeServer answers chat completions with reply(prompt).
func probeServer(t *testing.T, reply func(prompt string) string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"choices": []map[string]interface{}{
			{"message": map[string]string{"content": reply(req.Messages[0].Content)}}}})
	}))
}

// runProbes runs the Python part of outputProbeScript against url and returns its output and error.
func runProbes(t *testing.T, url string) (string, error) {
	start := strings.Index(outputProbeScript, "<<'PROBE_EOF'\n") + len("<<'PROBE_EOF'\n")
	code := outputProbeScript[start:strings.Index(outputProbeScript, "\nPROBE_EOF\n")]
	cmd := exec.Command("python3", "-c", code)
	cmd.Env = append(os.Environ(), "EVAL_PROBES="+testProbes, "EVAL_PROBE_ENDPOINT="+url, "EVAL_PROBE_MODEL=m-1")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestOutputProbes(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	fence := strings.Repeat("`", 3)
	good := probeServer(t, func(prompt string) string {
		if strings.Contains(prompt, "France") {
			return "<think>The capital is Paris.</think>\n\n Paris.\n"
		}
		return "Here it is:\n" + fence + "python\nimport triton\n\ndef add(x, y):\n    return x + y\n" + fence
	})
	defer good.Close()
	out, err := runProbes(t, good.URL)
	require.NoError(t, err, out)
	assert.Contains(t, out, "all 2 probes passed")

	garbled := probeServer(t, func(string) string { return "Paris Paris the the the" })
	defer garbled.Close()
	out, err = runProbes(t, garbled.URL)
	assert.Error(t, err)
	assert.Contains(t, out, "probe 1 got an unexpected reply")
}

func TestEvalCommandRunsProbesAfterTheEndpointCheck(t *testing.T) {
	r := &EvaluationJobReconciler{}
	benchmarks := `[{"datasetId":"d1","datasetName":"gsm8k"}]`
	cmd, err := r.buildEvalCommand(context.Background(), "http://m", "model", "", benchmarks, "task1", "", "", "", "",
		7200, 32, true)
	require.NoError(t, err)
	models, probes := strings.Index(cmd, `"http://m/models"`), strings.Index(cmd, "probing the model output")
	assert.True(t, models >= 0 && probes > models, "probes run once /models answers")
	out, err := exec.Command("sh", "-n", "-c", cmd).CombinedOutput()
	require.NoError(t, err, string(out))
	cmd, err = r.buildEvalCommand(context.Background(), "http://m", "model", "", benchmarks, "task1", "", "", "", "",
		7200, 32, false)
	require.NoError(t, err)
	assert.NotContains(t, cmd, "probing the model output", "evaluations without probes are unchanged")
}
