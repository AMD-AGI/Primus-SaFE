/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const (
	SharedMemoryVolume = "shared-memory"
	Launcher           = "chmod +x /shared-data/launcher.sh; /bin/sh /shared-data/launcher.sh"
)

func modifyObjectOnCreation(obj *unstructured.Unstructured,
	adminWorkload *v1.Workload, workspace *v1.Workspace, resourceSpec *v1.ResourceSpec) error {
	_, found, err := unstructured.NestedFieldNoCopy(obj.Object, resourceSpec.PrePaths...)
	if err != nil || !found {
		return nil
	}
	templatePath := resourceSpec.GetTemplatePath()

	path := append(templatePath, "metadata", "labels")
	if err = modifyLabels(obj, adminWorkload, path); err != nil {
		return err
	}
	path = append(templatePath, "spec",
		"affinity", "nodeAffinity", "requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms")
	if err = modifyNodeSelectorTerms(obj, adminWorkload, path); err != nil {
		return err
	}
	path = append(templatePath, "spec", "containers")
	if err = modifyMainContainer(obj, adminWorkload, workspace, path); err != nil {
		return err
	}
	path = append(templatePath, "spec", "volumes")
	if err = modifyVolumes(obj, adminWorkload, workspace, path); err != nil {
		return err
	}
	path = append(templatePath, "spec", "priorityClassName")
	if err = modifyPriorityClass(obj, adminWorkload, path); err != nil {
		return err
	}
	path = append(templatePath, "spec", "hostNetwork")
	if err = modifyHostNetWork(obj, adminWorkload, path); err != nil {
		return err
	}
	path = append(templatePath, "spec", "tolerations")
	if err = modifyTolerations(obj, adminWorkload, path); err != nil {
		return err
	}
	path = []string{"spec", "strategy"}
	if err = modifyStrategy(obj, adminWorkload, path); err != nil {
		return err
	}
	if adminWorkload.Spec.Service != nil {
		path = []string{"spec", "selector"}
		if err = modifySelector(obj, adminWorkload, path); err != nil {
			return err
		}
	}
	if err = modifyByOpsJob(obj, adminWorkload, templatePath); err != nil {
		return err
	}
	return nil
}

func modifyLabels(obj *unstructured.Unstructured, adminWorkload *v1.Workload, path []string) error {
	labels := buildLabels(adminWorkload)
	if err := unstructured.SetNestedMap(obj.Object, labels, path...); err != nil {
		return err
	}
	return nil
}

func modifyNodeSelectorTerms(obj *unstructured.Unstructured, adminWorkload *v1.Workload, path []string) error {
	nodeSelectorTerms, _, err := unstructured.NestedSlice(obj.Object, path...)
	if err != nil {
		return err
	}
	expression := buildMatchExpression(adminWorkload)
	if len(nodeSelectorTerms) == 0 {
		expressions := make(map[string]interface{})
		expressions["matchExpressions"] = expression
		nodeSelectorTerms = append(nodeSelectorTerms, expressions)
	} else {
		matchExpressions := nodeSelectorTerms[0].(map[string]interface{})
		objs, ok := matchExpressions["matchExpressions"]
		if ok {
			expressions := objs.([]interface{})
			expressions = append(expressions, expression...)
			matchExpressions["matchExpressions"] = expressions
		} else {
			matchExpressions["matchExpressions"] = []interface{}{expression}
		}
	}
	if err = unstructured.SetNestedSlice(obj.Object, nodeSelectorTerms, path...); err != nil {
		return err
	}
	return nil
}

func modifyMainContainer(obj *unstructured.Unstructured,
	adminWorkload *v1.Workload, workspace *v1.Workspace, path []string) error {
	containers, found, err := unstructured.NestedSlice(obj.Object, path...)
	if err != nil {
		return err
	}
	if !found || len(containers) == 0 {
		return fmt.Errorf("failed to find container with path: %v", path)
	}
	mainContainer, err := getMainContainer(containers, v1.GetMainContainer(adminWorkload))
	if err != nil {
		return err
	}
	env := buildEnvironment(adminWorkload)
	modifyEnv(mainContainer, env, v1.IsEnableHostNetwork(adminWorkload))
	modifyVolumeMounts(mainContainer, adminWorkload, workspace)
	modifySecurityContext(mainContainer, adminWorkload)
	mainContainer["ports"] = buildPorts(adminWorkload)
	if healthz := buildHealthCheck(adminWorkload.Spec.Liveness); healthz != nil {
		mainContainer["livenessProbe"] = healthz
	}
	if healthz := buildHealthCheck(adminWorkload.Spec.Readiness); healthz != nil {
		mainContainer["readinessProbe"] = healthz
	}
	if err = unstructured.SetNestedField(obj.Object, containers, path...); err != nil {
		return err
	}
	return nil
}

func modifyEnv(mainContainer map[string]interface{}, env []interface{}, isHostNetwork bool) {
	if len(env) == 0 && isHostNetwork {
		return
	}
	var currentEnv []interface{}
	envObjs, ok := mainContainer["env"]
	if ok {
		currentEnv = envObjs.([]interface{})
	}

	if !isHostNetwork {
		for i := range currentEnv {
			envObj := currentEnv[i].(map[string]interface{})
			name, ok := envObj["name"]
			if !ok {
				continue
			}
			if stringutil.StrCaseEqual(name.(string), "NCCL_SOCKET_IFNAME") ||
				stringutil.StrCaseEqual(name.(string), "GLOO_SOCKET_IFNAME") {
				envObj["value"] = "eth0"
			}
		}
	}
	if len(env) > 0 {
		currentEnv = append(currentEnv, env...)
	}
	mainContainer["env"] = currentEnv
}

func modifyVolumeMounts(mainContainer map[string]interface{}, workload *v1.Workload, workspace *v1.Workspace) {
	var volumeMounts []interface{}
	volumeMountObjs, ok := mainContainer["volumeMounts"]
	if ok {
		volumeMounts = volumeMountObjs.([]interface{})
	}
	volumeMounts = append(volumeMounts, buildVolumeMount(SharedMemoryVolume, "/dev/shm", ""))
	maxId := 0
	if workspace != nil {
		for _, vol := range workspace.Spec.Volumes {
			if vol.Id > maxId {
				maxId = vol.Id
			}
			if vol.MountPath != "" {
				volumeMount := buildVolumeMount(vol.GenFullVolumeId(), vol.MountPath, vol.SubPath)
				volumeMounts = append(volumeMounts, volumeMount)
			}
		}
	}
	for _, hostpath := range workload.Spec.Hostpath {
		maxId++
		volumeName := v1.GenFullVolumeId(v1.HOSTPATH, maxId)
		volumeMount := buildVolumeMount(volumeName, hostpath, "")
		volumeMounts = append(volumeMounts, volumeMount)
	}
	mainContainer["volumeMounts"] = volumeMounts
}

func modifyVolumes(obj *unstructured.Unstructured, workload *v1.Workload, workspace *v1.Workspace, path []string) error {
	volumes, _, err := unstructured.NestedSlice(obj.Object, path...)
	if err != nil {
		return err
	}

	maxId := 0
	hasNewVolume := false
	if workspace != nil {
		for _, vol := range workspace.Spec.Volumes {
			if vol.Id > maxId {
				maxId = vol.Id
			}
			volumeName := vol.GenFullVolumeId()
			var volume interface{}
			if vol.Type == v1.HOSTPATH {
				volume = buildHostPathVolume(volumeName, vol.HostPath)
			} else {
				volume = buildPvcVolume(volumeName)
			}
			volumes = append(volumes, volume)
			hasNewVolume = true
		}
	}

	for _, hostpath := range workload.Spec.Hostpath {
		maxId++
		volumeName := v1.GenFullVolumeId(v1.HOSTPATH, maxId)
		volume := buildHostPathVolume(volumeName, hostpath)
		volumes = append(volumes, volume)
		hasNewVolume = true
	}
	if !hasNewVolume {
		return nil
	}
	if err = unstructured.SetNestedSlice(obj.Object, volumes, path...); err != nil {
		return err
	}
	return nil
}

func modifySecurityContext(mainContainer map[string]interface{}, workload *v1.Workload) {
	if v1.GetOpsJobType(workload) != string(v1.OpsJobPreflightType) &&
		v1.GetOpsJobType(workload) != string(v1.OpsJobPreflightType) {
		return
	}
	mainContainer["securityContext"] = map[string]interface{}{
		"privileged": true,
	}
}

func modifyPriorityClass(obj *unstructured.Unstructured, adminWorkload *v1.Workload, path []string) error {
	priorityClass := commonworkload.GeneratePriorityClass(adminWorkload)
	if err := unstructured.SetNestedField(obj.Object, priorityClass, path...); err != nil {
		return err
	}
	return nil
}

func modifyHostNetWork(obj *unstructured.Unstructured, adminWorkload *v1.Workload, path []string) error {
	isEnableHostNetWork := v1.IsEnableHostNetwork(adminWorkload)
	if err := unstructured.SetNestedField(obj.Object, isEnableHostNetWork, path...); err != nil {
		return err
	}
	return nil
}

func modifyByOpsJob(obj *unstructured.Unstructured, adminWorkload *v1.Workload, templatePath []string) error {
	if v1.GetOpsJobType(adminWorkload) != string(v1.OpsJobPreflightType) &&
		v1.GetOpsJobType(adminWorkload) != string(v1.OpsJobPreflightType) {
		return nil
	}
	path := append(templatePath, "spec", "hostPID")
	if err := unstructured.SetNestedField(obj.Object, true, path...); err != nil {
		return err
	}
	path = append(templatePath, "spec", "hostIPC")
	if err := unstructured.SetNestedField(obj.Object, true, path...); err != nil {
		return err
	}
	return nil
}

func modifyStrategy(obj *unstructured.Unstructured, adminWorkload *v1.Workload, path []string) error {
	if adminWorkload.SpecKind() != common.DeploymentKind {
		return nil
	}
	rollingUpdate := buildStrategy(adminWorkload)
	if len(rollingUpdate) == 0 {
		return nil
	}
	if err := unstructured.SetNestedMap(obj.Object, rollingUpdate, path...); err != nil {
		return err
	}
	return nil
}

func modifySelector(obj *unstructured.Unstructured, adminWorkload *v1.Workload, path []string) error {
	selector := buildSelector(adminWorkload)
	if err := unstructured.SetNestedMap(obj.Object, selector, path...); err != nil {
		return err
	}
	return nil
}

func modifyTolerations(obj *unstructured.Unstructured, adminWorkload *v1.Workload, path []string) error {
	if !adminWorkload.Spec.IsTolerateAll {
		return nil
	}
	tolerations := []interface{}{
		map[string]interface{}{
			"operator": "Exists",
		},
	}
	if err := unstructured.SetNestedSlice(obj.Object, tolerations, path...); err != nil {
		return err
	}
	return nil
}

func getMainContainer(containers []interface{}, mainContainerName string) (map[string]interface{}, error) {
	var mainContainer map[string]interface{}
	for i := range containers {
		container := containers[i].(map[string]interface{})
		name := jobutils.GetUnstructuredString(container, []string{"name"})
		if name == mainContainerName {
			mainContainer = container
			break
		}
	}
	if mainContainer == nil {
		return nil, fmt.Errorf("failed to find main container, name: %s", mainContainerName)
	}
	return mainContainer, nil
}

func buildCommands(adminWorkload *v1.Workload) []interface{} {
	return []interface{}{"/bin/sh", "-c", buildEntryPoint(adminWorkload)}
}

func buildEntryPoint(adminWorkload *v1.Workload) string {
	result := ""
	if commonworkload.IsOpsJob(adminWorkload) {
		result = stringutil.Base64Decode(adminWorkload.Spec.EntryPoint)
	} else {
		result = Launcher + " '" + adminWorkload.Spec.EntryPoint + "'"
	}
	return result
}

func buildLabels(adminWorkload *v1.Workload) map[string]interface{} {
	return map[string]interface{}{
		v1.WorkloadIdLabel:          adminWorkload.Name,
		v1.WorkloadDispatchCntLabel: buildDispatchCount(adminWorkload),
	}
}

func buildResources(adminWorkload *v1.Workload) map[string]interface{} {
	result := map[string]interface{}{
		string(corev1.ResourceCPU):              adminWorkload.Spec.Resource.CPU,
		string(corev1.ResourceMemory):           adminWorkload.Spec.Resource.Memory,
		string(corev1.ResourceEphemeralStorage): adminWorkload.Spec.Resource.EphemeralStorage,
	}
	if adminWorkload.Spec.Resource.GPU != "" {
		result[adminWorkload.Spec.Resource.GPUName] = adminWorkload.Spec.Resource.GPU
	}
	if adminWorkload.Spec.Resource.RdmaResource != "" && commonconfig.GetRdmaName() != "" {
		result[commonconfig.GetRdmaName()] = adminWorkload.Spec.Resource.RdmaResource
	}
	return result
}

func buildEnvironment(adminWorkload *v1.Workload) []interface{} {
	var result []interface{}
	if adminWorkload.Spec.IsSupervised {
		result = append(result, map[string]interface{}{
			"name":  "ENABLE_SUPERVISE",
			"value": "true",
		})
		if commonconfig.GetWorkloadHangCheckInterval() > 0 {
			result = append(result, map[string]interface{}{
				"name":  "HANG_CHECK_INTERVAL",
				"value": strconv.Itoa(commonconfig.GetWorkloadHangCheckInterval()),
			})
		}
	}
	if adminWorkload.Spec.Resource.GPU != "" {
		result = append(result, map[string]interface{}{
			"name":  "GPUS_PER_NODE",
			"value": adminWorkload.Spec.Resource.GPU,
		})
	}
	result = append(result, map[string]interface{}{
		"name":  "WORKLOAD_ID",
		"value": adminWorkload.Name,
	})
	result = append(result, map[string]interface{}{
		"name":  "DISPATCH_COUNT",
		"value": strconv.Itoa(v1.GetWorkloadDispatchCnt(adminWorkload) + 1),
	})
	result = append(result, map[string]interface{}{
		"name":  "SSH_PORT",
		"value": strconv.Itoa(adminWorkload.Spec.SSHPort),
	})
	return result
}

func buildPorts(adminWorkload *v1.Workload) []interface{} {
	jobPort := map[string]interface{}{
		"containerPort": int64(adminWorkload.Spec.JobPort),
		"protocol":      "TCP",
	}
	if adminWorkload.SpecKind() == common.PytorchJobKind || adminWorkload.SpecKind() == common.AuthoringKind {
		jobPort["name"] = common.PytorchJobPortName
	}
	sshPort := map[string]interface{}{
		"containerPort": int64(adminWorkload.Spec.SSHPort),
		"protocol":      "TCP",
		"name":          common.SSHPortName,
	}
	return []interface{}{jobPort, sshPort}
}

func buildHealthCheck(healthz *v1.HealthCheck) map[string]interface{} {
	if healthz == nil {
		return nil
	}
	return map[string]interface{}{
		"failureThreshold":    int64(healthz.FailureThreshold),
		"initialDelaySeconds": int64(healthz.InitialDelaySeconds),
		"periodSeconds":       int64(healthz.PeriodSeconds),
		"httpGet": map[string]interface{}{
			"path": healthz.Path,
			"port": int64(healthz.Port),
		},
	}
}

func buildVolumeMount(name, mountPath, subPath string) interface{} {
	volMount := map[string]interface{}{
		"mountPath": mountPath,
		"name":      name,
	}
	if subPath != "" {
		volMount["subPath"] = subPath
	}
	return volMount
}

func buildHostPathVolume(volumeName, hostPath string) interface{} {
	return map[string]interface{}{
		"hostPath": map[string]interface{}{
			"path": hostPath,
		},
		"name": volumeName,
	}
}

func buildPvcVolume(volumeName string) interface{} {
	return map[string]interface{}{
		"persistentVolumeClaim": map[string]interface{}{
			"claimName": volumeName,
		},
		"name": volumeName,
	}
}

func buildMatchExpression(adminWorkload *v1.Workload) []interface{} {
	var result []interface{}
	if adminWorkload.Spec.Workspace != corev1.NamespaceDefault {
		result = append(result, map[string]interface{}{
			"key":      v1.WorkspaceIdLabel,
			"operator": "In",
			"values":   []interface{}{adminWorkload.Spec.Workspace},
		})
	}
	for key, val := range adminWorkload.Spec.CustomerLabels {
		var values []interface{}
		parts := strings.Fields(val)
		for i := range parts {
			values = append(values, parts[i])
		}
		result = append(result, map[string]interface{}{
			"key":      key,
			"operator": "In",
			"values":   values,
		})
	}
	return result
}

func buildSharedMemoryVolume(sizeLimit string) interface{} {
	return map[string]interface{}{
		"emptyDir": map[string]interface{}{
			"medium":    string(corev1.StorageMediumMemory),
			"sizeLimit": sizeLimit,
		},
		"name": SharedMemoryVolume,
	}
}

func buildStrategy(adminWorkload *v1.Workload) map[string]interface{} {
	keys := []string{"maxSurge", "maxUnavailable"}
	rollingUpdate := make(map[string]interface{})
	for _, key := range keys {
		rollingUpdate[key] = adminWorkload.Spec.Service.Extends[key]
	}
	if len(rollingUpdate) == 0 {
		return nil
	}
	return map[string]interface{}{
		"type":          "RollingUpdate",
		"rollingUpdate": rollingUpdate,
	}
}

func buildSelector(adminWorkload *v1.Workload) map[string]interface{} {
	return map[string]interface{}{
		"matchLabels": map[string]interface{}{
			v1.WorkloadIdLabel: adminWorkload.Name,
		},
	}
}

func convertToStringMap(input map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for key, value := range input {
		if strValue, ok := value.(string); ok {
			result[key] = strValue
		}
	}
	return result
}

func convertEnvsToStringMap(envs []interface{}) map[string]string {
	result := make(map[string]string)
	for _, e := range envs {
		env, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		name, ok := env["name"]
		if !ok {
			continue
		}
		value, ok := env["value"]
		if !ok {
			continue
		}
		result[name.(string)] = value.(string)
	}
	return result
}

func generateVolumeName(storageType string, id int) string {
	return fmt.Sprintf("%s-%d", storageType, id)
}
