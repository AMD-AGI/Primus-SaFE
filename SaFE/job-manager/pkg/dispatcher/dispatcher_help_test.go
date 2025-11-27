/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

func checkResources(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, template *v1.ResourceSpec, replica int) {
	path := append(template.PrePaths, template.ReplicasPaths...)
	if !commonworkload.IsCICDScalingRunnerSet(workload) {
		objReplica := jobutils.GetUnstructuredInt(obj.Object, path)
		assert.Equal(t, objReplica, int64(replica))
		if workload.SpecKind() == common.JobKind {
			path = append(template.PrePaths, "completions")
			objReplica = jobutils.GetUnstructuredInt(obj.Object, path)
			assert.Equal(t, objReplica, int64(replica))
		}
	}

	path = append(template.PrePaths, template.TemplatePaths...)
	path = append(path, "spec", "containers")
	containers, found, err := unstructured.NestedSlice(obj.Object, path...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	container := containers[0].(map[string]interface{})
	resources := container["resources"].(map[string]interface{})
	limits, ok := resources["limits"].(map[string]interface{})
	assert.Equal(t, ok, true)
	assert.Equal(t, limits["cpu"], workload.Spec.Resource.CPU)
	assert.Equal(t, limits["memory"], workload.Spec.Resource.Memory)
	assert.Equal(t, limits["ephemeral-storage"], workload.Spec.Resource.EphemeralStorage)
	if workload.Spec.Resource.GPU != "" {
		assert.Equal(t, limits[common.AmdGpu], workload.Spec.Resource.GPU)
		if replica > 1 {
			assert.Equal(t, limits[commonconfig.GetRdmaName()], "1k")
		}
	}
}

func checkPorts(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, template *v1.ResourceSpec) {
	containerPath := append(template.PrePaths, template.TemplatePaths...)
	containerPath = append(containerPath, "spec", "containers")

	values, found, err := unstructured.NestedSlice(obj.Object, containerPath...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(values) == 0, false)
	mainContainer, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&values[0])
	assert.NilError(t, err)

	ports, found, err := unstructured.NestedSlice(mainContainer, []string{"ports"}...)
	assert.NilError(t, err)
	assert.Equal(t, len(ports), 2)

	port := ports[0].(map[string]interface{})
	name, ok := port["name"]
	if workload.SpecKind() == common.PytorchJobKind {
		assert.Equal(t, ok, true)
		assert.Equal(t, name, common.PytorchJobPortName)
	}
	val, ok := port["containerPort"]
	assert.Equal(t, ok, true)
	assert.Equal(t, val, int64(workload.Spec.JobPort))

	port = ports[1].(map[string]interface{})
	name, ok = port["name"]
	assert.Equal(t, name, common.SSHPortName)
	val, ok = port["containerPort"]
	assert.Equal(t, ok, true)
	assert.Equal(t, val, int64(workload.Spec.SSHPort))
}

func checkEnvs(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) {
	envs := getEnvs(t, obj, resourceSpec)
	for key, val := range workload.Spec.Env {
		ok := findEnv(envs, key, val)
		assert.Equal(t, ok, true)
	}
	gpu := workload.Spec.Resource.GPU
	if gpu != "" {
		ok := findEnv(envs, "GPUS_PER_NODE", gpu)
		assert.Equal(t, ok, true)
	}
	ok := findEnv(envs, "HANG_CHECK_INTERVAL", "")
	assert.Equal(t, ok, false)

	if workload.SpecKind() != common.JobKind && !commonworkload.IsCICDScalingRunnerSet(workload) {
		if v1.IsEnableHostNetwork(workload) {
			ok = findEnv(envs, "NCCL_SOCKET_IFNAME", "ens51f0")
			assert.Equal(t, ok, true)
		} else {
			ok = findEnv(envs, "NCCL_SOCKET_IFNAME", "eth0")
			assert.Equal(t, ok, true)
		}
	}
}

func findEnv(envs []interface{}, name, val string) bool {
	for _, env := range envs {
		obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&env)
		if err != nil {
			continue
		}
		name2, ok := obj["name"]
		if !ok {
			continue
		}
		if name != name2 {
			continue
		}
		val2, ok := obj["value"]
		if val != val2 {
			continue
		}
		return true
	}
	return false
}

func checkVolumeMounts(t *testing.T, obj *unstructured.Unstructured, resourceSpec *v1.ResourceSpec) {
	containerPath := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	containerPath = append(containerPath, "spec", "containers")

	values, found, err := unstructured.NestedSlice(obj.Object, containerPath...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(values) == 0, false)
	mainContainer, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&values[0])
	assert.NilError(t, err)

	volumeMounts, found, err := unstructured.NestedSlice(mainContainer, []string{"volumeMounts"}...)
	assert.NilError(t, err)

	if obj.GetKind() == common.PytorchJobKind {
		volumeMount := findVolumeMount(volumeMounts, SharedMemoryVolume)
		assert.Equal(t, volumeMount != nil, true)
	}

	volumeMount := findVolumeMount(volumeMounts, v1.GenFullVolumeId(v1.PFS, 1))
	assert.Equal(t, volumeMount != nil, true)
	path, ok := volumeMount["mountPath"]
	assert.Equal(t, ok, true)
	assert.Equal(t, path, "/ceph")
	_, ok = volumeMount["subPath"]
	assert.Equal(t, ok, false)

	volumeMount = findVolumeMount(volumeMounts, v1.GenFullVolumeId(v1.HOSTPATH, 2))
	assert.Equal(t, volumeMount != nil, true)
	path, ok = volumeMount["mountPath"]
	assert.Equal(t, ok, true)
	assert.Equal(t, path, "/data")
	_, ok = volumeMount["subPath"]
	assert.Equal(t, ok, false)
}

func findVolumeMount(volumeMounts []interface{}, name string) map[string]interface{} {
	for i := range volumeMounts {
		volumeMount := volumeMounts[i].(map[string]interface{})
		name2, ok := volumeMount["name"]
		if !ok {
			continue
		}
		if name == name2 {
			return volumeMount
		}
	}
	return nil
}

func checkVolumes(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) {
	volumesPath := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	volumesPath = append(volumesPath, "spec", "volumes")

	volumes, found, err := unstructured.NestedSlice(obj.Object, volumesPath...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)

	if workload.SpecKind() == common.PytorchJobKind {
		volume := findVolume(volumes, SharedMemoryVolume)
		assert.Equal(t, volume != nil, true)
		emptyDir, ok := volume["emptyDir"]
		assert.Equal(t, ok, true)
		sizeLimit, ok := emptyDir.(map[string]interface{})["sizeLimit"]
		assert.Equal(t, ok, true)
		assert.Equal(t, sizeLimit.(string), workload.Spec.Resource.SharedMemory)
	}

	volumeName := v1.GenFullVolumeId(v1.PFS, 1)
	volume := findVolume(volumes, volumeName)
	assert.Equal(t, volume != nil, true)
	persistentVolumeObj, ok := volume["persistentVolumeClaim"]
	assert.Equal(t, ok, true)
	claimName, ok := persistentVolumeObj.(map[string]interface{})["claimName"]
	assert.Equal(t, ok, true)
	assert.Equal(t, claimName.(string), volumeName)

	volume = findVolume(volumes, v1.GenFullVolumeId(v1.HOSTPATH, 2))
	assert.Equal(t, volume != nil, true)
	hostPathObj, ok := volume["hostPath"]
	assert.Equal(t, ok, true)
	path, ok := hostPathObj.(map[string]interface{})["path"]
	assert.Equal(t, ok, true)
	assert.Equal(t, path.(string), "/apps")
}

func findVolume(volumes []interface{}, name string) map[string]interface{} {
	for i := range volumes {
		volume := volumes[i].(map[string]interface{})
		name2, ok := volume["name"]
		if !ok {
			continue
		}
		if name == name2 {
			return volume
		}
	}
	return nil
}

func checkNodeSelectorTerms(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) {
	nodeSelectorPath := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	nodeSelectorPath = append(nodeSelectorPath, "spec", "affinity", "nodeAffinity",
		"requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms")

	affinities, found, err := unstructured.NestedSlice(obj.Object, nodeSelectorPath...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(affinities), 1)
	affinity := affinities[0].(map[string]interface{})
	matchExpressionObj, ok := affinity["matchExpressions"]
	assert.Equal(t, ok, true)
	matchExpressionsSlice := matchExpressionObj.([]interface{})
	totalExpressions := len(workload.Spec.CustomerLabels)
	if workload.Spec.Workspace != "" && workload.Spec.Workspace != corev1.NamespaceDefault {
		totalExpressions++
	}
	assert.Equal(t, len(matchExpressionsSlice), totalExpressions)
	if totalExpressions == 0 {
		return
	}

	if workload.Spec.Workspace != "" && workload.Spec.Workspace != corev1.NamespaceDefault {
		matchExpression := matchExpressionsSlice[0].(map[string]interface{})
		key, ok := matchExpression["key"]
		assert.Equal(t, ok, true)
		assert.Equal(t, key, v1.WorkspaceIdLabel)
		values, ok := matchExpression["values"]
		assert.Equal(t, ok, true)
		valuesSlice := values.([]interface{})
		assert.Equal(t, len(valuesSlice), 1)
		assert.Equal(t, valuesSlice[0].(string), workload.Spec.Workspace)
	}

	matchExpression := matchExpressionsSlice[totalExpressions-1].(map[string]interface{})
	key, ok := matchExpression["key"]
	assert.Equal(t, ok, true)
	val, ok := workload.Spec.CustomerLabels[key.(string)]
	assert.Equal(t, ok, true)
	values, ok := matchExpression["values"]
	assert.Equal(t, ok, true)
	valuesSlice := values.([]interface{})
	assert.Equal(t, len(valuesSlice), 1)
	assert.Equal(t, valuesSlice[0].(string) == val, true)
}

func checkImage(t *testing.T, obj *unstructured.Unstructured, inputImage string, resourceSpec *v1.ResourceSpec) {
	containerPath := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	containerPath = append(containerPath, "spec", "containers")

	values, found, err := unstructured.NestedSlice(obj.Object, containerPath...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(values) == 0, false)
	mainContainer, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&values[0])
	assert.NilError(t, err)
	image, found, err := unstructured.NestedString(mainContainer, []string{"image"}...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, image, inputImage)
}

func checkHostNetwork(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) {
	path := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	path = append(path, "spec", "hostNetwork")

	isHostNetWork, found, err := unstructured.NestedBool(obj.Object, path...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, isHostNetWork, v1.IsEnableHostNetwork(workload))
}

func checkHostPid(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) {
	path := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	path = append(path, "spec", "hostPID")

	resp, found, err := unstructured.NestedBool(obj.Object, path...)
	assert.NilError(t, err)
	if v1.GetOpsJobType(workload) == string(v1.OpsJobPreflightType) {
		assert.Equal(t, found, true)
		assert.Equal(t, resp, true)
	} else {
		assert.Equal(t, found, false)
	}
}

func checkLabels(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) {
	path := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	path = append(path, "metadata", "labels")

	labels, found, err := unstructured.NestedMap(obj.Object, path...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, labels[v1.WorkloadDispatchCntLabel].(string), "1")
	assert.Equal(t, labels[v1.WorkloadIdLabel].(string), workload.Name)
}

func checkSelector(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload) {
	path := []string{"spec", "selector", "matchLabels"}
	labels, found, err := unstructured.NestedMap(obj.Object, path...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, labels[v1.WorkloadIdLabel].(string), workload.Name)
}

func checkStrategy(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload) {
	path := []string{"spec", "strategy", "rollingUpdate"}
	labels, found, err := unstructured.NestedMap(obj.Object, path...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, labels["maxSurge"].(string), workload.Spec.Service.Extends["maxSurge"])
	assert.Equal(t, labels["maxUnavailable"].(string), workload.Spec.Service.Extends["maxUnavailable"])
}

func checkTolerations(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) {
	path := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	path = append(path, "spec", "tolerations")

	tolerations, found, err := unstructured.NestedSlice(obj.Object, path...)
	assert.NilError(t, err)
	if workload.Spec.IsTolerateAll {
		assert.Equal(t, found, true)
		assert.Equal(t, len(tolerations), 1)
		toleration := tolerations[0].(map[string]interface{})
		assert.Equal(t, len(toleration), 1)
		op, ok := toleration["operator"]
		assert.Equal(t, ok, true)
		assert.Equal(t, op, "Exists")
	} else {
		assert.Equal(t, found, false)
	}
}

func checkPriorityClass(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) {
	path := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	path = append(path, "spec", "priorityClassName")
	priorityClassName, found, err := unstructured.NestedString(obj.Object, path...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, priorityClassName, commonworkload.GeneratePriorityClass(workload))
}

func checkSecurityContext(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, template *v1.ResourceSpec) {
	containerPath := append(template.PrePaths, template.TemplatePaths...)
	containerPath = append(containerPath, "spec", "containers")

	values, found, err := unstructured.NestedSlice(obj.Object, containerPath...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(values) == 0, false)
	mainContainer, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&values[0])
	assert.NilError(t, err)

	securityContext, found, err := unstructured.NestedMap(mainContainer, []string{"securityContext"}...)
	assert.NilError(t, err)
	privileged, ok := securityContext["privileged"]
	if v1.GetOpsJobType(workload) == string(v1.OpsJobPreflightType) || commonworkload.IsCICD(workload) {
		assert.Equal(t, ok, true)
		assert.Equal(t, privileged.(bool), true)
		_, ok := securityContext["capabilities"]
		assert.Equal(t, ok, false)
	} else {
		assert.Equal(t, ok, false)
		obj2, ok := securityContext["capabilities"]
		assert.Equal(t, ok, true)
		capabilities, ok := obj2.(map[string]interface{})
		assert.Equal(t, ok, true)
		obj2, ok = capabilities["add"]
		assert.Equal(t, ok, true)
		add, ok := obj2.([]interface{})
		assert.Equal(t, len(add) > 0, true)
	}
}

func checkImageSecrets(t *testing.T, obj *unstructured.Unstructured, resourceSpec *v1.ResourceSpec) {
	secretPath := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	secretPath = append(secretPath, "spec", "imagePullSecrets")

	secrets, found, err := unstructured.NestedSlice(obj.Object, secretPath...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(secrets), 1)

	secret := secrets[0]
	assert.Equal(t, secret != nil, true)
	name, ok := secret.(map[string]interface{})["name"]
	assert.Equal(t, ok, true)
	assert.Equal(t, name, "test-image")
}

func getEnvs(t *testing.T, obj *unstructured.Unstructured, resourceSpec *v1.ResourceSpec) []interface{} {
	containerPath := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	containerPath = append(containerPath, "spec", "containers")

	values, found, err := unstructured.NestedSlice(obj.Object, containerPath...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(values) == 0, false)

	mainContainer, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&values[0])
	assert.NilError(t, err)
	envs, found, err := unstructured.NestedSlice(mainContainer, []string{"env"}...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	return envs
}

func getContainer(obj *unstructured.Unstructured, name string, resourceSpec *v1.ResourceSpec) map[string]interface{} {
	containerPath := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	containerPath = append(containerPath, "spec", "containers")

	values, found, err := unstructured.NestedSlice(obj.Object, containerPath...)
	if err != nil || !found {
		return nil
	}

	for _, val := range values {
		container, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&val)
		if err != nil {
			return nil
		}
		_, ok := container["name"]
		if ok {
			if container["name"] == name {
				return container
			}
		}
	}
	return nil
}
