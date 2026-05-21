/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"fmt"
	"strconv"
	"testing"

	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

func checkResources(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, template *v1.ResourceSpec, replica, id int) {
	path := append(template.PrePaths, template.ReplicasPaths...)
	if replica > 0 && len(template.ReplicasPaths) > 0 {
		objReplica, found, err := jobutils.NestedInt64(obj.Object, path)
		assert.Equal(t, found, true)
		assert.NilError(t, err)
		assert.Equal(t, objReplica, int64(replica))
		if workload.SpecKind() == common.JobKind {
			path = append(template.PrePaths, template.MinReplicasPaths...)
			objReplica, found, err = jobutils.NestedInt64(obj.Object, path)
			assert.Equal(t, found, true)
			assert.NilError(t, err)
			assert.Equal(t, objReplica, int64(replica))
		} else if commonworkload.IsRayJob(workload) {
			path = append(template.PrePaths, template.MinReplicasPaths...)
			objReplica, found, err = jobutils.NestedInt64(obj.Object, path)
			assert.Equal(t, found, true)
			assert.Equal(t, objReplica, int64(replica))

			path = append(template.PrePaths, template.MaxReplicasPaths...)
			objReplica, found, err = jobutils.NestedInt64(obj.Object, path)
			assert.Equal(t, found, true)
			assert.Equal(t, objReplica, int64(replica))
		}
	}

	path = append(template.PrePaths, template.TemplatePaths...)
	podSpec := getPodSpec(workload)
	path = append(path, podSpec, "containers")
	containers, found, err := jobutils.NestedSlice(obj.Object, path)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	container := containers[0].(map[string]interface{})
	resources := container["resources"].(map[string]interface{})
	limits, ok := resources["limits"].(map[string]interface{})
	assert.Equal(t, ok, true)
	assert.Equal(t, limits["cpu"], workload.Spec.Resources[id].CPU)
	assert.Equal(t, limits["memory"], workload.Spec.Resources[id].Memory)
	assert.Equal(t, limits["ephemeral-storage"], workload.Spec.Resources[id].EphemeralStorage)
	if workload.Spec.Resources[id].GPU != "" {
		assert.Equal(t, limits[common.AmdGpu], workload.Spec.Resources[id].GPU)
		if replica > 1 {
			assert.Equal(t, limits[commonconfig.GetRdmaName()], "1k")
		}
	}
}

func checkRaySubmitterPod(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload) {
	podSpec := getPodSpec(workload)
	val, found, err := jobutils.NestedString(obj.Object, []string{podSpec, "entrypoint"})
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, val, "exec "+Launcher+fmt.Sprintf(" '%s'", workload.GetEnv(common.RayJobEntrypoint)))

	path := []string{podSpec, "submitterPodTemplate"}
	path = append(path, podSpec, "containers")
	containers, found, err := jobutils.NestedSlice(obj.Object, path)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(containers), 1)

	container := containers[0].(map[string]interface{})
	resources := container["resources"].(map[string]interface{})
	limits, ok := resources["limits"].(map[string]interface{})
	assert.Equal(t, ok, true)
	assert.Equal(t, limits["cpu"].(string), common.RayJobSubmitterCpu)
	assert.Equal(t, limits["memory"].(string), common.RayJobSubmitterMemory)
}

func checkPorts(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, template *v1.ResourceSpec, id int) {
	containerPath := append(template.PrePaths, template.TemplatePaths...)
	podPec := getPodSpec(workload)
	containerPath = append(containerPath, podPec, "containers")

	values, found, err := jobutils.NestedSlice(obj.Object, containerPath)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(values) == 0, false)
	mainContainer, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&values[0])
	assert.NilError(t, err)
	ports, found, err := jobutils.NestedSlice(mainContainer, []string{"ports"})
	assert.NilError(t, err)

	if workload.SpecKind() == common.PytorchJobKind {
		assert.Equal(t, len(ports) >= 1, true)
		portName := common.PytorchJobPortName
		findPort(t, ports, portName, int64(workload.Spec.JobPort))
	}

	if commonworkload.IsRayJob(workload) && id == 1 {
		assert.Equal(t, len(ports) >= 2, true)
		findPort(t, ports, "gcs-server", 6379)
		findPort(t, ports, "dashboard", 8265)
	}
	if commonworkload.IsMonarchMesh(workload) {
		n, found, err := jobutils.NestedInt64(obj.Object, []string{"spec", "port"})
		assert.NilError(t, err)
		assert.Equal(t, found, true)
		assert.Equal(t, n, int64(common.MonarchMeshPortNum))
	}
}

func findPort(t *testing.T, ports []interface{}, portName string, portValue int64) {
	hasFound := false
	for _, p := range ports {
		port := p.(map[string]interface{})
		val, ok := port["containerPort"]
		assert.Equal(t, ok, true)
		if val == portValue {
			hasFound = true
			name, ok := port["name"]
			if ok && portName != "" {
				assert.Equal(t, name, portName)
			}
			break
		}
	}
	assert.Equal(t, hasFound, true)
}

func checkEnvs(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec, id int) {
	envs := getEnvs(t, obj, workload, resourceSpec)
	for key, val := range workload.Spec.Env {
		ok := findEnv(envs, key, val)
		assert.Equal(t, ok, true)
	}
	gpu := workload.Spec.Resources[id].GPU
	if gpu != "" {
		ok := findEnv(envs, "GPUS_PER_NODE", gpu)
		assert.Equal(t, ok, true)
	}
	ok := findEnv(envs, "HANG_CHECK_INTERVAL", "")
	assert.Equal(t, ok, false)
	ok = findEnv(envs, "WORKLOAD_ID", workload.Name)
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

func checkVolumeMounts(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) {
	templatePath := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	podSpec := getPodSpec(workload)
	containerPath := append(templatePath, podSpec, "containers")

	values, found, err := jobutils.NestedSlice(obj.Object, containerPath)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(values) == 0, false)
	mainContainer, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&values[0])
	assert.NilError(t, err)

	volumeMounts, found, err := jobutils.NestedSlice(mainContainer, []string{"volumeMounts"})
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

func checkVolumes(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec, id int) {
	volumesPath := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	podSpec := getPodSpec(workload)
	volumesPath = append(volumesPath, podSpec, "volumes")

	volumes, found, err := jobutils.NestedSlice(obj.Object, volumesPath)
	assert.NilError(t, err)
	assert.Equal(t, found, true)

	if workload.SpecKind() == common.PytorchJobKind {
		volume := findVolume(volumes, SharedMemoryVolume)
		assert.Equal(t, volume != nil, true)
		emptyDir, ok := volume["emptyDir"]
		assert.Equal(t, ok, true)
		sizeLimit, ok := emptyDir.(map[string]interface{})["sizeLimit"]
		assert.Equal(t, ok, true)
		assert.Equal(t, sizeLimit.(string), workload.Spec.Resources[id].SharedMemory)
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

// checkSandboxTemplateCleaned verifies that cleanSandboxPodTemplate correctly removed
// hostPath, PVC, emptyDir(Memory) volumes and their mounts, as well as nodeSelector.
func checkSandboxTemplateCleaned(t *testing.T, obj *unstructured.Unstructured, resourceSpec *v1.ResourceSpec) {
	specPath := append(resourceSpec.TemplatePath(), "spec")
	volumesPath := append(append([]string{}, specPath...), "volumes")
	volumes, _, err := jobutils.NestedSlice(obj.Object, volumesPath)
	assert.NilError(t, err)

	// Template-origin volumes that should be removed
	assert.Equal(t, findVolume(volumes, "dshm") == nil, true)
	assert.Equal(t, findVolume(volumes, "shared-nfs") == nil, true)
	assert.Equal(t, findVolume(volumes, "hyperloom") == nil, true)
	// Retained: plain emptyDir and workload-constructed shared-memory
	assert.Equal(t, findVolume(volumes, "envd-bin") != nil, true)
	assert.Equal(t, findVolume(volumes, SharedMemoryVolume) != nil, true)

	containersPath := append(append([]string{}, specPath...), "containers")
	containers, _, err := jobutils.NestedSlice(obj.Object, containersPath)
	assert.NilError(t, err)
	assert.Equal(t, len(containers) > 0, true)
	mounts := containers[0].(map[string]interface{})["volumeMounts"].([]interface{})

	assert.Equal(t, findVolumeMount(mounts, "dshm") == nil, true)
	assert.Equal(t, findVolumeMount(mounts, "shared-nfs") == nil, true)
	assert.Equal(t, findVolumeMount(mounts, "hyperloom") == nil, true)
	assert.Equal(t, findVolumeMount(mounts, "envd-bin") != nil, true)
	assert.Equal(t, findVolumeMount(mounts, SharedMemoryVolume) != nil, true)

	// nodeSelector should be removed
	nodeSelectorPath := append(append([]string{}, specPath...), "nodeSelector")
	_, found, _ := unstructured.NestedMap(obj.Object, nodeSelectorPath...)
	assert.Equal(t, found, false)
}

func checkRequiredNodeSelectorTerms(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) {
	nodeSelectorPath := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	podSpec := getPodSpec(workload)
	nodeSelectorPath = append(nodeSelectorPath, podSpec, "affinity", "nodeAffinity",
		"requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms")

	affinities, found, err := jobutils.NestedSlice(obj.Object, nodeSelectorPath)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(affinities), 1)
	affinity := affinities[0].(map[string]interface{})
	matchExpressionObj, ok := affinity["matchExpressions"]
	assert.Equal(t, ok, true)
	matchExpressionsSlice := matchExpressionObj.([]interface{})

	id := 0
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
		id++
	}

	for ; id < len(matchExpressionsSlice); id++ {
		matchExpression := matchExpressionsSlice[id].(map[string]interface{})
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
}

func checkPreferredNodeSelectorTerms(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) {
	nodeSelectorPath := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	podSpec := getPodSpec(workload)
	nodeSelectorPath = append(nodeSelectorPath, podSpec, "affinity", "nodeAffinity",
		"preferredDuringSchedulingIgnoredDuringExecution")

	preferenceSlice, found, err := jobutils.NestedSlice(obj.Object, nodeSelectorPath)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(preferenceSlice), 1)
	preferenceMap := preferenceSlice[0].(map[string]interface{})
	preference, ok := preferenceMap["preference"]
	assert.Equal(t, ok, true)
	matchExpressionObj, ok := preference.(map[string]interface{})["matchExpressions"]
	assert.Equal(t, ok, true)
	matchExpressionsSlice := matchExpressionObj.([]interface{})
	count := v1.GetWorkloadDispatchCnt(workload)
	assert.Equal(t, count > 0, true)
	assert.Equal(t, len(matchExpressionsSlice), 1)

	matchExpression := matchExpressionsSlice[0].(map[string]interface{})
	key, ok := matchExpression["key"]
	assert.Equal(t, ok, true)
	assert.Equal(t, key, v1.K8sHostName)
	values, ok := matchExpression["values"]
	assert.Equal(t, ok, true)
	valuesSlice := values.([]interface{})
	assert.Equal(t, len(valuesSlice), len(workload.Status.Nodes[count-1]))
	for i, val := range valuesSlice {
		assert.Equal(t, val.(string) == workload.Status.Nodes[count-1][i], true)
	}
}

func checkPodAntiAffinity(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) {
	podAntiAffinityPath := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	podSpec := getPodSpec(workload)
	podAntiAffinityPath = append(podAntiAffinityPath, podSpec, "affinity", "podAntiAffinity",
		"requiredDuringSchedulingIgnoredDuringExecution")

	antiAffinities, found, err := jobutils.NestedSlice(obj.Object, podAntiAffinityPath)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(antiAffinities), 1)

	antiAffinity := antiAffinities[0].(map[string]interface{})
	topologyKey, ok := antiAffinity["topologyKey"]
	assert.Equal(t, ok, true)
	assert.Equal(t, topologyKey, "kubernetes.io/hostname")

	labelSelectorObj, ok := antiAffinity["labelSelector"]
	assert.Equal(t, ok, true)
	labelSelector, ok := labelSelectorObj.(map[string]interface{})
	assert.Equal(t, ok, true)
	matchLabelsObj, ok := labelSelector["matchLabels"]
	assert.Equal(t, ok, true)
	matchLabels, ok := matchLabelsObj.(map[string]interface{})
	assert.Equal(t, ok, true)
	assert.Equal(t, matchLabels[v1.WorkloadIdLabel], workload.Name)
}

func checkImage(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec, id int) {
	containerPath := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	podSpec := getPodSpec(workload)
	containerPath = append(containerPath, podSpec, "containers")

	values, found, err := jobutils.NestedSlice(obj.Object, containerPath)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(values) == 0, false)
	mainContainer, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&values[0])
	assert.NilError(t, err)

	if id < len(workload.Spec.Images) {
		image, found, err := jobutils.NestedString(mainContainer, []string{"image"})
		assert.NilError(t, err)
		assert.Equal(t, found, true)
		assert.Equal(t, image, workload.Spec.Images[id])
	}
}

func checkHostNetwork(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec, id int) {
	path := resourceSpec.TemplatePath()
	path = append(path, getPodSpec(workload), "hostNetwork")

	isHostNetWork, found, err := jobutils.NestedBool(obj.Object, path)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, isHostNetWork, commonworkload.IsEnabledHostNetwork(workload, id))
}

func checkHostPid(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) {
	path := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	path = append(path, getPodSpec(workload), "hostPID")

	resp, found, err := jobutils.NestedBool(obj.Object, path)
	assert.NilError(t, err)
	if v1.GetOpsJobType(workload) == string(v1.OpsJobPreflightType) {
		assert.Equal(t, found, true)
		assert.Equal(t, resp, true)
	} else {
		assert.Equal(t, found, false)
		assert.Equal(t, resp, false)
	}
}

func checkLabels(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec, id int) {
	rootPath := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	path := append(rootPath, "metadata", "labels")

	labels, found, err := jobutils.NestedMap(obj.Object, path)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, labels[v1.K8sObjectIdLabel].(string), workload.Name)

	path = append(rootPath, "metadata", "annotations")
	annotations, found, err := jobutils.NestedMap(obj.Object, path)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, annotations[v1.UserNameAnnotation].(string), v1.GetUserName(workload))
	assert.Equal(t, annotations["key"].(string), "val")
	assert.Equal(t, annotations[v1.ResourceIdAnnotation].(string), strconv.Itoa(id))
}

func checkSelector(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload) {
	path := []string{getPodSpec(workload), "selector", "matchLabels"}
	labels, found, err := jobutils.NestedMap(obj.Object, path)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, labels[v1.K8sObjectIdLabel].(string), workload.Name)
}

func checkStrategy(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload) {
	path := []string{getPodSpec(workload), "strategy", "rollingUpdate"}
	labels, found, err := jobutils.NestedMap(obj.Object, path)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, labels["maxSurge"].(string), workload.Spec.Service.Extends["maxSurge"])
	assert.Equal(t, labels["maxUnavailable"].(string), workload.Spec.Service.Extends["maxUnavailable"])
}

func checkTolerations(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) {
	path := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	path = append(path, getPodSpec(workload), "tolerations")

	tolerations, found, err := jobutils.NestedSlice(obj.Object, path)
	assert.NilError(t, err)
	if workload.Spec.IsTolerateAll {
		assert.Equal(t, found, true)
		assert.Equal(t, len(tolerations), 1)
		toleration := tolerations[0].(map[string]interface{})
		assert.Equal(t, len(toleration), 1)
		op, ok := toleration["operator"]
		assert.Equal(t, ok, true)
		assert.Equal(t, op, "Exists")
	} else if v1.IsRetryingOnOriginal(workload) {
		assert.Equal(t, found, true)
		assert.Equal(t, len(tolerations), 1)
		toleration := tolerations[0].(map[string]interface{})
		assert.Equal(t, len(toleration), 3)
		key, ok := toleration["key"]
		assert.Equal(t, ok, true)
		assert.Equal(t, key, commonfaults.GenerateTaintKey(v1.StickyNodesMonitorId))
		op, ok := toleration["operator"]
		assert.Equal(t, ok, true)
		assert.Equal(t, op, "Equal")
		val, ok := toleration["value"]
		assert.Equal(t, ok, true)
		assert.Equal(t, val, workload.Name)
	} else {
		assert.Equal(t, found, false)
	}
}

func checkPriorityClass(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) {
	path := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	path = append(path, getPodSpec(workload), "priorityClassName")
	priorityClassName, found, err := jobutils.NestedString(obj.Object, path)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, priorityClassName, commonworkload.GeneratePriorityClass(workload))
}

func checkSecurityContext(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, template *v1.ResourceSpec) {
	containerPath := append(template.PrePaths, template.TemplatePaths...)
	containerPath = append(containerPath, getPodSpec(workload), "containers")

	values, found, err := jobutils.NestedSlice(obj.Object, containerPath)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(values) == 0, false)
	mainContainer, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&values[0])
	assert.NilError(t, err)

	securityContext, found, err := jobutils.NestedMap(mainContainer, []string{"securityContext"})
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

func checkImageSecrets(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) {
	secretPath := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	secretPath = append(secretPath, getPodSpec(workload), "imagePullSecrets")

	secrets, found, err := jobutils.NestedSlice(obj.Object, secretPath)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(secrets), 1)

	secret := secrets[0]
	assert.Equal(t, secret != nil, true)
	name, ok := secret.(map[string]interface{})["name"]
	assert.Equal(t, ok, true)
	assert.Equal(t, name, "test-image")
}

func getEnvs(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) []interface{} {
	containerPath := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	podSpec := getPodSpec(workload)
	containerPath = append(containerPath, podSpec, "containers")

	values, found, err := jobutils.NestedSlice(obj.Object, containerPath)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(values) == 0, false)

	mainContainer, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&values[0])
	assert.NilError(t, err)
	envs, found, err := jobutils.NestedSlice(mainContainer, []string{"env"})
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	return envs
}

func getContainer(obj *unstructured.Unstructured, name string, workload *v1.Workload, resourceSpec *v1.ResourceSpec) map[string]interface{} {
	containerPath := append(resourceSpec.PrePaths, resourceSpec.TemplatePaths...)
	containerPath = append(containerPath, getPodSpec(workload), "containers")

	values, found, err := jobutils.NestedSlice(obj.Object, containerPath)
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

func TestModifyPodAntiAffinity(t *testing.T) {
	tests := []struct {
		name           string
		existingTerms  []interface{}
		workloadName   string
		expectedCount  int
		expectedLabels map[string]interface{}
	}{
		{
			name:          "add anti-affinity to empty object",
			existingTerms: nil,
			workloadName:  "test-workload",
			expectedCount: 1,
			expectedLabels: map[string]interface{}{
				v1.WorkloadIdLabel: "test-workload",
			},
		},
		{
			name: "append anti-affinity to existing terms",
			existingTerms: []interface{}{
				map[string]interface{}{
					"labelSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"existing-label": "existing-value",
						},
					},
					"topologyKey": "kubernetes.io/hostname",
				},
			},
			workloadName:  "new-workload",
			expectedCount: 2,
			expectedLabels: map[string]interface{}{
				v1.WorkloadIdLabel: "new-workload",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{},
			}
			path := []string{"spec", "affinity", "podAntiAffinity", "requiredDuringSchedulingIgnoredDuringExecution"}

			if tt.existingTerms != nil {
				err := unstructured.SetNestedSlice(obj.Object, tt.existingTerms, path...)
				assert.NilError(t, err)
			}

			workload := &v1.Workload{}
			workload.Name = tt.workloadName

			err := modifyPodAntiAffinity(obj, workload, path)
			assert.NilError(t, err)

			terms, found, err := jobutils.NestedSlice(obj.Object, path)
			assert.NilError(t, err)
			assert.Equal(t, found, true)
			assert.Equal(t, len(terms), tt.expectedCount)

			// Check the last term (newly added)
			lastTerm := terms[len(terms)-1].(map[string]interface{})
			topologyKey, ok := lastTerm["topologyKey"]
			assert.Equal(t, ok, true)
			assert.Equal(t, topologyKey, "kubernetes.io/hostname")

			labelSelector, ok := lastTerm["labelSelector"].(map[string]interface{})
			assert.Equal(t, ok, true)
			matchLabels, ok := labelSelector["matchLabels"].(map[string]interface{})
			assert.Equal(t, ok, true)
			assert.Equal(t, matchLabels[v1.WorkloadIdLabel], tt.expectedLabels[v1.WorkloadIdLabel])
		})
	}
}

func TestModifyServiceAccountName(t *testing.T) {
	tests := []struct {
		name        string
		opsJobType  string
		expectedSA  string
		shouldBeSet bool
	}{
		{
			name:        "CD job should set primus-safe service account",
			opsJobType:  string(v1.OpsJobCDType),
			expectedSA:  common.PrimusSafeName,
			shouldBeSet: true,
		},
		{
			name:        "Preflight job should not set service account",
			opsJobType:  string(v1.OpsJobPreflightType),
			shouldBeSet: false,
		},
		{
			name:        "Empty ops job type should not set service account",
			opsJobType:  "",
			shouldBeSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{},
						},
					},
				},
			}

			workload := &v1.Workload{}
			if tt.opsJobType != "" {
				workload.Labels = map[string]string{
					v1.OpsJobTypeLabel: tt.opsJobType,
				}
			}

			path := []string{"spec", "template", "spec", "serviceAccountName"}
			err := modifyServiceAccountName(obj, workload, path)
			assert.NilError(t, err)

			sa, found, err := unstructured.NestedString(obj.Object, path...)
			assert.NilError(t, err)

			if tt.shouldBeSet {
				assert.Equal(t, found, true)
				assert.Equal(t, sa, tt.expectedSA)
			} else {
				assert.Equal(t, found, false)
			}
		})
	}
}

func TestModifyHostPid(t *testing.T) {
	tests := []struct {
		name          string
		opsJobType    string
		expectHostPID bool
		expectHostIPC bool
	}{
		{
			name:          "Privileged job should set hostPID and hostIPC",
			opsJobType:    string(v1.OpsJobPreflightType),
			expectHostPID: true,
			expectHostIPC: true,
		},
		{
			name:          "None-Privileged job type should not set hostPID and hostIPC",
			opsJobType:    "",
			expectHostPID: false,
			expectHostIPC: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{},
						},
					},
				},
			}

			workload := &v1.Workload{}
			if tt.opsJobType != "" {
				workload.Labels = map[string]string{
					v1.OpsJobTypeLabel: tt.opsJobType,
				}
				if tt.opsJobType == string(v1.OpsJobPreflightType) {
					v1.SetAnnotation(workload, v1.WorkloadPrivilegedAnnotation, v1.TrueStr)
				}
			}

			templatePath := []string{"spec", "template"}
			err := modifyHostPid(obj, workload, templatePath)
			assert.NilError(t, err)

			hostPID, foundPID, _ := jobutils.NestedBool(obj.Object, []string{"spec", "template", "spec", "hostPID"})
			hostIPC, foundIPC, _ := jobutils.NestedBool(obj.Object, []string{"spec", "template", "spec", "hostIPC"})
			if tt.expectHostPID {
				assert.Equal(t, foundPID, true)
				assert.Equal(t, hostPID, true)
			} else {
				assert.Equal(t, foundPID, false)
			}

			if tt.expectHostIPC {
				assert.Equal(t, foundIPC, true)
				assert.Equal(t, hostIPC, true)
			} else {
				assert.Equal(t, foundIPC, false)
			}
		})
	}
}

// buildFakeDGD constructs the unstructured DGD object the dispatcher's generic
// flow would produce: spec.services holds numRoles role-agnostic slots
// (role0..role(numRoles-1)), each carrying a placeholder pod template with a
// container named "main" containing one preexisting NCCL env var. Real
// dispatcher runs would also leave a placeholder componentType=worker; tests
// rely on normalizeDynamoDGD to overwrite it per role.
func buildFakeDGD(numRoles int) *unstructured.Unstructured {
	services := map[string]interface{}{}
	for i := 0; i < numRoles; i++ {
		services["role"+strconv.Itoa(i)] = map[string]interface{}{
			"componentType":    "worker",
			"subComponentType": nil,
			"replicas":         int64(1),
			"extraPodSpec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"image": "test-image:latest",
						"env": []interface{}{
							map[string]interface{}{
								"name":  "NCCL_SOCKET_IFNAME",
								"value": "eno0",
							},
						},
					},
				},
			},
		}
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "nvidia.com/v1alpha1",
			"kind":       "DynamoGraphDeployment",
			"metadata":   map[string]interface{}{"name": "test-dgd"},
			"spec": map[string]interface{}{
				"backendFramework": "sglang",
				"services":         services,
			},
		},
	}
}

// buildDynamoTestWorkload constructs a minimal SaFE Workload typed as
// DynamoDeployment with the given comma-separated service-roles. extra
// annotations (e.g. multinode.<role>=N) are merged in.
func buildDynamoTestWorkload(roles string, extraAnnotations map[string]string) *v1.Workload {
	annotations := map[string]string{
		v1.DynamoServiceRolesAnnotation: roles,
	}
	for k, v := range extraAnnotations {
		annotations[k] = v
	}
	// Resources count must equal the number of roles; values are placeholders
	// since normalizeDynamoDGD only reads annotation and len(Resources).
	roleList := []string{}
	for _, r := range splitNonEmpty(roles, ",") {
		roleList = append(roleList, r)
	}
	resources := make([]v1.WorkloadResource, len(roleList))
	for i := range resources {
		resources[i] = v1.WorkloadResource{Replica: 1, CPU: "1", Memory: "1Gi"}
	}
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-dgd",
			Annotations: annotations,
		},
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{
				Kind:    common.DynamoDeploymentKind,
				Version: common.DefaultVersion,
			},
			Resources: resources,
		},
	}
}

// splitNonEmpty splits sep-delimited strings and drops empty fragments.
func splitNonEmpty(s, sep string) []string {
	out := []string{}
	for _, p := range splitS(s, sep) {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitS(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := []string{}
	cur := ""
	for _, c := range s {
		if string(c) == sep {
			parts = append(parts, cur)
			cur = ""
			continue
		}
		cur += string(c)
	}
	parts = append(parts, cur)
	return parts
}

// findDynamoEnv finds an env var in a slice of {name,value} maps. Returns the
// value and whether it was found.
func findDynamoEnv(env []interface{}, name string) (string, bool) {
	for _, e := range env {
		m, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		if n, _ := m["name"].(string); n == name {
			v, _ := m["value"].(string)
			return v, true
		}
	}
	return "", false
}

// getDynamoService retrieves the rewritten DGD service map at
// spec.services[key]; fails the test if the key is missing.
func getDynamoService(t *testing.T, obj *unstructured.Unstructured, key string) map[string]interface{} {
	t.Helper()
	svc, found, err := jobutils.NestedMap(obj.Object, []string{"spec", "services", key})
	assert.NilError(t, err)
	assert.Equal(t, found, true, "service %s should be present", key)
	return svc
}

// getDynamoMainContainer returns the rewritten extraPodSpec.mainContainer for
// the given service key. Verifies the conversion from containers[0] also
// removed the original containers entry (or kept only sidecars).
func getDynamoMainContainer(t *testing.T, obj *unstructured.Unstructured, svcKey string) map[string]interface{} {
	t.Helper()
	main, found, err := jobutils.NestedMap(obj.Object,
		[]string{"spec", "services", svcKey, "extraPodSpec", "mainContainer"})
	assert.NilError(t, err)
	assert.Equal(t, found, true, "service %s mainContainer should exist", svcKey)
	return main
}

// TestNormalizeDynamoDGD_AggregatedMinimal covers the 2-resource shape
// `[Frontend, Worker]` produced by the default annotation inference. Asserts
// slot keys remain role0/role1 (NOT renamed — dispatcher reconcile relies
// on stable map keys), serviceName field tags the role, componentType is
// set, mainContainer conversion happens.
func TestNormalizeDynamoDGD_AggregatedMinimal(t *testing.T) {
	obj := buildFakeDGD(2)
	workload := buildDynamoTestWorkload("frontend,worker", nil)

	err := normalizeDynamoDGD(obj, workload)
	assert.NilError(t, err)

	services, _, err := jobutils.NestedMap(obj.Object, []string{"spec", "services"})
	assert.NilError(t, err)
	assert.Equal(t, len(services), 2, "should have exactly role0 + role1")
	_, hasRole0 := services["role0"]
	assert.Equal(t, hasRole0, true, "role0 slot key must be preserved")
	_, hasRole1 := services["role1"]
	assert.Equal(t, hasRole1, true, "role1 slot key must be preserved")

	fe := getDynamoService(t, obj, "role0")
	assert.Equal(t, fe["componentType"], "Frontend")
	assert.Equal(t, fe["serviceName"], "Frontend")
	_, hasSubComp := fe["subComponentType"]
	assert.Equal(t, hasSubComp, false, "Frontend must not have subComponentType")

	feMain := getDynamoMainContainer(t, obj, "role0")
	assert.Equal(t, feMain["name"], "main")
	feExtra := fe["extraPodSpec"].(map[string]interface{})
	_, hasContainers := feExtra["containers"]
	assert.Equal(t, hasContainers, false, "containers should be removed after mainContainer conversion")

	wk := getDynamoService(t, obj, "role1")
	assert.Equal(t, wk["componentType"], "Main")
	assert.Equal(t, wk["serviceName"], "Worker")
	_, hasSubCompWk := wk["subComponentType"]
	assert.Equal(t, hasSubCompWk, false, "Worker must not have subComponentType")

	// Aggregated mode never injects sglang disagg env.
	wkMain := getDynamoMainContainer(t, obj, "role1")
	wkEnv := wkMain["env"].([]interface{})
	_, hasMode := findDynamoEnv(wkEnv, "SGLANG_DISAGGREGATION_MODE")
	assert.Equal(t, hasMode, false, "aggregated worker must not carry disagg env")
}

// TestNormalizeDynamoDGD_DisaggMinimal covers `[Frontend, PrefillWorker,
// DecodeWorker]` and verifies subComponentType + sglang disagg envs land on
// the right services with the right bootstrap port and transfer backend.
func TestNormalizeDynamoDGD_DisaggMinimal(t *testing.T) {
	obj := buildFakeDGD(3)
	workload := buildDynamoTestWorkload("frontend,prefill,decode", nil)

	err := normalizeDynamoDGD(obj, workload)
	assert.NilError(t, err)

	pf := getDynamoService(t, obj, "role1")
	assert.Equal(t, pf["componentType"], "Main")
	assert.Equal(t, pf["subComponentType"], "prefill")
	assert.Equal(t, pf["serviceName"], "PrefillWorker")

	pfMain := getDynamoMainContainer(t, obj, "role1")
	pfEnv := pfMain["env"].([]interface{})
	mode, ok := findDynamoEnv(pfEnv, "SGLANG_DISAGGREGATION_MODE")
	assert.Equal(t, ok, true)
	assert.Equal(t, mode, "prefill")
	port, ok := findDynamoEnv(pfEnv, "SGLANG_DISAGGREGATION_BOOTSTRAP_PORT")
	assert.Equal(t, ok, true)
	assert.Equal(t, port, strconv.Itoa(common.DynamoBootstrapPort))
	backend, ok := findDynamoEnv(pfEnv, "SGLANG_DISAGGREGATION_TRANSFER_BACKEND")
	assert.Equal(t, ok, true)
	assert.Equal(t, backend, common.DynamoKVBackendNixl)

	dec := getDynamoService(t, obj, "role2")
	assert.Equal(t, dec["componentType"], "Main")
	assert.Equal(t, dec["subComponentType"], "decode")
	assert.Equal(t, dec["serviceName"], "DecodeWorker")

	decMain := getDynamoMainContainer(t, obj, "role2")
	decEnv := decMain["env"].([]interface{})
	mode, ok = findDynamoEnv(decEnv, "SGLANG_DISAGGREGATION_MODE")
	assert.Equal(t, ok, true)
	assert.Equal(t, mode, "decode")

	// Frontend in disagg mode is still a normal Frontend (no disagg envs).
	feMain := getDynamoMainContainer(t, obj, "role0")
	feEnv := feMain["env"].([]interface{})
	_, hasMode := findDynamoEnv(feEnv, "SGLANG_DISAGGREGATION_MODE")
	assert.Equal(t, hasMode, false)
}

// TestNormalizeDynamoDGD_DisaggWithPlanner verifies the 4-resource shape
// `[Frontend, PrefillWorker, DecodeWorker, Planner]` and asserts the planner
// service gets componentType=Planner without disagg env contamination.
func TestNormalizeDynamoDGD_DisaggWithPlanner(t *testing.T) {
	obj := buildFakeDGD(4)
	workload := buildDynamoTestWorkload("frontend,prefill,decode,planner", nil)

	err := normalizeDynamoDGD(obj, workload)
	assert.NilError(t, err)

	services, _, err := jobutils.NestedMap(obj.Object, []string{"spec", "services"})
	assert.NilError(t, err)
	assert.Equal(t, len(services), 4)

	plan := getDynamoService(t, obj, "role3")
	assert.Equal(t, plan["componentType"], "Planner")
	assert.Equal(t, plan["serviceName"], "Planner")
	_, hasSubComp := plan["subComponentType"]
	assert.Equal(t, hasSubComp, false)

	planMain := getDynamoMainContainer(t, obj, "role3")
	planEnv := planMain["env"].([]interface{})
	_, hasMode := findDynamoEnv(planEnv, "SGLANG_DISAGGREGATION_MODE")
	assert.Equal(t, hasMode, false, "planner must not carry disagg env")

	// Disagg pair still in place.
	pf := getDynamoService(t, obj, "role1")
	assert.Equal(t, pf["subComponentType"], "prefill")
	assert.Equal(t, pf["serviceName"], "PrefillWorker")
	dec := getDynamoService(t, obj, "role2")
	assert.Equal(t, dec["subComponentType"], "decode")
	assert.Equal(t, dec["serviceName"], "DecodeWorker")
}

// TestNormalizeDynamoDGD_MultiNodeTP verifies the multinode annotation lifts
// the PrefillWorker service to LeaderWorkerSet topology (numberOfNodes > 1)
// while leaving the other services single-node.
func TestNormalizeDynamoDGD_MultiNodeTP(t *testing.T) {
	obj := buildFakeDGD(3)
	workload := buildDynamoTestWorkload(
		"frontend,prefill,decode",
		map[string]string{
			v1.DynamoMultinodePrefix + "prefill": "2",
		},
	)

	err := normalizeDynamoDGD(obj, workload)
	assert.NilError(t, err)

	pf := getDynamoService(t, obj, "role1")
	assert.Equal(t, pf["serviceName"], "PrefillWorker")
	mn, ok := pf["multinode"].(map[string]interface{})
	assert.Equal(t, ok, true, "PrefillWorker should have multinode block")
	assert.Equal(t, mn["numberOfNodes"], int64(2))

	dec := getDynamoService(t, obj, "role2")
	assert.Equal(t, dec["serviceName"], "DecodeWorker")
	_, hasMn := dec["multinode"]
	assert.Equal(t, hasMn, false, "DecodeWorker must remain single-node (no annotation)")

	fe := getDynamoService(t, obj, "role0")
	assert.Equal(t, fe["serviceName"], "Frontend")
	_, hasMnFe := fe["multinode"]
	assert.Equal(t, hasMnFe, false, "Frontend must remain single-node")
}
