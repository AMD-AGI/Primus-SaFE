/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	modelprewarm "github.com/AMD-AIG-AIMA/SAFE/common/pkg/model_prewarm"
	commonjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// externalWorkloadName returns the deterministic name of the prewarm workload of a job on one node.
func externalWorkloadName(jobName, adminNodeName string) string {
	sum := sha256.Sum256([]byte(adminNodeName))
	suffix := hex.EncodeToString(sum[:4])
	maxBase := utils.MaxGeneratedNameLength - len(suffix) - 1
	if len(jobName) > maxBase {
		jobName = strings.TrimRight(jobName[:maxBase], "-")
	}
	return jobName + "-" + suffix
}

// getExternalNodes returns the admin nodes among the targets that have no node-agent (external nodes).
func (r *ModelPrewarmJobReconciler) getExternalNodes(ctx context.Context, targets []string) []*v1.Node {
	var nodes []*v1.Node
	for _, name := range targets {
		node, err := r.getAdminNode(ctx, name)
		if err == nil && node.IsExternal() {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// syncExternalWorkloads creates the missing prewarm workloads of the external nodes, at most
// max_concurrent at the same time, and returns the result of each node by admin node name.
// A node without an entry has no result yet.
func (r *ModelPrewarmJobReconciler) syncExternalWorkloads(ctx context.Context, job *v1.OpsJob,
	k8sClients *k8sclient.ClientFactory, nodes []*v1.Node) map[string]*modelprewarm.Result {
	results := make(map[string]*modelprewarm.Result, len(nodes))
	if len(nodes) == 0 {
		return results
	}
	cfg, err := r.parseConfig(job)
	if err != nil {
		for _, node := range nodes {
			results[node.Name] = failedResult(job, err.Error())
		}
		return results
	}
	existing, err := r.listExternalWorkloads(ctx, job)
	if err != nil {
		klog.ErrorS(err, "failed to list model prewarm workloads", "job", job.Name)
		return results
	}

	inFlight := 0
	var missing []*v1.Node
	for _, node := range nodes {
		workload, ok := existing[externalWorkloadName(job.Name, node.Name)]
		if !ok {
			missing = append(missing, node)
			continue
		}
		result := r.externalWorkloadResult(ctx, job, workload, k8sClients)
		results[node.Name] = result
		if result == nil || result.Phase == modelprewarm.PhaseRunning {
			inFlight++
		}
	}

	limit := commonconfig.GetModelPrewarmMaxConcurrent()
	for _, node := range missing {
		if limit > 0 && inFlight >= limit {
			break
		}
		name := externalWorkloadName(job.Name, node.Name)
		workload, err := r.generateExternalWorkload(ctx, job, cfg, node, name)
		if err == nil {
			err = r.Create(ctx, workload)
		}
		switch {
		case err == nil || apierrors.IsAlreadyExists(err):
			inFlight++
		case isDispatchRejected(err):
			results[node.Name] = failedResult(job, err.Error())
		default:
			klog.ErrorS(err, "failed to create model prewarm workload", "job", job.Name, "node", node.Name)
		}
	}
	return results
}

// listExternalWorkloads returns the prewarm workloads of the job by name.
func (r *ModelPrewarmJobReconciler) listExternalWorkloads(
	ctx context.Context, job *v1.OpsJob) (map[string]*v1.Workload, error) {
	list := &v1.WorkloadList{}
	if err := r.List(ctx, list, client.MatchingLabels{v1.OpsJobIdLabel: job.Name}); err != nil {
		return nil, err
	}
	result := make(map[string]*v1.Workload, len(list.Items))
	for i := range list.Items {
		result[list.Items[i].Name] = &list.Items[i]
	}
	return result, nil
}

// isDispatchRejected reports whether the error means the workload will not be accepted on a retry.
func isDispatchRejected(err error) bool {
	return commonerrors.IsNonRetryableError(err) || apierrors.IsInvalid(err) || apierrors.IsBadRequest(err)
}

// failedResult builds a failed node result with the message as given.
func failedResult(job *v1.OpsJob, message string) *modelprewarm.Result {
	return &modelprewarm.Result{
		OpsJobId: job.Name,
		Phase:    modelprewarm.PhaseFailed,
		Message:  modelprewarm.TruncateMessage(message),
	}
}

// generateExternalWorkload builds the prewarm workload of one external node. It is dispatched like any
// other workload of the workspace of the node: the scheduler admits it, the node is selected by hostname
// affinity, and the tolerations of the external workspace are added by the dispatcher.
func (r *ModelPrewarmJobReconciler) generateExternalWorkload(
	ctx context.Context, job *v1.OpsJob, cfg *modelPrewarmConfig, adminNode *v1.Node, name string) (*v1.Workload, error) {
	workspaceId := v1.GetWorkspaceId(adminNode)
	if workspaceId == "" {
		return nil, commonerrors.NewBadRequest("node " + adminNode.Name + " does not belong to a workspace")
	}
	k8sNodeName := adminNode.GetK8sNodeName()
	if k8sNodeName == "" {
		return nil, fmt.Errorf("k8s node name is empty for admin node %s", adminNode.Name)
	}
	workspace := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: workspaceId}, workspace); err != nil {
		return nil, err
	}

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				v1.ClusterIdLabel:   v1.GetClusterId(job),
				v1.UserIdLabel:      v1.GetUserId(job),
				v1.OpsJobIdLabel:    job.Name,
				v1.OpsJobTypeLabel:  string(job.Spec.Type),
				v1.DisplayNameLabel: name,
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation:    v1.GetUserName(job),
				v1.DescriptionAnnotation: v1.OpsJobKind,
				// Only the model path is mounted, as a hostpath.
				v1.UseWorkspaceStorageAnnotation: "false",
			},
		},
		Spec: v1.WorkloadSpec{
			Resources: []v1.WorkloadResource{{Replica: 1, CPU: "1", Memory: "1Gi", EphemeralStorage: "1Gi"}},
			GroupVersionKind: v1.GroupVersionKind{
				Version: common.DefaultVersion,
				Kind:    common.JobKind,
			},
			Workspace:      workspaceId,
			Images:         []string{commonconfig.GetModelPrewarmImage()},
			EntryPoints:    []string{stringutil.Base64Encode(modelprewarm.PodEntryScript(cfg.modelPath, cfg.glob, cfg.parallelism))},
			Env:            job.Spec.Env,
			CustomerLabels: map[string]string{v1.K8sHostName: k8sNodeName},
		},
	}
	hostPath, err := prewarmHostPath(workspace, workload, cfg.modelPath)
	if err != nil {
		return nil, err
	}
	workload.Spec.Hostpath = []string{hostPath}

	// A workload of an external workspace must be admitted by the scheduler like any other workload there.
	if !v1.IsExternalWorkspace(workspace) {
		workload.Annotations[v1.WorkloadScheduledAnnotation] = timeutil.FormatRFC3339(time.Now().UTC())
	}
	if err = controllerutil.SetControllerReference(job, workload, r.Client.Scheme()); err != nil {
		return nil, err
	}
	if job.Spec.TimeoutSecond > 0 {
		workload.Spec.Timeout = pointer.Int(job.Spec.TimeoutSecond)
	}
	return workload, nil
}

// prewarmHostPath returns the host path a prewarm pod mounts to read the model. An external pod can mount
// only the hostpath volume root the workspace declares, unless user_dir_mounts is enabled, which also
// admits the model path below it.
func prewarmHostPath(workspace *v1.Workspace, workload *v1.Workload, modelPath string) (string, error) {
	root, sub := commonworkload.HostPathVolumeRoot(workspace, commonworkload.GetScope(workload), modelPath)
	if root == "" {
		return "", commonerrors.NewBadRequest(fmt.Sprintf(
			"model path %s is not under a hostpath volume of workspace %s", modelPath, workspace.Name))
	}
	if sub != "" && !commonconfig.IsExternalUserDirMountsEnable() {
		return root, nil
	}
	return filepath.Clean(modelPath), nil
}

// externalWorkloadResult maps the prewarm workload of a node to a node result.
// It returns nil while the workload is waiting.
func (r *ModelPrewarmJobReconciler) externalWorkloadResult(ctx context.Context, job *v1.OpsJob,
	workload *v1.Workload, k8sClients *k8sclient.ClientFactory) *modelprewarm.Result {
	if workload.IsEnd() {
		return r.endedWorkloadResult(ctx, job, workload)
	}
	if workload.IsRunning() {
		return &modelprewarm.Result{OpsJobId: job.Name, Phase: modelprewarm.PhaseRunning}
	}
	if reason := r.unscheduledReason(ctx, workload, k8sClients); reason != "" {
		return failedResult(job, reason)
	}
	return nil
}

// endedWorkloadResult reads the exit code and the termination message of a finished prewarm pod.
func (r *ModelPrewarmJobReconciler) endedWorkloadResult(
	ctx context.Context, job *v1.OpsJob, workload *v1.Workload) *modelprewarm.Result {
	raw := r.getWorkloadCompletionMessage(ctx, workload)
	termination, parseErr := modelprewarm.ParseTerminationMessage(raw)

	if workload.Status.Phase != v1.WorkloadSucceeded {
		message := raw
		if parseErr == nil && termination.Error != "" {
			message = termination.Error
		}
		if message == "" {
			message = workload.Status.Message
		}
		if message == "" {
			message = "workload " + string(workload.Status.Phase)
		}
		return failedResult(job, message)
	}
	if parseErr != nil {
		return failedResult(job, "failed to parse the result of the prewarm pod: "+raw)
	}
	if termination.BytesRead <= 0 {
		return failedResult(job, "no weight bytes were read")
	}
	return &modelprewarm.Result{
		OpsJobId:        job.Name,
		Phase:           modelprewarm.PhaseSucceeded,
		BytesRead:       termination.BytesRead,
		DurationSeconds: termination.DurationSeconds,
	}
}

// unscheduledReason returns why a prewarm pod could not be placed or created, as the cluster states it,
// once the workload has waited longer than the pending timeout. It returns "" while nothing is stated:
// a workload held in the queue of SaFE has no pod and no event.
func (r *ModelPrewarmJobReconciler) unscheduledReason(
	ctx context.Context, workload *v1.Workload, k8sClients *k8sclient.ClientFactory) string {
	timeout := time.Duration(commonconfig.GetModelPrewarmPendingTimeoutSecond()) * time.Second
	if timeout <= 0 || time.Since(workload.CreationTimestamp.Time) < timeout ||
		k8sClients == nil || !k8sClients.IsValid() {
		return ""
	}
	clientSet := k8sClients.ClientSet()
	namespace := workload.Spec.Workspace
	pods, err := clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", v1.WorkloadIdLabel, workload.Name),
	})
	if err != nil {
		klog.V(4).ErrorS(err, "failed to list prewarm pods", "workload", workload.Name)
		return ""
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase != corev1.PodPending {
			continue
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse && cond.Message != "" {
				return cond.Message
			}
		}
		if message := latestWarningEvent(ctx, clientSet.CoreV1().Events(namespace), pod.Name); message != "" {
			return message
		}
	}
	if len(pods.Items) == 0 {
		// The pod was refused when its job tried to create it: the reason is an event of the job.
		return latestWarningEvent(ctx, clientSet.CoreV1().Events(namespace), workload.Name)
	}
	return ""
}

// latestWarningEvent returns the message of the newest warning event of the named object, or "".
func latestWarningEvent(ctx context.Context, events interface {
	List(ctx context.Context, opts metav1.ListOptions) (*corev1.EventList, error)
}, objectName string) string {
	list, err := events.List(ctx, metav1.ListOptions{FieldSelector: "involvedObject.name=" + objectName})
	if err != nil {
		klog.V(4).ErrorS(err, "failed to list events", "object", objectName)
		return ""
	}
	var latest time.Time
	message := ""
	for i := range list.Items {
		event := &list.Items[i]
		if event.Type != corev1.EventTypeWarning || event.Message == "" {
			continue
		}
		at := event.LastTimestamp.Time
		if at.IsZero() {
			at = event.EventTime.Time
		}
		if message == "" || at.After(latest) {
			latest, message = at, event.Message
		}
	}
	return message
}

// cleanupJobResources removes everything a finished job left on the nodes: the annotations of the nodes
// with node-agent and the prewarm workloads of the external nodes.
func (r *ModelPrewarmJobReconciler) cleanupJobResources(ctx context.Context, job *v1.OpsJob) error {
	if err := r.cleanupAnnotations(ctx, job); err != nil {
		return err
	}
	return r.cleanupExternalWorkloads(ctx, job)
}

// cleanupExternalWorkloads deletes the prewarm workloads created for the external nodes of the job.
func (r *ModelPrewarmJobReconciler) cleanupExternalWorkloads(ctx context.Context, job *v1.OpsJob) error {
	return commonjob.CleanupJobRelatedResource(ctx, r.Client, job.Name)
}
