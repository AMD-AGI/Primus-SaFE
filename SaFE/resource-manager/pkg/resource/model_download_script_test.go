/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonopsjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
)

// stubHFCLI stands in for the HuggingFace CLI. It "downloads" every file of the
// repository listed in $STUB_FILES that matches one of the --include patterns, writing
// the file's own name as its content, and logs the patterns it was asked for. It reads
// both the huggingface-cli form (--include a b c) and the hf form (--include a --include b).
const stubHFCLI = `#!/bin/sh
set -f
shift
repo=$1; shift
inc=""; mode=""
while [ $# -gt 0 ]; do
  case "$1" in
    --local-dir) dir=$2; mode=""; shift 2 ;;
    --include) mode=inc; shift ;;
    --exclude) mode=exc; shift ;;
    *) [ "$mode" = inc ] && inc="$inc $1"; shift ;;
  esac
done
echo "fetch $repo:$inc" >> "$STUB_LOG"
[ "${STUB_FAIL:-}" = 1 ] && exit 7
for f in $STUB_FILES; do
  for p in $inc; do
    case "$f" in
      $p) mkdir -p "$dir/$(dirname "$f")"; printf '%s' "$f" > "$dir/$f"; break ;;
    esac
  done
done
exit 0
`

type scriptRun struct {
	out   string
	ok    bool
	dest  string
	fetch string
}

// runHFDownloadScript runs hfDownloadScript with the stub CLI installed as cli against
// a repository holding files.
func runHFDownloadScript(t *testing.T, cli string, files []string, fail bool) scriptRun {
	t.Helper()
	for _, name := range []string{"hf", "huggingface-cli"} {
		if p, err := exec.LookPath(name); err == nil {
			t.Skipf("a real %s is installed at %s", name, p)
		}
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	require.NoError(t, os.MkdirAll(bin, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bin, cli), []byte(stubHFCLI), 0o755))
	dest := filepath.Join(tmp, "models", "org--repo")
	log := filepath.Join(tmp, "fetch.log")

	cmd := exec.Command("sh", "-c", hfDownloadScript)
	cmd.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HF_REPO_ID=org/repo", "DEST_PATH="+dest, "STUB_LOG="+log,
		"STUB_FILES="+strings.Join(files, " "))
	if fail {
		cmd.Env = append(cmd.Env, "STUB_FAIL=1")
	}
	out, err := cmd.CombinedOutput()
	fetched, _ := os.ReadFile(log)
	return scriptRun{out: string(out), ok: err == nil, dest: dest, fetch: string(fetched)}
}

// downloadOutputs turns what the download printed into the outputs of its OpsJob the
// way the platform does: the job-manager keeps the marked lines of the pod log, and the
// OpsJob controller stores them as the "result" output and the completion message.
func downloadOutputs(log string, succeeded bool) *v1.OpsJob {
	kept := commonopsjob.FilterResultLog([]byte(log))
	job := &v1.OpsJob{Status: v1.OpsJobStatus{Phase: v1.OpsJobSucceeded}}
	if kept != "" {
		job.Status.Outputs = []v1.Parameter{{Name: "result", Value: kept}}
	}
	if !succeeded {
		job.Status.Phase = v1.OpsJobFailed
		job.Status.Conditions = []metav1.Condition{{Type: opsJobCompletedCondition, Status: metav1.ConditionFalse,
			Reason: opsJobFailedReason, Message: kept}}
	}
	return job
}

func dirSize(t *testing.T, root string) int64 {
	t.Helper()
	out, err := exec.Command("du", "-sb", "--exclude=.cache", root).Output()
	require.NoError(t, err)
	size, err := strconv.ParseInt(strings.Fields(string(out))[0], 10, 64)
	require.NoError(t, err)
	return size
}

// TestHFDownloadScriptWeights runs the real download script: safetensors are preferred,
// a repository without them gets its PyTorch weights, and one with neither fails with
// the reason instead of reporting a model without weights as downloaded.
func TestHFDownloadScriptWeights(t *testing.T) {
	support := []string{"config.json", "tokenizer.json", "tokenizer_config.json"}
	for _, cli := range []string{"hf", "huggingface-cli"} {
		t.Run(cli+"/safetensors", func(t *testing.T) {
			run := runHFDownloadScript(t, cli, append([]string{"model.safetensors", "pytorch_model.bin", "tf_model.h5"}, support...), false)
			require.True(t, run.ok, run.out)
			assert.FileExists(t, filepath.Join(run.dest, "model.safetensors"))
			assert.FileExists(t, filepath.Join(run.dest, "config.json"))
			assert.NoFileExists(t, filepath.Join(run.dest, "pytorch_model.bin"), "a second copy of the weights is not fetched")
			assert.NoFileExists(t, filepath.Join(run.dest, "tf_model.h5"))
			assert.Equal(t, 1, strings.Count(run.fetch, "fetch "), run.fetch)
		})
		t.Run(cli+"/pytorch-only", func(t *testing.T) {
			// e.g. EleutherAI/gpt-j-6b: PyTorch and TensorFlow weights, no safetensors.
			run := runHFDownloadScript(t, cli, append([]string{"pytorch_model.bin", "tf_model.h5", "flax_model.msgpack"}, support...), false)
			require.True(t, run.ok, run.out)
			assert.FileExists(t, filepath.Join(run.dest, "pytorch_model.bin"))
			assert.FileExists(t, filepath.Join(run.dest, "config.json"))
			assert.NoFileExists(t, filepath.Join(run.dest, "tf_model.h5"))
			assert.Contains(t, run.fetch, "*.bin")
		})
		t.Run(cli+"/no-weights", func(t *testing.T) {
			run := runHFDownloadScript(t, cli, append([]string{"model-q4_k_m.gguf", "tf_model.h5"}, support...), false)
			require.False(t, run.ok, "a download without weights must fail: %s", run.out)
			assert.NoFileExists(t, filepath.Join(run.dest, "model-q4_k_m.gguf"), "GGUF quantizations are not fetched")
		})
	}
}

// TestHFDownloadScriptSizeReachesModel follows the size from the real script's log,
// through the job-manager log filter, into the OpsJob outputs and the model's sizeBytes.
func TestHFDownloadScriptSizeReachesModel(t *testing.T) {
	run := runHFDownloadScript(t, "hf", []string{"model.safetensors", "config.json"}, false)
	require.True(t, run.ok, run.out)
	want := dirSize(t, run.dest)
	require.Positive(t, want)
	job := downloadOutputs(run.out, true)
	assert.Equal(t, want, reportedModelSize(job), "log: %s", run.out)
}

// TestHFDownloadScriptFailureReachesModel: why a download failed reaches the model.
func TestHFDownloadScriptFailureReachesModel(t *testing.T) {
	r := newMockModelReconciler(nil)

	run := runHFDownloadScript(t, "hf", []string{"model-q4_k_m.gguf", "config.json"}, false)
	require.False(t, run.ok)
	reason := r.extractOpsJobFailureReason(downloadOutputs(run.out, false))
	assert.Contains(t, reason, "has no safetensors or PyTorch")
	assert.Contains(t, reason, "org/repo")

	run = runHFDownloadScript(t, "hf", []string{"model.safetensors"}, true)
	require.False(t, run.ok)
	assert.Contains(t, r.extractOpsJobFailureReason(downloadOutputs(run.out, false)), "downloading org/repo failed")
}
