/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"fmt"
	"reflect"
	"strconv"

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

type K8sObjectStatus struct {
	Phase         string
	Message       string
	SpecReplica   int
	ActiveReplica int
	// only for cicd AutoscalingRunnerSet
	RunnerScaleSetId string
}

func (s *K8sObjectStatus) IsPending() bool {
	if s.Phase == string(v1.K8sPending) ||
		s.Phase == "" {
		return true
	}
	return false
}

// GetK8sObjectStatus retrieves the status of a Kubernetes resource based on its unstructured object and resource template.
func GetK8sObjectStatus(unstructuredObj *unstructured.Unstructured, rt *v1.ResourceTemplate) (*K8sObjectStatus, error) {
	result := &K8sObjectStatus{}
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
		if err = getK8sObjectStatusImpl(unstructuredObj, rt, result); err != nil {
			break
		}
		if result.Phase == "" && result.ActiveReplica > 0 {
			result.Phase = string(v1.K8sRunning)
			result.Message = "the job is running"
		}
	case common.CICDScaleRunnerSetKind:
		result.RunnerScaleSetId = v1.GetAnnotation(unstructuredObj, v1.CICDScaleSetIdAnnotation)
	default:
		err = getK8sObjectStatusImpl(unstructuredObj, rt, result)
	}
	return result, err
}

// getK8sObjectStatusImpl implements object status retrieval based on resource template configuration.
func getK8sObjectStatusImpl(unstructuredObj *unstructured.Unstructured, rt *v1.ResourceTemplate, result *K8sObjectStatus) error {
	if len(rt.Spec.ResourceStatus.PrePaths) == 0 {
		return nil
	}
	m, found, err := NestedField(unstructuredObj.Object, rt.Spec.ResourceStatus.PrePaths)
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
func getStatefulSetStatus(obj map[string]interface{}, result *K8sObjectStatus) {
	currentRevision := NestedStringSilently(obj, []string{"status", "currentRevision"})
	updateRevision := NestedStringSilently(obj, []string{"status", "updateRevision"})
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

// getStatusByExpression matches resource status based on phase expressions and message paths of resource-template.
func getStatusByExpression(objects []map[string]interface{},
	expression v1.PhaseExpression, messagePaths []string, result *K8sObjectStatus) bool {
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
		if msg := NestedStringSilently(obj, messagePaths); msg != "" {
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
	result, found, err := NestedField(obj, paths)
	if err != nil || !found || result == nil {
		return ""
	}
	return stringutil.ConvertToString(result)
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
			replica, found, err := NestedInt64(unstructuredObj.Object, path)
			if err != nil {
				klog.ErrorS(err, "failed to find replicas", "path", path)
				return nil, nil, err
			}
			if found {
				replicaList = append(replicaList, replica)
			}
		}

		path := t.PrePaths
		path = append(path, t.TemplatePaths...)
		path = append(path, "spec", "containers")
		containers, found, err := NestedSlice(unstructuredObj.Object, path)
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
			limits, found, err := NestedMap(obj, path)
			if err != nil || !found {
				klog.ErrorS(err, "failed to find limits", "path", path)
				return nil, nil, err
			}
			rl, err := quantity.CvtToResourceList(
				NestedStringSilently(limits, []string{string(corev1.ResourceCPU)}),
				NestedStringSilently(limits, []string{string(corev1.ResourceMemory)}),
				NestedStringSilently(limits, []string{gpuName}), gpuName,
				NestedStringSilently(limits, []string{string(corev1.ResourceEphemeralStorage)}),
				NestedStringSilently(limits, []string{commonconfig.GetRdmaName()}), 1)
			if err != nil {
				return nil, nil, err
			}
			resourceList = append(resourceList, rl)
			break
		}
	}
	return replicaList, resourceList, nil
}

// GetCommands Retrieve the command of the main container.
func GetCommands(unstructuredObj *unstructured.Unstructured,
	rt *v1.ResourceTemplate, mainContainer string) ([][]string, error) {
	result := make([][]string, 0, len(rt.Spec.ResourceSpecs))
	for _, t := range rt.Spec.ResourceSpecs {
		path := t.PrePaths
		path = append(path, t.TemplatePaths...)
		path = append(path, "spec", "containers")
		containers, found, err := NestedSlice(unstructuredObj.Object, path)
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
			commandObj, ok := obj["command"]
			if !ok {
				return nil, fmt.Errorf("failed to find container command, path: %s", path)
			}
			commandObjList, ok := commandObj.([]interface{})
			if !ok {
				return nil, fmt.Errorf("failed to find container command, path: %s", path)
			}
			commandStrList := make([]string, 0, len(commandObjList))
			for i := range commandObjList {
				if str, ok := commandObjList[i].(string); ok {
					commandStrList = append(commandStrList, str)
				}
			}
			result = append(result, commandStrList)
			break
		}
	}
	return result, nil
}

// GetImages Retrieve all the image address of the main container.
func GetImages(unstructuredObj *unstructured.Unstructured,
	rt *v1.ResourceTemplate, mainContainer string) ([]string, error) {
	result := make([]string, 0, len(rt.Spec.ResourceSpecs))
	for _, t := range rt.Spec.ResourceSpecs {
		path := t.PrePaths
		path = append(path, t.TemplatePaths...)
		path = append(path, "spec", "containers")
		containers, found, err := NestedSlice(unstructuredObj.Object, path)
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
			image, ok := obj["image"]
			if !ok {
				return nil, fmt.Errorf("failed to find container image, path: %s", path)
			}
			if imageStr, ok := image.(string); ok {
				result = append(result, imageStr)
			}
			break
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("failed to find container image")
	}
	return result, nil
}

// GetMemoryStorageSize retrieves the memory storage size from volume specifications.
func GetMemoryStorageSize(unstructuredObj *unstructured.Unstructured, rt *v1.ResourceTemplate) ([]string, error) {
	var result []string
	for _, t := range rt.Spec.ResourceSpecs {
		path := t.PrePaths
		path = append(path, t.TemplatePaths...)
		path = append(path, "spec", "volumes")
		volumes, found, err := NestedSlice(unstructuredObj.Object, path)
		if err != nil {
			klog.ErrorS(err, "failed to find volumes", "path", path)
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("failed to find volumes, path: %s", path)
		}

		shareMemory := GetMemoryStorageVolume(volumes)
		if shareMemory != nil {
			result = append(result, shareMemory["sizeLimit"].(string))
		} else {
			result = append(result, "0")
		}
	}
	return result, nil
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
		name, found, err := NestedString(unstructuredObj.Object, path)
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
	val, found, err := NestedString(unstructuredObj.Object, path)
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
	m, found, err := NestedField(unstructuredObj.Object, prePaths)
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

// GetLabels retrieves the labels from Unstructured object.
func GetLabels(unstructuredObj *unstructured.Unstructured, resourceSpec v1.ResourceSpec) (map[string]interface{}, error) {
	path := resourceSpec.PrePaths
	path = append(path, resourceSpec.TemplatePaths...)
	path = append(path, "metadata", "labels")
	labels, found, err := NestedMap(unstructuredObj.Object, path)
	if err != nil {
		klog.ErrorS(err, "failed to find labels", "path", path)
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return labels, nil
}

// GetSelectorLabels retrieves the labels of selector from Unstructured object.
func GetSelectorLabels(unstructuredObj *unstructured.Unstructured) (map[string]interface{}, error) {
	path := []string{"spec", "selector", "matchLabels"}
	labels, found, err := NestedMap(unstructuredObj.Object, path)
	if err != nil {
		klog.ErrorS(err, "failed to find labels", "path", path)
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return labels, nil
}

// GetEnv Retrieve the environment value of the main container.
func GetEnv(unstructuredObj *unstructured.Unstructured,
	rt *v1.ResourceTemplate, mainContainer string) ([]interface{}, error) {
	for _, t := range rt.Spec.ResourceSpecs {
		templatePath := t.GetTemplatePath()
		path := append(templatePath, "spec", "containers")
		containers, found, err := NestedSlice(unstructuredObj.Object, path)
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

// NestedString retrieves string value from unstructured object at specified paths.
func NestedString(obj map[string]interface{}, paths []string) (string, bool, error) {
	result, found, err := NestedField(obj, paths)
	if err != nil || !found || result == nil {
		return "", found, err
	}
	if str, ok := result.(string); ok {
		return str, true, nil
	}
	return "", true, fmt.Errorf("invalid field type")
}

// NestedStringSilently retrieves a string value from unstructured object at specified paths,
// ignoring any errors that might occur during the retrieval.
// Returns an empty string if the path is not found or if the value is not a string.
func NestedStringSilently(obj map[string]interface{}, paths []string) string {
	result, _, _ := NestedString(obj, paths)
	return result
}

// NestedBool retrieves boolean value from unstructured object at specified paths.
func NestedBool(obj map[string]interface{}, paths []string) (bool, bool, error) {
	result, found, err := NestedField(obj, paths)
	if err != nil || !found || result == nil {
		return false, found, err
	}
	if b, ok := result.(bool); ok {
		return b, true, nil
	}
	return false, true, fmt.Errorf("invalid field type")
}

// NestedInt64 retrieves integer value from unstructured object at specified paths.
func NestedInt64(obj map[string]interface{}, paths []string) (int64, bool, error) {
	result, found, err := NestedField(obj, paths)
	if err != nil || !found || result == nil {
		return 0, found, err
	}

	switch v := result.(type) {
	case int:
		return int64(v), true, nil
	case int32:
		return int64(v), true, nil
	case int64:
		return v, true, nil
	default:
		return 0, true, fmt.Errorf("invalid field type")
	}
}

// SetNestedField sets a nested field value with array index support.
// Allows setting values in nested objects using paths like "spec.containers[0].image"
// where [0] represents array index access.
func SetNestedField(obj map[string]interface{}, value interface{}, path []string) error {
	if len(path) == 0 {
		return fmt.Errorf("empty path")
	}

	if len(path) == 1 {
		obj[path[0]] = value
		return nil
	}

	key := path[0]
	remaining := path[1:]

	if index, err := strconv.Atoi(remaining[0]); err == nil {
		arr, found, err := unstructured.NestedSlice(obj, key)
		if err != nil {
			return fmt.Errorf("failed to get slice at %s: %v", key, err)
		}
		if !found {
			return fmt.Errorf("slice not found at %s", key)
		}
		if index < 0 || index >= len(arr) {
			return fmt.Errorf("index %d out of range for slice at %s (len=%d)", index, key, len(arr))
		}
		elem, ok := arr[index].(map[string]interface{})
		if !ok {
			return fmt.Errorf("element at index %d is not a map", index)
		}
		if err = SetNestedField(elem, value, remaining[1:]); err != nil {
			return err
		}
		arr[index] = elem
		return unstructured.SetNestedSlice(obj, arr, key)
	}

	nested, found, err := unstructured.NestedMap(obj, key)
	if err != nil {
		return fmt.Errorf("failed to get map at %s: %v", key, err)
	}
	if !found {
		nested = make(map[string]interface{})
	}

	if err = SetNestedField(nested, value, remaining); err != nil {
		return err
	}

	return unstructured.SetNestedMap(obj, nested, key)
}

// NestedMap gets a nested map value with array index support.
// Allows getting values from nested objects using paths like ["spec", "containers", "0", "resources"]
// where "0" represents array index access.
func NestedMap(obj map[string]interface{}, path []string) (map[string]interface{}, bool, error) {
	if len(path) == 0 {
		return nil, false, fmt.Errorf("empty path")
	}

	key := path[0]
	remaining := path[1:]

	// If no remaining path, get the map at current key directly
	if len(remaining) == 0 {
		return unstructured.NestedMap(obj, key)
	}

	// Check if the next path element is an array index (numeric)
	if index, err := strconv.Atoi(remaining[0]); err == nil {
		// Current key corresponds to an array
		arr, found, err := unstructured.NestedSlice(obj, key)
		if err != nil || !found {
			return nil, found, err
		}
		if index < 0 || index >= len(arr) {
			return nil, false, fmt.Errorf("index %d out of range for slice at %s (len=%d)", index, key, len(arr))
		}

		elem, ok := arr[index].(map[string]interface{})
		if !ok {
			return nil, false, fmt.Errorf("element at index %d is not a map", index)
		}

		// If only the index remains, return this element
		if len(remaining) == 1 {
			return elem, true, nil
		}

		// Recursively process the remaining path (skip the index)
		return NestedMap(elem, remaining[1:])
	}

	// Regular map field, process recursively
	nested, found, err := unstructured.NestedMap(obj, key)
	if err != nil || !found {
		return nil, found, err
	}

	return NestedMap(nested, remaining)
}

// NestedSlice gets a nested slice value with array index support.
// Allows getting values from nested objects using paths like ["spec", "workerGroupSpecs", "0", "template", "spec", "volumes"]
// where "0" represents array index access.
func NestedSlice(obj map[string]interface{}, path []string) ([]interface{}, bool, error) {
	if len(path) == 0 {
		return nil, false, fmt.Errorf("empty path")
	}

	key := path[0]
	remaining := path[1:]

	// If no remaining path, get the slice at current key directly
	if len(remaining) == 0 {
		return unstructured.NestedSlice(obj, key)
	}

	// Check if the next path element is an array index (numeric)
	if index, err := strconv.Atoi(remaining[0]); err == nil {
		// Current key corresponds to an array
		arr, found, err := unstructured.NestedSlice(obj, key)
		if err != nil || !found {
			return nil, found, err
		}
		if index < 0 || index >= len(arr) {
			return nil, false, fmt.Errorf("index %d out of range for slice at %s (len=%d)", index, key, len(arr))
		}

		elem, ok := arr[index].(map[string]interface{})
		if !ok {
			return nil, false, fmt.Errorf("element at index %d is not a map", index)
		}

		// If only the index remains, this is an error - expecting a slice not a map element
		if len(remaining) == 1 {
			return nil, false, fmt.Errorf("path ends at array element, expected slice")
		}

		// Recursively process the remaining path (skip the index)
		return NestedSlice(elem, remaining[1:])
	}

	// Regular map field, process recursively
	nested, found, err := unstructured.NestedMap(obj, key)
	if err != nil || !found {
		return nil, found, err
	}

	return NestedSlice(nested, remaining)
}

// NestedField gets a nested field value with array index support.
// Allows getting values from nested objects using paths like ["spec", "containers", "0", "image"]
// where "0" represents array index access.
// Returns the value as interface{}, which can be any type (string, bool, map, slice, etc.)
func NestedField(obj map[string]interface{}, path []string) (interface{}, bool, error) {
	if len(path) == 0 {
		return nil, false, fmt.Errorf("empty path")
	}

	key := path[0]
	remaining := path[1:]

	// If no remaining path, get the value at current key directly
	if len(remaining) == 0 {
		val, found := obj[key]
		return val, found, nil
	}

	// Check if the next path element is an array index (numeric)
	if index, err := strconv.Atoi(remaining[0]); err == nil {
		// Current key corresponds to an array
		arr, found, err := unstructured.NestedSlice(obj, key)
		if err != nil || !found {
			return nil, found, err
		}
		if index < 0 || index >= len(arr) {
			return nil, false, fmt.Errorf("index %d out of range for slice at %s (len=%d)", index, key, len(arr))
		}

		// If only the index remains, return the element directly
		if len(remaining) == 1 {
			return arr[index], true, nil
		}

		// Element must be a map to continue traversing
		elem, ok := arr[index].(map[string]interface{})
		if !ok {
			return nil, false, fmt.Errorf("element at index %d is not a map", index)
		}

		// Recursively process the remaining path (skip the index)
		return NestedField(elem, remaining[1:])
	}

	// Regular map field, process recursively
	nested, found, err := unstructured.NestedMap(obj, key)
	if err != nil || !found {
		return nil, found, err
	}
	return NestedField(nested, remaining)
}

// RemoveNestedField removes a nested field with array index support.
// Allows removing values from nested objects using paths like ["spec", "containers", "0", "resources"]
// where "0" represents array index access.
func RemoveNestedField(obj map[string]interface{}, path []string) error {
	if len(path) == 0 {
		return fmt.Errorf("empty path")
	}

	if len(path) == 1 {
		delete(obj, path[0])
		return nil
	}

	key := path[0]
	remaining := path[1:]

	// Check if the next path element is an array index (numeric)
	if index, err := strconv.Atoi(remaining[0]); err == nil {
		// Current key corresponds to an array
		arr, found, err := unstructured.NestedSlice(obj, key)
		if err != nil || !found {
			// If not found, nothing to remove
			return nil
		}
		if index < 0 || index >= len(arr) {
			// Index out of range, nothing to remove
			return nil
		}

		// If only the index remains, remove that element from the array
		if len(remaining) == 1 {
			newArr := append(arr[:index], arr[index+1:]...)
			return unstructured.SetNestedSlice(obj, newArr, key)
		}

		elem, ok := arr[index].(map[string]interface{})
		if !ok {
			return fmt.Errorf("element at index %d is not a map", index)
		}

		// Recursively process the remaining path (skip the index)
		if err = RemoveNestedField(elem, remaining[1:]); err != nil {
			return err
		}
		arr[index] = elem
		return unstructured.SetNestedSlice(obj, arr, key)
	}

	// Regular map field, process recursively
	nested, found, err := unstructured.NestedMap(obj, key)
	if err != nil || !found {
		// If not found, nothing to remove
		return nil
	}

	if err = RemoveNestedField(nested, remaining); err != nil {
		return err
	}

	return unstructured.SetNestedMap(obj, nested, key)
}
