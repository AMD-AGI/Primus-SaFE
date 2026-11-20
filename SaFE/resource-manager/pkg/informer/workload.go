/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package informer

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/informers/externalversions"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
	workloadtopic "github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/topic/workload"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

var submitNotification = func(ctx context.Context, topic, uid string, data map[string]interface{}) error {
	return notification.GetNotificationManager().SubmitNotification(ctx, topic, uid, data)
}

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
	if v1.GetAnnotation(oldWorkload, v1.WorkloadLowUtilDetectedAtAnnotation) == "" &&
		v1.GetAnnotation(workload, v1.WorkloadLowUtilDetectedAtAnnotation) != "" {
		w.submitLowUtilizationNotification(workload)
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
	phase := string(workload.Status.Phase)
	notifyData := map[string]interface{}{
		"topic":     model.TopicWorkload,
		"condition": phase,
		"workload":  workload,
	}
	if workload.Status.Phase == v1.WorkloadStopped {
		if cond := meta.FindStatusCondition(workload.Status.Conditions, string(v1.AdminStopped)); cond != nil {
			notifyData["message"] = cond.Message
		}
	}
	w.submit(context.Background(), workload, fmt.Sprintf("%s-%s", workload.Name, phase), notifyData)
}

// submitLowUtilizationNotification tells the owner when the workload will be
// stopped for low GPU utilization, once per detection.
func (w *WorkloadInformer) submitLowUtilizationNotification(workload *v1.Workload) {
	ctx := context.Background()
	detectedAt := v1.GetAnnotation(workload, v1.WorkloadLowUtilDetectedAtAnnotation)
	detected, err := time.Parse(time.RFC3339, detectedAt)
	if err != nil {
		klog.Errorf("Invalid low utilization detection time %q of workload %s", detectedAt, workload.Name)
		return
	}
	workspace := &v1.Workspace{}
	if err = w.Get(ctx, client.ObjectKey{Name: workload.Spec.Workspace}, workspace); err != nil {
		klog.Errorf("Failed to get workspace %s: %v", workload.Spec.Workspace, err)
		return
	}
	policy := workspace.GetLowUtilization(commonworkload.GetScope(workload))
	if policy == nil || policy.DryRun {
		return
	}
	notifyBefore, _ := time.ParseDuration(policy.NotifyBefore)
	notice := &workloadtopic.LowUtilizationNotice{
		Threshold: policy.Threshold,
		Window:    policy.Window,
		StopTime:  detected.Add(notifyBefore).UTC().Format(time.DateTime) + " UTC",
		Rules:     lowUtilizationDetections(workload),
	}
	// job-manager writes the detected-at mark before the Detected entries, so they may not be here yet.
	if len(notice.Rules) == 0 {
		for _, rule := range policy.Rules {
			notice.Rules = append(notice.Rules, fmt.Sprintf("low GPU utilization below %d%% for %s", rule.Threshold, rule.Window))
		}
	}
	notifyData := map[string]interface{}{
		"topic":          model.TopicWorkload,
		"condition":      workloadtopic.ConditionLowUtilization,
		"workload":       workload,
		"lowUtilization": notice,
	}
	uid := fmt.Sprintf("%s-%s-%s", workload.Name, workloadtopic.ConditionLowUtilization, detectedAt)
	w.submit(ctx, workload, uid, notifyData)
}

// lowUtilizationDetections returns the descriptions of the AdminLowUtilization
// Detected_<n>[_<rule>] entries of the latest idle episode, or nil when that
// episode has recovered.
func lowUtilizationDetections(workload *v1.Workload) []string {
	latest := 0
	byEpisode := make(map[int][]string)
	for _, c := range workload.Status.Conditions {
		if c.Type != string(v1.AdminLowUtilization) {
			continue
		}
		suffix, ok := strings.CutPrefix(c.Reason, "Detected_")
		if !ok {
			continue
		}
		index, _, _ := strings.Cut(suffix, "_")
		n, err := strconv.Atoi(index)
		if err != nil {
			continue
		}
		description, _, _ := strings.Cut(c.Message, "; ")
		byEpisode[n] = append(byEpisode[n], description)
		latest = max(latest, n)
	}
	if hasCondition(workload, string(v1.AdminLowUtilization), fmt.Sprintf("Recovered_%d", latest)) {
		return nil
	}
	return byEpisode[latest]
}

func hasCondition(workload *v1.Workload, conditionType, reason string) bool {
	for _, c := range workload.Status.Conditions {
		if c.Type == conditionType && c.Reason == reason {
			return true
		}
	}
	return false
}

func (w *WorkloadInformer) submit(ctx context.Context,
	workload *v1.Workload, uid string, notifyData map[string]interface{}) {
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

	err = submitNotification(ctx, model.TopicWorkload, uid, notifyData)
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
