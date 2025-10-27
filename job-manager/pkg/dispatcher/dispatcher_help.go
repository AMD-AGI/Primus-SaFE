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
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const (
	SharedMemoryVolume = "shared-memory"
	Launcher           = "chmod +x /shared-data/launcher.sh; /bin/sh /shared-data/launcher.sh"
)

// modifyObjectOnCreation modifies various aspects of a Kubernetes object during workload creation.
// It applies labels, node selectors, container configurations, volumes, and other settings
// based on the admin workload specification and workspace configuration.
//
// Parameters:
//   - obj: The unstructured Kubernetes object to modify
//   - workload: The workload specification containing configuration details
//   - workspace: The workspace which the workload belongs to
//   - resourceSpec: The specification of resource template
//
// Returns:
//   - error: Any error encountered during modification, or nil on success
func modifyObjectOnCreation(obj *unstructured.Unstructured,
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
	if err = modifyMainContainer(obj, workload, workspace, path); err != nil {
		return fmt.Errorf("failed to modify main container: %v", err.Error())
	}
	path = append(templatePath, "spec", "volumes")
	if err = modifyVolumes(obj, workload, workspace, path); err != nil {
		return fmt.Errorf("failed to modify volumes: %v", err.Error())
	}
	path = append(templatePath, "spec", "imagePullSecrets")
	if err = modifyImageSecrets(obj, workload, workspace, path); err != nil {
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
	path = []string{"spec", "strategy"}
	if err = modifyStrategy(obj, workload, path); err != nil {
		return fmt.Errorf("failed to modify strategy: %v", err.Error())
	}
	if workload.Spec.Service != nil {
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
// returns:
//   - error: Any error encountered during label modification, or nil on success
func modifyLabels(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	labels := buildLabels(workload)
	return unstructured.SetNestedMap(obj.Object, labels, path...)
}

// modifyNodeSelectorTerms updates node selector terms in the object's node affinity configuration.
// It adds custom match expressions based on the workload specification.
// Returns:
//   - error: Any error encountered during modification, or nil on success
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

// modifyMainContainer configures the main container of a workload with environment variables,
// volume mounts, security context, ports, and health checks based on the workload specification.
//
// Parameters:
//   - obj: The unstructured Kubernetes object containing the container
//   - workload: The workload specification with container configuration
//   - workspace: The workspace providing additional context
//   - path: Path to the containers array in the object
//
// Returns:
//   - error: Any error encountered during container modification, or nil on success
func modifyMainContainer(obj *unstructured.Unstructured,
	workload *v1.Workload, workspace *v1.Workspace, path []string) error {
	containers, found, err := unstructured.NestedSlice(obj.Object, path...)
	if err != nil {
		return err
	}
	if !found || len(containers) == 0 {
		return fmt.Errorf("failed to find container with path: %v", path)
	}
	mainContainer, err := getMainContainer(containers, v1.GetMainContainer(workload))
	if err != nil {
		return err
	}
	env := buildEnvironment(workload)
	modifyEnv(mainContainer, env, v1.IsEnableHostNetwork(workload))
	modifyVolumeMounts(mainContainer, workload, workspace)
	modifySecurityContext(mainContainer, workload)
	mainContainer["ports"] = buildPorts(workload)
	if healthz := buildHealthCheck(workload.Spec.Liveness); healthz != nil {
		mainContainer["livenessProbe"] = healthz
	}
	if healthz := buildHealthCheck(workload.Spec.Readiness); healthz != nil {
		mainContainer["readinessProbe"] = healthz
	}
	if err = unstructured.SetNestedField(obj.Object, containers, path...); err != nil {
		return err
	}
	return nil
}

// modifyEnv updates environment variables in the main container.
// It handles special network interface names when host networking is not enabled.
//
// Parameters:
//   - mainContainer: The main container of pod to modify
//   - env: Environment variables to add
//   - isHostNetwork: Whether host networking is enabled
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

// modifyVolumeMounts configures volume mounts for the container based on workspace and workload specifications.
// It includes shared memory volumes, workspace volumes, and host path volumes of workload.
//
// Parameters:
//   - mainContainer: The main container of pod to modify
//   - workload: The workload specification
//   - workspace: The workspace providing volume context
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

// modifyVolumes adds volume definitions to the Kubernetes object based on workspace and workload specifications.
//
// Parameters:
//   - obj: The unstructured Kubernetes object to modify
//   - workload: The workload specification containing host path volumes
//   - workspace: The workspace providing additional volumes
//   - path: Path to the volumes array in the object
//
// Returns:
//   - error: Any error encountered during volume modification, or nil on success
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

// modifyImageSecrets adds image pull secrets to the Kubernetes object based on workspace configuration.
//
// Parameters:
//   - obj: The unstructured Kubernetes object to modify
//   - workload: The workload specification containing host path volumes
//   - workspace: The workspace providing image secrets
//   - path: Path to the imagePullSecrets array in the object
//
// Returns:
//   - error: Any error encountered during secret modification, or nil on success
func modifyImageSecrets(obj *unstructured.Unstructured, workload *v1.Workload, workspace *v1.Workspace, path []string) error {
	secrets, _, err := unstructured.NestedSlice(obj.Object, path...)
	if err != nil {
		return err
	}

	if workspace != nil {
		for _, s := range workspace.Spec.ImageSecrets {
			secrets = append(secrets, buildImageSecret(s.Name))
		}
	} else if commonconfig.GetImageSecret() != "" {
		imageSecret := commonutils.GenerateClusterSecret(v1.GetClusterId(workload), commonconfig.GetImageSecret())
		secrets = append(secrets, buildImageSecret(imageSecret))
	}
	if err = unstructured.SetNestedSlice(obj.Object, secrets, path...); err != nil {
		return err
	}
	return nil
}

// modifySecurityContext configures the security context for OpsJob preflight operations.
// Sets privileged mode for preflight checks.
//
// Parameters:
//   - mainContainer: The container map to modify
//   - workload: The workload specification to check for OpsJob type
func modifySecurityContext(mainContainer map[string]interface{}, workload *v1.Workload) {
	if v1.GetOpsJobType(workload) != string(v1.OpsJobPreflightType) {
		return
	}
	mainContainer["securityContext"] = map[string]interface{}{
		"privileged": true,
	}
}

// modifyPriorityClass sets the priority class for the workload based on its specification.
//
// Parameters:
//   - obj: The unstructured Kubernetes object to modify
//   - workload: The workload specification containing priority information
//   - path: Path to the priorityClassName field in the object
//
// Returns:
//   - error: Any error encountered during priority class modification, or nil on success
func modifyPriorityClass(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	priorityClass := commonworkload.GeneratePriorityClass(workload)
	if err := unstructured.SetNestedField(obj.Object, priorityClass, path...); err != nil {
		return err
	}
	return nil
}

// modifyHostNetwork enables or disables host networking based on workload annotations.
//
// Parameters:
//   - obj: The unstructured Kubernetes object to modify
//   - workload: The workload specification containing host network settings
//   - path: Path to the hostNetwork field in the object
//
// Returns:
//   - error: Any error encountered during host network modification, or nil on success
func modifyHostNetwork(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	isEnableHostNetwork := v1.IsEnableHostNetwork(workload)
	if err := unstructured.SetNestedField(obj.Object, isEnableHostNetwork, path...); err != nil {
		return err
	}
	return nil
}

// modifyByOpsJob configures host PID and IPC settings for OpsJob preflight operations.
//
// Parameters:
//   - obj: The unstructured Kubernetes object to modify
//   - workload: The workload specification to check for OpsJob type
//   - templatePath: Base path for the pod template specification
//
// Returns:
//   - error: Any error encountered during modification, or nil on success
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
//
// Parameters:
//   - obj: The unstructured Kubernetes object to modify
//   - workload: The workload specification containing strategy settings
//   - path: Path to the strategy field in the object
//
// Returns:
//   - error: Any error encountered during strategy modification, or nil on success
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
//
// Parameters:
//   - obj: The unstructured Kubernetes object to modify
//   - workload: The workload specification for generating selector
//   - path: Path to the selector field in the object
//
// Returns:
//   - error: Any error encountered during selector modification, or nil on success
func modifySelector(obj *unstructured.Unstructured, workload *v1.Workload, path []string) error {
	selector := buildSelector(workload)
	if err := unstructured.SetNestedMap(obj.Object, selector, path...); err != nil {
		return err
	}
	return nil
}

// modifyTolerations adds tolerations to tolerate all taints when IsTolerateAll is enabled.
//
// Parameters:
//   - obj: The unstructured Kubernetes object to modify
//   - workload: The workload specification containing toleration settings
//   - path: Path to the tolerations array in the object
//
// Returns:
//   - error: Any error encountered during toleration modification, or nil on success
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

// getMainContainer finds and returns the main container from a list of containers.
//
// Parameters:
//   - containers: Slice of container definitions
//   - mainContainerName: Name of the container to find
//
// Returns:
//   - map[string]interface{}: The main container definition
//   - error: Error if container is not found
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

// buildCommands constructs the command array for executing the workload entry point.
//
// Parameters:
//   - workload: The workload specification containing the entry point
//
// Returns:
//   - []interface{}: Command array for container execution
func buildCommands(workload *v1.Workload) []interface{} {
	return []interface{}{"/bin/sh", "-c", buildEntryPoint(workload)}
}

// buildEntryPoint constructs the command entry point for a workload.
// For OpsJobs, it decodes the base64 encoded entry point.
// For regular workloads, it wraps the entry point with launcher script execution.
//
// Parameters:
//   - workload: The workload specification containing the entry point
//
// Returns:
//   - string: The constructed entry point command
func buildEntryPoint(workload *v1.Workload) string {
	result := ""
	if commonworkload.IsOpsJob(workload) {
		result = stringutil.Base64Decode(workload.Spec.EntryPoint)
	} else {
		result = Launcher + " '" + workload.Spec.EntryPoint + "'"
	}
	return result
}

// buildLabels creates a map of labels for workload identification and tracking.
//
// Parameters:
//   - workload: The workload specification for generating labels
//
// Returns:
//   - map[string]interface{}: Map of label key-value pairs
func buildLabels(workload *v1.Workload) map[string]interface{} {
	return map[string]interface{}{
		v1.WorkloadIdLabel:          workload.Name,
		v1.WorkloadDispatchCntLabel: buildDispatchCount(workload),
	}
}

// buildResources constructs resource requirements for the workload container.
//
// Parameters:
//   - workload: The workload specification containing resource requirements
//
// Returns:
//   - map[string]interface{}: Map of resource requirements
func buildResources(workload *v1.Workload) map[string]interface{} {
	result := map[string]interface{}{
		string(corev1.ResourceCPU):              workload.Spec.Resource.CPU,
		string(corev1.ResourceMemory):           workload.Spec.Resource.Memory,
		string(corev1.ResourceEphemeralStorage): workload.Spec.Resource.EphemeralStorage,
	}
	if workload.Spec.Resource.GPU != "" {
		result[workload.Spec.Resource.GPUName] = workload.Spec.Resource.GPU
	}
	if workload.Spec.Resource.RdmaResource != "" && commonconfig.GetRdmaName() != "" {
		result[commonconfig.GetRdmaName()] = workload.Spec.Resource.RdmaResource
	}
	return result
}

// buildEnvironment creates environment variables for the workload container.
//
// Parameters:
//   - workload: The workload specification containing environment settings
//
// Returns:
//   - []interface{}: Slice of environment variable definitions
func buildEnvironment(workload *v1.Workload) []interface{} {
	var result []interface{}
	if workload.Spec.IsSupervised {
		result = append(result, map[string]interface{}{
			"name":  "ENABLE_SUPERVISE",
			"value": v1.TrueStr,
		})
		if commonconfig.GetWorkloadHangCheckInterval() > 0 {
			result = append(result, map[string]interface{}{
				"name":  "HANG_CHECK_INTERVAL",
				"value": strconv.Itoa(commonconfig.GetWorkloadHangCheckInterval()),
			})
		}
	}
	if workload.Spec.Resource.GPU != "" {
		result = append(result, map[string]interface{}{
			"name":  "GPUS_PER_NODE",
			"value": workload.Spec.Resource.GPU,
		})
	}
	result = append(result, map[string]interface{}{
		"name":  "WORKLOAD_ID",
		"value": workload.Name,
	})
	result = append(result, map[string]interface{}{
		"name":  "DISPATCH_COUNT",
		"value": strconv.Itoa(v1.GetWorkloadDispatchCnt(workload) + 1),
	})
	result = append(result, map[string]interface{}{
		"name":  "SSH_PORT",
		"value": strconv.Itoa(workload.Spec.SSHPort),
	})
	return result
}

// buildPorts constructs port definitions for the workload container.
//
// Parameters:
//   - workload: The workload specification containing port settings
//
// Returns:
//   - []interface{}: Slice of port definitions
func buildPorts(workload *v1.Workload) []interface{} {
	jobPort := map[string]interface{}{
		"containerPort": int64(workload.Spec.JobPort),
		"protocol":      "TCP",
	}
	if workload.SpecKind() == common.PytorchJobKind || workload.SpecKind() == common.AuthoringKind {
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
//
// Parameters:
//   - healthz: Health check specification
//
// Returns:
//   - map[string]interface{}: Health check probe configuration, or nil if healthz is nil
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
//
// Parameters:
//   - name: Name of the volume to mount
//   - mountPath: Path where the volume should be mounted
//   - subPath: Sub-path within the volume to mount (optional)
//
// Returns:
//   - interface{}: Volume mount definition
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

// buildHostPathVolume creates a host path volume definition.
//
// Parameters:
//   - volumeName: Name for the volume
//   - hostPath: Path on the host machine
//
// Returns:
//   - interface{}: Host path volume definition
func buildHostPathVolume(volumeName, hostPath string) interface{} {
	return map[string]interface{}{
		"hostPath": map[string]interface{}{
			"path": hostPath,
		},
		"name": volumeName,
	}
}

// buildPvcVolume creates a persistent volume claim volume definition.
//
// Parameters:
//   - volumeName: Name of the PVC to reference
//
// Returns:
//   - interface{}: PVC volume definition
func buildPvcVolume(volumeName string) interface{} {
	return map[string]interface{}{
		"persistentVolumeClaim": map[string]interface{}{
			"claimName": volumeName,
		},
		"name": volumeName,
	}
}

// buildMatchExpression creates node selector match expressions based on workload specifications.
//
// Parameters:
//   - workload: The workload specification containing selection criteria
//
// Returns:
//   - []interface{}: Slice of match expression definitions
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
		result = append(result, map[string]interface{}{
			"key":      key,
			"operator": "In",
			"values":   values,
		})
	}
	return result
}

// buildSharedMemoryVolume creates an emptyDir volume with memory medium for shared memory.
//
// Parameters:
//   - sizeLimit: Maximum size for the shared memory volume
//
// Returns:
//   - interface{}: Shared memory volume definition
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
//
// Parameters:
//   - workload: The workload specification containing strategy settings
//
// Returns:
//   - map[string]interface{}: Strategy configuration, or nil if no settings
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
//
// Parameters:
//   - workload: The workload specification for generating selector
//
// Returns:
//   - map[string]interface{}: Label selector definition
func buildSelector(workload *v1.Workload) map[string]interface{} {
	return map[string]interface{}{
		"matchLabels": map[string]interface{}{
			v1.WorkloadIdLabel: workload.Name,
		},
	}
}

// buildImageSecret creates an image pull secret reference.
//
// Parameters:
//   - secretId: Name of the image pull secret
//
// Returns:
//   - interface{}: Image secret reference definition
func buildImageSecret(secretId string) interface{} {
	return map[string]interface{}{
		"name": secretId,
	}
}

// convertToStringMap converts a map[string]interface{} to map[string]string.
// Only includes entries where the value is a string.
//
// Parameters:
//   - input: Map with interface{} values
//
// Returns:
//   - map[string]string: Map with string values only
func convertToStringMap(input map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for key, value := range input {
		if strValue, ok := value.(string); ok {
			result[key] = strValue
		}
	}
	return result
}

// convertEnvsToStringMap extracts name-value pairs from environment variable definitions.
//
// Parameters:
//   - envs: Slice of environment variable definitions
//
// Returns:
//   - map[string]string: Map of environment variable names to values
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
