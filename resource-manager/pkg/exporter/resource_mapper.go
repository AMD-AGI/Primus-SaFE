/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exporter

import (
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

func truncateString(s string, maxLength int) string {
	if utf8.RuneCountInString(s) <= maxLength {
		return s
	}

	runes := []rune(s)
	return string(runes[:maxLength])
}

func workloadMapper(obj *unstructured.Unstructured) *dbclient.Workload {
	workload := &v1.Workload{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, workload)
	if err != nil {
		klog.ErrorS(err, "failed to convert object to workload", "data", obj)
		return nil
	}

	if !workload.GetDeletionTimestamp().IsZero() {
		if workload.Status.Phase != v1.WorkloadSucceeded &&
			workload.Status.Phase != v1.WorkloadFailed {
			workload.Status.Phase = v1.WorkloadStopped
		}
	}
	result := &dbclient.Workload{
		WorkloadId:     workload.Name,
		DisplayName:    v1.GetDisplayName(workload),
		Workspace:      workload.Spec.Workspace,
		Cluster:        v1.GetClusterId(workload),
		Resource:       string(jsonutils.MarshalSilently(workload.Spec.Resource)),
		Image:          workload.Spec.Image,
		EntryPoint:     workload.Spec.EntryPoint,
		GVK:            string(jsonutils.MarshalSilently(workload.Spec.GroupVersionKind)),
		Phase:          dbutils.NullString(string(workload.Status.Phase)),
		UserName:       dbutils.NullString(v1.GetUserName(workload)),
		CreateTime:     dbutils.NullMetaV1Time(&workload.CreationTimestamp),
		StartTime:      dbutils.NullMetaV1Time(workload.Status.StartTime),
		EndTime:        dbutils.NullMetaV1Time(workload.Status.EndTime),
		DeleteTime:     dbutils.NullMetaV1Time(workload.GetDeletionTimestamp()),
		IsSupervised:   workload.Spec.IsSupervised,
		IsTolerateAll:  workload.Spec.IsTolerateAll,
		Priority:       workload.Spec.Priority,
		MaxRetry:       workload.Spec.MaxRetry,
		SchedulerOrder: workload.Status.SchedulerOrder,
		DispatchCount:  v1.GetWorkloadDispatchCnt(workload),
		Timeout:        workload.GetTimeout(),
		Description:    dbutils.NullString(v1.GetDescription(workload)),
	}
	if workload.Spec.TTLSecondsAfterFinished != nil {
		result.TTLSecond = *workload.Spec.TTLSecondsAfterFinished
	}
	if len(workload.Status.Conditions) > 0 {
		result.Conditions = dbutils.NullString(string(jsonutils.MarshalSilently(workload.Status.Conditions)))
	}
	if len(workload.Spec.Env) > 0 {
		result.Env = dbutils.NullString(string(jsonutils.MarshalSilently(workload.Spec.Env)))
	}
	if len(workload.Status.Pods) > 0 {
		if !workload.GetDeletionTimestamp().IsZero() {
			for i := range workload.Status.Pods {
				if workload.Status.Pods[i].Phase != corev1.PodSucceeded &&
					workload.Status.Pods[i].Phase != corev1.PodFailed {
					workload.Status.Pods[i].Phase = corev1.PodPhase(v1.WorkloadStopped)
				}
			}
		}
		result.Pods = dbutils.NullString(string(jsonutils.MarshalSilently(workload.Status.Pods)))
	}
	if len(workload.Status.Nodes) > 0 {
		result.Nodes = dbutils.NullString(string(jsonutils.MarshalSilently(workload.Status.Nodes)))
	}
	if len(workload.Spec.CustomerLabels) > 0 {
		result.CustomerLabels = dbutils.NullString(string(jsonutils.MarshalSilently(workload.Spec.CustomerLabels)))
	}
	if workload.Spec.Service != nil {
		result.Service = dbutils.NullString(string(jsonutils.MarshalSilently(workload.Spec.Service)))
	}
	if workload.Spec.Liveness != nil {
		result.Liveness = dbutils.NullString(string(jsonutils.MarshalSilently(workload.Spec.Liveness)))
	}
	if workload.Spec.Readiness != nil {
		result.Readiness = dbutils.NullString(string(jsonutils.MarshalSilently(workload.Spec.Readiness)))
	}
	return result
}

func workloadFilter(oldObj, newObj *unstructured.Unstructured) bool {
	if oldObj == nil || newObj == nil {
		return false
	}
	oldWorkload := &v1.Workload{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(oldObj.Object, oldWorkload)
	if err != nil {
		return true
	}
	newWorkload := &v1.Workload{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(newObj.Object, newWorkload)
	if err != nil {
		return true
	}
	if oldWorkload.GetDeletionTimestamp().IsZero() && !newWorkload.GetDeletionTimestamp().IsZero() {
		return false
	}
	if v1.GetDescription(oldWorkload) != v1.GetDescription(newWorkload) {
		return false
	}
	if reflect.DeepEqual(oldWorkload.Spec, newWorkload.Spec) &&
		reflect.DeepEqual(oldWorkload.Status, newWorkload.Status) {
		return true
	}
	return false
}

func faultMapper(obj *unstructured.Unstructured) *dbclient.Fault {
	fault := &v1.Fault{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, fault)
	if err != nil {
		klog.ErrorS(err, "fail to convert object to fault", "data", obj)
		return nil
	}

	message := truncateString(fault.Spec.Message, 256)
	result := &dbclient.Fault{
		FaultId:        fault.Name,
		MonitorId:      fault.Spec.Id,
		Message:        dbutils.NullString(message),
		UUid:           string(fault.UID),
		Action:         dbutils.NullString(fault.Spec.Action),
		Phase:          dbutils.NullString(string(fault.Status.Phase)),
		Cluster:        dbutils.NullString(v1.GetClusterId(fault)),
		CreateTime:     dbutils.NullMetaV1Time(&fault.CreationTimestamp),
		UpdateTime:     dbutils.NullMetaV1Time(fault.Status.UpdateTime),
		DeleteTime:     dbutils.NullMetaV1Time(fault.GetDeletionTimestamp()),
		IsAutoRepaired: fault.Spec.IsAutoRepairEnabled,
	}
	if fault.Spec.Node != nil {
		result.Node = dbutils.NullString(fault.Spec.Node.AdminName)
	}
	return result
}

func faultFilter(oldObj, newObj *unstructured.Unstructured) bool {
	if oldObj == nil || newObj == nil {
		return false
	}
	oldFault := &v1.Fault{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(oldObj.Object, oldFault)
	if err != nil {
		return true
	}
	newFault := &v1.Fault{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(newObj.Object, newFault)
	if err != nil {
		return true
	}
	if oldFault.GetDeletionTimestamp().IsZero() && !newFault.GetDeletionTimestamp().IsZero() {
		return false
	}
	if reflect.DeepEqual(oldFault.Spec, newFault.Spec) &&
		reflect.DeepEqual(oldFault.Status, newFault.Status) {
		return true
	}
	return false
}

func jobMapper(obj *unstructured.Unstructured) *dbclient.Job {
	job := &v1.Job{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, job)
	if err != nil {
		klog.ErrorS(err, "failed to convert object to job", "data", obj)
		return nil
	}

	var inputs []string
	for _, p := range job.Spec.Inputs {
		inputs = append(inputs, v1.CvtParamToString(&p))
	}
	strInputs := fmt.Sprintf("{%s}", fmt.Sprintf("\"%s\"", strings.Join(inputs, "\",\"")))
	result := &dbclient.Job{
		JobId:      job.Name,
		Cluster:    v1.GetClusterId(job),
		Inputs:     []byte(strInputs),
		Type:       string(job.Spec.Type),
		Timeout:    job.Spec.TimeoutSecond,
		UserName:   dbutils.NullString(v1.GetUserName(job)),
		JobName:    dbutils.NullString(v1.GetDisplayName(job)),
		Workspace:  dbutils.NullString(v1.GetWorkspaceId(job)),
		CreateTime: dbutils.NullMetaV1Time(&job.CreationTimestamp),
		StartTime:  dbutils.NullMetaV1Time(job.Status.StartedAt),
		EndTime:    dbutils.NullMetaV1Time(job.Status.FinishedAt),
		DeleteTime: dbutils.NullMetaV1Time(job.GetDeletionTimestamp()),
		Phase:      dbutils.NullString(string(job.Status.Phase)),
		Message:    dbutils.NullString(job.Status.Message),
	}
	if len(job.Status.Conditions) > 0 {
		result.Conditions = dbutils.NullString(
			string(jsonutils.MarshalSilently(job.Status.Conditions)))
	}
	if len(job.Status.Outputs) > 0 {
		result.Outputs = dbutils.NullString(
			string(jsonutils.MarshalSilently(job.Status.Outputs)))
	}
	if !job.GetDeletionTimestamp().IsZero() {
		if job.Status.Phase == v1.JobRunning || job.Status.Phase == "" {
			job.Status.Phase = v1.JobFailed
		}
	}
	return result
}
