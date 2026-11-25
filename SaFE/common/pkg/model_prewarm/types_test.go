/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_prewarm

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runPreloadScript runs the preload script with bash and returns its output and exit code.
func runPreloadScript(t *testing.T, modelPath string) (string, int) {
	t.Helper()
	out, err := exec.Command("bash", "-c", PreloadScript(modelPath, DefaultGlob, DefaultParallelism)).CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "unexpected error: %v", err)
	return string(out), exitErr.ExitCode()
}

func TestPreloadScriptReportsBytesOfMatchingFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.safetensors"), make([]byte, 1000), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.safetensors"), make([]byte, 500), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), make([]byte, 300), 0o644))

	out, code := runPreloadScript(t, dir)
	assert.Equal(t, 0, code, out)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	assert.Equal(t, "1500", lines[len(lines)-1])
}

func TestPreloadScriptFailsOnMissingDirectory(t *testing.T) {
	out, code := runPreloadScript(t, filepath.Join(t.TempDir(), "missing"))
	assert.NotEqual(t, 0, code)
	assert.Contains(t, out, "model path not found")
}

func TestPreloadScriptFailsWithoutWeightFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o644))

	out, code := runPreloadScript(t, dir)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, out, "no matching regular weight files")
}

func TestPreloadScriptQuotesModelPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "it's model")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "w.safetensors"), make([]byte, 64), 0o644))

	out, code := runPreloadScript(t, dir)
	assert.Equal(t, 0, code, out)
	assert.True(t, strings.HasSuffix(strings.TrimSpace(out), "64"), out)
}

func TestPodEntryScriptEmbedsPreloadAndTerminationMessage(t *testing.T) {
	script := PodEntryScript("/models/glm", DefaultGlob, 2)
	assert.Contains(t, script, PreloadScript("/models/glm", DefaultGlob, 2))
	assert.Contains(t, script, "/dev/termination-log")
}

func TestParseTerminationMessage(t *testing.T) {
	msg, err := ParseTerminationMessage(`{"bytesRead":42,"durationSeconds":7}` + "\n")
	require.NoError(t, err)
	assert.Equal(t, int64(42), msg.BytesRead)
	assert.Equal(t, int64(7), msg.DurationSeconds)
	assert.Empty(t, msg.Error)

	failed, err := ParseTerminationMessage(`{"error":"boom"}`)
	require.NoError(t, err)
	assert.Equal(t, "boom", failed.Error)

	_, err = ParseTerminationMessage("not json")
	assert.Error(t, err)
}

func TestParseRequestAndResultRoundTrip(t *testing.T) {
	raw, err := MarshalRequest(&Request{OpsJobId: "job", ModelPath: "/models/glm", Glob: DefaultGlob,
		Parallelism: 4, TimeoutSeconds: 60})
	require.NoError(t, err)
	req, err := ParseRequest(raw)
	require.NoError(t, err)
	assert.Equal(t, "/models/glm", req.ModelPath)

	raw, err = MarshalResult(&Result{OpsJobId: "job", Phase: PhaseSucceeded, BytesRead: 9})
	require.NoError(t, err)
	res, err := ParseResult(raw)
	require.NoError(t, err)
	assert.Equal(t, int64(9), res.BytesRead)

	_, err = ParseRequest("{")
	assert.Error(t, err)
	_, err = ParseResult("{")
	assert.Error(t, err)
}

func TestValidateModelPath(t *testing.T) {
	assert.NoError(t, ValidateModelPath("/shared_nfs/models/GLM-5.3"))
	assert.Error(t, ValidateModelPath(""))
	assert.Error(t, ValidateModelPath("relative/path"))
	assert.Error(t, ValidateModelPath("/shared_nfs/../etc/passwd"))
}

func TestValidateGlob(t *testing.T) {
	assert.NoError(t, ValidateGlob("*.safetensors"))
	assert.Error(t, ValidateGlob(""))
	assert.Error(t, ValidateGlob("*.safetensors;rm -rf /"))
}

func TestMarshalRoundTrip(t *testing.T) {
	req := &Request{
		OpsJobId:    "job-1",
		ModelPath:   "/models/foo",
		Glob:        "*.safetensors",
		Parallelism: 4,
	}
	raw, err := MarshalRequest(req)
	assert.NoError(t, err)
	parsed, err := ParseRequest(raw)
	assert.NoError(t, err)
	assert.Equal(t, req.OpsJobId, parsed.OpsJobId)
	assert.Equal(t, req.ModelPath, parsed.ModelPath)
}

func TestAnnotationKeys(t *testing.T) {
	jobUID := "edcb8cbd-b452-405e-b368-65e15734b726"
	reqKey := RequestAnnotationKey(jobUID)
	resKey := ResultAnnotationKey(jobUID)
	assert.Equal(t, "primus-safe.ops.job.mp.r."+jobUID, reqKey)
	assert.Equal(t, "primus-safe.ops.job.mp.s."+jobUID, resKey)
	assert.LessOrEqual(t, len(reqKey), MaxK8sAnnotationKeyLength)
	assert.LessOrEqual(t, len(resKey), MaxK8sAnnotationKeyLength)
	assert.NoError(t, ValidateAnnotationKeySuffix(jobUID))
}

func TestTruncateMessage(t *testing.T) {
	long := strings.Repeat("x", MaxResultMessageLength+100)
	truncated := TruncateMessage(long)
	assert.Equal(t, MaxResultMessageLength, len(truncated))
}

func TestRequestTimeout(t *testing.T) {
	assert.Equal(t, time.Duration(DefaultTimeoutSeconds)*time.Second, RequestTimeout(nil))
	assert.Equal(t, 600*time.Second, RequestTimeout(&Request{TimeoutSeconds: 600}))
}

func TestAnnotationKeyFitsProductionJobName(t *testing.T) {
	// Regression: prewarm-glm53-ws-ntvmh exceeded the old 64-char key limit.
	jobUID := "edcb8cbd-b452-405e-b368-65e15734b726"
	key := RequestAnnotationKey(jobUID)
	assert.LessOrEqual(t, len(key), MaxK8sAnnotationKeyLength)
}
