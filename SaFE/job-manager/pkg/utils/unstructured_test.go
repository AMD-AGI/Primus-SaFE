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

	status, err := GetK8sResourceStatus(pytorchJob, rt)
	assert.NilError(t, err)
	assert.Equal(t, status.Phase, "")

	newCondition := map[string]interface{}{
		"type":    "Running",
		"status":  "False",
		"reason":  "JobRunning",
		"message": "job is running",
	}
	addK8sJobCond(t, pytorchJob, newCondition)
	status, err = GetK8sResourceStatus(pytorchJob, rt)
	assert.NilError(t, err)
	assert.Equal(t, status.Phase, "")

	newCondition["status"] = "True"
	addK8sJobCond(t, pytorchJob, newCondition)
	status, err = GetK8sResourceStatus(pytorchJob, rt)
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
	status, err = GetK8sResourceStatus(pytorchJob, rt)
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
	rt := TestJobTemplate.DeepCopy()

	status, err := GetK8sResourceStatus(job, rt)
	assert.NilError(t, err)
	assert.Equal(t, status.Phase, string(v1.K8sRunning))

	newCondition := map[string]interface{}{
		"type":    "Failed",
		"status":  "True",
		"reason":  "BackoffLimitExceeded",
		"message": "Job has reached the specified backoff limit",
	}
	addK8sJobCond(t, job, newCondition)
	status, err = GetK8sResourceStatus(job, rt)
	assert.NilError(t, err)
	assert.Equal(t, status.Phase, "K8sFailed")
}

func TestGetJobActiveReplica(t *testing.T) {
	job, err := jsonutils.ParseYamlToJson(TestJobData)
	assert.NilError(t, err)
	rt := TestJobTemplate.DeepCopy()

	replica, err := GetActiveReplica(job, rt)
	assert.NilError(t, err)
	assert.Equal(t, replica, 2)
}

func TestGetDeploymentPhase(t *testing.T) {
	deploy, err := jsonutils.ParseYamlToJson(TestDeploymentData)
	assert.NilError(t, err)
	rt := TestDeploymentTemplate.DeepCopy()

	status, err := GetK8sResourceStatus(deploy, rt)
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
	status, err = GetK8sResourceStatus(deploy, rt)
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
	status, err = GetK8sResourceStatus(deploy, rt)
	assert.NilError(t, err)
	assert.Equal(t, status != nil, true)
	assert.Equal(t, status.Phase, string(v1.K8sRunning))
}

func TestGetDeploymentResources(t *testing.T) {
	deploy, err := jsonutils.ParseYamlToJson(TestDeploymentData)
	assert.NilError(t, err)
	rt := TestDeploymentTemplate.DeepCopy()

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
	rt := TestDeploymentTemplate.DeepCopy()

	image, err := GetImage(deploy, rt, "test")
	assert.NilError(t, err)
	assert.Equal(t, image, "test-image:latest")
}

func TestGetDeploymentCommand(t *testing.T) {
	deploy, err := jsonutils.ParseYamlToJson(TestDeploymentData)
	assert.NilError(t, err)
	rt := TestDeploymentTemplate.DeepCopy()

	commands, err := GetCommand(deploy, rt, "test")
	assert.NilError(t, err)
	assert.Equal(t, len(commands), 3)
	assert.Equal(t, commands[0], "sh")
	assert.Equal(t, commands[1], "c")
	assert.Equal(t, commands[2], "/bin/sh run.sh 'abcd'")
}

func TestGetDeploymentShareMemorySize(t *testing.T) {
	deploy, err := jsonutils.ParseYamlToJson(TestDeploymentData)
	assert.NilError(t, err)
	rt := TestDeploymentTemplate.DeepCopy()

	memoryStorageSize, err := GetMemoryStorageSize(deploy, rt)
	assert.NilError(t, err)
	assert.Equal(t, memoryStorageSize, "20Gi")
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
	rt := TestStatefulSetTemplate.DeepCopy()

	status, err := GetK8sResourceStatus(statefulSet, rt)
	assert.NilError(t, err)
	assert.Equal(t, status != nil, true)
	assert.Equal(t, status.SpecReplica, 2)
	assert.Equal(t, status.ActiveReplica, 2)
	assert.Equal(t, status.Phase, string(v1.K8sRunning))

	err = unstructured.SetNestedField(statefulSet.Object, "123", []string{"status", "currentRevision"}...)
	assert.NilError(t, err)
	status, err = GetK8sResourceStatus(statefulSet, rt)
	assert.NilError(t, err)
	assert.Equal(t, status != nil, true)
	assert.Equal(t, status.SpecReplica, 2)
	assert.Equal(t, status.ActiveReplica, 2)
	assert.Equal(t, status.Phase, string(v1.K8sUpdating))
}

func TestGetDeploymentEnv(t *testing.T) {
	deploy, err := jsonutils.ParseYamlToJson(TestDeploymentData)
	assert.NilError(t, err)
	rt := TestDeploymentTemplate.DeepCopy()

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
	rt := TestCICDEphemeralRunnerTemplate.DeepCopy()

	status, err := GetK8sResourceStatus(data, rt)
	assert.NilError(t, err)
	assert.Equal(t, status.Phase, string(v1.K8sRunning))

	newStatus := map[string]interface{}{
		"phase":   "Failed",
		"message": "Job has reached the specified backoff limit",
		"reason":  "BackoffLimitExceeded",
	}
	err = unstructured.SetNestedMap(data.Object, newStatus, "status")
	assert.NilError(t, err)

	status, err = GetK8sResourceStatus(data, rt)
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
