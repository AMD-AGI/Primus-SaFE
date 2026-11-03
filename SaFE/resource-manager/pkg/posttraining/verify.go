/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

const (
	verifyMarker  = "VERIFY="
	verifyTimeout = 30 * time.Minute
)

// runVerifyScript checks the run directory $DEST_PATH: with result.json and the .done mark of its output it
// prints the result after VERIFY= on a "[SUCCESS]" line, otherwise an "[ERROR]" line with the .noretry reason
// when there is one. Both carry the sizes of the directories that hold checkpoints and exported weights.
const runVerifyScript = `set -u
if ! command -v python3 >/dev/null 2>&1; then
  echo "[ERROR] VERIFY={}"
  exit 1
fi
python3 -c '
import json, os, sys
d = sys.argv[1]
def size(p):
    total = 0
    for root, _, names in os.walk(p):
        for n in names:
            try:
                total += os.path.getsize(os.path.join(root, n))
            except OSError:
                pass
    return total
def load(name):
    try:
        with open(os.path.join(d, name)) as f:
            return json.load(f)
    except (OSError, ValueError):
        return None
disk = {k: size(os.path.join(d, n)) for k, n in (("checkpoints", "checkpoints"),
        ("criticCheckpoints", "critic_checkpoints"), ("finalModel", "final_model"), ("finalAdapter", "final_adapter"))}
disk["total"] = sum(disk.values())
out = {"disk": disk}
result = load("result.json")
if isinstance(result, dict) and result.get("output") and os.path.exists(os.path.join(result["output"], ".done")):
    out.update(result)
    print("[SUCCESS] VERIFY=" + json.dumps(out, separators=(",", ":")))
    sys.exit(0)
noretry = load(".noretry")
if isinstance(noretry, dict):
    out.update(noretry)
print("[ERROR] VERIFY=" + json.dumps(out, separators=(",", ":")))
sys.exit(1)
' "$DEST_PATH"
`

// verifyResult is what the verification job reports about a run directory.
type verifyResult struct {
	Output string          `json:"output"`
	Stage  string          `json:"stage"`
	Reason string          `json:"reason"`
	Disk   json.RawMessage `json:"disk"`
}

// verifyJobName is the name of the job that checks a run's directory.
func verifyJobName(runID string) string {
	sum := sha256.Sum256([]byte(runID))
	return "pt-verify-" + hex.EncodeToString(sum[:8])
}

// verify finishes a run whose result marker is missing by checking its run directory with a model-inspect
// job: a complete output is registered, and anything else fails the run with the .noretry reason, or with
// fallbackStage and fallbackReason. The job's run directory usage is recorded either way.
func (r *RunReconciler) verify(ctx context.Context, u *runUpdate, endTime time.Time, fallbackStage,
	fallbackReason string) (ctrlruntime.Result, error) {
	run := u.run
	job := &v1.OpsJob{}
	err := r.Get(ctx, client.ObjectKey{Name: verifyJobName(run.RunID)}, job)
	if apierrors.IsNotFound(err) {
		if run.OutputPath == "" {
			u.fail(fallbackStage, fallbackReason, endTime)
			return ctrlruntime.Result{}, r.save(ctx, u)
		}
		if err = r.Create(ctx, newVerifyJob(run)); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrlruntime.Result{}, err
		}
		u.event("verify", "The result marker is missing; checking the run directory")
		return ctrlruntime.Result{RequeueAfter: pendingRequeue}, r.save(ctx, u)
	}
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if job.Status.Phase != v1.OpsJobSucceeded && job.Status.Phase != v1.OpsJobFailed {
		if u.now.Sub(job.CreationTimestamp.Time) > verifyTimeout {
			u.fail("export", "Unable to verify the output of the run", endTime)
			return ctrlruntime.Result{}, r.save(ctx, u)
		}
		return ctrlruntime.Result{RequeueAfter: pendingRequeue}, nil
	}
	kind, res := parseVerifyJob(job)
	if res != nil && len(res.Disk) > 0 {
		u.set("disk_usage", run.DiskUsage, string(res.Disk))
	}
	if kind == markerSuccess && res.Output != "" {
		return r.register(ctx, u, &marker{Kind: markerSuccess, Body: markerBody{Run: run.RunID, Output: res.Output}}, endTime)
	}
	stage, reason := fallbackStage, fallbackReason
	if res != nil && res.Reason != "" {
		reason = res.Reason
		if launcherStage(res.Stage) {
			stage = res.Stage
		}
	}
	u.fail(stage, reason, endTime)
	return ctrlruntime.Result{}, r.save(ctx, u)
}

// newVerifyJob builds the model-inspect job that checks the run directory in the run's workspace.
func newVerifyJob(run *dbclient.PostTrainingRun) *v1.OpsJob {
	image := commonconfig.GetDownloadJoImage()
	entryPoint := base64.StdEncoding.EncodeToString([]byte(runVerifyScript))
	name := verifyJobName(run.RunID)
	return &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				v1.ClusterIdLabel:         run.Cluster,
				v1.WorkspaceIdLabel:       run.Workspace,
				v1.PostTrainingRunIdLabel: run.RunID,
				v1.DisplayNameLabel:       name,
				v1.UserIdLabel:            run.UserID,
				v1.OpsJobTypeLabel:        string(v1.OpsJobModelInspectType),
			},
			Annotations: map[string]string{v1.UserNameAnnotation: run.UserName},
		},
		Spec: v1.OpsJobSpec{
			Type:                    v1.OpsJobModelInspectType,
			Image:                   &image,
			EntryPoint:              &entryPoint,
			TimeoutSecond:           int(verifyTimeout / time.Second),
			TTLSecondsAfterFinished: 3600,
			Inputs: []v1.Parameter{
				{Name: v1.ParameterDestPath, Value: run.OutputPath},
				{Name: v1.ParameterWorkspace, Value: run.Workspace},
			},
		},
	}
}

// parseVerifyJob returns the marker kind and the result of the VERIFY= line in a finished job's outputs, or
// in its completion message when the outputs are empty.
func parseVerifyJob(job *v1.OpsJob) (string, *verifyResult) {
	texts := make([]string, 0, len(job.Status.Outputs)+len(job.Status.Conditions))
	for _, out := range job.Status.Outputs {
		texts = append(texts, out.Value)
	}
	for _, c := range job.Status.Conditions {
		texts = append(texts, c.Message)
	}
	for _, text := range texts {
		var lines []string
		if json.Unmarshal([]byte(text), &lines) != nil {
			lines = strings.Split(text, "\n")
		}
		for _, line := range lines {
			for _, kind := range []string{markerSuccess, markerError} {
				prefix := kind + " " + verifyMarker
				if !strings.HasPrefix(line, prefix) {
					continue
				}
				res := &verifyResult{}
				if json.Unmarshal([]byte(line[len(prefix):]), res) == nil {
					return kind, res
				}
			}
		}
	}
	return "", nil
}
