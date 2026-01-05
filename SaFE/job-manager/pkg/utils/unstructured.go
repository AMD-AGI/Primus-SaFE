/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const (
	WorkspaceIdEnv       = "WORKSPACE_ID"
	UserIdEnv            = "USER_ID"
	PriorityEnv          = "PRIORITY"
	AdminControlPlaneEnv = "ADMIN_CONTROL_PLANE"
	GithubSecretEnv      = "GITHUB_SECRET_ID"
	NfsPathEnv           = "SAFE_NFS_PATH"
	NfsInputEnv          = "SAFE_NFS_INPUT"
	NfsOutputEnv         = "SAFE_NFS_OUTPUT"
)

type K8sResourceStatus struct {
	Phase       string
	Message     string
	SpecReplica int
	// only for cicd AutoscalingRunnerSet
	RunnerScaleSetId string
	ActiveReplica    int
}

func (s *K8sResourceStatus) IsPending() bool {
	if s.Phase == string(v1.K8sPending) ||
		s.Phase == "" {
		return true
	}
	return false
}

// GetK8sResourceStatus retrieves the status of a Kubernetes resource based on its unstructured object and resource template.
func GetK8sResourceStatus(unstructuredObj *unstructured.Unstructured, rt *v1.ResourceTemplate) (*K8sResourceStatus, error) {
	result := &K8sResourceStatus{}
	var err error
	if result.SpecReplica, err = GetSpecReplica(unstructuredObj, rt); err != nil {
		return nil, err
	}
	if result.ActiveReplica, err = GetActiveReplica(unstructuredObj, rt); err != nil {
		return nil, err
	}

	switch rt.SpecKind() {
	case common.StatefulSetKind:
		getStatefulSetStatus(unstructuredObj.Object, result)
	case common.JobKind:
		if err = getResourceStatusImpl(unstructuredObj, rt, result); err != nil {
			break
		}
		if result.Phase == "" && result.ActiveReplica > 0 {
			result.Phase = string(v1.K8sRunning)
			result.Message = "the job is running"
		}
	case common.CICDScaleRunnerSetKind:
		result.RunnerScaleSetId = v1.GetAnnotation(unstructuredObj, v1.CICDScaleSetIdAnnotation)
	default:
		err = getResourceStatusImpl(unstructuredObj, rt, result)
	}
	return result, err
}

// getResourceStatusImpl implements resource status retrieval based on resource template configuration.
func getResourceStatusImpl(unstructuredObj *unstructured.Unstructured, rt *v1.ResourceTemplate, result *K8sResourceStatus) error {
	if len(rt.Spec.ResourceStatus.PrePaths) == 0 {
		return nil
	}
	m, found, err := unstructured.NestedFieldNoCopy(unstructuredObj.Object, rt.Spec.ResourceStatus.PrePaths...)
	if !found || err != nil {
		return err
	}
	var objects []map[string]interface{}
	switch val := m.(type) {
	case map[string]interface{}:
		objects = append(objects, val)
	case []interface{}:
		for _, item := range val {
			obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&item)
			if err != nil {
				return err
			}
			objects = append(objects, obj)
		}
	default:
		return fmt.Errorf("invalid path: %v", rt.Spec.ResourceStatus.Phases)
	}
	for _, phase := range rt.Spec.ResourceStatus.Phases {
		if getStatusByExpression(objects, phase, rt.Spec.ResourceStatus.MessagePaths, result) {
			return nil
		}
	}
	return nil
}

// getStatefulSetStatus determines the status of a StatefulSet based on its revision information.
func getStatefulSetStatus(obj map[string]interface{}, result *K8sResourceStatus) {
	currentRevision := GetUnstructuredString(obj, []string{"status", "currentRevision"})
	updateRevision := GetUnstructuredString(obj, []string{"status", "updateRevision"})
	switch {
	case currentRevision != updateRevision:
		result.Phase = string(v1.K8sUpdating)
		result.Message = "the statefulSet is updating"
	case result.SpecReplica == result.ActiveReplica:
		result.Phase = string(v1.K8sRunning)
		result.Message = "the statefulSet is ready"
	default:
		result.Phase = string(v1.K8sFailed)
		result.Message = "the statefulSet is not ready"
	}
}

// getStatusByExpression matches resource status based on phase expressions and message paths of resource-tempalte.
func getStatusByExpression(objects []map[string]interface{},
	expression v1.PhaseExpression, messagePaths []string, result *K8sResourceStatus) bool {
	match := func(obj map[string]interface{}, phase v1.PhaseExpression) bool {
		for key, val := range phase.MatchExpressions {
			val2 := convertUnstructuredToString(obj, []string{key})
			if val != val2 {
				return false
			}
		}
		return true
	}
	for _, obj := range objects {
		if !match(obj, expression) {
			continue
		}
		result.Phase = expression.Phase
		if msg := GetUnstructuredString(obj, messagePaths); msg != "" {
			result.Message = msg
		} else {
			result.Message = buildMessage(expression.Phase)
		}
		return true
	}
	return false
}

// buildMessage constructs default status messages based on the phase.
func buildMessage(phase string) string {
	switch phase {
	case string(v1.K8sSucceeded):
		return "Job is successfully completed"
	case string(v1.K8sFailed):
		return "Job is failed"
	case string(v1.K8sRunning):
		return "Job is running"
	default:
		return "unknown"
	}
}

// convertUnstructuredToString converts unstructured object field to string representation.
func convertUnstructuredToString(obj map[string]interface{}, paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	result, found, err := unstructured.NestedFieldNoCopy(obj, paths...)
	if err != nil || !found {
		return ""
	}
	return stringutil.ConvertToString(result)
}

// GetUnstructuredString retrieves string value from unstructured object at specified paths.
func GetUnstructuredString(obj map[string]interface{}, paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	result, found, err := unstructured.NestedString(obj, paths...)
	if err != nil || !found {
		return ""
	}
	return result
}

// GetUnstructuredInt retrieves integer value from unstructured object at specified paths.
func GetUnstructuredInt(obj map[string]interface{}, paths []string) int64 {
	if len(paths) == 0 {
		return 0
	}
	result, found, err := unstructured.NestedInt64(obj, paths...)
	if err != nil || !found {
		return 0
	}
	return result
}

// GetResources Retrieve the replica count and the resource specifications of the main container.
func GetResources(unstructuredObj *unstructured.Unstructured,
	rt *v1.ResourceTemplate, mainContainer, gpuName string) ([]int64, []corev1.ResourceList, error) {
	var replicaList []int64
	var resourceList []corev1.ResourceList
	for _, t := range rt.Spec.ResourceSpecs {
		if len(t.ReplicasPaths) > 0 {
			path := t.PrePaths
			path = append(path, t.ReplicasPaths...)
			replica, found, err := unstructured.NestedInt64(unstructuredObj.Object, path...)
			if err != nil {
				klog.ErrorS(err, "failed to find replica", "path", path)
				return nil, nil, err
			}
			if !found {
				continue
			}
			replicaList = append(replicaList, replica)
		}

		path := t.PrePaths
		path = append(path, t.TemplatePaths...)
		path = append(path, "spec", "containers")
		containers, found, err := unstructured.NestedSlice(unstructuredObj.Object, path...)
		if err != nil {
			klog.ErrorS(err, "failed to find containers", "path", path)
			return nil, nil, err
		}
		if !found {
			continue
		}
		for _, c := range containers {
			obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&c)
			if err != nil {
				return nil, nil, err
			}
			name, _ := obj["name"]
			if mainContainer != "" && name != mainContainer {
				continue
			}
			path = []string{"resources", "limits"}
			limits, found, err := unstructured.NestedMap(obj, path...)
			if err != nil || !found {
				klog.ErrorS(err, "failed to find limits", "path", path)
				return nil, nil, err
			}
			rl, err := quantity.CvtToResourceList(
				GetUnstructuredString(limits, []string{string(corev1.ResourceCPU)}),
				GetUnstructuredString(limits, []string{string(corev1.ResourceMemory)}),
				GetUnstructuredString(limits, []string{gpuName}), gpuName,
				GetUnstructuredString(limits, []string{string(corev1.ResourceEphemeralStorage)}),
				GetUnstructuredString(limits, []string{commonconfig.GetRdmaName()}), 1)
			if err != nil {
				return nil, nil, err
			}
			resourceList = append(resourceList, rl)
			break
		}
	}
	return replicaList, resourceList, nil
}

// GetCommand Retrieve the command of the main container.
func GetCommand(unstructuredObj *unstructured.Unstructured,
	rt *v1.ResourceTemplate, mainContainer string) ([]string, error) {
	for _, t := range rt.Spec.ResourceSpecs {
		path := t.PrePaths
		path = append(path, t.TemplatePaths...)
		path = append(path, "spec", "containers")
		containers, found, err := unstructured.NestedSlice(unstructuredObj.Object, path...)
		if err != nil {
			klog.ErrorS(err, "failed to find containers", "path", path)
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("failed to find containers, path: %s", path)
		}
		for _, c := range containers {
			obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&c)
			if err != nil {
				return nil, err
			}
			name, _ := obj["name"]
			if mainContainer != "" && name != mainContainer {
				continue
			}
			commands, ok := obj["command"]
			if !ok {
				return nil, fmt.Errorf("failed to find container command, path: %s", path)
			}
			commandList, ok := commands.([]interface{})
			if !ok {
				return nil, fmt.Errorf("failed to find container command, path: %s", path)
			}
			result := make([]string, 0, len(commandList))
			for i := range commandList {
				if str, ok := commandList[i].(string); ok {
					result = append(result, str)
				}
			}
			return result, nil
		}
	}
	return nil, fmt.Errorf("no command found")
}

// GetImage Retrieve the image address of the main container.
func GetImage(unstructuredObj *unstructured.Unstructured,
	rt *v1.ResourceTemplate, mainContainer string) (string, error) {
	for _, t := range rt.Spec.ResourceSpecs {
		path := t.PrePaths
		path = append(path, t.TemplatePaths...)
		path = append(path, "spec", "containers")
		containers, found, err := unstructured.NestedSlice(unstructuredObj.Object, path...)
		if err != nil {
			klog.ErrorS(err, "failed to find containers", "path", path)
			return "", err
		}
		if !found {
			return "", fmt.Errorf("failed to find containers, path: %s", path)
		}
		for _, c := range containers {
			obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&c)
			if err != nil {
				return "", err
			}
			name, _ := obj["name"]
			if mainContainer != "" && name != mainContainer {
				continue
			}
			image, ok := obj["image"]
			if !ok {
				return "", fmt.Errorf("failed to find container image, path: %s", path)
			}
			if imageStr, ok := image.(string); ok {
				return imageStr, nil
			}
			return "", fmt.Errorf("image is not a string")
		}
	}
	return "", fmt.Errorf("no image found")
}

// GetMemoryStorageSize retrieves the memory storage size from volume specifications.
func GetMemoryStorageSize(unstructuredObj *unstructured.Unstructured, rt *v1.ResourceTemplate) (string, error) {
	for _, t := range rt.Spec.ResourceSpecs {
		path := t.PrePaths
		path = append(path, t.TemplatePaths...)
		path = append(path, "spec", "volumes")
		volumes, found, err := unstructured.NestedSlice(unstructuredObj.Object, path...)
		if err != nil {
			klog.ErrorS(err, "failed to find volumes", "path", path)
			return "", err
		}
		if !found {
			return "", fmt.Errorf("failed to find volumes, path: %s", path)
		}

		shareMemory := GetMemoryStorageVolume(volumes)
		if shareMemory == nil {
			break
		}
		return shareMemory["sizeLimit"].(string), nil
	}
	return "", fmt.Errorf("no share memory found")
}

// GetMemoryStorageVolume finds the memory storage volume from a list of volumes.
func GetMemoryStorageVolume(volumes []interface{}) map[string]interface{} {
	for i := range volumes {
		volume, ok := volumes[i].(map[string]interface{})
		if !ok {
			continue
		}
		emptyDirObj, ok := volume["emptyDir"]
		if !ok {
			continue
		}
		emptyDir, ok := emptyDirObj.(map[string]interface{})
		if !ok {
			continue
		}
		medium, ok := emptyDir["medium"].(string)
		if !ok || medium != string(corev1.StorageMediumMemory) {
			continue
		}
		return emptyDir
	}
	return nil
}

// GetSpecReplica retrieves the specified replica count from the unstructured object.
func GetSpecReplica(unstructuredObj *unstructured.Unstructured, rt *v1.ResourceTemplate) (int, error) {
	if len(rt.Spec.ResourceSpecs) == 0 {
		return 0, nil
	}
	replica := 0
	for _, t := range rt.Spec.ResourceSpecs {
		if t.Replica > 0 {
			replica += int(t.Replica)
			continue
		}
		l := len(t.ReplicasPaths)
		if l == 0 {
			continue
		}
		prePaths := slice.Copy(t.PrePaths, len(t.PrePaths))
		if l > 1 {
			prePaths = append(prePaths, t.ReplicasPaths[:l]...)
		}
		n, err := getReplica(unstructuredObj, prePaths, t.ReplicasPaths[l-1])
		if err != nil {
			return 0, err
		}
		replica += n
	}
	return replica, nil
}

// GetActiveReplica retrieves the active replica count from the unstructured object.
func GetActiveReplica(unstructuredObj *unstructured.Unstructured, rt *v1.ResourceTemplate) (int, error) {
	if len(rt.Spec.ActiveReplica.PrePaths) == 0 && rt.Spec.ActiveReplica.ReplicaPath == "" {
		return 0, nil
	}
	return getReplica(unstructuredObj, rt.Spec.ActiveReplica.PrePaths, rt.Spec.ActiveReplica.ReplicaPath)
}

// GetPriorityClassName retrieves the priorityClassName from the unstructured object.
func GetPriorityClassName(unstructuredObj *unstructured.Unstructured, rt *v1.ResourceTemplate) (string, error) {
	for _, t := range rt.Spec.ResourceSpecs {
		path := t.PrePaths
		path = append(path, t.TemplatePaths...)
		path = append(path, "spec", "priorityClassName")
		name, found, err := unstructured.NestedString(unstructuredObj.Object, path...)
		if err != nil {
			klog.ErrorS(err, "failed to find priorityClassName", "path", path)
			return "", err
		}
		if !found {
			continue
		}
		return name, nil
	}
	return "", fmt.Errorf("no priorityClassName found")
}

// GetGithubConfigSecret retrieves the githubConfigSecret from the unstructured object.
func GetGithubConfigSecret(unstructuredObj *unstructured.Unstructured) (string, error) {
	path := []string{"spec", "githubConfigSecret"}
	val, found, err := unstructured.NestedString(unstructuredObj.Object, path...)
	if err != nil {
		klog.ErrorS(err, "failed to find githubConfigSecret", "path", path)
		return "", err
	}
	if !found {
		return "", fmt.Errorf("no githubConfigSecret found")
	}
	return val, nil
}

// getReplica retrieves replica count based on pre-paths and replica path of resource template.
func getReplica(unstructuredObj *unstructured.Unstructured, prePaths []string, name string) (int, error) {
	m, found, err := unstructured.NestedFieldNoCopy(unstructuredObj.Object, prePaths...)
	if !found || err != nil {
		return 0, err
	}
	var objects []map[string]interface{}
	switch val := m.(type) {
	case map[string]interface{}:
		objects = append(objects, val)
	case []interface{}:
		for _, item := range val {
			obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&item)
			if err != nil {
				return 0, err
			}
			objects = append(objects, obj)
		}
	default:
		return 0, fmt.Errorf("failed to get replica, path: %v, name: %s", prePaths, name)
	}

	var result int64 = 0
	for _, obj := range objects {
		n, ok := getIntValueByName(obj, name)
		if ok {
			result += n
		} else {
			for _, val := range obj {
				if obj2, ok := val.(map[string]interface{}); ok {
					n, _ = getIntValueByName(obj2, name)
					result += n
				}
			}
		}
	}
	return int(result), nil
}

// getIntValueByName retrieves integer value by field name from objects.
func getIntValueByName(objects map[string]interface{}, name string) (int64, bool) {
	obj, ok := objects[name]
	if !ok {
		return 0, false
	}
	v := reflect.ValueOf(obj)
	if v.CanInt() {
		return v.Int(), true
	}
	return 0, true
}

// GetEnv Retrieve the environment value of the main container.
func GetEnv(unstructuredObj *unstructured.Unstructured,
	rt *v1.ResourceTemplate, mainContainer string) ([]interface{}, error) {
	for _, t := range rt.Spec.ResourceSpecs {
		templatePath := t.GetTemplatePath()
		path := append(templatePath, "spec", "containers")
		containers, found, err := unstructured.NestedSlice(unstructuredObj.Object, path...)
		if err != nil {
			klog.ErrorS(err, "failed to find containers", "path", path)
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("failed to find containers, path: %s", path)
		}
		for _, c := range containers {
			obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&c)
			if err != nil {
				return nil, err
			}
			name, _ := obj["name"]
			if mainContainer != "" && name != mainContainer {
				continue
			}
			env, ok := obj["env"]
			if !ok {
				return nil, nil
			}
			result, ok := env.([]interface{})
			if !ok {
				return nil, nil
			}
			return result, nil
		}
	}
	return nil, fmt.Errorf("no env found")
}

// getEnvValue retrieves the value of a specific environment variable from the main container
// Returns empty string if the environment variable is not found
func getEnvValue(unstructuredObj *unstructured.Unstructured,
	rt *v1.ResourceTemplate, mainContainer, name string) (string, error) {
	envs, err := GetEnv(unstructuredObj, rt, mainContainer)
	if err != nil {
		return "", err
	}
	for _, env := range envs {
		envObj, ok := env.(map[string]interface{})
		if !ok {
			continue
		}
		if envObj["name"] == name {
			return envObj["value"].(string), nil
		}
	}
	return "", nil
}
