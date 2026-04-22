/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

var (
	reportCommandFence = regexp.MustCompile("(?s)```bash\\s*(.*?)```")
	portFlagRegex      = regexp.MustCompile(`(?:^|\\s)--port(?:=|\\s+)(\\d+)`)
)

// ApplyTask materializes the optimization result into an actual SaFE Workload.
// v1 keeps the API intentionally small: it creates a single Deployment using
// the optimized launch command and the image already used during Hyperloom.
func (h *Handler) ApplyTask(c *gin.Context) {
	task, err := h.getTaskForAction(c)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	var req ApplyTaskRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("invalid request body: "+err.Error()))
			return
		}
	}

	model, err := ResolveModelForOptimization(c.Request.Context(), h.dbClient, task.ModelID, firstNonEmpty(req.Workspace, task.Workspace))
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest(err.Error()))
		return
	}

	clawCtx := WithClawBearer(c.Request.Context(), clawBearerForGin(c))
	reportPath, launchCommand, err := h.resolveLaunchCommand(clawCtx, task, model)
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to resolve optimized launch command: "+err.Error()))
		return
	}
	if strings.TrimSpace(launchCommand) == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("empty launch command"))
		return
	}

	workload, err := h.buildOptimizedWorkload(c.Request.Context(), task, model, launchCommand, req)
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to build workload: "+err.Error()))
		return
	}
	if err := h.k8sClient.Create(c.Request.Context(), workload); err != nil {
		klog.ErrorS(err, "create optimized workload", "task_id", task.ID, "workload", workload.Name)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to create workload: "+err.Error()))
		return
	}
	workload.Status.Phase = v1.WorkloadPending
	_ = h.k8sClient.Status().Update(c.Request.Context(), workload)

	c.JSON(201, ApplyTaskResponse{
		TaskID:        task.ID,
		WorkloadID:    workload.Name,
		DisplayName:   v1.GetDisplayName(workload),
		LaunchCommand: launchCommand,
		ReportPath:    reportPath,
	})
}

func (h *Handler) resolveLaunchCommand(
	ctx context.Context,
	task *dbclient.OptimizationTask,
	model *ResolvedModel,
) (string, string, error) {
	reportPath := task.ReportPath
	if reportPath == "" {
		items, err := h.clawClient.ListSessionFiles(ctx, task.ClawSessionID)
		if err != nil {
			return "", "", err
		}
		for _, item := range items {
			if looksLikeOptimizationReport(item.Path) {
				reportPath = item.Path
				_ = h.dbClient.UpdateOptimizationTaskResult(context.Background(), task.ID, task.FinalMetrics, reportPath)
				break
			}
		}
	}
	if reportPath != "" {
		content, err := h.clawClient.ReadSessionFile(ctx, task.ClawSessionID, reportPath)
		if err == nil {
			if cmd := extractLaunchCommandFromReport(string(content)); cmd != "" {
				cmd = strings.ReplaceAll(cmd, "$MODEL", model.LocalPath)
				cmd = strings.ReplaceAll(cmd, "${MODEL}", model.LocalPath)
				return reportPath, cmd, nil
			}
		} else {
			klog.Warningf("apply: failed to read report %s for task %s: %v", reportPath, task.ID, err)
		}
	}
	// Fallback: derive a reasonable baseline launch command from the task params.
	return reportPath, buildDefaultLaunchCommand(task.Framework, model.LocalPath, model.ModelName, task.TP), nil
}

func extractLaunchCommandFromReport(report string) string {
	matches := reportCommandFence.FindAllStringSubmatch(report, -1)
	for _, m := range matches {
		block := strings.TrimSpace(m[1])
		if strings.Contains(block, "sglang.launch_server") || strings.Contains(block, "vllm serve") {
			return block
		}
	}
	// Fallback: line-based scan.
	lines := strings.Split(report, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "sglang.launch_server") || strings.Contains(trimmed, "vllm serve") {
			return trimmed
		}
	}
	return ""
}

func buildDefaultLaunchCommand(framework, modelPath, modelName string, tp int) string {
	if framework == FrameworkSGLang {
		return fmt.Sprintf("python3 -m sglang.launch_server --model-path %s --tp %d --host 0.0.0.0 --port 8888", modelPath, safePositive(tp, 1))
	}
	return fmt.Sprintf("vllm serve %s --served-model-name %s --tensor-parallel-size %d --host 0.0.0.0 --port 8000",
		modelPath, modelName, safePositive(tp, 1))
}

func (h *Handler) buildOptimizedWorkload(
	ctx context.Context,
	task *dbclient.OptimizationTask,
	model *ResolvedModel,
	command string,
	req ApplyTaskRequest,
) (*v1.Workload, error) {
	workspace := firstNonEmpty(req.Workspace, task.Workspace)
	displayName := firstNonEmpty(req.DisplayName, stringutil.NormalizeForDNS(fmt.Sprintf("%s-opt-infer", task.DisplayName)))
	normalizedName := stringutil.NormalizeForDNS(displayName)
	image := firstNonEmpty(req.Image, task.Image)
	if image == "" {
		return nil, fmt.Errorf("no image configured on task")
	}
	port := req.Port
	if port <= 0 {
		port = extractPortFromCommand(command)
	}
	if port <= 0 {
		port = 8000
	}
	replica := safePositive(req.Replica, 1)
	cpu := firstNonEmpty(req.CPU, "16")
	memory := firstNonEmpty(req.Memory, "64Gi")
	gpu := firstNonEmpty(req.GPU, "1")
	controlPlaneIP, err := h.getAdminControlPlaneIP(ctx)
	if err != nil {
		return nil, err
	}

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName(normalizedName),
			Labels: map[string]string{
				v1.DisplayNameLabel:  normalizedName,
				v1.WorkloadKindLabel: common.DeploymentKind,
				v1.UserIdLabel:       task.UserID,
			},
			Annotations: map[string]string{
				v1.DescriptionAnnotation:         fmt.Sprintf("Optimized inference service for %s", task.DisplayName),
				v1.UserNameAnnotation:            task.UserName,
				v1.AdminControlPlaneAnnotation:   controlPlaneIP,
				v1.UseWorkspaceStorageAnnotation: v1.TrueStr,
			},
		},
		Spec: v1.WorkloadSpec{
			Workspace: workspace,
			Resources: []v1.WorkloadResource{{
				Replica:          replica,
				CPU:              cpu,
				GPU:              gpu,
				Memory:           memory,
				EphemeralStorage: "50Gi",
			}},
			Images:      []string{image},
			EntryPoints: []string{command},
			Env: map[string]string{
				"PRIMUS_SOURCE_MODEL": model.ID,
				"MODEL_PATH":          model.LocalPath,
			},
			GroupVersionKind: v1.GroupVersionKind{
				Version: common.DefaultVersion,
				Kind:    common.DeploymentKind,
			},
			Priority: 1,
			Service: &v1.Service{
				Protocol:    corev1.ProtocolTCP,
				Port:        port,
				TargetPort:  port,
				ServiceType: corev1.ServiceTypeClusterIP,
			},
		},
	}
	return workload, nil
}

func extractPortFromCommand(cmd string) int {
	m := portFlagRegex.FindStringSubmatch(cmd)
	if m == nil {
		return 0
	}
	v, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return v
}

func safePositive(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

func (h *Handler) getAdminControlPlaneIP(ctx context.Context) (string, error) {
	nodeList := &corev1.NodeList{}
	if err := h.k8sClient.List(ctx, nodeList, ctrlclient.MatchingLabels{common.KubernetesControlPlane: ""}); err != nil {
		return "", err
	}
	if len(nodeList.Items) == 0 {
		return "", fmt.Errorf("failed to find control plane node")
	}
	for _, addr := range nodeList.Items[0].Status.Addresses {
		if addr.Type == corev1.NodeInternalIP && addr.Address != "" {
			return addr.Address, nil
		}
	}
	return "", fmt.Errorf("failed to find control plane internal IP")
}
