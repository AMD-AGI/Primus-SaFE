/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
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
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

// truncateString truncates a string to the specified maximum length in runes.
func truncateString(s string, maxLength int) string {
	if utf8.RuneCountInString(s) <= maxLength {
		return s
	}

	runes := []rune(s)
	return string(runes[:maxLength])
}

// escapePostgresArrayElement escapes special characters for PostgreSQL array literal syntax.
// In PostgreSQL array literals, backslashes, double quotes, and newlines need to be escaped.
func escapePostgresArrayElement(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	return s
}

// workloadMapper converts an unstructured workload object to a database workload model.
func workloadMapper(obj *unstructured.Unstructured) *dbclient.Workload {
	workload := &v1.Workload{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, workload)
	if err != nil {
		klog.ErrorS(err, "failed to convert object to workload", "data", obj)
		return nil
	}

	if !workload.GetDeletionTimestamp().IsZero() {
		if workload.Status.Phase != v1.WorkloadSucceeded &&
			workload.Status.Phase != v1.WorkloadFailed && workload.Status.Phase != v1.WorkloadStopped {
			workload.Status.Phase = v1.WorkloadStopped
			workload.Status.EndTime = workload.GetDeletionTimestamp()
		}
	} else if workload.Status.Phase == "" {
		workload.Status.Phase = v1.WorkloadPending
	}

	result := &dbclient.Workload{
		WorkloadId:    workload.Name,
		DisplayName:   v1.GetDisplayName(workload),
		Workspace:     workload.Spec.Workspace,
		Cluster:       v1.GetClusterId(workload),
		Resources:     dbutils.NullString(string(jsonutils.MarshalSilently(workload.Spec.Resources))),
		Image:         workload.Spec.Image,
		EntryPoint:    workload.Spec.EntryPoint,
		GVK:           string(jsonutils.MarshalSilently(workload.Spec.GroupVersionKind)),
		Phase:         dbutils.NullString(string(workload.Status.Phase)),
		UserName:      dbutils.NullString(v1.GetUserName(workload)),
		CreationTime:  dbutils.NullMetaV1Time(&workload.CreationTimestamp),
		StartTime:     dbutils.NullMetaV1Time(workload.Status.StartTime),
		EndTime:       dbutils.NullMetaV1Time(workload.Status.EndTime),
		DeletionTime:  dbutils.NullMetaV1Time(workload.GetDeletionTimestamp()),
		IsSupervised:  workload.Spec.IsSupervised,
		IsTolerateAll: workload.Spec.IsTolerateAll,
		Priority:      workload.Spec.Priority,
		MaxRetry:      workload.Spec.MaxRetry,
		QueuePosition: workload.Status.QueuePosition,
		DispatchCount: v1.GetWorkloadDispatchCnt(workload),
		Timeout:       workload.GetTimeout(),
		Description:   dbutils.NullString(v1.GetDescription(workload)),
		UserId:        dbutils.NullString(v1.GetUserId(workload)),
		WorkloadUId:   dbutils.NullString(string(workload.UID)),
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
	if len(workload.Status.Ranks) > 0 {
		result.Ranks = dbutils.NullString(string(jsonutils.MarshalSilently(workload.Status.Ranks)))
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
	if len(workload.Spec.Dependencies) > 0 {
		result.Dependencies = dbutils.NullString(string(jsonutils.MarshalSilently(workload.Spec.Dependencies)))
	}
	if len(workload.Spec.CronJobs) > 0 {
		result.CronJobs = dbutils.NullString(string(jsonutils.MarshalSilently(workload.Spec.CronJobs)))
	}
	if len(workload.Spec.Secrets) > 0 {
		result.Secrets = dbutils.NullString(string(jsonutils.MarshalSilently(workload.Spec.Secrets)))
	}
	if val := workload.GetEnv(common.ScaleRunnerSetID); val != "" {
		result.ScaleRunnerSet = dbutils.NullString(val)
	}
	if val := workload.GetEnv(common.ScaleRunnerID); val != "" {
		result.ScaleRunnerId = dbutils.NullString(val)
	}
	return result
}

// workloadFilter determines whether a workload update should be processed.
// Returns true if the update should be filtered out (ignored), false otherwise.
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

// faultMapper converts an unstructured fault object to a database fault model.
func faultMapper(obj *unstructured.Unstructured) *dbclient.Fault {
	fault := &v1.Fault{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, fault)
	if err != nil {
		klog.ErrorS(err, "failed to convert object to fault", "data", obj)
		return nil
	}

	message := truncateString(fault.Spec.Message, 256)
	result := &dbclient.Fault{
		Uid:            string(fault.UID),
		MonitorId:      fault.Spec.MonitorId,
		Message:        dbutils.NullString(message),
		Action:         dbutils.NullString(fault.Spec.Action),
		Phase:          dbutils.NullString(string(fault.Status.Phase)),
		Cluster:        dbutils.NullString(v1.GetClusterId(fault)),
		CreationTime:   dbutils.NullMetaV1Time(&fault.CreationTimestamp),
		UpdateTime:     dbutils.NullMetaV1Time(fault.Status.UpdateTime),
		DeletionTime:   dbutils.NullMetaV1Time(fault.GetDeletionTimestamp()),
		IsAutoRepaired: fault.Spec.IsAutoRepairEnabled,
	}
	if fault.Spec.Node != nil {
		result.Node = dbutils.NullString(fault.Spec.Node.AdminName)
	}
	return result
}

// faultFilter determines whether a fault update should be processed.
// Returns true if the update should be filtered out (ignored), false otherwise.
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

// opsJobMapper converts an unstructured ops job object to a database ops job model.
func opsJobMapper(obj *unstructured.Unstructured) *dbclient.OpsJob {
	job := &v1.OpsJob{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, job)
	if err != nil {
		klog.ErrorS(err, "failed to convert object to job", "data", obj)
		return nil
	}

	inputs := make([]string, 0, len(job.Spec.Inputs))
	for _, p := range job.Spec.Inputs {
		inputs = append(inputs, escapePostgresArrayElement(v1.CvtParamToString(&p)))
	}
	strInputs := fmt.Sprintf("{\"%s\"}", strings.Join(inputs, "\",\""))
	result := &dbclient.OpsJob{
		JobId:         job.Name,
		Cluster:       v1.GetClusterId(job),
		Inputs:        []byte(strInputs),
		Type:          string(job.Spec.Type),
		Timeout:       job.Spec.TimeoutSecond,
		UserName:      dbutils.NullString(v1.GetUserName(job)),
		Workspace:     dbutils.NullString(v1.GetWorkspaceId(job)),
		CreationTime:  dbutils.NullMetaV1Time(&job.CreationTimestamp),
		StartTime:     dbutils.NullMetaV1Time(job.Status.StartedAt),
		EndTime:       dbutils.NullMetaV1Time(job.Status.FinishedAt),
		DeletionTime:  dbutils.NullMetaV1Time(job.GetDeletionTimestamp()),
		Phase:         dbutils.NullString(string(job.Status.Phase)),
		UserId:        dbutils.NullString(v1.GetUserId(job)),
		IsTolerateAll: job.Spec.IsTolerateAll,
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
		if job.Status.Phase == v1.OpsJobRunning || job.Status.Phase == "" {
			job.Status.Phase = v1.OpsJobFailed
		}
	}
	if len(job.Spec.Env) > 0 {
		result.Env = dbutils.NullString(
			string(jsonutils.MarshalSilently(job.Spec.Env)))
	}
	if job.Spec.Resource != nil {
		result.Resource = dbutils.NullString(string(jsonutils.MarshalSilently(job.Spec.Resource)))
	}
	if job.Spec.Image != nil {
		result.Image = dbutils.NullString(*job.Spec.Image)
	}
	if job.Spec.EntryPoint != nil {
		result.EntryPoint = dbutils.NullString(*job.Spec.EntryPoint)
	}
	if len(job.Spec.Hostpath) > 0 {
		result.Hostpath = dbutils.NullString(string(jsonutils.MarshalSilently(job.Spec.Hostpath)))
	}
	if len(job.Spec.ExcludedNodes) > 0 {
		result.ExcludedNodes = dbutils.NullString(string(jsonutils.MarshalSilently(job.Spec.ExcludedNodes)))
	}
	return result
}

// modelMapper converts an unstructured object to a database Model.
func modelMapper(obj *unstructured.Unstructured) *dbclient.Model {
	cr := &v1.Model{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, cr)
	if err != nil {
		klog.ErrorS(err, "failed to convert object to model", "data", obj)
		return nil
	}

	dbModel := &dbclient.Model{
		ID:           cr.Name,
		DisplayName:  cr.Spec.DisplayName,
		Description:  cr.Spec.Description,
		Icon:         cr.Spec.Icon,
		Label:        cr.Spec.Label,
		Tags:         strings.Join(cr.Spec.Tags, ","),
		MaxTokens:    cr.Spec.MaxTokens,
		SourceURL:    cr.Spec.Source.URL,
		AccessMode:   string(cr.Spec.Source.AccessMode),
		ModelName:    cr.Spec.Source.ModelName,
		Workspace:    cr.Spec.Workspace,
		Phase:        string(cr.Status.Phase),
		Message:      cr.Status.Message,
		S3Path:       cr.Status.S3Path,
		CreatedAt:    dbutils.NullTime(cr.CreationTimestamp.Time),
		UpdatedAt:    dbutils.NullMetaV1Time(cr.Status.UpdateTime),
		DeletionTime: dbutils.NullMetaV1Time(cr.GetDeletionTimestamp()),
		IsDeleted:    !cr.GetDeletionTimestamp().IsZero(),
	}

	// Serialize local paths to JSON
	if len(cr.Status.LocalPaths) > 0 {
		dbModel.LocalPaths = string(jsonutils.MarshalSilently(cr.Status.LocalPaths))
	} else {
		dbModel.LocalPaths = "[]"
	}

	// Store Secret name (not the actual token)
	if cr.Spec.Source.Token != nil {
		dbModel.SourceToken = cr.Spec.Source.Token.Name
	}

	return dbModel
}
