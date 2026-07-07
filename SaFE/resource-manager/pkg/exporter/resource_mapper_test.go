/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exporter

import (
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/unstructured"
)

var (
	TestWorkloadData = &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-workload",
			Labels: map[string]string{
				v1.ClusterIdLabel: "test-cluster",
			},
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: v1.WorkloadSpec{
			Workspace: "test-workspace",
			MaxRetry:  2,
			JobPort:   12345,
			GroupVersionKind: v1.GroupVersionKind{
				Group:   "kubeflow.org",
				Version: "v1",
				Kind:    "PyTorchJob",
			},
			Resources: []v1.WorkloadResource{{
				Replica:          1,
				CPU:              "32",
				GPU:              "4",
				GPUName:          "amd.com/gpu",
				Memory:           "256Gi",
				SharedMemory:     "32Gi",
				EphemeralStorage: "20Gi",
			}},
		},
	}
)

func TestWorkloadMapper(t *testing.T) {
	w := TestWorkloadData.DeepCopy()
	unstructuredObj, err := unstructured.ConvertObjectToUnstructured(w)
	assert.NilError(t, err)
	dbWorkload := workloadMapper(unstructuredObj)
	assert.Equal(t, dbWorkload.WorkloadId, w.Name)
	assert.Equal(t, dbWorkload.DisplayName, v1.GetDisplayName(w))
	assert.Equal(t, dbutils.ParseNullString(dbWorkload.Resources), string(jsonutils.MarshalSilently(w.Spec.Resources)))
	assert.Equal(t, dbutils.ParseNullTime(dbWorkload.CreationTime).Unix(), w.CreationTimestamp.Time.Unix())
}

func TestTruncateString(t *testing.T) {
	assert.Equal(t, "abc", truncateString("abc", 5))
	assert.Equal(t, "ab", truncateString("abcdef", 2))
	assert.Equal(t, "", truncateString("abc", 0))
}

func TestEscapePostgresArrayElement(t *testing.T) {
	out := escapePostgresArrayElement("a\\b\"c\nd\re")
	assert.Equal(t, `a\\b\"c\nd\re`, out)
}

func TestNormalizeModelOrigin(t *testing.T) {
	assert.Equal(t, "external", normalizeModelOrigin(""))
	assert.Equal(t, "internal", normalizeModelOrigin("internal"))
}

func TestFaultMapper(t *testing.T) {
	fault := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "f1",
			Labels:            map[string]string{v1.ClusterIdLabel: "c1"},
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: v1.FaultSpec{
			MonitorId: "m1",
			Message:   "boom",
			Action:    "restart",
		},
	}
	u, err := unstructured.ConvertObjectToUnstructured(fault)
	assert.NilError(t, err)
	res := faultMapper(u)
	assert.Equal(t, res.Uid, string(fault.UID))
	assert.Equal(t, res.MonitorId, "m1")
}

func TestOpsJobMapper(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "j1",
			Labels:            map[string]string{v1.ClusterIdLabel: "c1"},
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: v1.OpsJobSpec{
			Type:          "reboot",
			TimeoutSecond: 100,
		},
	}
	u, err := unstructured.ConvertObjectToUnstructured(job)
	assert.NilError(t, err)
	res := opsJobMapper(u)
	assert.Equal(t, res.JobId, "j1")
	assert.Equal(t, res.Type, "reboot")
}

func TestModelMapper(t *testing.T) {
	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "m1",
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: v1.ModelSpec{
			DisplayName: "My Model",
			Tags:        []string{"a", "b"},
		},
	}
	u, err := unstructured.ConvertObjectToUnstructured(model)
	assert.NilError(t, err)
	res := modelMapper(u)
	assert.Equal(t, res.ID, "m1")
	assert.Equal(t, res.DisplayName, "My Model")
	assert.Equal(t, res.Tags, "a,b")
	assert.Equal(t, res.LocalPaths, "[]")
}

func TestWorkloadFilter(t *testing.T) {
	w := TestWorkloadData.DeepCopy()
	u1, err := unstructured.ConvertObjectToUnstructured(w)
	assert.NilError(t, err)
	u2, err := unstructured.ConvertObjectToUnstructured(w.DeepCopy())
	assert.NilError(t, err)

	assert.Equal(t, false, workloadFilter(nil, u2))
	assert.Equal(t, true, workloadFilter(u1, u2))
}

func TestFaultFilter(t *testing.T) {
	fault := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{Name: "f1"},
		Spec:       v1.FaultSpec{MonitorId: "m1"},
	}
	u1, err := unstructured.ConvertObjectToUnstructured(fault)
	assert.NilError(t, err)
	u2, err := unstructured.ConvertObjectToUnstructured(fault.DeepCopy())
	assert.NilError(t, err)

	assert.Equal(t, false, faultFilter(nil, u2))
	assert.Equal(t, true, faultFilter(u1, u2))
}

func TestWorkloadMapperRichBranches(t *testing.T) {
	now := metav1.Now()
	w := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "w2",
			Labels:            map[string]string{v1.ClusterIdLabel: "c1"},
			CreationTimestamp: now,
			DeletionTimestamp: &now,
			Finalizers:        []string{"f"},
		},
		Spec: v1.WorkloadSpec{
			Workspace: "ws",
			Resources: []v1.WorkloadResource{{Replica: 1, CPU: "2"}},
			Images:    []string{"img:1"},
			Env:       map[string]string{"K": "V"},
		},
		Status: v1.WorkloadStatus{
			Phase: v1.WorkloadRunning,
			Pods: []v1.WorkloadPod{
				{Phase: corev1.PodRunning},
			},
			Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}},
		},
	}
	u, err := unstructured.ConvertObjectToUnstructured(w)
	assert.NilError(t, err)
	res := workloadMapper(u)
	assert.Assert(t, res != nil)
	assert.Equal(t, res.WorkloadId, "w2")
}

func TestWorkloadMapperServiceLivenessBranches(t *testing.T) {
	w := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "w3",
			Labels:            map[string]string{v1.ClusterIdLabel: "c1"},
			CreationTimestamp: metav1.Now(),
		},
		Spec: v1.WorkloadSpec{
			Workspace:      "ws",
			Resources:      []v1.WorkloadResource{{Replica: 1, CPU: "2"}},
			Images:         []string{"img:1"},
			Service:        &v1.Service{},
			Liveness:       &v1.HealthCheck{},
			Readiness:      &v1.HealthCheck{},
			Dependencies:   []string{"dep1"},
			Secrets:        []v1.SecretEntity{{Id: "s1", Type: v1.SecretGeneral}},
			CustomerLabels: map[string]string{"k": "v"},
		},
		Status: v1.WorkloadStatus{
			Phase: v1.WorkloadRunning,
			Nodes: [][]string{{"n1"}},
		},
	}
	u, err := unstructured.ConvertObjectToUnstructured(w)
	assert.NilError(t, err)
	res := workloadMapper(u)
	assert.Equal(t, res.WorkloadId, "w3")
}

func TestModelMapperRichBranches(t *testing.T) {
	now := metav1.Now()
	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{Name: "m2", CreationTimestamp: now},
		Spec: v1.ModelSpec{
			DisplayName: "M2",
			Tags:        []string{"x"},
			Origin:      "internal",
			Source: v1.ModelSource{
				URL:   "s3://bucket/m2",
				Token: &corev1.LocalObjectReference{Name: "tok"},
			},
		},
		Status: v1.ModelStatus{
			Phase:      v1.ModelPhaseReady,
			LocalPaths: []v1.ModelLocalPath{{Workspace: "ws1", Path: "/data/m2"}},
		},
	}
	u, err := unstructured.ConvertObjectToUnstructured(model)
	assert.NilError(t, err)
	res := modelMapper(u)
	assert.Equal(t, res.ID, "m2")
	assert.Equal(t, res.SourceToken, "tok")
	assert.Equal(t, res.Origin, "internal")
	assert.Assert(t, res.LocalPaths != "[]")
}

func TestFaultMapperWithNode(t *testing.T) {
	fault := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{Name: "f2", CreationTimestamp: metav1.Now()},
		Spec: v1.FaultSpec{
			MonitorId: "m1",
			Message:   "boom",
			Node:      &v1.FaultNode{AdminName: "n1", ClusterName: "c1"},
		},
	}
	u, err := unstructured.ConvertObjectToUnstructured(fault)
	assert.NilError(t, err)
	res := faultMapper(u)
	assert.Equal(t, res.MonitorId, "m1")
}

func TestOpsJobMapperWithDeletion(t *testing.T) {
	now := metav1.Now()
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "j2",
			Labels:            map[string]string{v1.ClusterIdLabel: "c1"},
			CreationTimestamp: now,
			DeletionTimestamp: &now,
			Finalizers:        []string{"f"},
		},
		Spec: v1.OpsJobSpec{
			Type:   "reboot",
			Env:    map[string]string{"K": "V"},
			Inputs: []v1.Parameter{{Name: "node", Value: "n1"}},
		},
		Status: v1.OpsJobStatus{
			Phase:      v1.OpsJobRunning,
			Conditions: []metav1.Condition{{Type: "Done", Status: metav1.ConditionTrue}},
		},
	}
	u, err := unstructured.ConvertObjectToUnstructured(job)
	assert.NilError(t, err)
	res := opsJobMapper(u)
	assert.Assert(t, res != nil)
	assert.Equal(t, res.JobId, "j2")
}
