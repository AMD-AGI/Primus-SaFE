/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exporter

import (
	"testing"
	"time"

	"gotest.tools/assert"
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
			Workspace:  "test-workspace",
			MaxRetry:   2,
			Image:      "test-image",
			EntryPoint: "sh -c test.sh",
			JobPort:    12345,
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
	assert.Equal(t, dbWorkload.Resources, string(jsonutils.MarshalSilently(w.Spec.Resources)))
	assert.Equal(t, dbutils.ParseNullTime(dbWorkload.CreationTime).Unix(), w.CreationTimestamp.Time.Unix())
}
