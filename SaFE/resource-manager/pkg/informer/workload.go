/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package informer

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/informers/externalversions"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
)

// NewWorkloadInformer creates a new workload informer instance.
func NewWorkloadInformer(c client.Client) *WorkloadInformer {
	return &WorkloadInformer{
		Client: c,
	}
}

type WorkloadInformer struct {
	client.Client
}

// OnAdd is called when a workload is added.
func (w *WorkloadInformer) OnAdd(obj interface{}, isInInitialList bool) {
	workload, ok := obj.(*v1.Workload)
	if !ok {
		klog.Errorf("Failed to convert obj to Workload")
		return
	}
	if isInInitialList {
		return
	}
	if workload.Status.Phase == "" {
		return
	}
	w.submitWorkloadNotification(workload)
}

// OnUpdate is called when a workload is updated.
func (w *WorkloadInformer) OnUpdate(oldObj, newObj interface{}) {
	workload, ok := newObj.(*v1.Workload)
	if !ok {
		klog.Errorf("Failed to convert obj to Workload")
		return
	}
	oldWorkload, ok := oldObj.(*v1.Workload)
	if !ok {
		klog.Errorf("Failed to convert oldObj to Workload")
		return
	}
	if workload.Status.Phase == "" {
		return
	}
	// Only notify when the workload-level Phase actually changes,
	// so that multi-pod updates for the same phase produce a single notification.
	if oldWorkload.Status.Phase == workload.Status.Phase {
		return
	}
	w.submitWorkloadNotification(workload)
}

// submitWorkloadNotification deduplicates by workload name + phase, ensuring
// at most one notification per workload per phase transition.
func (w *WorkloadInformer) submitWorkloadNotification(workload *v1.Workload) {
	ctx := context.Background()
	phase := string(workload.Status.Phase)
	uid := fmt.Sprintf("%s-%s", workload.Name, phase)

	notifyData := map[string]interface{}{
		"topic":     model.TopicWorkload,
		"condition": phase,
		"workload":  workload,
	}
	userId := v1.GetUserId(workload)
	user := &v1.User{}
	err := w.Get(ctx, client.ObjectKey{Name: userId}, user)
	if err != nil {
		klog.Errorf("Failed to get user %s: %v", userId, err)
		return
	}
	if !v1.IsUserEnableNotification(user) {
		return
	}
	notifyData["users"] = []*v1.User{user}

	notificationManager := notification.GetNotificationManager()
	err = notificationManager.SubmitNotification(ctx, model.TopicWorkload, uid, notifyData)
	if err != nil {
		klog.Errorf("Failed to submit notification for workload %s: %v", workload.Name, err)
	}
}

// OnDelete is called when a workload is deleted.
func (w *WorkloadInformer) OnDelete(obj interface{}) {
	return
}

// Register registers the informer with the factory.
func (w *WorkloadInformer) Register(factory externalversions.SharedInformerFactory) error {
	_, err := factory.Amd().V1().Workloads().Informer().AddEventHandler(w)
	if err != nil {
		klog.Errorf("Failed to register WorkloadInformer")
		return errors.New(fmt.Sprintf("Failed to register WorkloadInformer for informer: %s", err))
	}
	return nil
}
