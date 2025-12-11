/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
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
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const (
	SharedMemoryVolume = "shared-memory"
	Launcher           = "chmod +x /shared-data/launcher.sh; /bin/sh /shared-data/launcher.sh"
)

// initializeObject modifies various aspects of a Kubernetes object during workload creation.
// It applies labels, node selectors, container configurations, volumes, and other settings.
// based on the admin workload specification and workspace configuration.
func initializeObject(obj *unstructured.Unstructured,
	workload *v1.Workload, workspace *v1.Workspace, resourceSpec *v1.ResourceSpec) error {
	_, found, err := unstructured.NestedFieldNoCopy(obj.Object, resourceSpec.PrePaths...)
	if err != nil || !found {
		return nil
	}
	templatePath := resourceSpec.GetTemplatePath()

	path := append(templatePath, "metadata", "labels")
	if err = modifyLabels(obj, workload, path); err != nil {
		return fmt.Errorf("failed to modify labels: %v", err.Error())
	}
	path = append(templatePath, "spec",
		"affinity", "nodeAffinity", "requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms")
	if err = modifyNodeSelectorTerms(obj, workload, path); err != nil {
		return fmt.Errorf("failed to modify nodeSelectorTerms: %v", err.Error())
	}
	path = append(templatePath, "spec", "containers")
	if err = modifyContainers(obj, workload, workspace, path); err != nil {
		return fmt.Errorf("failed to modify main container: %v", err.Error())
	}
	path = append(templatePath, "spec", "volumes")
	if err = modifyVolumes(obj, workload, workspace, path); err != nil {
		return fmt.Errorf("failed to modify volumes: %v", err.Error())
	}
	path = append(templatePath, "spec", "imagePullSecrets")
	if err = modifyImageSecrets(obj, workload, path); err != nil {
		return fmt.Errorf("failed to modify image secrets: %v", err.Error())
	}
	path = append(templatePath, "spec", "priorityClassName")
	if err = modifyPriorityClass(obj, workload, path); err != nil {
		return fmt.Errorf("failed to modify priority: %v", err.Error())
	}
	path = append(templatePath, "spec", "hostNetwork")
	if err = modifyHostNetwork(obj, workload, path); err != nil {
		return fmt.Errorf("failed to modify host network: %v", err.Error())
	}
	path = append(templatePath, "spec", "tolerations")
	if err = modifyTolerations(obj, workload, path); err != nil {
		return fmt.Errorf("failed to modify tolerations: %v", err.Error())
	}
	if workload.Spec.Service != nil {
		path = []string{"spec", "strategy"}
		if err = modifyStrategy(obj, workload, path); err != nil {
			return fmt.Errorf("failed to modify strategy: %v", err.Error())
		}
		path = []string{"spec", "selector"}
		if err = modifySelector(obj, workload, path); err != nil {
			return fmt.Errorf("failed to modify selector: %v", err.Error())
		}
	}
	if err = modifyByOpsJob(obj, workload, templatePath); err != nil {
		return fmt.Errorf("failed to modify by opsjob: %v", err.Error())
	}
	return nil
}

// modifyLabels updates the metadata labels of a Kubernetes object based on the workload specification.
func modifyLabels(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	labels := buildLabels(workload)
	return unstructured.SetNestedMap(obj.Object, labels, path...)
}

// modifyNodeSelectorTerms updates node selector terms in the object's node affinity configuration.
// It adds custom match expressions based on the workload specification.
func modifyNodeSelectorTerms(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	nodeSelectorTerms, _, err := unstructured.NestedSlice(obj.Object, path...)
	if err != nil {
		return err
	}
	expression := buildMatchExpression(workload)
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

// modifyContainers configures the containers of a workload with environment variables,
// volume mounts, security context, ports, and health checks based on the workload specification.
func modifyContainers(obj *unstructured.Unstructured,
	workload *v1.Workload, workspace *v1.Workspace, path []string) error {
	containers, found, err := unstructured.NestedSlice(obj.Object, path...)
	if err != nil {
		return err
	}
	if !found || len(containers) == 0 {
		return fmt.Errorf("failed to find container with path: %v", path)
	}
	env := buildEnvironment(workload)
	mainContainerName := v1.GetMainContainer(workload)
	for i := range containers {
		container := containers[i].(map[string]interface{})
		modifyEnv(container, env, v1.IsEnableHostNetwork(workload))
		modifyVolumeMounts(container, workload, workspace)
		modifySecurityContext(container, workload)

		name := jobutils.GetUnstructuredString(container, []string{"name"})
		if name == mainContainerName {
			container["ports"] = buildPorts(workload)
			if healthz := buildHealthCheck(workload.Spec.Liveness); healthz != nil {
				container["livenessProbe"] = healthz
			}
			if healthz := buildHealthCheck(workload.Spec.Readiness); healthz != nil {
				container["readinessProbe"] = healthz
			}
		}
	}
	if err = unstructured.SetNestedField(obj.Object, containers, path...); err != nil {
		return err
	}
	return nil
}

// modifyEnv updates environment variables in the main container.
// It handles special network interface names when host networking is not enabled.
func modifyEnv(container map[string]interface{}, envs []interface{}, isHostNetwork bool) {
	if len(envs) == 0 && isHostNetwork {
		return
	}
	var currentEnv []interface{}
	envObjs, ok := container["env"]
	if ok {
		currentEnv = envObjs.([]interface{})
	}

	currentNameSet := sets.NewSet()
	for i := range currentEnv {
		envObj := currentEnv[i].(map[string]interface{})
		name, ok := envObj["name"]
		if !ok {
			continue
		}
		nameStr := name.(string)
		currentNameSet.Insert(nameStr)
		if !isHostNetwork {
			if stringutil.StrCaseEqual(nameStr, "NCCL_SOCKET_IFNAME") ||
				stringutil.StrCaseEqual(nameStr, "GLOO_SOCKET_IFNAME") {
				envObj["value"] = "eth0"
			}
		}
	}
	for i := range envs {
		envObj := envs[i].(map[string]interface{})
		name, ok := envObj["name"]
		if !ok {
			continue
		}
		nameStr := name.(string)
		if currentNameSet.Has(nameStr) {
			continue
		}
		currentEnv = append(currentEnv, envs[i])
	}
	container["env"] = currentEnv
}

// modifyVolumeMounts configures volume mounts for the container based on workspace and workload specifications.
// It includes shared memory volumes, workspace volumes, host path volumes and secret with default-type of workload.
func modifyVolumeMounts(container map[string]interface{}, workload *v1.Workload, workspace *v1.Workspace) {
	var volumeMounts []interface{}
	volumeMountObjs, ok := container["volumeMounts"]
	if ok {
		volumeMounts = volumeMountObjs.([]interface{})
	}
	if !commonworkload.IsCICDScalingRunnerSet(workload) {
		volumeMounts = append(volumeMounts, buildVolumeMount(SharedMemoryVolume, "/dev/shm", "", false))
	}
	maxId := 0
	if workspace != nil {
		for _, vol := range workspace.Spec.Volumes {
			if vol.Id > maxId {
				maxId = vol.Id
			}
			if vol.MountPath != "" {
				volumeMount := buildVolumeMount(vol.GenFullVolumeId(), vol.MountPath, vol.SubPath, false)
				volumeMounts = append(volumeMounts, volumeMount)
			}
		}
	}
	for _, hostpath := range workload.Spec.Hostpath {
		maxId++
		volumeName := v1.GenFullVolumeId(v1.HOSTPATH, maxId)
		volumeMount := buildVolumeMount(volumeName, hostpath, "", false)
		volumeMounts = append(volumeMounts, volumeMount)
	}
	for _, secret := range workload.Spec.Secrets {
		if secret.Type != v1.SecretGeneral {
			continue
		}
		mountPath := fmt.Sprintf("/etc/secrets/%s", secret.Id)
		volumeMount := buildVolumeMount(secret.Id, mountPath, "", true)
		volumeMounts = append(volumeMounts, volumeMount)
	}
	container["volumeMounts"] = volumeMounts
}

// modifyVolumes adds volume definitions to the Kubernetes object based on workspace and workload specifications.
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
		volumes = append(volumes, buildHostPathVolume(volumeName, hostpath))
		hasNewVolume = true
	}
	for _, secret := range workload.Spec.Secrets {
		if secret.Type == v1.SecretGeneral {
			volumes = append(volumes, buildSecretVolume(secret.Id))
			hasNewVolume = true
		}
	}
	if !hasNewVolume {
		return nil
	}
	if err = unstructured.SetNestedSlice(obj.Object, volumes, path...); err != nil {
		return err
	}
	return nil
}

// modifyImageSecrets adds image pull secrets to the Kubernetes object based on workload configuration.
func modifyImageSecrets(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	secrets, _, err := unstructured.NestedSlice(obj.Object, path...)
	if err != nil {
		return err
	}
	for _, s := range workload.Spec.Secrets {
		if s.Type == v1.SecretImage {
			secrets = append(secrets, buildImageSecret(s.Id))
		}
	}
	if err = unstructured.SetNestedSlice(obj.Object, secrets, path...); err != nil {
		return err
	}
	return nil
}

// modifySecurityContext configures the security context for OpsJob preflight operations.
// Sets privileged mode for preflight checks.
func modifySecurityContext(container map[string]interface{}, workload *v1.Workload) {
	if v1.GetOpsJobType(workload) == string(v1.OpsJobPreflightType) {
		container["securityContext"] = map[string]interface{}{
			"privileged": true,
		}
	}
}

// modifyPriorityClass sets the priority class for the workload based on its specification.
func modifyPriorityClass(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	priorityClass := commonworkload.GeneratePriorityClass(workload)
	if err := unstructured.SetNestedField(obj.Object, priorityClass, path...); err != nil {
		return err
	}
	return nil
}

// modifyHostNetwork enables or disables host networking based on workload annotations.
func modifyHostNetwork(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	isEnableHostNetwork := v1.IsEnableHostNetwork(workload)
	if err := unstructured.SetNestedField(obj.Object, isEnableHostNetwork, path...); err != nil {
		return err
	}
	return nil
}

// modifyByOpsJob configures host PID and IPC settings for OpsJob preflight operations.
func modifyByOpsJob(obj *unstructured.Unstructured, workload *v1.Workload, templatePath []string) error {
	if v1.GetOpsJobType(workload) != string(v1.OpsJobPreflightType) {
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

// modifyStrategy configures deployment update strategy for Deployment workloads.
func modifyStrategy(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	if workload.SpecKind() != common.DeploymentKind {
		return nil
	}
	rollingUpdate := buildStrategy(workload)
	if len(rollingUpdate) == 0 {
		return nil
	}
	if err := unstructured.SetNestedMap(obj.Object, rollingUpdate, path...); err != nil {
		return err
	}
	return nil
}

// modifySelector sets the selector for service objects to match the workload.
func modifySelector(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	selector := buildSelector(workload)
	if err := unstructured.SetNestedMap(obj.Object, selector, path...); err != nil {
		return err
	}
	return nil
}

// modifyTolerations adds tolerations to tolerate all taints when IsTolerateAll is enabled.
func modifyTolerations(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	if !workload.Spec.IsTolerateAll {
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

// buildCommands constructs the command array for executing the workload entry point.
func buildCommands(workload *v1.Workload) []interface{} {
	return []interface{}{"/bin/sh", "-c", buildEntryPoint(workload)}
}

// buildEntryPoint constructs the command entry point for a workload.
func buildEntryPoint(workload *v1.Workload) string {
	result := ""
	switch workload.SpecKind() {
	case common.CICDScaleRunnerSetKind:
		result = workload.Spec.EntryPoint
	case common.CICDEphemeralRunnerKind, common.JobKind:
		result = stringutil.Base64Decode(workload.Spec.EntryPoint)
	default:
		result = Launcher + " '" + workload.Spec.EntryPoint + "'"
	}
	return result
}

// buildLabels creates a map of labels for object tracking.
func buildLabels(workload *v1.Workload) map[string]interface{} {
	result := map[string]interface{}{
		v1.WorkloadIdLabel:          workload.Name,
		v1.WorkloadDispatchCntLabel: buildDispatchCount(workload),
	}
	for key, value := range workload.Labels {
		if !strings.HasPrefix(key, v1.PrimusSafePrefix) {
			result[key] = value
		}
	}
	return result
}

// buildAnnotations creates a map of annotations for object tracking.
func buildAnnotations(workload *v1.Workload) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range workload.Annotations {
		if !strings.HasPrefix(key, v1.PrimusSafePrefix) {
			result[key] = value
		}
	}
	if v1.GetUserName(workload) != "" {
		result[v1.UserNameAnnotation] = v1.GetUserName(workload)
	}
	return result
}

// buildResources constructs resource requirements for the workload container.
func buildResources(resourceList corev1.ResourceList) map[string]interface{} {
	result := make(map[string]interface{})
	for key, val := range resourceList {
		result[string(key)] = val.String()
	}
	return result
}

// buildEnvironment creates environment variables for the workload container.
func buildEnvironment(workload *v1.Workload) []interface{} {
	var result []interface{}
	if workload.Spec.IsSupervised {
		result = addEnvVar(result, workload, "ENABLE_SUPERVISE", v1.TrueStr)
		if commonconfig.GetWorkloadHangCheckInterval() > 0 {
			result = addEnvVar(result, workload, "HANG_CHECK_INTERVAL",
				strconv.Itoa(commonconfig.GetWorkloadHangCheckInterval()))
		}
	}
	if workload.Spec.Resource.GPU != "" {
		result = addEnvVar(result, workload, "GPUS_PER_NODE", workload.Spec.Resource.GPU)
	}
	result = addEnvVar(result, workload, "WORKLOAD_ID", workload.Name)
	result = addEnvVar(result, workload, "WORKLOAD_KIND", workload.SpecKind())
	result = addEnvVar(result, workload, "DISPATCH_COUNT", strconv.Itoa(v1.GetWorkloadDispatchCnt(workload)+1))
	if workload.Spec.SSHPort > 0 {
		result = addEnvVar(result, workload, "SSH_PORT", strconv.Itoa(workload.Spec.SSHPort))
	}
	return result
}

func addEnvVar(result []interface{}, workload *v1.Workload, name, value string) []interface{} {
	_, ok := workload.Spec.Env[name]
	if ok {
		return result
	}
	return append(result, map[string]interface{}{
		"name":  name,
		"value": value,
	})
}

// buildPorts constructs port definitions for the workload container.
func buildPorts(workload *v1.Workload) []interface{} {
	jobPort := map[string]interface{}{
		"containerPort": int64(workload.Spec.JobPort),
		"protocol":      "TCP",
	}
	kind := workload.SpecKind()
	if kind == common.PytorchJobKind || kind == common.AuthoringKind || kind == common.UnifiedJobKind {
		jobPort["name"] = common.PytorchJobPortName
	}
	sshPort := map[string]interface{}{
		"containerPort": int64(workload.Spec.SSHPort),
		"protocol":      "TCP",
		"name":          common.SSHPortName,
	}
	return []interface{}{jobPort, sshPort}
}

// buildHealthCheck creates a health check probe configuration.
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

// buildVolumeMount creates a volume mount definition.
func buildVolumeMount(name, mountPath, subPath string, readOnly bool) interface{} {
	volMount := map[string]interface{}{
		"mountPath": mountPath,
		"name":      name,
		"readOnly":  readOnly,
	}
	if subPath != "" {
		volMount["subPath"] = subPath
	}
	return volMount
}

// buildHostPathVolume creates a host path volume definition.
func buildHostPathVolume(volumeName, hostPath string) interface{} {
	return map[string]interface{}{
		"hostPath": map[string]interface{}{
			"path": hostPath,
		},
		"name": volumeName,
	}
}

// buildPvcVolume creates a persistent volume claim volume definition.
func buildPvcVolume(volumeName string) interface{} {
	return map[string]interface{}{
		"persistentVolumeClaim": map[string]interface{}{
			"claimName": volumeName,
		},
		"name": volumeName,
	}
}

// buildSecretVolume creates a volume definition for a Kubernetes secret.
// This allows containers to mount the secret data as a volume.
func buildSecretVolume(secretName string) interface{} {
	return map[string]interface{}{
		"secret": map[string]interface{}{
			"secretName": secretName,
		},
		"name": secretName,
	}
}

// buildMatchExpression creates node selector match expressions based on workload specifications.
func buildMatchExpression(workload *v1.Workload) []interface{} {
	var result []interface{}
	if workload.Spec.Workspace != corev1.NamespaceDefault {
		result = append(result, map[string]interface{}{
			"key":      v1.WorkspaceIdLabel,
			"operator": "In",
			"values":   []interface{}{workload.Spec.Workspace},
		})
	}
	for key, val := range workload.Spec.CustomerLabels {
		var values []interface{}
		parts := strings.Fields(val)
		for i := range parts {
			values = append(values, parts[i])
		}
		if key == common.ExcludedNodes {
			result = append(result, map[string]interface{}{
				"key":      v1.K8sHostName,
				"operator": "NotIn",
				"values":   values,
			})
		} else {
			result = append(result, map[string]interface{}{
				"key":      key,
				"operator": "In",
				"values":   values,
			})
		}
	}
	return result
}

// buildSharedMemoryVolume creates an emptyDir volume with memory medium for shared memory.
func buildSharedMemoryVolume(sizeLimit string) interface{} {
	return map[string]interface{}{
		"emptyDir": map[string]interface{}{
			"medium":    string(corev1.StorageMediumMemory),
			"sizeLimit": sizeLimit,
		},
		"name": SharedMemoryVolume,
	}
}

// buildStrategy creates deployment strategy configuration.
func buildStrategy(workload *v1.Workload) map[string]interface{} {
	keys := []string{"maxSurge", "maxUnavailable"}
	rollingUpdate := make(map[string]interface{})
	for _, key := range keys {
		rollingUpdate[key] = workload.Spec.Service.Extends[key]
	}
	if len(rollingUpdate) == 0 {
		return nil
	}
	return map[string]interface{}{
		"type":          "RollingUpdate",
		"rollingUpdate": rollingUpdate,
	}
}

// buildSelector creates label selector for matching pods.
func buildSelector(workload *v1.Workload) map[string]interface{} {
	return map[string]interface{}{
		"matchLabels": map[string]interface{}{
			v1.WorkloadIdLabel: workload.Name,
		},
	}
}

// buildImageSecret creates an image pull secret reference.
func buildImageSecret(secretId string) interface{} {
	return map[string]interface{}{
		"name": secretId,
	}
}

// convertEnvsToStringMap extracts name-value pairs from environment variable definitions.
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
