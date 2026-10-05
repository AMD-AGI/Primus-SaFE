/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/imagedigest"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
)

const (
	// ExternalWaitingScaleUpReason is shown while a single Pod waits for B to provision.
	ExternalWaitingScaleUpReason = "In queue - waiting for scale-up"
	// ExternalWaitingPRAcceptReason is shown when the PR has no conditions yet.
	ExternalWaitingPRAcceptReason = "In queue - waiting for capacity service to accept"
	// ExternalWaitingPRReason is shown while a gang waits for Provisioned=True.
	ExternalWaitingPRReason = "In queue - waiting for capacity service"
	// ExternalCapacityUnavailableReason is shown for Provisioned=False/CapacityUnavailable.
	ExternalCapacityUnavailableReason = "In queue - external capacity unavailable"
	// ExternalPRFailedReason prefixes a terminal PR Failed condition.
	ExternalPRFailedReason = "Rejected - provisioning request failed"
	// ExternalPRExpiredReason is shown briefly while a booking is rebuilt.
	ExternalPRExpiredReason = "In queue - capacity reservation expired, retrying"
	// ExternalImageResolveReason prefixes a failed digest resolve (terminal).
	ExternalImageResolveReason = "Rejected - image cannot be resolved"

	externalLeaseOverheadSec int64 = 600
)

var (
	podTemplateGVR = schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "podtemplates",
	}
	provisioningRequestGVR = schema.GroupVersionResource{
		Group: "autoscaling.x-k8s.io", Version: "v1", Resource: "provisioningrequests",
	}
)

// admitExternalViaScheduler admits an external workload on the kube-scheduler path.
// Single-replica workloads are admitted immediately so the dispatcher can create the Pod;
// Unschedulable pods then trigger B scale-up. Gang workloads require a ProvisioningRequest
// before the PyTorchJob is created (see ensureExternalProvisioning).
func (r *SchedulerReconciler) admitExternalViaScheduler(ctx context.Context,
	workload *v1.Workload, workspace *v1.Workspace) (bool, string, error) {
	if err := validateExternalShapeForScheduler(workload); err != nil {
		return false, ExternalUnsupportedReason, err
	}
	state, err := r.ensureExternalSchedulerState(ctx, workload)
	if err != nil {
		return false, "", err
	}
	state, err = r.ensureExternalResolvedImages(ctx, workload, state)
	if err != nil {
		return false, ExternalImageResolveReason + " - " + err.Error(), nil
	}
	if commonworkload.IsExternalRDMAGang(workload) {
		return r.ensureExternalProvisioning(ctx, workload, workspace, state)
	}
	klog.V(2).InfoS("admitted external workload via kube-scheduler path",
		"workload", workload.Name, "workspace", workspace.Name,
		"generation", state.DispatchGeneration)
	return true, "", nil
}

// ensureExternalResolvedImages pins Spec.Images to digests and stores them on status (R5).
func (r *SchedulerReconciler) ensureExternalResolvedImages(ctx context.Context,
	workload *v1.Workload, state *v1.WorkloadExternalExecution) (*v1.WorkloadExternalExecution, error) {
	if state == nil {
		return nil, fmt.Errorf("nil external execution state")
	}
	if len(state.ResolvedImages) == len(workload.Spec.Images) && len(state.ResolvedImages) > 0 {
		allPinned := true
		for _, img := range state.ResolvedImages {
			if !imagedigest.IsPinned(img) {
				allPinned = false
				break
			}
		}
		if allPinned {
			return state, nil
		}
	}
	resolved, err := imagedigest.ResolveWorkloadImages(ctx, r.Client, workload)
	if err != nil {
		return nil, err
	}
	updated := state.DeepCopy()
	updated.ResolvedImages = resolved
	if err = r.patchExternalState(ctx, workload, updated); err != nil {
		return nil, err
	}
	return updated, nil
}

// ensureExternalSchedulerState persists PlacementMode=kube-scheduler and a dispatch
// generation before any Pod or ProvisioningRequest is created.
func (r *SchedulerReconciler) ensureExternalSchedulerState(ctx context.Context,
	workload *v1.Workload) (*v1.WorkloadExternalExecution, error) {
	generation := int32(v1.GetWorkloadDispatchCnt(workload) + 1)
	current := workload.Status.ExternalExecution
	if current != nil &&
		current.PlacementMode == v1.ExternalPlacementKubeScheduler &&
		current.DispatchGeneration == generation {
		return current.DeepCopy(), nil
	}
	state := &v1.WorkloadExternalExecution{
		PlacementMode:      v1.ExternalPlacementKubeScheduler,
		DispatchGeneration: generation,
	}
	if current != nil && current.PlacementMode == v1.ExternalPlacementKubeScheduler {
		state.ProvisioningRequest = current.ProvisioningRequest
		state.ProvisioningAttempt = current.ProvisioningAttempt
		state.ProvisioningCondition = current.ProvisioningCondition
		state.ResolvedImages = append([]string{}, current.ResolvedImages...)
	}
	if err := r.patchExternalState(ctx, workload, state); err != nil {
		return nil, err
	}
	return state, nil
}

// ensureExternalProvisioning creates PodTemplate + ProvisioningRequest and admits once
// Provisioned=True. Failed is terminal; BookingExpired/CapacityRevoked rebuild a new attempt.
func (r *SchedulerReconciler) ensureExternalProvisioning(ctx context.Context,
	workload *v1.Workload, workspace *v1.Workspace,
	state *v1.WorkloadExternalExecution) (bool, string, error) {
	attempt := state.ProvisioningAttempt
	if attempt < 1 {
		attempt = 1
	}
	prName := externalProvisioningRequestName(workload, state.DispatchGeneration, attempt)
	ptName := externalPodTemplateName(workload, state.DispatchGeneration, attempt)
	if state.ProvisioningRequest != prName || state.ProvisioningAttempt != attempt {
		updated := state.DeepCopy()
		updated.ProvisioningRequest = prName
		updated.ProvisioningAttempt = attempt
		if err := r.patchExternalState(ctx, workload, updated); err != nil {
			return false, "", err
		}
		state = updated
	}

	dyn, err := r.dataPlaneDynamic(workload)
	if err != nil {
		return false, "", err
	}
	ns := workspace.Name
	count := externalGangMemberCount(workload)
	template, err := buildGangPodTemplate(workload, ns, ptName, prName)
	if err != nil {
		return false, ExternalUnsupportedReason, err
	}
	if err = ensureUnstructured(ctx, dyn, podTemplateGVR, ns, template); err != nil {
		return false, "", err
	}
	pr := buildProvisioningRequest(ns, prName, ptName, count)
	if err = ensureUnstructured(ctx, dyn, provisioningRequestGVR, ns, pr); err != nil {
		return false, "", err
	}

	got, err := dyn.Resource(provisioningRequestGVR).Namespace(ns).Get(ctx, prName, metav1.GetOptions{})
	if err != nil {
		return false, "", err
	}
	outcome := interpretProvisioningRequest(got)
	if outcome.condition != "" && outcome.condition != state.ProvisioningCondition {
		updated := state.DeepCopy()
		updated.ProvisioningCondition = outcome.condition
		if patchErr := r.patchExternalState(ctx, workload, updated); patchErr != nil {
			klog.ErrorS(patchErr, "failed to record PR condition", "workload", workload.Name)
		} else {
			state = updated
		}
	}

	switch outcome.action {
	case prAdmit:
		klog.V(2).InfoS("external gang ProvisioningRequest is Provisioned",
			"workload", workload.Name, "pr", prName)
		return true, "", nil
	case prFail:
		return false, outcome.reason, nil
	case prRebuild:
		if err = deleteProvisioningObjects(ctx, dyn, ns, prName, ptName); err != nil {
			return false, "", err
		}
		updated := state.DeepCopy()
		updated.ProvisioningAttempt = attempt + 1
		updated.ProvisioningRequest = ""
		updated.ProvisioningCondition = outcome.condition
		if err = r.patchExternalState(ctx, workload, updated); err != nil {
			return false, "", err
		}
		return false, ExternalPRExpiredReason, nil
	default:
		reason := ExternalWaitingPRReason
		if outcome.reason != "" {
			reason = outcome.reason
		}
		return false, reason, nil
	}
}

type prAction int

const (
	prWait prAction = iota
	prAdmit
	prFail
	prRebuild
)

type prOutcome struct {
	action    prAction
	reason    string
	condition string
}

// interpretProvisioningRequest maps PR conditions to admit / wait / fail / rebuild (R11).
func interpretProvisioningRequest(pr *unstructured.Unstructured) prOutcome {
	if pr == nil {
		return prOutcome{action: prWait, reason: ExternalWaitingPRAcceptReason}
	}
	conditions, _, _ := unstructured.NestedSlice(pr.Object, "status", "conditions")
	var provisioned, failed, bookingExpired, capacityRevoked *metav1.Condition
	var unknown *metav1.Condition
	for i := range conditions {
		c, ok := conditions[i].(map[string]interface{})
		if !ok {
			continue
		}
		cond := mapToCondition(c)
		switch cond.Type {
		case "Provisioned":
			provisioned = &cond
		case "Failed":
			failed = &cond
		case "BookingExpired":
			bookingExpired = &cond
		case "CapacityRevoked":
			capacityRevoked = &cond
		default:
			if unknown == nil && cond.Status == metav1.ConditionTrue {
				copied := cond
				unknown = &copied
			}
		}
	}
	if failed != nil && failed.Status == metav1.ConditionTrue {
		msg := conditionDetail(failed)
		if msg == "" {
			msg = "ProvisioningRequest Failed"
		}
		return prOutcome{
			action:    prFail,
			reason:    ExternalPRFailedReason + " - " + msg,
			condition: formatCondition(failed),
		}
	}
	if bookingExpired != nil && bookingExpired.Status == metav1.ConditionTrue {
		return prOutcome{action: prRebuild, condition: formatCondition(bookingExpired)}
	}
	if capacityRevoked != nil && capacityRevoked.Status == metav1.ConditionTrue {
		return prOutcome{action: prRebuild, condition: formatCondition(capacityRevoked)}
	}
	if provisioned != nil && provisioned.Status == metav1.ConditionTrue {
		return prOutcome{action: prAdmit, condition: formatCondition(provisioned)}
	}
	if provisioned != nil {
		detail := conditionDetail(provisioned)
		reason := ExternalWaitingPRReason
		if provisioned.Reason == "CapacityUnavailable" {
			reason = ExternalCapacityUnavailableReason
		}
		if detail != "" {
			reason = reason + " - " + detail
		}
		return prOutcome{action: prWait, reason: reason, condition: formatCondition(provisioned)}
	}
	if unknown != nil {
		detail := conditionDetail(unknown)
		reason := ExternalWaitingPRReason
		if detail != "" {
			reason = reason + " - " + detail
		}
		return prOutcome{action: prWait, reason: reason, condition: formatCondition(unknown)}
	}
	return prOutcome{action: prWait, reason: ExternalWaitingPRAcceptReason, condition: "pending"}
}

// conditionDetail joins reason and message for UI, omitting empty parts.
func conditionDetail(c *metav1.Condition) string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(strings.Trim(c.Reason+": "+c.Message, ": "))
}

func mapToCondition(c map[string]interface{}) metav1.Condition {
	cond := metav1.Condition{}
	cond.Type, _ = c["type"].(string)
	cond.Reason, _ = c["reason"].(string)
	cond.Message, _ = c["message"].(string)
	if s, _ := c["status"].(string); s != "" {
		cond.Status = metav1.ConditionStatus(s)
	}
	return cond
}

func formatCondition(c *metav1.Condition) string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf("%s=%s/%s", c.Type, c.Status, c.Reason)
}

func (r *SchedulerReconciler) dataPlaneDynamic(workload *v1.Workload) (dynamic.Interface, error) {
	if r.dataPlaneDynamicOverride != nil {
		return r.dataPlaneDynamicOverride, nil
	}
	clientSets, err := syncer.GetClusterClientSets(r.clusterClientSets, v1.GetClusterId(workload))
	if err != nil {
		return nil, err
	}
	factory := clientSets.ClientFactory()
	if factory == nil || factory.DynamicClient() == nil {
		return nil, fmt.Errorf("data-plane dynamic client unavailable for cluster %s", v1.GetClusterId(workload))
	}
	return factory.DynamicClient(), nil
}

// deleteExternalProvisioningObjects removes PR and PodTemplate for a kube-scheduler gang.
func (r *SchedulerReconciler) deleteExternalProvisioningObjects(ctx context.Context,
	workload *v1.Workload) error {
	state := workload.Status.ExternalExecution
	if state == nil || state.PlacementMode != v1.ExternalPlacementKubeScheduler {
		return nil
	}
	if state.ProvisioningRequest == "" && state.DispatchGeneration == 0 {
		return nil
	}
	dyn, err := r.dataPlaneDynamic(workload)
	if err != nil {
		return err
	}
	ns := workload.Spec.Workspace
	attempt := state.ProvisioningAttempt
	if attempt < 1 {
		attempt = 1
	}
	prName := state.ProvisioningRequest
	if prName == "" {
		prName = externalProvisioningRequestName(workload, state.DispatchGeneration, attempt)
	}
	ptName := externalPodTemplateName(workload, state.DispatchGeneration, attempt)
	return deleteProvisioningObjects(ctx, dyn, ns, prName, ptName)
}

func deleteProvisioningObjects(ctx context.Context, dyn dynamic.Interface,
	ns, prName, ptName string) error {
	if prName != "" {
		err := dyn.Resource(provisioningRequestGVR).Namespace(ns).Delete(ctx, prName, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	if ptName != "" {
		err := dyn.Resource(podTemplateGVR).Namespace(ns).Delete(ctx, ptName, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func ensureUnstructured(ctx context.Context, dyn dynamic.Interface, gvr schema.GroupVersionResource,
	ns string, obj *unstructured.Unstructured) error {
	_, err := dyn.Resource(gvr).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
	if err == nil || !apierrors.IsAlreadyExists(err) {
		return err
	}
	_, err = dyn.Resource(gvr).Namespace(ns).Get(ctx, obj.GetName(), metav1.GetOptions{})
	return err
}

func buildProvisioningRequest(ns, name, templateName string, count int64) *unstructured.Unstructured {
	pr := &unstructured.Unstructured{}
	pr.SetAPIVersion("autoscaling.x-k8s.io/v1")
	pr.SetKind("ProvisioningRequest")
	pr.SetNamespace(ns)
	pr.SetName(name)
	_ = unstructured.SetNestedField(pr.Object, v1.ProvisioningRequestClassName,
		"spec", "provisioningClassName")
	podSets := []interface{}{
		map[string]interface{}{
			"count": count,
			"podTemplateRef": map[string]interface{}{
				"name": templateName,
			},
		},
	}
	_ = unstructured.SetNestedSlice(pr.Object, podSets, "spec", "podSets")
	return pr
}

func buildGangPodTemplate(workload *v1.Workload, ns, name, prName string) (*unstructured.Unstructured, error) {
	if len(workload.Spec.Resources) == 0 {
		return nil, fmt.Errorf("workload %s has no resources", workload.Name)
	}
	res := &workload.Spec.Resources[0]
	resourceList, err := quantity.CvtToResourceList(res.CPU, res.Memory, res.GPU,
		res.GPUName, res.EphemeralStorage, res.RdmaResource, 1)
	if err != nil {
		return nil, err
	}
	requests := map[string]interface{}{}
	for k, v := range resourceList {
		requests[string(k)] = v.String()
	}
	if rdma := commonconfig.GetRdmaName(); rdma != "" && res.RdmaResource != "" {
		requests[rdma] = res.RdmaResource
	}
	image := ""
	if state := workload.Status.ExternalExecution; state != nil && len(state.ResolvedImages) > 0 {
		image = state.ResolvedImages[0]
	} else if len(workload.Spec.Images) > 0 {
		image = workload.Spec.Images[0]
	}
	leaseDeadline := strconv.FormatInt(time.Now().UTC().Unix()+
		externalRuntimeSeconds(workload)+externalLeaseOverheadSec, 10)
	bookingValue := ns + "." + prName
	priorityClass := commonworkload.ExternalPriorityClass(
		commonworkload.GeneratePriority(workload.Spec.Priority))

	podSpec := map[string]interface{}{
		"schedulerName":     v1.ExternalSchedulerName,
		"priorityClassName": priorityClass,
		"hostNetwork":       true,
		"dnsPolicy":         string(corev1.DNSClusterFirstWithHostNet),
		"containers": []interface{}{
			map[string]interface{}{
				"name":  "pytorch",
				"image": image,
				"resources": map[string]interface{}{
					"requests": requests,
					"limits":   requests,
				},
			},
		},
		"tolerations": []interface{}{
			map[string]interface{}{
				"key":      v1.ExternalVirtualKubeletTaint,
				"operator": string(corev1.TolerationOpExists),
				"effect":   string(corev1.TaintEffectNoSchedule),
			},
			map[string]interface{}{
				"key":      v1.ExternalVirtualKubeletTaintLegacy,
				"operator": string(corev1.TolerationOpExists),
				"effect":   string(corev1.TaintEffectNoSchedule),
			},
			map[string]interface{}{
				"key":      v1.ExternalProvisioningRequestTaint,
				"operator": string(corev1.TolerationOpEqual),
				"value":    bookingValue,
				"effect":   string(corev1.TaintEffectNoSchedule),
			},
		},
		"affinity": map[string]interface{}{
			"nodeAffinity": map[string]interface{}{
				"requiredDuringSchedulingIgnoredDuringExecution": map[string]interface{}{
					"nodeSelectorTerms": []interface{}{
						map[string]interface{}{
							"matchExpressions": []interface{}{
								map[string]interface{}{
									"key":      v1.ExternalWorkspaceLabel,
									"operator": "In",
									"values":   []interface{}{workload.Spec.Workspace},
								},
								map[string]interface{}{
									"key":      v1.ExternalLeaseEndLabel,
									"operator": "Gt",
									"values":   []interface{}{leaseDeadline},
								},
							},
						},
					},
				},
			},
		},
	}

	pt := &unstructured.Unstructured{}
	pt.SetAPIVersion("v1")
	pt.SetKind("PodTemplate")
	pt.SetNamespace(ns)
	pt.SetName(name)
	_ = unstructured.SetNestedMap(pt.Object, podSpec, "template", "spec")
	_ = unstructured.SetNestedStringMap(pt.Object, map[string]string{
		v1.WorkloadIdLabel: workload.Name,
	}, "template", "metadata", "labels")
	return pt, nil
}

func externalRuntimeSeconds(workload *v1.Workload) int64 {
	if workload != nil && workload.Spec.Timeout != nil && *workload.Spec.Timeout > 0 {
		return int64(*workload.Spec.Timeout)
	}
	return 0
}

func externalGangMemberCount(workload *v1.Workload) int64 {
	if !commonworkload.IsExternalRDMAGang(workload) {
		return 1
	}
	return int64(1 + workload.Spec.Resources[1].Replica)
}

func externalProvisioningRequestName(workload *v1.Workload, generation, attempt int32) string {
	return fmt.Sprintf("pr-%s-%d-%d", shortUID(workload), generation, attempt)
}

func externalPodTemplateName(workload *v1.Workload, generation, attempt int32) string {
	return fmt.Sprintf("pt-%s-%d-%d", shortUID(workload), generation, attempt)
}

func shortUID(workload *v1.Workload) string {
	uid := string(workload.UID)
	if len(uid) > 8 {
		uid = uid[:8]
	}
	if uid == "" {
		uid = workload.Name
		if len(uid) > 8 {
			uid = uid[:8]
		}
	}
	return strings.ToLower(uid)
}

// validateExternalShapeForScheduler keeps the same shape gate the claim path used,
// plus InferaDeployment: frontend/prefill/decode are heterogeneous roles, not an
// RDMA gang. They admit immediately; unschedulable GPU pods trigger scale-up.
func validateExternalShapeForScheduler(workload *v1.Workload) error {
	if workload == nil {
		return fmt.Errorf("nil workload")
	}
	if commonworkload.IsExternalRDMAGang(workload) ||
		commonworkload.GetTotalReplica(workload) == 1 ||
		commonworkload.IsInferaDeployment(workload) {
		return nil
	}
	return fmt.Errorf("external kube-scheduler path supports one replica, an RDMA gang, or InferaDeployment")
}
