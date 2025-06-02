/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"strconv"
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

func checkResources(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, template *v1.Template, replica int) {
	path := append(template.PrePaths, template.ReplicasPaths...)
	objReplica := jobutils.GetUnstructuredInt(obj.Object, path)
	assert.Equal(t, objReplica, int64(replica))

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
			assert.Equal(t, limits[common.Rdma], "1")
		}
	}
}

func checkPorts(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, template *v1.Template) {
	containerPath := append(template.PrePaths, template.TemplatePaths...)
	containerPath = append(containerPath, "spec", "containers")

	values, found, err := unstructured.NestedSlice(obj.Object, containerPath...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(values) == 0, false)
	obj2, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&values[0])
	assert.NilError(t, err)

	ports, found, err := unstructured.NestedSlice(obj2, []string{"ports"}...)
	assert.NilError(t, err)
	if workload.Spec.IsSSHEnabled {
		assert.Equal(t, len(ports), 2)
	} else {
		assert.Equal(t, len(ports), 1)
	}

	port := ports[0].(map[string]interface{})
	name, ok := port["name"]
	assert.Equal(t, ok, true)
	assert.Equal(t, name, common.PytorchJobPortName)
	val, ok := port["containerPort"]
	assert.Equal(t, ok, true)
	assert.Equal(t, val, int64(workload.Spec.Resource.JobPort))

	if workload.Spec.IsSSHEnabled {
		port = ports[1].(map[string]interface{})
		name, ok = port["name"]
		assert.Equal(t, ok, true)
		assert.Equal(t, name, common.SSHPortName)
		val, ok = port["containerPort"]
		assert.Equal(t, ok, true)
		assert.Equal(t, val, int64(workload.Spec.Resource.SSHPort))
	}
}

func checkEnvs(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, template *v1.Template) {
	containerPath := append(template.PrePaths, template.TemplatePaths...)
	containerPath = append(containerPath, "spec", "containers")

	values, found, err := unstructured.NestedSlice(obj.Object, containerPath...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(values) == 0, false)

	obj2, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&values[0])
	assert.NilError(t, err)
	envs, found, err := unstructured.NestedSlice(obj2, []string{"env"}...)
	assert.NilError(t, err)

	for key, val := range workload.Spec.Env {
		ok := findEnv(envs, key, val)
		assert.Equal(t, ok, true)
	}
	gpu := workload.Spec.Resource.GPU
	if gpu != "" {
		ok := findEnv(envs, "GPUS_PER_NODE", gpu)
		assert.Equal(t, ok, true)
		ok = findEnv(envs, "NUM_HOSTS", strconv.Itoa(workload.Spec.Resource.Replica))
		assert.Equal(t, ok, true)
	}
	if workload.Spec.IsSSHEnabled {
		ok := findEnv(envs, "SSH_PORT", strconv.Itoa(workload.Spec.Resource.SSHPort))
		assert.Equal(t, ok, true)
	}
	ok := findEnv(envs, "HANG_CHECK_INTERVAL", "")
	assert.Equal(t, ok, false)
	ok = findEnv(envs, "DISPATCH_COUNT", "1")
	assert.Equal(t, ok, true)
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

func checkVolumeMounts(t *testing.T, obj *unstructured.Unstructured, template *v1.Template) {
	containerPath := append(template.PrePaths, template.TemplatePaths...)
	containerPath = append(containerPath, "spec", "containers")

	values, found, err := unstructured.NestedSlice(obj.Object, containerPath...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(values) == 0, false)
	obj2, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&values[0])
	assert.NilError(t, err)

	volumeMounts, found, err := unstructured.NestedSlice(obj2, []string{"volumeMounts"}...)
	assert.NilError(t, err)

	volumeMount := findVolumeMount(volumeMounts, ShareMemoryVolumeName)
	assert.Equal(t, volumeMount != nil, true)
	volumeMount = findVolumeMount(volumeMounts, string(v1.FS))
	assert.Equal(t, volumeMount != nil, true)
	path, ok := volumeMount["mountPath"]
	assert.Equal(t, ok, true)
	assert.Equal(t, path, "/ceph")
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

func checkVolumes(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, template *v1.Template) {
	volumesPath := append(template.PrePaths, template.TemplatePaths...)
	volumesPath = append(volumesPath, "spec", "volumes")

	volumes, found, err := unstructured.NestedSlice(obj.Object, volumesPath...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)

	volume := findVolume(volumes, ShareMemoryVolumeName)
	assert.Equal(t, volume != nil, true)
	emptyDir, ok := volume["emptyDir"]
	assert.Equal(t, ok, true)
	sizeLimit, ok := emptyDir.(map[string]interface{})["sizeLimit"]
	assert.Equal(t, ok, true)
	assert.Equal(t, sizeLimit.(string), workload.Spec.Resource.ShareMemory)

	volume = findVolume(volumes, string(v1.FS))
	assert.Equal(t, volume != nil, true)
	persistentVolumeClaim, ok := volume["persistentVolumeClaim"]
	assert.Equal(t, ok, true)
	claimName, ok := persistentVolumeClaim.(map[string]interface{})["claimName"]
	assert.Equal(t, ok, true)
	assert.Equal(t, claimName.(string), string(v1.FS))
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

func checkNodeSelectorTerms(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, template *v1.Template) {
	nodeSelectorPath := append(template.PrePaths, template.TemplatePaths...)
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
	assert.Equal(t, len(matchExpressionsSlice), 3)

	matchExpression := matchExpressionsSlice[0].(map[string]interface{})
	key, ok := matchExpression["key"]
	assert.Equal(t, ok, true)
	assert.Equal(t, key, v1.WorkspaceIdLabel)
	values, ok := matchExpression["values"]
	assert.Equal(t, ok, true)
	valuesSlice := values.([]interface{})
	assert.Equal(t, len(valuesSlice), 1)
	assert.Equal(t, valuesSlice[0].(string), workload.Spec.Workspace)

	matchExpression = matchExpressionsSlice[1].(map[string]interface{})
	key, ok = matchExpression["key"]
	assert.Equal(t, ok, true)
	assert.Equal(t, key == "key1" || key == "key2", true)
	values, ok = matchExpression["values"]
	assert.Equal(t, ok, true)
	valuesSlice = values.([]interface{})
	assert.Equal(t, len(valuesSlice), 1)
	assert.Equal(t, valuesSlice[0].(string) == "val1" || valuesSlice[0].(string) == "val2", true)
}

func checkImage(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, template *v1.Template) {
	containerPath := append(template.PrePaths, template.TemplatePaths...)
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
	assert.Equal(t, image, workload.Spec.Image)
}

func checkHostNetwork(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, template *v1.Template) {
	path := append(template.PrePaths, template.TemplatePaths...)
	path = append(path, "spec", "hostNetwork")

	isHostNetWork, found, err := unstructured.NestedBool(obj.Object, path...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, isHostNetWork, v1.IsEnableHostNetwork(workload))
}

func checkLabels(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, template *v1.Template) {
	path := append(template.PrePaths, template.TemplatePaths...)
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
