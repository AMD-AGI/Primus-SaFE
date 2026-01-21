/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

func addK8sJobCond(t *testing.T, pytorchJob *unstructured.Unstructured, cond map[string]interface{}) {
	object := pytorchJob.Object
	conditions, _, err := unstructured.NestedSlice(object, "status", "conditions")
	assert.NilError(t, err)
	conditions = append(conditions, cond)
	err = unstructured.SetNestedSlice(object, conditions, "status", "conditions")
	assert.NilError(t, err)
}

func TestGetPytorchJobPhase(t *testing.T) {
	pytorchJob, err := jsonutils.ParseYamlToJson(TestPytorchData)
	assert.NilError(t, err)
	rt := TestPytorchResourceTemplate.DeepCopy()

	status, err := GetK8sObjectStatus(pytorchJob, rt)
	assert.NilError(t, err)
	assert.Equal(t, status.Phase, "")

	newCondition := map[string]interface{}{
		"type":    "Running",
		"status":  "False",
		"reason":  "JobRunning",
		"message": "job is running",
	}
	addK8sJobCond(t, pytorchJob, newCondition)
	status, err = GetK8sObjectStatus(pytorchJob, rt)
	assert.NilError(t, err)
	assert.Equal(t, status.Phase, "")

	newCondition["status"] = "True"
	addK8sJobCond(t, pytorchJob, newCondition)
	status, err = GetK8sObjectStatus(pytorchJob, rt)
	assert.NilError(t, err)
	assert.Equal(t, status.Phase, "K8sRunning")
	assert.Equal(t, status.Message, "job is running")

	newCondition = map[string]interface{}{
		"type":    "Succeeded",
		"status":  "True",
		"reason":  "succeed",
		"message": "job is succeed",
	}
	addK8sJobCond(t, pytorchJob, newCondition)
	status, err = GetK8sObjectStatus(pytorchJob, rt)
	assert.NilError(t, err)
	assert.Equal(t, status.Phase, "K8sSucceeded")
	assert.Equal(t, status.Message, "job is succeed")
}

func TestPytorchJobActiveCount(t *testing.T) {
	pytorchJob, err := jsonutils.ParseYamlToJson(TestPytorchData)
	assert.NilError(t, err)
	rt := TestPytorchResourceTemplate.DeepCopy()

	count, err := GetActiveReplica(pytorchJob, rt)
	assert.NilError(t, err)
	assert.Equal(t, count, 64)
}

func TestPytorchJobSpecCount(t *testing.T) {
	pytorchJob, err := jsonutils.ParseYamlToJson(TestPytorchData)
	assert.NilError(t, err)
	rt := TestPytorchResourceTemplate.DeepCopy()

	count, err := GetSpecReplica(pytorchJob, rt)
	assert.NilError(t, err)
	assert.Equal(t, count, 64)
}

func TestGetJobPhase(t *testing.T) {
	job, err := jsonutils.ParseYamlToJson(TestJobData)
	assert.NilError(t, err)
	rt := TestJobResourceTemplate.DeepCopy()

	status, err := GetK8sObjectStatus(job, rt)
	assert.NilError(t, err)
	assert.Equal(t, status.Phase, string(v1.K8sRunning))

	newCondition := map[string]interface{}{
		"type":    "Failed",
		"status":  "True",
		"reason":  "BackoffLimitExceeded",
		"message": "Job has reached the specified backoff limit",
	}
	addK8sJobCond(t, job, newCondition)
	status, err = GetK8sObjectStatus(job, rt)
	assert.NilError(t, err)
	assert.Equal(t, status.Phase, "K8sFailed")
}

func TestGetJobActiveReplica(t *testing.T) {
	job, err := jsonutils.ParseYamlToJson(TestJobData)
	assert.NilError(t, err)
	rt := TestJobResourceTemplate.DeepCopy()

	replica, err := GetActiveReplica(job, rt)
	assert.NilError(t, err)
	assert.Equal(t, replica, 2)
}

func TestGetDeploymentPhase(t *testing.T) {
	deploy, err := jsonutils.ParseYamlToJson(TestDeploymentData)
	assert.NilError(t, err)
	rt := TestDeploymentResourceTemplate.DeepCopy()

	status, err := GetK8sObjectStatus(deploy, rt)
	assert.NilError(t, err)
	assert.Equal(t, status != nil, true)
	assert.Equal(t, status.SpecReplica, 2)
	assert.Equal(t, status.ActiveReplica, 2)
	assert.Equal(t, status.Phase, string(v1.K8sRunning))

	conditions, _, err := unstructured.NestedSlice(deploy.Object, "status", "conditions")
	assert.NilError(t, err)
	conditions2 := conditions
	cond := map[string]interface{}{
		"type":   "Progressing",
		"status": "True",
		"reason": "ReplicaSetUpdated",
	}
	conditions = append(conditions, cond)
	err = unstructured.SetNestedSlice(deploy.Object, conditions, "status", "conditions")
	assert.NilError(t, err)
	status, err = GetK8sObjectStatus(deploy, rt)
	assert.NilError(t, err)
	assert.Equal(t, status != nil, true)
	assert.Equal(t, status.Phase, string(v1.K8sUpdating))

	cond = map[string]interface{}{
		"type":   "Progressing",
		"status": "True",
		"reason": "NewReplicaSetAvailable",
	}
	conditions2 = append(conditions2, cond)
	err = unstructured.SetNestedSlice(deploy.Object, conditions2, "status", "conditions")
	assert.NilError(t, err)
	status, err = GetK8sObjectStatus(deploy, rt)
	assert.NilError(t, err)
	assert.Equal(t, status != nil, true)
	assert.Equal(t, status.Phase, string(v1.K8sRunning))
}

func TestGetDeploymentResources(t *testing.T) {
	deploy, err := jsonutils.ParseYamlToJson(TestDeploymentData)
	assert.NilError(t, err)
	rt := TestDeploymentResourceTemplate.DeepCopy()

	replicaList, resourceList, err := GetResources(deploy, rt, "test", common.AmdGpu)
	assert.NilError(t, err)
	assert.Equal(t, len(resourceList), 1)
	rl := resourceList[0]
	assert.Equal(t, rl.Cpu().Value(), int64(64))
	assert.Equal(t, rl.Memory().String(), "200Gi")
	assert.Equal(t, rl.StorageEphemeral().String(), "100Gi")
	gpuQuantity, ok := rl[common.AmdGpu]
	assert.Equal(t, ok, true)
	assert.Equal(t, gpuQuantity.Value(), int64(8))

	assert.Equal(t, replicaList[0], int64(2))
}

func TestGetDeploymentImage(t *testing.T) {
	deploy, err := jsonutils.ParseYamlToJson(TestDeploymentData)
	assert.NilError(t, err)
	rt := TestDeploymentResourceTemplate.DeepCopy()

	images, err := GetImages(deploy, rt, "test")
	assert.NilError(t, err)
	assert.Equal(t, len(images), 1)
	assert.Equal(t, images[0], "test-image:latest")
}

func TestGetDeploymentCommand(t *testing.T) {
	deploy, err := jsonutils.ParseYamlToJson(TestDeploymentData)
	assert.NilError(t, err)
	rt := TestDeploymentResourceTemplate.DeepCopy()

	commands, err := GetCommands(deploy, rt, "test")
	assert.NilError(t, err)
	assert.Equal(t, len(commands), 1)
	assert.Equal(t, len(commands[0]), 3)
	assert.Equal(t, commands[0][0], "sh")
	assert.Equal(t, commands[0][1], "-c")
	assert.Equal(t, commands[0][2], "/bin/sh run.sh 'abcd'")
}

func TestGetPytorchJobCommands(t *testing.T) {
	deploy, err := jsonutils.ParseYamlToJson(TestPytorchData)
	assert.NilError(t, err)
	rt := TestPytorchResourceTemplate.DeepCopy()

	commands, err := GetCommands(deploy, rt, "pytorch")
	assert.NilError(t, err)
	assert.Equal(t, len(commands), 2)
	assert.Equal(t, len(commands[0]), 3)
	assert.Equal(t, commands[0][0], "sh")
	assert.Equal(t, commands[0][1], "-c")
	assert.Equal(t, commands[0][2], "test.sh")

	assert.Equal(t, len(commands[1]), 3)
	assert.Equal(t, commands[1][0], "sh")
	assert.Equal(t, commands[1][1], "-c")
	assert.Equal(t, commands[1][2], "test.sh")
}

func TestGetDeploymentShareMemorySize(t *testing.T) {
	deploy, err := jsonutils.ParseYamlToJson(TestDeploymentData)
	assert.NilError(t, err)
	rt := TestDeploymentResourceTemplate.DeepCopy()

	memoryStorageSizes, err := GetMemoryStorageSize(deploy, rt)
	assert.NilError(t, err)
	assert.Equal(t, len(memoryStorageSizes), 1)
	assert.Equal(t, memoryStorageSizes[0], "20Gi")
}

func TestGetPytorchJobResources(t *testing.T) {
	job, err := jsonutils.ParseYamlToJson(TestPytorchData)
	assert.NilError(t, err)
	rt := TestPytorchResourceTemplate.DeepCopy()

	replicaList, resourceList, err := GetResources(job, rt, "pytorch", common.AmdGpu)
	assert.NilError(t, err)
	assert.Equal(t, len(replicaList), 2)
	assert.Equal(t, len(resourceList), 2)

	assert.Equal(t, replicaList[0], int64(1))
	rl := resourceList[0]
	assert.Equal(t, rl.Cpu().Value(), int64(48))
	assert.Equal(t, rl.Memory().String(), "960Gi")
	assert.Equal(t, rl.StorageEphemeral().IsZero(), true)
	gpuQuantity, ok := rl[common.AmdGpu]
	assert.Equal(t, ok, true)
	assert.Equal(t, gpuQuantity.Value(), int64(8))

	assert.Equal(t, replicaList[1], int64(63))
	rl = resourceList[1]
	assert.Equal(t, rl.Cpu().Value(), int64(48))
	assert.Equal(t, rl.Memory().String(), "960Gi")
	assert.Equal(t, rl.StorageEphemeral().IsZero(), true)
	gpuQuantity, ok = rl[common.AmdGpu]
	assert.Equal(t, ok, true)
	assert.Equal(t, gpuQuantity.Value(), int64(8))
}

func TestGetPytorchJobMasterResource(t *testing.T) {
	job, err := jsonutils.ParseYamlToJson(TestPytorchData2)
	assert.NilError(t, err)
	rt := TestPytorchResourceTemplate.DeepCopy()

	replicaList, resourceList, err := GetResources(job, rt, "pytorch", common.AmdGpu)
	assert.NilError(t, err)
	assert.Equal(t, len(replicaList), 1)
	assert.Equal(t, len(resourceList), 1)

	assert.Equal(t, replicaList[0], int64(1))
	rl := resourceList[0]
	assert.Equal(t, rl.Cpu().Value(), int64(48))
	assert.Equal(t, rl.Memory().String(), "960Gi")
	assert.Equal(t, rl.StorageEphemeral().IsZero(), true)
	gpuQuantity, ok := rl[common.AmdGpu]
	assert.Equal(t, ok, true)
	assert.Equal(t, gpuQuantity.Value(), int64(8))
}

func TestGetStatefulSetPhase(t *testing.T) {
	statefulSet, err := jsonutils.ParseYamlToJson(TestStatefulSetData)
	assert.NilError(t, err)
	rt := TestStatefulSetResourceTemplate.DeepCopy()

	status, err := GetK8sObjectStatus(statefulSet, rt)
	assert.NilError(t, err)
	assert.Equal(t, status != nil, true)
	assert.Equal(t, status.SpecReplica, 2)
	assert.Equal(t, status.ActiveReplica, 2)
	assert.Equal(t, status.Phase, string(v1.K8sRunning))

	err = unstructured.SetNestedField(statefulSet.Object, "123", []string{"status", "currentRevision"}...)
	assert.NilError(t, err)
	status, err = GetK8sObjectStatus(statefulSet, rt)
	assert.NilError(t, err)
	assert.Equal(t, status != nil, true)
	assert.Equal(t, status.SpecReplica, 2)
	assert.Equal(t, status.ActiveReplica, 2)
	assert.Equal(t, status.Phase, string(v1.K8sUpdating))
}

func TestGetDeploymentEnv(t *testing.T) {
	deploy, err := jsonutils.ParseYamlToJson(TestDeploymentData)
	assert.NilError(t, err)
	rt := TestDeploymentResourceTemplate.DeepCopy()

	envs, err := GetEnv(deploy, rt, "test")
	assert.NilError(t, err)
	assert.Equal(t, len(envs), 2)
	env, ok := envs[0].(map[string]interface{})
	assert.Equal(t, ok, true)
	assert.Equal(t, env["name"].(string), "NCCL_SOCKET_IFNAME")
	assert.Equal(t, env["value"].(string), "eth0")

	env, ok = envs[1].(map[string]interface{})
	assert.Equal(t, ok, true)
	assert.Equal(t, env["name"].(string), "GLOO_SOCKET_IFNAME")
	assert.Equal(t, env["value"].(string), "eth0")
}

func TestGetPytorchJobPriorityClass(t *testing.T) {
	pytorchJob, err := jsonutils.ParseYamlToJson(TestPytorchData)
	assert.NilError(t, err)
	rt := TestPytorchResourceTemplate.DeepCopy()

	name, err := GetPriorityClassName(pytorchJob, rt)
	assert.NilError(t, err)
	assert.Equal(t, name, "test-med-priority")
}

func TestGetCICDEphemeralRunnerPhase(t *testing.T) {
	data, err := jsonutils.ParseYamlToJson(TestCICDEphemeralRunnerData)
	assert.NilError(t, err)
	rt := TestCICDRunnerResourceTemplate.DeepCopy()

	status, err := GetK8sObjectStatus(data, rt)
	assert.NilError(t, err)
	assert.Equal(t, status.Phase, string(v1.K8sRunning))

	newStatus := map[string]interface{}{
		"phase":   "Failed",
		"message": "Job has reached the specified backoff limit",
		"reason":  "BackoffLimitExceeded",
	}
	err = unstructured.SetNestedMap(data.Object, newStatus, "status")
	assert.NilError(t, err)

	status, err = GetK8sObjectStatus(data, rt)
	assert.NilError(t, err)
	assert.Equal(t, status.Phase, "K8sFailed")
	assert.Equal(t, status.Message, "Job has reached the specified backoff limit")
}

func TestGetGithubConfigSecret(t *testing.T) {
	runnerSetData, err := jsonutils.ParseYamlToJson(TestAutoscalingRunnerSetData)
	assert.NilError(t, err)

	val, err := GetGithubConfigSecret(runnerSetData)
	assert.NilError(t, err)
	assert.Equal(t, val, "primus-safe-cicd")
}

func TestNestedInt64(t *testing.T) {
	// Create an object with nested arrays
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": int64(3),
			"workerGroupSpecs": []interface{}{
				map[string]interface{}{
					"groupName":   "worker-group-0",
					"replicas":    int64(2),
					"minReplicas": int64(1),
					"maxReplicas": int64(5), // int type
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name": "container-0",
									"resources": map[string]interface{}{
										"limits": map[string]interface{}{
											"cpu": int64(4),
										},
									},
								},
								map[string]interface{}{
									"name": "container-1",
									"resources": map[string]interface{}{
										"limits": map[string]interface{}{
											"cpu": int64(8),
										},
									},
								},
							},
						},
					},
				},
				map[string]interface{}{
					"groupName": "worker-group-1",
					"replicas":  int64(4),
				},
			},
		},
	}

	tests := []struct {
		name      string
		path      []string
		expected  int64
		wantFound bool
		wantErr   bool
	}{
		{
			name:      "simple path without array",
			path:      []string{"spec", "replicas"},
			expected:  3,
			wantFound: true,
			wantErr:   false,
		},
		{
			name:      "path with array index - first element",
			path:      []string{"spec", "workerGroupSpecs", "0", "replicas"},
			expected:  2,
			wantFound: true,
			wantErr:   false,
		},
		{
			name:      "path with array index - second element",
			path:      []string{"spec", "workerGroupSpecs", "1", "replicas"},
			expected:  4,
			wantFound: true,
			wantErr:   false,
		},
		{
			name:      "nested array path - first container",
			path:      []string{"spec", "workerGroupSpecs", "0", "template", "spec", "containers", "0", "resources", "limits", "cpu"},
			expected:  4,
			wantFound: true,
			wantErr:   false,
		},
		{
			name:      "nested array path - second container",
			path:      []string{"spec", "workerGroupSpecs", "0", "template", "spec", "containers", "1", "resources", "limits", "cpu"},
			expected:  8,
			wantFound: true,
			wantErr:   false,
		},
		{
			name:      "path not found",
			path:      []string{"spec", "workerGroupSpecs", "0", "notExist"},
			expected:  0,
			wantFound: false,
			wantErr:   false,
		},
		{
			name:      "array index out of range",
			path:      []string{"spec", "workerGroupSpecs", "10", "replicas"},
			expected:  0,
			wantFound: false,
			wantErr:   true,
		},
		{
			name:      "empty path",
			path:      []string{},
			expected:  0,
			wantFound: false,
			wantErr:   true,
		},
		{
			name:      "invalid field type - string value",
			path:      []string{"spec", "workerGroupSpecs", "0", "groupName"},
			expected:  0,
			wantFound: true,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, found, err := NestedInt64(obj, tt.path)

			if tt.wantErr {
				assert.Assert(t, err != nil || !found, "expected error or not found")
			} else {
				assert.NilError(t, err)
			}

			assert.Equal(t, found, tt.wantFound)
			if tt.wantFound && !tt.wantErr {
				assert.Equal(t, result, tt.expected)
			}
		})
	}
}

func TestNestedMap(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": int64(3),
			"workerGroupSpecs": []interface{}{
				map[string]interface{}{
					"groupName": "worker-0",
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"app": "test",
							},
						},
					},
				},
			},
		},
	}

	result, found, err := NestedMap(obj, []string{"spec"})
	assert.NilError(t, err)
	assert.Assert(t, found)
	assert.Equal(t, result["replicas"].(int64), int64(3))

	// Test: path with array index
	result, found, err = NestedMap(obj, []string{"spec", "workerGroupSpecs", "0", "template", "metadata", "labels"})
	assert.NilError(t, err)
	assert.Assert(t, found)
	assert.Equal(t, result["app"], "test")

	// Test: path without array
	result, found, err = NestedMap(obj, []string{"spec", "workerGroupSpecs", "0", "template", "metadata"})
	assert.NilError(t, err)
	assert.Assert(t, found)
	assert.Assert(t, result["labels"] != nil)

	// Test: path not found
	_, found, err = NestedMap(obj, []string{"spec", "notExist"})
	assert.NilError(t, err)
	assert.Assert(t, !found)

	// Test: array index out of range
	_, found, err = NestedMap(obj, []string{"spec", "workerGroupSpecs", "10", "template"})
	assert.Assert(t, err != nil)
	assert.Assert(t, !found)
}

func TestNestedSlice(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"volumes": []interface{}{
				map[string]interface{}{"name": "vol1"},
				map[string]interface{}{"name": "vol2"},
			},
			"workerGroupSpecs": []interface{}{
				map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{"name": "c1"},
								map[string]interface{}{"name": "c2"},
							},
						},
					},
				},
			},
		},
	}

	// Test: simple path without array index
	result, found, err := NestedSlice(obj, []string{"spec", "volumes"})
	assert.NilError(t, err)
	assert.Assert(t, found)
	assert.Equal(t, len(result), 2)

	// Test: path with array index
	result, found, err = NestedSlice(obj, []string{"spec", "workerGroupSpecs", "0", "template", "spec", "containers"})
	assert.NilError(t, err)
	assert.Assert(t, found)
	assert.Equal(t, len(result), 2)

	// Test: path not found
	_, found, err = NestedSlice(obj, []string{"spec", "notExist"})
	assert.NilError(t, err)
	assert.Assert(t, !found)

	// Test: array index out of range
	_, found, err = NestedSlice(obj, []string{"spec", "workerGroupSpecs", "10", "template", "spec", "containers"})
	assert.Assert(t, err != nil)
	assert.Assert(t, !found)
}

func TestRemoveNestedField(t *testing.T) {
	// Test: remove field with array index
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"workerGroupSpecs": []interface{}{
				map[string]interface{}{
					"groupName": "worker-0",
					"replicas":  int64(2),
				},
			},
		},
	}

	err := RemoveNestedField(obj, []string{"spec", "workerGroupSpecs", "0", "replicas"})
	assert.NilError(t, err)
	_, found, _ := NestedInt64(obj, []string{"spec", "workerGroupSpecs", "0", "replicas"})
	assert.Assert(t, !found)

	// Test: remove simple field
	obj2 := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      "test",
			"namespace": "default",
		},
	}

	err = RemoveNestedField(obj2, []string{"metadata", "namespace"})
	assert.NilError(t, err)
	_, found, _ = unstructured.NestedString(obj2, "metadata", "namespace")
	assert.Assert(t, !found)

	// Test: remove array element
	obj3 := map[string]interface{}{
		"items": []interface{}{"a", "b", "c"},
	}

	err = RemoveNestedField(obj3, []string{"items", "1"})
	assert.NilError(t, err)
	arr, _, _ := unstructured.NestedSlice(obj3, "items")
	assert.Equal(t, len(arr), 2)

	// Test: remove non-existent field (no error)
	err = RemoveNestedField(obj, []string{"spec", "notExist"})
	assert.NilError(t, err)
}

func TestSetNestedField(t *testing.T) {
	// Test: set field with array index
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"workerGroupSpecs": []interface{}{
				map[string]interface{}{
					"groupName": "worker-0",
					"replicas":  int64(2),
				},
			},
		},
	}

	err := SetNestedField(obj, int64(5), []string{"spec", "workerGroupSpecs", "0", "replicas"})
	assert.NilError(t, err)
	result, found, _ := NestedInt64(obj, []string{"spec", "workerGroupSpecs", "0", "replicas"})
	assert.Assert(t, found)
	assert.Equal(t, result, int64(5))

	// Test: set simple field
	err = SetNestedField(obj, "new-name", []string{"spec", "workerGroupSpecs", "0", "groupName"})
	assert.NilError(t, err)
	val, found, _ := NestedField(obj, []string{"spec", "workerGroupSpecs", "0", "groupName"})
	assert.Assert(t, found)
	assert.Equal(t, val.(string), "new-name")

	// Test: set slice value
	volumes := []interface{}{
		map[string]interface{}{"name": "vol1"},
	}
	err = SetNestedField(obj, volumes, []string{"spec", "workerGroupSpecs", "0", "volumes"})
	assert.NilError(t, err)
	arr, found, _ := NestedSlice(obj, []string{"spec", "workerGroupSpecs", "0", "volumes"})
	assert.Assert(t, found)
	assert.Equal(t, len(arr), 1)

	// Test: array index out of range
	err = SetNestedField(obj, "test", []string{"spec", "workerGroupSpecs", "10", "name"})
	assert.Assert(t, err != nil)
}
