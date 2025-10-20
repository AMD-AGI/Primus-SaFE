/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

var (
	eventMessagePath           = []string{"message"}
	eventInvolvedNamePath      = []string{"involvedObject", "name"}
	eventInvolvedNamespacePath = []string{"involvedObject", "namespace"}
	eventTypePath              = []string{"type"}
	eventReasonPath            = []string{"reason"}
	eventInvolvedKindPath      = []string{"involvedObject", "kind"}

	cardEventReasons = []string{"BackOff", "FreeDiskSpaceFailed"}

	PullingReason       = "Pulling"
	PulledReason        = "Pulled"
	PullingImageMessage = "Pulling image"

	workloadIdPath = []string{"metadata", "labels", v1.WorkloadIdLabel}
)

func (r *SyncerReconciler) handleEvent(ctx context.Context, msg *resourceMessage, informer *ClusterInformer) (ctrlruntime.Result, error) {
	eventInformer, err := informer.GetResourceInformer(ctx, msg.gvk)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	eventObj, err := jobutils.GetObject(eventInformer, msg.name, msg.namespace)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	adminWorkload, err := r.getAdminWorkloadByEvent(ctx, informer, eventObj)
	if adminWorkload == nil {
		return ctrlruntime.Result{}, err
	}
	if err = r.updatePendingMessage(ctx, adminWorkload, eventObj); err != nil {
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

func (r *SyncerReconciler) getAdminWorkloadByEvent(ctx context.Context,
	informer *ClusterInformer, eventObj *unstructured.Unstructured) (*v1.Workload, error) {
	podName := jobutils.GetUnstructuredString(eventObj.Object, eventInvolvedNamePath)
	if podName == "" {
		return nil, nil
	}
	podNamespace := jobutils.GetUnstructuredString(eventObj.Object, eventInvolvedNamespacePath)
	if podNamespace == "" {
		return nil, nil
	}
	podInformer, err := informer.GetResourceInformer(ctx, corev1.SchemeGroupVersion.WithKind(common.PodKind))
	if err != nil {
		return nil, nil
	}
	podObj, err := jobutils.GetObject(podInformer, podName, podNamespace)
	if err != nil {
		return nil, err
	}
	workloadId := jobutils.GetUnstructuredString(podObj.Object, workloadIdPath)
	if workloadId == "" {
		return nil, nil
	}
	return r.getAdminWorkload(ctx, workloadId)
}

func (r *SyncerReconciler) updatePendingMessage(ctx context.Context, adminWorkload *v1.Workload, eventObj *unstructured.Unstructured) error {
	// At present, processing is focused on the Pending state
	if !adminWorkload.IsPending() {
		return nil
	}
	message := jobutils.GetUnstructuredString(eventObj.Object, eventMessagePath)
	reason := jobutils.GetUnstructuredString(eventObj.Object, eventReasonPath)
	switch {
	case reason == PulledReason:
		// If the image has already been pulled, clear the Pulling status message
		if strings.Contains(message, PullingImageMessage) {
			message = ""
		} else {
			return nil
		}
	case message == "", adminWorkload.Status.Message == message:
		return nil
	case strings.Contains(message, "already exists"):
		// ignore "already exists" warning
		return nil
	}

	originalWorkload := client.MergeFrom(adminWorkload.DeepCopy())
	adminWorkload.Status.Message = message
	if err := r.Status().Patch(ctx, adminWorkload, originalWorkload); err != nil {
		return err
	}
	return nil
}

func isRelevantPodEvent(obj *unstructured.Unstructured) bool {
	eventInvolvedKind := jobutils.GetUnstructuredString(obj.Object, eventInvolvedKindPath)
	if eventInvolvedKind != common.PodKind {
		return false
	}

	eventType := jobutils.GetUnstructuredString(obj.Object, eventTypePath)
	eventReason := jobutils.GetUnstructuredString(obj.Object, eventReasonPath)
	if eventType == corev1.EventTypeNormal {
		if eventReason == PullingReason || eventReason == PulledReason {
			return true
		}
		return false
	}

	if eventType != corev1.EventTypeWarning {
		return false
	}
	if strings.HasPrefix(eventReason, "Failed") {
		return true
	}
	for _, reason := range cardEventReasons {
		if reason == eventReason {
			return true
		}
	}
	return false
}
