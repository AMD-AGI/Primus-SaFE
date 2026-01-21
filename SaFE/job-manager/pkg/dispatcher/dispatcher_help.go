/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/maps"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"

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
	workload *v1.Workload, workspace *v1.Workspace, resourceSpec *v1.ResourceSpec, resourceId int) error {
	_, found, err := jobutils.NestedField(obj.Object, resourceSpec.PrePaths)
	if err != nil || !found {
		return nil
	}
	templatePath := resourceSpec.GetTemplatePath()

	path := append(templatePath, "spec",
		"affinity", "nodeAffinity", "requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms")
	if err = modifyNodeSelectorTerms(obj, workload, path); err != nil {
		return fmt.Errorf("failed to modify nodeSelectorTerms: %v", err.Error())
	}
	if v1.IsRequireNodeSpread(workload) {
		path = append(templatePath, "spec",
			"affinity", "podAntiAffinity", "requiredDuringSchedulingIgnoredDuringExecution")
		if err = modifyPodAntiAffinity(obj, workload, path); err != nil {
			return fmt.Errorf("failed to modify podAntiAffinity: %v", err.Error())
		}
	}
	path = append(templatePath, "spec", "containers")
	if err = modifyContainers(obj, workload, workspace, path, resourceId); err != nil {
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
	path = append(templatePath, "spec", "serviceAccountName")
	if err = modifyServiceAccountName(obj, workload, path); err != nil {
		return fmt.Errorf("failed to modify sa: %v", err.Error())
	}
	path = append(templatePath, "spec", "hostNetwork")
	if err = modifyHostNetwork(obj, workload, path, resourceId); err != nil {
		return fmt.Errorf("failed to modify host network: %v", err.Error())
	}
	path = append(templatePath, "spec", "tolerations")
	if err = modifyTolerations(obj, workload, path); err != nil {
		return fmt.Errorf("failed to modify tolerations: %v", err.Error())
	}
	if commonworkload.IsApplication(workload) {
		path = []string{"spec", "strategy"}
		if err = modifyStrategy(obj, workload, path); err != nil {
			return fmt.Errorf("failed to modify strategy: %v", err.Error())
		}
		path = []string{"spec", "selector"}
		if err = modifySelector(obj, workload, path); err != nil {
			return fmt.Errorf("failed to modify selector: %v", err.Error())
		}
	}
	if err = modifyHostPid(obj, workload, templatePath); err != nil {
		return fmt.Errorf("failed to modify by opsjob: %v", err.Error())
	}
	return nil
}

// modifyNodeSelectorTerms updates node selector terms in the object's node affinity configuration.
// It adds custom match expressions based on the workload specification.
func modifyNodeSelectorTerms(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	nodeSelectorTerms, _, err := jobutils.NestedSlice(obj.Object, path)
	if err != nil {
		return err
	}
	expression := buildMatchExpression(workload)
	if len(expression) == 0 {
		return nil
	}
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
	if err = jobutils.SetNestedField(obj.Object, nodeSelectorTerms, path); err != nil {
		return err
	}
	return nil
}

// modifyPodAntiAffinity adds pod anti-affinity configuration to spread pods across nodes.
// It configures requiredDuringSchedulingIgnoredDuringExecution with topology key kubernetes.io/hostname.
func modifyPodAntiAffinity(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	podAntiAffinityTerms, _, err := jobutils.NestedSlice(obj.Object, path)
	if err != nil {
		return err
	}
	antiAffinityTerm := map[string]interface{}{
		"labelSelector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				v1.WorkloadIdLabel: workload.Name,
			},
		},
		"topologyKey": "kubernetes.io/hostname",
	}
	podAntiAffinityTerms = append(podAntiAffinityTerms, antiAffinityTerm)
	if err = jobutils.SetNestedField(obj.Object, podAntiAffinityTerms, path); err != nil {
		return err
	}
	return nil
}

// modifyContainers configures the containers of a workload with environment variables,
// volume mounts, security context, ports, and health checks based on the workload specification.
func modifyContainers(obj *unstructured.Unstructured,
	workload *v1.Workload, workspace *v1.Workspace, path []string, resourceId int) error {
	containers, found, err := jobutils.NestedSlice(obj.Object, path)
	if err != nil {
		return err
	}
	if !found || len(containers) == 0 {
		return fmt.Errorf("failed to find container with path: %v", path)
	}
	env := buildEnvironment(workload, resourceId)
	mainContainerName := v1.GetMainContainer(workload)
	for i := range containers {
		container := containers[i].(map[string]interface{})
		modifyEnv(container, env, workload.Spec.Resources[resourceId].RdmaResource != "")
		modifyVolumeMounts(container, workload, workspace)
		modifyPrivilegedSecurity(container, workload)

		name := jobutils.NestedStringSilently(container, []string{"name"})
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
	if err = jobutils.SetNestedField(obj.Object, containers, path); err != nil {
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
	volumeMounts = append(volumeMounts, buildVolumeMount(SharedMemoryVolume, "/dev/shm", "", false))
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
		mountPath := fmt.Sprintf("%s/%s", common.SecretPath, secret.Id)
		volumeMount := buildVolumeMount(secret.Id, mountPath, "", true)
		volumeMounts = append(volumeMounts, volumeMount)
	}
	container["volumeMounts"] = volumeMounts
}

// modifyVolumes adds volume definitions to the Kubernetes object based on workspace and workload specifications.
func modifyVolumes(obj *unstructured.Unstructured, workload *v1.Workload, workspace *v1.Workspace, path []string) error {
	volumes, _, err := jobutils.NestedSlice(obj.Object, path)
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
	if err = jobutils.SetNestedField(obj.Object, volumes, path); err != nil {
		return err
	}
	return nil
}

// modifyImageSecrets adds image pull secrets to the Kubernetes object based on workload configuration.
func modifyImageSecrets(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	secrets, _, err := jobutils.NestedSlice(obj.Object, path)
	if err != nil {
		return err
	}
	if len(workload.Spec.Secrets) == 0 {
		return nil
	}
	for _, s := range workload.Spec.Secrets {
		if s.Type == v1.SecretImage {
			secrets = append(secrets, buildImageSecret(s.Id))
		}
	}
	if err = jobutils.SetNestedField(obj.Object, secrets, path); err != nil {
		return err
	}
	return nil
}

// modifyPrivilegedSecurity configures the security context for OpsJob operations and privileged workloads.
// For OpsJob types, runs as root user. For privileged workloads, also sets privileged mode.
func modifyPrivilegedSecurity(container map[string]interface{}, workload *v1.Workload) {
	if v1.GetOpsJobType(workload) == "" && !v1.IsPrivileged(workload) {
		return
	}
	// All OpsJobs run as root
	securityContext := map[string]interface{}{
		"runAsUser":  int64(0),
		"runAsGroup": int64(0),
	}
	if v1.IsPrivileged(workload) {
		securityContext["privileged"] = true
	}
	container["securityContext"] = securityContext
}

// modifyPriorityClass sets the priority class for the workload based on its specification.
func modifyPriorityClass(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	priorityClass := commonworkload.GeneratePriorityClass(workload)
	if err := jobutils.SetNestedField(obj.Object, priorityClass, path); err != nil {
		return err
	}
	return nil
}

// modifyServiceAccountName sets the service account name for the workload based on its specification.
func modifyServiceAccountName(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	if v1.GetOpsJobType(workload) == string(v1.OpsJobCDType) {
		if err := jobutils.SetNestedField(obj.Object, common.PrimusSafeName, path); err != nil {
			return err
		}
	}
	return nil
}

// modifyHostNetwork enables or disables host networking based on workload annotations.
func modifyHostNetwork(obj *unstructured.Unstructured, workload *v1.Workload, path []string, resourceId int) error {
	isEnableHostNetwork := workload.Spec.Resources[resourceId].RdmaResource != ""
	if err := jobutils.SetNestedField(obj.Object, isEnableHostNetwork, path); err != nil {
		return err
	}
	return nil
}

// modifyHostPid configures host PID and IPC settings for OpsJob preflight operations.
func modifyHostPid(obj *unstructured.Unstructured, workload *v1.Workload, templatePath []string) error {
	if !v1.IsPrivileged(workload) {
		return nil
	}
	path := append(templatePath, "spec", "hostPID")
	if err := jobutils.SetNestedField(obj.Object, true, path); err != nil {
		return err
	}
	path = append(templatePath, "spec", "hostIPC")
	if err := jobutils.SetNestedField(obj.Object, true, path); err != nil {
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
	if err := jobutils.SetNestedField(obj.Object, rollingUpdate, path); err != nil {
		return err
	}
	return nil
}

// modifySelector sets the selector for service objects to match the workload.
func modifySelector(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	selector := buildSelector(workload)
	if len(selector) == 0 {
		return nil
	}
	if err := jobutils.SetNestedField(obj.Object, selector, path); err != nil {
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
	if err := jobutils.SetNestedField(obj.Object, tolerations, path); err != nil {
		return err
	}
	return nil
}

// buildCommands constructs the command array for executing the workload entry point.
func buildCommands(workload *v1.Workload, id int) []interface{} {
	return []interface{}{"/bin/sh", "-c", buildEntryPoint(workload, id)}
}

// buildEntryPoint constructs the command entry point for a workload.
func buildEntryPoint(workload *v1.Workload, id int) string {
	if workload.Spec.EntryPoints[id] == "" {
		return ""
	}
	result := ""
	switch workload.SpecKind() {
	case common.CICDScaleRunnerSetKind:
		result = workload.Spec.EntryPoints[id]
	default:
		result = Launcher + " '" + workload.Spec.EntryPoints[id] + "'"
	}
	return result
}

// buildObjectLabels creates a map of labels for object tracking.
func buildObjectLabels(workload *v1.Workload) map[string]interface{} {
	result := map[string]interface{}{
		v1.WorkloadIdLabel:          getRootWorkloadId(workload),
		v1.WorkloadDispatchCntLabel: buildDispatchCount(workload),
	}
	for key, value := range workload.Labels {
		if !strings.HasPrefix(key, v1.PrimusSafePrefix) {
			result[key] = value
		}
	}
	return result
}

// buildObjectAnnotations creates a map of annotations for object tracking.
func buildObjectAnnotations(workload *v1.Workload) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range workload.Annotations {
		if !strings.HasPrefix(key, v1.PrimusSafePrefix) {
			result[key] = value
		}
	}
	if v1.GetUserName(workload) != "" {
		result[v1.UserNameAnnotation] = v1.GetUserName(workload)
	}
	if v1.GetGroupId(workload) != "" {
		result[v1.GroupIdAnnotation] = v1.GetGroupId(workload)
	}
	return result
}

// buildPodLabels creates a map of labels for pod of k8s object.
func buildPodLabels(workload *v1.Workload) map[string]interface{} {
	result := buildObjectLabels(workload)
	result[v1.K8sObjectIdLabel] = workload.Name
	return result
}

// buildPodAnnotations creates a map of annotations for pod of k8s object.
func buildPodAnnotations(workload *v1.Workload, resourceId int) map[string]interface{} {
	result := buildObjectAnnotations(workload)
	result[v1.ResourceIdAnnotation] = strconv.Itoa(resourceId)
	if v1.GetMainContainer(workload) != "" {
		result[v1.MainContainerAnnotation] = v1.GetMainContainer(workload)
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
func buildEnvironment(workload *v1.Workload, resourceId int) []interface{} {
	var result []interface{}
	if workload.Spec.IsSupervised {
		result = addEnvVar(result, workload, "ENABLE_SUPERVISE", v1.TrueStr)
		if commonconfig.GetWorkloadHangCheckInterval() > 0 {
			result = addEnvVar(result, workload, "HANG_CHECK_INTERVAL",
				strconv.Itoa(commonconfig.GetWorkloadHangCheckInterval()))
		}
	}
	if workload.Spec.Resources[resourceId].GPU != "" {
		result = addEnvVar(result, workload, "GPUS_PER_NODE", workload.Spec.Resources[resourceId].GPU)
	}
	result = addEnvVar(result, workload, "WORKLOAD_ID", getRootWorkloadId(workload))
	result = addEnvVar(result, workload, "WORKLOAD_KIND", workload.SpecKind())
	result = addEnvVar(result, workload, "DISPATCH_COUNT", strconv.Itoa(v1.GetWorkloadDispatchCnt(workload)+1))
	if workload.Spec.SSHPort > 0 {
		result = addEnvVar(result, workload, "SSH_PORT", strconv.Itoa(workload.Spec.SSHPort))
	}
	if commonworkload.IsAuthoring(workload) {
		result = addEnvVar(result, workload, jobutils.AdminControlPlaneEnv, v1.GetAdminControlPlane(workload))
	}
	return result
}

func addEnvVar(result []interface{}, workload *v1.Workload, name, value string) []interface{} {
	if len(workload.Spec.Env) > 0 {
		if _, ok := workload.Spec.Env[name]; ok {
			return result
		}
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
	if kind == common.PytorchJobKind || kind == common.AuthoringKind ||
		kind == common.UnifiedJobKind || kind == common.TorchFTKind {
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
			"type": "Directory",
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
			v1.K8sObjectIdLabel: workload.Name,
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

// updateReplica updates the replica count in the unstructured object.
func updateReplica(adminWorkload *v1.Workload,
	obj *unstructured.Unstructured, resourceSpec v1.ResourceSpec, id int) error {
	if len(resourceSpec.ReplicasPaths) == 0 {
		return nil
	}
	replica := int64(adminWorkload.Spec.Resources[id].Replica)
	path := resourceSpec.PrePaths
	path = append(path, resourceSpec.ReplicasPaths...)
	if err := jobutils.SetNestedField(obj.Object, replica, path); err != nil {
		return err
	}
	if err := updateMinReplicas(obj, resourceSpec, replica); err != nil {
		return err
	}
	if err := updateMaxReplicas(obj, resourceSpec, replica); err != nil {
		return err
	}
	return nil
}

// updateMaxReplicas updates the max-replicas in the unstructured object. only for ray-job
// The current job's max-replicas is equal to its replicas, meaning elastic Ray clusters are not supported.
func updateMaxReplicas(obj *unstructured.Unstructured, resourceSpec v1.ResourceSpec, replica int64) error {
	if len(resourceSpec.MaxReplicasPaths) == 0 {
		return nil
	}
	path := resourceSpec.PrePaths
	path = append(path, resourceSpec.MaxReplicasPaths...)
	return jobutils.SetNestedField(obj.Object, replica, path)
}

// updateMinReplicas updates the min-replicas(for job, it's completions count) in the unstructured object. only for job or ray-job
// The current job's min-replicas is equal to its replicas, meaning all tasks run concurrently and all must succeed.
func updateMinReplicas(obj *unstructured.Unstructured, resourceSpec v1.ResourceSpec, replica int64) error {
	if len(resourceSpec.MinReplicasPaths) == 0 {
		return nil
	}
	path := resourceSpec.PrePaths
	path = append(path, resourceSpec.MinReplicasPaths...)
	return jobutils.SetNestedField(obj.Object, replica, path)
}

// updateCICDScaleSet updates the CICD scale set configuration in the unstructured object.
// It first updates the GitHub configuration, then conditionally updates the environments for build
// or removes unnecessary containers based on whether CICD unified build is enabled.
// Returns an error if no resource templates are found or if any update operation fails.
func updateCICDScaleSet(obj *unstructured.Unstructured,
	adminWorkload *v1.Workload, workspace *v1.Workspace, rt *v1.ResourceTemplate) error {
	if len(rt.Spec.ResourceSpecs) == 0 {
		return fmt.Errorf("no resource template found")
	}
	if err := updateCICDGithub(adminWorkload, obj); err != nil {
		return err
	}
	if err := updateCICDScaleSetEnvs(obj, adminWorkload, workspace, rt.Spec.ResourceSpecs[0]); err != nil {
		return err
	}
	return nil
}

// updateCICDEphemeralRunner updates the CICD ephemeral runner configuration
func updateCICDEphemeralRunner(ctx context.Context, clientSets *syncer.ClusterClientSets,
	obj *unstructured.Unstructured, adminWorkload *v1.Workload, rt *v1.ResourceTemplate) error {
	if len(rt.Spec.ResourceSpecs) == 0 {
		return fmt.Errorf("no resource template found")
	}
	if err := updateCICDGithub(adminWorkload, obj); err != nil {
		return err
	}
	// Set owner reference to the parent scale runner if CICDScaleRunnerIdLabel is present
	if scaleRunnerId := v1.GetLabel(adminWorkload, v1.CICDScaleRunnerIdLabel); scaleRunnerId != "" {
		if clientSets != nil && !commonutils.HasOwnerReferences(obj, scaleRunnerId) {
			ownerObj, err := jobutils.GetObject(ctx,
				clientSets.ClientFactory(), scaleRunnerId, adminWorkload.Spec.Workspace, rt.ToSchemaGVK())
			if err != nil {
				return fmt.Errorf("failed to get owner scale runner: %v", err.Error())
			}
			ownerRef := metav1.OwnerReference{
				APIVersion:         ownerObj.GetAPIVersion(),
				Kind:               ownerObj.GetKind(),
				Name:               ownerObj.GetName(),
				UID:                ownerObj.GetUID(),
				BlockOwnerDeletion: pointer.Bool(true),
				Controller:         pointer.Bool(true),
			}
			obj.SetOwnerReferences([]metav1.OwnerReference{ownerRef})
		}
	}
	return nil
}

// updateCICDGithub updates the CICD scale set configuration in the unstructured object.
// It updates the GitHub configuration and then configures environment variables based on unified build settings.
// Returns an error if no resource templates are found or if any update operation fails.
func updateCICDGithub(adminWorkload *v1.Workload, obj *unstructured.Unstructured) error {
	specObject, ok, err := jobutils.NestedMap(obj.Object, []string{"spec"})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("failed to find object with path: [spec]")
	}
	if v1.GetGithubSecretId(adminWorkload) == "" || len(adminWorkload.Spec.Env) == 0 ||
		adminWorkload.Spec.Env[common.GithubConfigUrl] == "" {
		return fmt.Errorf("github config is not set")
	}

	specObject["githubConfigSecret"] = v1.GetGithubSecretId(adminWorkload)
	specObject["githubConfigUrl"] = adminWorkload.Spec.Env[common.GithubConfigUrl]
	if commonworkload.IsCICDEphemeralRunner(adminWorkload) {
		if runnerSetId := v1.GetCICDRunnerScaleSetId(adminWorkload); runnerSetId != "" {
			specObject["runnerScaleSetId"], err = strconv.ParseInt(runnerSetId, 10, 0)
			if err != nil {
				return fmt.Errorf("invalid runner scale set id %s", runnerSetId)
			}
		}
	}
	if err = jobutils.SetNestedField(obj.Object, specObject, []string{"spec"}); err != nil {
		return err
	}
	return nil
}

// updateCICDScaleSetEnvs configures environment variables for CICD workloads based on unified build settings.
// When unified build is enabled, it updates all containers with NFS paths and environment variables,
// When unified build is disabled, it keeps only the main container with environment variables.
func updateCICDScaleSetEnvs(obj *unstructured.Unstructured,
	adminWorkload *v1.Workload, workspace *v1.Workspace, resourceSpec v1.ResourceSpec) error {
	containers, path, err := getContainers(obj, resourceSpec)
	if err != nil {
		return err
	}
	envs := maps.Copy(adminWorkload.Spec.Env)
	envs[jobutils.UserIdEnv] = v1.GetUserId(adminWorkload)
	envs[jobutils.PriorityEnv] = strconv.Itoa(adminWorkload.Spec.Priority)
	envs[jobutils.WorkspaceIdEnv] = adminWorkload.Spec.Workspace
	envs[jobutils.AdminControlPlaneEnv] = v1.GetAdminControlPlane(adminWorkload)
	envs[jobutils.GithubSecretEnv] = v1.GetGithubSecretId(adminWorkload)
	envs[common.ScaleRunnerSetID] = adminWorkload.Name

	val := ""
	if len(adminWorkload.Spec.Env) > 0 {
		val, _ = adminWorkload.Spec.Env[common.UnifiedJobEnable]
	}
	if val == v1.TrueStr {
		pfsPath := getNfsPathFromWorkspace(workspace)
		if pfsPath == "" {
			return fmt.Errorf("failed to get NFS path from workspace")
		}
		envs[jobutils.NfsPathEnv] = pfsPath + "/cicd"
		envs[jobutils.NfsInputEnv] = UnifiedJobInput
		envs[jobutils.NfsOutputEnv] = UnifiedJobOutput
		// When unified build is enabled, update all containers with envs
		for i := range containers {
			container := containers[i].(map[string]interface{})
			updateContainerEnv(envs, container, nil)
		}
		if err = jobutils.SetNestedField(obj.Object, containers, path); err != nil {
			return err
		}
	} else {
		mainContainerName := v1.GetMainContainer(adminWorkload)
		// When unified build is disabled, keep only main container with resource variables
		for i := range containers {
			container := containers[i].(map[string]interface{})
			name := jobutils.NestedStringSilently(container, []string{"name"})
			if name == mainContainerName {
				updateContainerEnv(envs, container, nil)
				// Keep only the main container and remove other container
				newContainers := []interface{}{container}
				return jobutils.SetNestedField(obj.Object, newContainers, path)
			}
		}
		return fmt.Errorf("no main container found")
	}
	return nil
}

// updateMetadata updates the template metadata annotations in the unstructured object.
func updateMetadata(adminWorkload *v1.Workload,
	obj *unstructured.Unstructured, resourceSpec v1.ResourceSpec, id int) error {
	_, found, err := jobutils.NestedMap(obj.Object, resourceSpec.GetTemplatePath())
	if err != nil || !found {
		return err
	}
	labels := buildPodLabels(adminWorkload)
	path := append(resourceSpec.GetTemplatePath(), "metadata", "labels")
	if err = jobutils.SetNestedField(obj.Object, labels, path); err != nil {
		return err
	}

	if id2, ok := v1.GetResourceId(adminWorkload); ok {
		id = id2
	}
	annotations := buildPodAnnotations(adminWorkload, id)
	path = append(resourceSpec.GetTemplatePath(), "metadata", "annotations")
	if err = jobutils.SetNestedField(obj.Object, annotations, path); err != nil {
		return err
	}
	return nil
}

// updateContainers updates all container configurations in the unstructured object.
// For each container, it updates environment variables. For the main container,
// it also updates resources, image, and command based on the workload spec.
func updateContainers(adminWorkload *v1.Workload,
	obj *unstructured.Unstructured, resourceSpec v1.ResourceSpec, id int) error {
	containers, path, err := getContainers(obj, resourceSpec)
	if err != nil {
		return err
	}

	mainContainerName := v1.GetMainContainer(adminWorkload)
	res := &adminWorkload.Spec.Resources[id]
	resourceList, err := quantity.CvtToResourceList(res.CPU, res.Memory, res.GPU,
		res.GPUName, res.EphemeralStorage, res.RdmaResource, 1.0/float64(len(containers)))
	if err != nil {
		return err
	}

	resources := buildResources(resourceList)
	for i := range containers {
		container := containers[i].(map[string]interface{})
		updateContainerEnv(adminWorkload.Spec.Env, container, v1.GetEnvToBeRemoved(adminWorkload))
		container["resources"] = map[string]interface{}{
			"limits":   resources,
			"requests": resources,
		}
		name := jobutils.NestedStringSilently(container, []string{"name"})
		if name == mainContainerName {
			if len(adminWorkload.Spec.Images) > id && adminWorkload.Spec.Images[id] != "" {
				container["image"] = adminWorkload.Spec.Images[id]
			}
			if len(adminWorkload.Spec.EntryPoints) > id && adminWorkload.Spec.EntryPoints[id] != "" {
				container["command"] = buildCommands(adminWorkload, id)
			}
		}
	}
	if err = jobutils.SetNestedField(obj.Object, containers, path); err != nil {
		return err
	}
	return nil
}

// updateContainerEnv updates environment variables in the container.
func updateContainerEnv(envs map[string]string, container map[string]interface{}, toBeRemovedKeys []string) {
	if len(envs) == 0 && len(toBeRemovedKeys) == 0 {
		return
	}
	var existingEnvs []interface{}
	if obj, ok := container["env"]; ok {
		existingEnvs = obj.([]interface{})
	}

	toBeRemovedKeySet := sets.NewSetByKeys(toBeRemovedKeys...)
	isChanged := false
	updatedEnvs := make([]interface{}, 0, len(existingEnvs))
	existingEnvNames := sets.NewSet()
	for _, envItem := range existingEnvs {
		env, ok := envItem.(map[string]interface{})
		if !ok {
			continue
		}
		name, ok := env["name"]
		if !ok {
			continue
		}
		nameStr := name.(string)
		if toBeRemovedKeySet.Has(nameStr) {
			isChanged = true
			continue
		}
		existingEnvNames.Insert(nameStr)

		if newValue, exists := envs[nameStr]; exists {
			currentValue, valueOk := env["value"]
			if valueOk && newValue != currentValue.(string) {
				isChanged = true
				updatedEnvs = append(updatedEnvs, map[string]interface{}{
					"name":  nameStr,
					"value": newValue,
				})
			} else {
				updatedEnvs = append(updatedEnvs, envItem)
			}
		} else {
			updatedEnvs = append(updatedEnvs, envItem)
		}
	}

	for key, val := range envs {
		if !existingEnvNames.Has(key) {
			isChanged = true
			updatedEnvs = append(updatedEnvs, map[string]interface{}{
				"name":  key,
				"value": val,
			})
		}
	}
	if isChanged {
		container["env"] = updatedEnvs
	}
}

// updateSharedMemory updates the shared memory volume configuration.
func updateSharedMemory(adminWorkload *v1.Workload, obj *unstructured.Unstructured, resourceSpec v1.ResourceSpec, id int) error {
	path := resourceSpec.PrePaths
	path = append(path, resourceSpec.TemplatePaths...)
	path = append(path, "spec", "volumes")
	volumes, found, err := jobutils.NestedSlice(obj.Object, path)
	if err != nil {
		return err
	}
	if !found {
		sharedMemoryVolume := buildSharedMemoryVolume(adminWorkload.Spec.Resources[id].SharedMemory)
		volumes = []interface{}{sharedMemoryVolume}
		if err = jobutils.SetNestedField(obj.Object, volumes, path); err != nil {
			return err
		}
		return nil
	}

	sharedMemory := jobutils.GetMemoryStorageVolume(volumes)
	if sharedMemory != nil {
		sharedMemory["sizeLimit"] = adminWorkload.Spec.Resources[id].SharedMemory
		if err = jobutils.SetNestedField(obj.Object, volumes, path); err != nil {
			return err
		}
	} else {
		volumes = append(volumes, buildSharedMemoryVolume(adminWorkload.Spec.Resources[id].SharedMemory))
		if err = jobutils.SetNestedField(obj.Object, volumes, path); err != nil {
			return err
		}
	}
	return nil
}

// updateHostNetwork updates the host network configuration.
func updateHostNetwork(adminWorkload *v1.Workload,
	obj *unstructured.Unstructured, resourceSpec v1.ResourceSpec, resourceId int) error {
	templatePath := resourceSpec.GetTemplatePath()
	path := append(templatePath, "spec", "hostNetwork")
	return modifyHostNetwork(obj, adminWorkload, path, resourceId)
}

// updatePriorityClass updates the priority class configuration.
func updatePriorityClass(adminWorkload *v1.Workload,
	obj *unstructured.Unstructured, resourceSpec v1.ResourceSpec) error {
	templatePath := resourceSpec.GetTemplatePath()
	path := append(templatePath, "spec", "priorityClassName")
	return modifyPriorityClass(obj, adminWorkload, path)
}

// getContainers retrieves the containers slice and its path from the unstructured object based on the resource specification.
// Returns the containers slice, the path to the containers field, and an error if the operation fails or no containers are found.
func getContainers(obj *unstructured.Unstructured, resourceSpec v1.ResourceSpec) ([]interface{}, []string, error) {
	templatePath := resourceSpec.GetTemplatePath()
	path := append(templatePath, "spec", "containers")
	containers, found, err := jobutils.NestedSlice(obj.Object, path)
	if err != nil {
		return nil, nil, err
	}
	if !found || len(containers) == 0 {
		return nil, nil, fmt.Errorf("failed to find container with path: %v", path)
	}
	return containers, path, nil
}

// getNfsPathFromWorkspace retrieves the NFS path from the workspace's volumes.
// It prioritizes PFS type volumes, otherwise falls back to the first available volume's mount path.
func getNfsPathFromWorkspace(workspace *v1.Workspace) string {
	result := ""
	for _, vol := range workspace.Spec.Volumes {
		if vol.Type == v1.PFS {
			result = vol.MountPath
			break
		}
	}
	if result == "" && len(workspace.Spec.Volumes) > 0 {
		result = workspace.Spec.Volumes[0].MountPath
	}
	return result
}

func getRootWorkloadId(workload *v1.Workload) string {
	rootWorkloadId := v1.GetRootWorkloadId(workload)
	if rootWorkloadId == "" {
		rootWorkloadId = workload.Name
	}
	return rootWorkloadId
}

// buildDispatchCount generates the dispatch count as a string.
func buildDispatchCount(w *v1.Workload) string {
	// The count for the first dispatch is 1, so it needs to be incremented by 1 here.
	return strconv.Itoa(v1.GetWorkloadDispatchCnt(w) + 1)
}
