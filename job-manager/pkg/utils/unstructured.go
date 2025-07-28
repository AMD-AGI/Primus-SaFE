/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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

type K8sResourceStatus struct {
	Phase         string
	Message       string
	SpecReplica   int
	ActiveReplica int
}

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
	default:
		err = getResourceStatusImpl(unstructuredObj, rt, result)
	}
	return result, err
}

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

func getStatusByExpression(objects []map[string]interface{},
	expression v1.PhaseExpression, messagePaths []string, result *K8sResourceStatus) bool {
	match := func(obj map[string]interface{}, phase v1.PhaseExpression) bool {
		for key, val := range phase.MatchExpressions {
			val2 := getUnstructuredToString(obj, []string{key})
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

func getUnstructuredToString(obj map[string]interface{}, paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	result, found, err := unstructured.NestedFieldNoCopy(obj, paths...)
	if err != nil || !found {
		return ""
	}
	return stringutil.ConvertToString(result)
}

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

// Retrieve the replica count and the resource specifications of the main container
func GetResources(unstructuredObj *unstructured.Unstructured,
	rt *v1.ResourceTemplate, mainContainer, gpuName string) ([]int64, []corev1.ResourceList, error) {
	var replicaList []int64
	var resourceList []corev1.ResourceList
	for _, t := range rt.Spec.ResourceSpecs {
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

		path = t.PrePaths
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
		replicaList = append(replicaList, replica)
	}
	if len(replicaList) != len(resourceList) {
		return nil, nil, fmt.Errorf("internal error. the replica and limits is not match")
	}
	return replicaList, resourceList, nil
}

// Retrieve the command of the main container
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
			objs, ok := commands.([]interface{})
			if !ok {
				return nil, fmt.Errorf("failed to find container command, path: %s", path)
			}
			result := make([]string, 0, len(objs))
			for i := range objs {
				result = append(result, objs[i].(string))
			}
			return result, nil
		}
	}
	return nil, fmt.Errorf("no command found")
}

// Retrieve the image address of the main container
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
			return image.(string), nil
		}
	}
	return "", fmt.Errorf("no image found")
}

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

func GetActiveReplica(unstructuredObj *unstructured.Unstructured, rt *v1.ResourceTemplate) (int, error) {
	if len(rt.Spec.ActiveReplica.PrePaths) == 0 && rt.Spec.ActiveReplica.ReplicaPath == "" {
		return 0, nil
	}
	return getReplica(unstructuredObj, rt.Spec.ActiveReplica.PrePaths, rt.Spec.ActiveReplica.ReplicaPath)
}

// Retrieve the priorityClassName
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

// Retrieve the environment value of the main container
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
