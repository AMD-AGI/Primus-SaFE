/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_prewarm

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"

	modelprewarm "github.com/AMD-AIG-AIMA/SAFE/common/pkg/model_prewarm"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/utils"
)

const (
	nsenterPrefix = "nsenter --target 1 --mount --uts --ipc --net --pid --"
	// prewarmTimeout bounds a single model prewarm execution on the host.
	prewarmTimeout = 2 * time.Hour
)

// Handler executes model prewarm requests observed on the local Kubernetes node.
type Handler struct {
	ctx       context.Context
	nodeName  string
	k8sClient typedcorev1.CoreV1Interface
	mu        sync.Mutex
	running   map[string]bool
}

// NewHandler creates a model prewarm handler for the given node.
func NewHandler(ctx context.Context, nodeName string, k8sClient typedcorev1.CoreV1Interface) *Handler {
	return &Handler{
		ctx:       ctx,
		nodeName:  nodeName,
		k8sClient: k8sClient,
		running:   make(map[string]bool),
	}
}

// HandleNodeUpdate scans node annotations and starts pending model prewarm jobs.
func (h *Handler) HandleNodeUpdate(node *corev1.Node) {
	if node == nil || node.Annotations == nil {
		return
	}
	requestPrefix := modelprewarm.RequestAnnotationPrefix()
	for key, raw := range node.Annotations {
		if !strings.HasPrefix(key, requestPrefix) {
			continue
		}
		jobUID := strings.TrimPrefix(key, requestPrefix)
		if jobUID == "" {
			continue
		}
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

func (h *Handler) startJob(jobUID string, req *modelprewarm.Request) {
	h.mu.Lock()
	if h.running[jobUID] {
		h.mu.Unlock()
		return
	}
	h.running[jobUID] = true
	h.mu.Unlock()

	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.running, jobUID)
			h.mu.Unlock()
		}()
		h.execute(jobUID, req)
	}()
}

func (h *Handler) execute(jobUID string, req *modelprewarm.Request) {
	startedAt := time.Now().UTC()
	if err := h.writeResult(jobUID, &modelprewarm.Result{
		OpsJobId: req.OpsJobId,
		Phase:    modelprewarm.PhaseRunning,
	}); err != nil {
		klog.ErrorS(err, "failed to write running model prewarm result", "jobUID", jobUID, "opsJobId", req.OpsJobId)
		return
	}

	bytesRead, err := h.prewarmOnHost(req)
	finishedAt := time.Now().UTC()
	result := &modelprewarm.Result{
		OpsJobId:        req.OpsJobId,
		BytesRead:       bytesRead,
		DurationSeconds: int64(finishedAt.Sub(startedAt).Seconds()),
		FinishedAt:      finishedAt,
	}
	if err != nil {
		result.Phase = modelprewarm.PhaseFailed
		result.Message = err.Error()
		klog.ErrorS(err, "model prewarm failed", "jobUID", jobUID, "opsJobId", req.OpsJobId)
	} else {
		result.Phase = modelprewarm.PhaseSucceeded
		klog.Infof("model prewarm succeeded, opsJobId=%s bytesRead=%d duration=%ds",
			req.OpsJobId, bytesRead, int64(result.DurationSeconds))
	}
	if err := h.writeResult(jobUID, result); err != nil {
		klog.ErrorS(err, "failed to write model prewarm result", "jobUID", jobUID, "opsJobId", req.OpsJobId)
	}
}

func (h *Handler) prewarmOnHost(req *modelprewarm.Request) (int64, error) {
	modelPath := shellSingleQuote(req.ModelPath)
	glob := shellSingleQuote(req.Glob)
	parallelism := req.Parallelism
	if parallelism <= 0 {
		parallelism = modelprewarm.DefaultParallelism
	}
	script := fmt.Sprintf(
		`MODEL_PATH=%s; GLOB=%s; PAR=%d;
if [ ! -d "$MODEL_PATH" ]; then echo "model path not found: $MODEL_PATH"; exit 1; fi;
BYTES=$(find "$MODEL_PATH" -type f -name "$GLOB" -print0 | xargs -0 -r stat -c%%s | awk '{s+=$1} END {print s+0}');
find "$MODEL_PATH" -type f -name "$GLOB" -print0 | sort -z | xargs -0 -P"$PAR" -r cat >/dev/null;
echo "$BYTES"`,
		modelPath, glob, parallelism,
	)
	cmd := fmt.Sprintf("%s bash -c %s", nsenterPrefix, shellSingleQuote(script))
	statusCode, output := utils.ExecuteCommand(cmd, prewarmTimeout)
	if statusCode != types.StatusOk {
		return 0, fmt.Errorf("preload command failed: %s", output)
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	bytesLine := lines[len(lines)-1]
	bytesRead, err := strconv.ParseInt(strings.TrimSpace(bytesLine), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse bytes read: %v", err)
	}
	return bytesRead, nil
}

func (h *Handler) writeResult(jobUID string, result *modelprewarm.Result) error {
	value, err := modelprewarm.MarshalResult(result)
	if err != nil {
		return err
	}
	key := modelprewarm.ResultAnnotationKey(jobUID)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, getErr := h.k8sClient.Nodes().Get(h.ctx, h.nodeName, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		if node.Annotations == nil {
			node.Annotations = make(map[string]string)
		}
		node.Annotations[key] = value
		patch := []byte(fmt.Sprintf(`{"metadata":{"annotations":{"%s":%s}}}`, key, jsonEscape(value)))
		_, patchErr := h.k8sClient.Nodes().Patch(h.ctx, h.nodeName, apitypes.MergePatchType, patch, metav1.PatchOptions{})
		return patchErr
	})
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func jsonEscape(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	return "\"" + escaped + "\""
}
