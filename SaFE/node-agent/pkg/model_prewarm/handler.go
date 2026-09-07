/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_prewarm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"

	modelprewarm "github.com/AMD-AIG-AIMA/SAFE/common/pkg/model_prewarm"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/utils"
)

var errPrewarmCancelled = errors.New("model prewarm cancelled")
var errRequestRevoked = errors.New("model prewarm request revoked")

type jsonPatchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value,omitempty"`
}

type jobRunner struct {
	cancel context.CancelFunc
}

// Handler executes model prewarm requests observed on the local Kubernetes node.
type Handler struct {
	ctx       context.Context
	nodeName  string
	k8sClient typedcorev1.CoreV1Interface
	mu        sync.Mutex
	running   map[string]*jobRunner
}

// NewHandler creates a model prewarm handler for the given node.
func NewHandler(ctx context.Context, nodeName string, k8sClient typedcorev1.CoreV1Interface) *Handler {
	return &Handler{
		ctx:       ctx,
		nodeName:  nodeName,
		k8sClient: k8sClient,
		running:   make(map[string]*jobRunner),
	}
}

// HandleNodeUpdate scans node annotations and starts or cancels model prewarm jobs.
func (h *Handler) HandleNodeUpdate(node *corev1.Node) {
	requestPrefix := modelprewarm.RequestAnnotationPrefix()
	seen := map[string]bool{}
	if node != nil && node.Annotations != nil {
		for key, raw := range node.Annotations {
			if !strings.HasPrefix(key, requestPrefix) {
				continue
			}
			jobUID := strings.TrimPrefix(key, requestPrefix)
			if jobUID == "" {
				continue
			}
			seen[jobUID] = true
			resultKey := modelprewarm.ResultAnnotationKey(jobUID)
			if resultRaw, ok := node.Annotations[resultKey]; ok && resultRaw != "" {
				result, err := modelprewarm.ParseResult(resultRaw)
				if err == nil && modelprewarm.IsTerminal(result.Phase) {
					continue
				}
			}
			req, err := modelprewarm.ParseRequest(raw)
			if err != nil {
				klog.ErrorS(err, "failed to parse model prewarm request", "jobUID", jobUID)
				continue
			}
			h.startJob(jobUID, req)
		}
	}
	h.cancelRemovedJobs(seen)
}

func (h *Handler) cancelRemovedJobs(seen map[string]bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for jobUID, runner := range h.running {
		if !seen[jobUID] {
			runner.cancel()
		}
	}
}

func (h *Handler) startJob(jobUID string, req *modelprewarm.Request) {
	h.mu.Lock()
	if h.running[jobUID] != nil {
		h.mu.Unlock()
		return
	}
	jobCtx, cancel := context.WithCancel(h.ctx)
	h.running[jobUID] = &jobRunner{cancel: cancel}
	h.mu.Unlock()

	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.running, jobUID)
			h.mu.Unlock()
			cancel()
		}()
		h.execute(jobCtx, jobUID, req)
	}()
}

func (h *Handler) execute(ctx context.Context, jobUID string, req *modelprewarm.Request) {
	if ctx.Err() != nil {
		return
	}
	requestValue, err := modelprewarm.MarshalRequest(req)
	if err != nil {
		klog.ErrorS(err, "failed to marshal model prewarm request", "jobUID", jobUID, "opsJobId", req.OpsJobId)
		return
	}
	startedAt := time.Now().UTC()
	if err := h.writeResult(ctx, jobUID, requestValue, &modelprewarm.Result{
		OpsJobId: req.OpsJobId,
		Phase:    modelprewarm.PhaseRunning,
	}, false); err != nil {
		if errors.Is(err, errRequestRevoked) || ctx.Err() != nil {
			return
		}
		klog.ErrorS(err, "failed to write running model prewarm result", "jobUID", jobUID, "opsJobId", req.OpsJobId)
		return
	}
	if ctx.Err() != nil {
		return
	}

	bytesRead, err := h.prewarmOnHost(ctx, req)
	if errors.Is(err, errPrewarmCancelled) || ctx.Err() != nil {
		klog.InfoS("model prewarm cancelled", "jobUID", jobUID, "opsJobId", req.OpsJobId)
		return
	}

	finishedAt := time.Now().UTC()
	result := &modelprewarm.Result{
		OpsJobId:        req.OpsJobId,
		BytesRead:       bytesRead,
		DurationSeconds: int64(finishedAt.Sub(startedAt).Seconds()),
		FinishedAt:      finishedAt,
	}
	if err != nil {
		result.Phase = modelprewarm.PhaseFailed
		result.Message = modelprewarm.TruncateMessage(err.Error())
		klog.ErrorS(err, "model prewarm failed", "jobUID", jobUID, "opsJobId", req.OpsJobId)
	} else {
		result.Phase = modelprewarm.PhaseSucceeded
		klog.Infof("model prewarm succeeded, opsJobId=%s bytesRead=%d duration=%ds",
			req.OpsJobId, bytesRead, int64(result.DurationSeconds))
	}
	if ctx.Err() != nil {
		return
	}
	if err := h.writeResult(ctx, jobUID, requestValue, result, true); err != nil {
		if errors.Is(err, errRequestRevoked) || ctx.Err() != nil {
			return
		}
		klog.ErrorS(err, "failed to write model prewarm result", "jobUID", jobUID, "opsJobId", req.OpsJobId)
	}
}

func (h *Handler) prewarmOnHost(ctx context.Context, req *modelprewarm.Request) (int64, error) {
	parallelism := req.Parallelism
	if parallelism <= 0 {
		parallelism = modelprewarm.DefaultParallelism
	}
	script := preloadScript(req.ModelPath, req.Glob, parallelism)
	cmd := fmt.Sprintf("%s bash -c %s", nsenterPrefix, shellSingleQuote(script))
	timeout := modelprewarm.RequestTimeout(req)
	statusCode, output := utils.ExecuteCommandContext(ctx, cmd, timeout)
	if ctx.Err() != nil {
		return 0, errPrewarmCancelled
	}
	if statusCode != types.StatusOk {
		return 0, fmt.Errorf("preload command failed: %s", modelprewarm.TruncateMessage(output))
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	bytesLine := lines[len(lines)-1]
	bytesRead, err := strconv.ParseInt(strings.TrimSpace(bytesLine), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse bytes read: %v", err)
	}
	if bytesRead == 0 {
		return 0, fmt.Errorf("no weight bytes were read from %s", req.ModelPath)
	}
	return bytesRead, nil
}

func (h *Handler) writeResult(
	ctx context.Context, jobUID, requestValue string, result *modelprewarm.Result, replace bool) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	resultValue, err := modelprewarm.MarshalResult(result)
	if err != nil {
		return err
	}
	patch, err := buildConditionalResultPatch(jobUID, requestValue, resultValue, replace)
	if err != nil {
		return err
	}
	return retry.OnError(retry.DefaultRetry, func(err error) bool {
		return apierrors.IsTimeout(err) ||
			apierrors.IsServerTimeout(err) ||
			apierrors.IsInternalError(err) ||
			apierrors.IsServiceUnavailable(err) ||
			apierrors.IsTooManyRequests(err)
	}, func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, patchErr := h.k8sClient.Nodes().Patch(ctx, h.nodeName, apitypes.JSONPatchType, patch, metav1.PatchOptions{})
		if patchErr == nil {
			return nil
		}
		if apierrors.IsInvalid(patchErr) {
			return errRequestRevoked
		}
		return patchErr
	})
}

func buildConditionalResultPatch(jobUID, requestValue, resultValue string, replace bool) ([]byte, error) {
	reqKey := modelprewarm.RequestAnnotationKey(jobUID)
	resKey := modelprewarm.ResultAnnotationKey(jobUID)
	writeOp := "add"
	if replace {
		writeOp = "replace"
	}
	ops := []jsonPatchOp{
		{Op: "test", Path: annotationJSONPointer(reqKey), Value: requestValue},
		{Op: writeOp, Path: annotationJSONPointer(resKey), Value: resultValue},
	}
	return json.Marshal(ops)
}

func annotationJSONPointer(key string) string {
	return "/metadata/annotations/" + jsonPointerEscape(key)
}

func jsonPointerEscape(value string) string {
	value = strings.ReplaceAll(value, "~", "~0")
	return strings.ReplaceAll(value, "/", "~1")
}

const nsenterPrefix = "nsenter --target 1 --mount --uts --ipc --net --pid --"

func preloadScript(modelPath, glob string, parallelism int) string {
	return fmt.Sprintf(
		`set -o pipefail;
MODEL_PATH=%s; GLOB=%s; PAR=%d;
if [ ! -d "$MODEL_PATH" ]; then echo "model path not found: $MODEL_PATH"; exit 1; fi;
FILE_COUNT=$(find "$MODEL_PATH" -type f -name "$GLOB" | wc -l);
if [ "$FILE_COUNT" -eq 0 ]; then echo "no matching regular weight files under $MODEL_PATH (glob=$GLOB; symlinks excluded)"; exit 1; fi;
BYTES=$(find "$MODEL_PATH" -type f -name "$GLOB" -print0 | xargs -0 -r stat -c%%s | awk '{s+=$1} END {printf "%%.0f\n", s+0}');
find "$MODEL_PATH" -type f -name "$GLOB" -print0 | sort -z | xargs -0 -n 1 -P"$PAR" -r cat >/dev/null;
rc=$?;
echo "$BYTES";
exit $rc`,
		shellSingleQuote(modelPath), shellSingleQuote(glob), parallelism,
	)
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
