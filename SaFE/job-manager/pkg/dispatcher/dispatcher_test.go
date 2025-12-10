/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"strings"
	"testing"

	"gotest.tools/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	unstructuredutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/unstructured"
)

type PytorchSpec struct {
	PytorchReplicaSpecs struct {
		Master struct {
			Replicas int                    `json:"replicas"`
			Template corev1.PodTemplateSpec `json:"template"`
		} `json:"Master"`
		Worker struct {
			Replicas int                    `json:"replicas"`
			Template corev1.PodTemplateSpec `json:"template"`
		} `json:"Worker"`
	} `json:"pytorchReplicaSpecs"`
}
type PytorchJob struct {
	Spec PytorchSpec `json:"spec"`
}

func genMockScheme() (*runtime.Scheme, error) {
	result := runtime.NewScheme()
	err := v1.AddToScheme(result)
	if err != nil {
		return nil, err
	}
	err = corev1.AddToScheme(result)
	if err != nil {
		return nil, err
	}
	err = appsv1.AddToScheme(result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func parseConfigmap(content string) (*corev1.ConfigMap, error) {
	decoder := yamlutil.NewYAMLOrJSONDecoder(strings.NewReader(content), 100)
	var configMap corev1.ConfigMap
	if err := decoder.Decode(&configMap); err != nil {
		return nil, err
	}
	return &configMap, nil
}

func TestCreatePytorchJob(t *testing.T) {
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")
	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.Workspace = workspace.Name
	workload.Spec.Secrets = []v1.SecretEntity{{
		Id:   workspace.Spec.ImageSecrets[0].Name,
		Type: v1.SecretImage,
	}}
	metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.EnableHostNetworkAnnotation, "true")

	configmap, err := parseConfigmap(TestPytorchJobTemplateConfig)
	assert.NilError(t, err)
	metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap, jobutils.TestPytorchResourceTemplate, workspace).WithScheme(scheme).Build()

	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)
	templates := jobutils.TestPytorchResourceTemplate.Spec.ResourceSpecs

	checkResources(t, obj, workload, &templates[0], 1)
	checkPorts(t, obj, workload, &templates[0])
	checkEnvs(t, obj, workload, &templates[0])
	checkVolumeMounts(t, obj, &templates[0])
	checkVolumes(t, obj, workload, &templates[0])
	checkNodeSelectorTerms(t, obj, workload, &templates[0])
	checkImage(t, obj, workload.Spec.Image, &templates[0])
	checkLabels(t, obj, workload, &templates[0])
	checkHostNetwork(t, obj, workload, &templates[0])
	checkTolerations(t, obj, workload, &templates[0])
	checkPriorityClass(t, obj, workload, &templates[0])
	checkImageSecrets(t, obj, &templates[0])
	_, found, err := unstructured.NestedSlice(obj.Object, templates[1].PrePaths...)
	assert.NilError(t, err)
	assert.Equal(t, found, false)

	// enable worker
	workload.Spec.Resource.Replica = 3
	workload.Spec.IsTolerateAll = true
	metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.EnableHostNetworkAnnotation, "true")
	obj, err = r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)
	checkResources(t, obj, workload, &templates[1], 2)
	checkEnvs(t, obj, workload, &templates[1])
	checkPorts(t, obj, workload, &templates[1])
	checkVolumeMounts(t, obj, &templates[1])
	checkVolumes(t, obj, workload, &templates[1])
	checkNodeSelectorTerms(t, obj, workload, &templates[1])
	checkImage(t, obj, workload.Spec.Image, &templates[1])
	checkLabels(t, obj, workload, &templates[1])
	checkHostNetwork(t, obj, workload, &templates[1])
	checkTolerations(t, obj, workload, &templates[1])
	checkPriorityClass(t, obj, workload, &templates[1])
	checkImageSecrets(t, obj, &templates[1])
	// fmt.Println(unstructuredutils.ToString(obj))
}

func TestCreateDeployment(t *testing.T) {
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")
	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.Workspace = workspace.Name
	workload.Spec.GroupVersionKind = v1.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}
	workload.Spec.Service = &v1.Service{
		ServiceType: corev1.ServiceTypeNodePort,
		NodePort:    32198,
		Extends: map[string]string{
			"maxSurge":       "25%",
			"maxUnavailable": "25%",
		},
	}

	configmap, err := parseConfigmap(TestDeploymentTemplateConfig)
	assert.NilError(t, err)
	metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap, jobutils.TestDeploymentTemplate, workspace).WithScheme(scheme).Build()

	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)
	templates := jobutils.TestDeploymentTemplate.Spec.ResourceSpecs

	checkResources(t, obj, workload, &templates[0], 1)
	checkPorts(t, obj, workload, &templates[0])
	checkEnvs(t, obj, workload, &templates[0])
	checkVolumeMounts(t, obj, &templates[0])
	checkVolumes(t, obj, workload, &templates[0])
	checkNodeSelectorTerms(t, obj, workload, &templates[0])
	checkImage(t, obj, workload.Spec.Image, &templates[0])
	checkLabels(t, obj, workload, &templates[0])
	checkHostNetwork(t, obj, workload, &templates[0])
	checkSelector(t, obj, workload)
	checkStrategy(t, obj, workload)
	// fmt.Println(unstructuredutils.ToString(obj))
}

func TestUpdateDeployment(t *testing.T) {
	workloadObj, err := jsonutils.ParseYamlToJson(jobutils.TestDeploymentData)
	assert.NilError(t, err)
	adminWorkload := jobutils.TestWorkloadData.DeepCopy()
	metav1.SetMetaDataAnnotation(&adminWorkload.ObjectMeta, v1.MainContainerAnnotation, "test")

	err = applyWorkloadSpecToObject(context.Background(), nil, workloadObj, adminWorkload, nil, jobutils.TestDeploymentTemplate)
	assert.NilError(t, err)
	deployment := &appsv1.Deployment{}
	err = unstructuredutils.ConvertUnstructuredToObject(workloadObj, deployment)
	assert.NilError(t, err)

	assert.Equal(t, *deployment.Spec.Replicas, int32(1))
	assert.Equal(t, len(deployment.Spec.Template.Spec.Containers), 1)
	assert.Equal(t, deployment.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().Value(), int64(32))
	assert.Equal(t, deployment.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String(), "256Gi")
	gpuQuantity, ok := deployment.Spec.Template.Spec.Containers[0].Resources.Limits[common.AmdGpu]
	assert.Equal(t, ok, true)
	assert.Equal(t, gpuQuantity.Value(), int64(4))

	assert.Equal(t, deployment.Spec.Template.Spec.Containers[0].Image, "test-image")
	assert.Equal(t, deployment.Spec.Template.Spec.PriorityClassName, commonworkload.GeneratePriorityClass(adminWorkload))
	assert.Equal(t, len(deployment.Spec.Template.Spec.Containers[0].Command), 3)
	adminWorkload.Spec.EntryPoint = "sh -c test.sh"
	cmd := buildEntryPoint(adminWorkload)
	assert.Equal(t, deployment.Spec.Template.Spec.Containers[0].Command[2], cmd)

	shareMemorySize, err := jobutils.GetMemoryStorageSize(workloadObj, jobutils.TestDeploymentTemplate)
	assert.NilError(t, err)
	assert.Equal(t, shareMemorySize, "32Gi")
}

func TestUpdatePytorchJob(t *testing.T) {
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")

	workloadObj, err := jsonutils.ParseYamlToJson(jobutils.TestPytorchData)
	assert.NilError(t, err)
	adminWorkload := jobutils.TestWorkloadData.DeepCopy()
	adminWorkload.Spec.Resource = v1.WorkloadResource{
		Replica:          3,
		CPU:              "64",
		GPU:              "8",
		GPUName:          "amd.com/gpu",
		Memory:           "512Gi",
		SharedMemory:     "512Gi",
		EphemeralStorage: "100Gi",
		RdmaResource:     "1k",
	}
	metav1.SetMetaDataAnnotation(&adminWorkload.ObjectMeta, v1.EnableHostNetworkAnnotation, "true")
	metav1.SetMetaDataAnnotation(&adminWorkload.ObjectMeta, v1.MainContainerAnnotation, "pytorch")
	err = applyWorkloadSpecToObject(context.Background(), nil, workloadObj, adminWorkload, nil, jobutils.TestPytorchResourceTemplate)
	assert.NilError(t, err)

	pytorchJob := &PytorchJob{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(workloadObj.Object, pytorchJob)
	assert.NilError(t, err)
	assert.Equal(t, pytorchJob.Spec.PytorchReplicaSpecs.Master.Replicas, 1)
	template := pytorchJob.Spec.PytorchReplicaSpecs.Master.Template
	assert.Equal(t, len(template.Spec.Containers), 1)
	assert.Equal(t, template.Spec.Containers[0].Resources.Limits.Cpu().Value(), int64(64))
	assert.Equal(t, template.Spec.Containers[0].Resources.Limits.Memory().String(), "512Gi")
	gpuQuantity, ok := template.Spec.Containers[0].Resources.Limits[common.AmdGpu]
	assert.Equal(t, ok, true)
	assert.Equal(t, gpuQuantity.Value(), int64(8))
	rdmaQuantity, ok := template.Spec.Containers[0].Resources.Limits[corev1.ResourceName(commonconfig.GetRdmaName())]
	assert.Equal(t, ok, true)
	assert.Equal(t, rdmaQuantity.Value(), int64(1000))
	assert.Equal(t, pytorchJob.Spec.PytorchReplicaSpecs.Master.Template.Spec.PriorityClassName,
		commonworkload.GeneratePriorityClass(adminWorkload))

	assert.Equal(t, pytorchJob.Spec.PytorchReplicaSpecs.Worker.Replicas, 2)
	template = pytorchJob.Spec.PytorchReplicaSpecs.Worker.Template
	assert.Equal(t, len(template.Spec.Containers), 1)
	assert.Equal(t, template.Spec.Containers[0].Resources.Limits.Cpu().Value(), int64(64))
	assert.Equal(t, template.Spec.Containers[0].Resources.Limits.Memory().String(), "512Gi")
	gpuQuantity, ok = template.Spec.Containers[0].Resources.Limits[common.AmdGpu]
	assert.Equal(t, ok, true)
	assert.Equal(t, gpuQuantity.Value(), int64(8))
	rdmaQuantity, ok = template.Spec.Containers[0].Resources.Limits[corev1.ResourceName(commonconfig.GetRdmaName())]
	assert.Equal(t, ok, true)
	assert.Equal(t, rdmaQuantity.Value(), int64(1000))
}

func TestUpdatePytorchJobMaster(t *testing.T) {
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")

	workloadObj, err := jsonutils.ParseYamlToJson(jobutils.TestPytorchData)
	assert.NilError(t, err)
	adminWorkload := jobutils.TestWorkloadData.DeepCopy()
	adminWorkload.Spec.Resource.RdmaResource = ""
	metav1.SetMetaDataAnnotation(&adminWorkload.ObjectMeta, v1.MainContainerAnnotation, "pytorch")
	err = applyWorkloadSpecToObject(context.Background(), nil, workloadObj, adminWorkload, nil, jobutils.TestPytorchResourceTemplate)
	assert.NilError(t, err)

	pytorchJob := &PytorchJob{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(workloadObj.Object, pytorchJob)
	assert.NilError(t, err)
	assert.Equal(t, pytorchJob.Spec.PytorchReplicaSpecs.Master.Replicas, 1)
	template := pytorchJob.Spec.PytorchReplicaSpecs.Master.Template
	assert.Equal(t, len(template.Spec.Containers), 1)
	assert.Equal(t, template.Spec.Containers[0].Resources.Limits.Cpu().Value(), int64(32))
	assert.Equal(t, template.Spec.Containers[0].Resources.Limits.Memory().String(), "256Gi")
	gpuQuantity, ok := template.Spec.Containers[0].Resources.Limits[common.AmdGpu]
	assert.Equal(t, ok, true)
	assert.Equal(t, gpuQuantity.Value(), int64(4))
	_, ok = template.Spec.Containers[0].Resources.Limits[corev1.ResourceName(commonconfig.GetRdmaName())]
	assert.Equal(t, ok, false)

	assert.Equal(t, pytorchJob.Spec.PytorchReplicaSpecs.Worker.Replicas, 0)
}

func TestIsImageChanged(t *testing.T) {
	workloadObj, err := jsonutils.ParseYamlToJson(jobutils.TestDeploymentData)
	assert.NilError(t, err)
	adminWorkload := jobutils.TestWorkloadData.DeepCopy()
	metav1.SetMetaDataAnnotation(&adminWorkload.ObjectMeta, v1.MainContainerAnnotation, "test")

	adminWorkload.Spec.Image = "test-image:latest"
	ok := isImageChanged(adminWorkload, workloadObj, jobutils.TestDeploymentTemplate)
	assert.Equal(t, ok, false)

	adminWorkload.Spec.Image = "test-image:1234"
	ok = isImageChanged(adminWorkload, workloadObj, jobutils.TestDeploymentTemplate)
	assert.Equal(t, ok, true)
}

func TestIsPriorityClassChanged(t *testing.T) {
	workloadObj, err := jsonutils.ParseYamlToJson(jobutils.TestPytorchData)
	assert.NilError(t, err)
	adminWorkload := jobutils.TestWorkloadData.DeepCopy()
	adminWorkload.Spec.Priority = common.MedPriorityInt
	v1.SetLabel(adminWorkload, v1.ClusterIdLabel, "test")
	ok := isPriorityClassChanged(adminWorkload, workloadObj, jobutils.TestPytorchResourceTemplate)
	assert.Equal(t, ok, false)

	adminWorkload.Spec.Priority = common.HighPriorityInt
	ok = isPriorityClassChanged(adminWorkload, workloadObj, jobutils.TestPytorchResourceTemplate)
	assert.Equal(t, ok, true)
}

func TestIsShareMemoryChanged(t *testing.T) {
	workloadObj, err := jsonutils.ParseYamlToJson(jobutils.TestDeploymentData)
	assert.NilError(t, err)
	adminWorkload := jobutils.TestWorkloadData.DeepCopy()

	adminWorkload.Spec.Resource.SharedMemory = "20Gi"
	ok := isSharedMemoryChanged(adminWorkload, workloadObj, jobutils.TestDeploymentTemplate)
	assert.Equal(t, ok, false)

	adminWorkload.Spec.Resource.SharedMemory = "30Gi"
	ok = isSharedMemoryChanged(adminWorkload, workloadObj, jobutils.TestDeploymentTemplate)
	assert.Equal(t, ok, true)
}

func TestIsEnvChanged(t *testing.T) {
	workloadObj, err := jsonutils.ParseYamlToJson(jobutils.TestDeploymentData)
	assert.NilError(t, err)
	adminWorkload := jobutils.TestWorkloadData.DeepCopy()
	metav1.SetMetaDataAnnotation(&adminWorkload.ObjectMeta, v1.MainContainerAnnotation, "test")

	ok := isEnvChanged(adminWorkload, workloadObj, jobutils.TestDeploymentTemplate)
	assert.Equal(t, ok, true)

	adminWorkload.Spec.Env = map[string]string{
		"NCCL_SOCKET_IFNAME": "eth0",
	}
	ok = isEnvChanged(adminWorkload, workloadObj, jobutils.TestDeploymentTemplate)
	assert.Equal(t, ok, false)

	adminWorkload.Spec.Env = map[string]string{
		"NCCL_SOCKET_IFNAME": "eth1",
	}
	ok = isEnvChanged(adminWorkload, workloadObj, jobutils.TestDeploymentTemplate)
	assert.Equal(t, ok, true)

	adminWorkload = jobutils.TestWorkloadData.DeepCopy()
	metav1.SetMetaDataAnnotation(&adminWorkload.ObjectMeta, v1.MainContainerAnnotation, "test")
	adminWorkload.Spec.Env = map[string]string{
		"NCCL_SOCKET_IFNAME": "eth0",
		"GLOO_SOCKET_IFNAME": "",
	}
	ok = isEnvChanged(adminWorkload, workloadObj, jobutils.TestDeploymentTemplate)
	assert.Equal(t, ok, true)

	adminWorkload.Spec.Env = map[string]string{
		"NCCL_SOCKET_IFNAME": "eth0",
		"GLOO_SOCKET_IFNAME": "eth0",
		"key":                "val",
	}
	ok = isEnvChanged(adminWorkload, workloadObj, jobutils.TestDeploymentTemplate)
	assert.Equal(t, ok, true)
}

func TestUpdateDeploymentEnv(t *testing.T) {
	workloadObj, err := jsonutils.ParseYamlToJson(jobutils.TestDeploymentData)
	assert.NilError(t, err)
	adminWorkload := jobutils.TestWorkloadData.DeepCopy()
	metav1.SetMetaDataAnnotation(&adminWorkload.ObjectMeta, v1.MainContainerAnnotation, "test")

	err = applyWorkloadSpecToObject(context.Background(), nil, workloadObj, adminWorkload, nil, jobutils.TestDeploymentTemplate)
	assert.NilError(t, err)
	envs, err := jobutils.GetEnv(workloadObj, jobutils.TestDeploymentTemplate, "test")
	assert.NilError(t, err)
	assert.Equal(t, len(envs), 3)
	env, ok := envs[0].(map[string]interface{})
	assert.Equal(t, ok, true)
	assert.Equal(t, env["name"].(string), "NCCL_SOCKET_IFNAME")
	assert.Equal(t, env["value"].(string), "eth0")
	env, ok = envs[1].(map[string]interface{})
	assert.Equal(t, ok, true)
	assert.Equal(t, env["name"].(string), "GLOO_SOCKET_IFNAME")
	assert.Equal(t, env["value"].(string), "eth0")
	env, ok = envs[2].(map[string]interface{})
	assert.Equal(t, ok, true)
	assert.Equal(t, env["name"].(string), "key")
	assert.Equal(t, env["value"].(string), "value")

	adminWorkload.Spec.Env = map[string]string{
		"NCCL_SOCKET_IFNAME": "eth1",
		"key":                "val",
	}
	err = applyWorkloadSpecToObject(context.Background(), nil, workloadObj, adminWorkload, nil, jobutils.TestDeploymentTemplate)
	assert.NilError(t, err)
	envs, err = jobutils.GetEnv(workloadObj, jobutils.TestDeploymentTemplate, "test")
	assert.NilError(t, err)
	assert.Equal(t, len(envs), 3)
	env, ok = envs[0].(map[string]interface{})
	assert.Equal(t, ok, true)
	assert.Equal(t, env["name"].(string), "NCCL_SOCKET_IFNAME")
	assert.Equal(t, env["value"].(string), "eth1")
	env, ok = envs[1].(map[string]interface{})
	assert.Equal(t, ok, true)
	assert.Equal(t, env["name"].(string), "GLOO_SOCKET_IFNAME")
	assert.Equal(t, env["value"].(string), "eth0")
	env, ok = envs[2].(map[string]interface{})
	assert.Equal(t, ok, true)
	assert.Equal(t, env["name"].(string), "key")
	assert.Equal(t, env["value"].(string), "val")

	adminWorkload.Spec.Env = map[string]string{
		"NCCL_SOCKET_IFNAME": "eth1",
	}
	v1.SetAnnotation(adminWorkload, v1.EnvToBeRemovedAnnotation, string(jsonutils.MarshalSilently([]string{"key"})))
	err = applyWorkloadSpecToObject(context.Background(), nil, workloadObj, adminWorkload, nil, jobutils.TestDeploymentTemplate)
	assert.NilError(t, err)
	envs, err = jobutils.GetEnv(workloadObj, jobutils.TestDeploymentTemplate, "test")
	assert.NilError(t, err)
	assert.Equal(t, len(envs), 2)
	env, ok = envs[0].(map[string]interface{})
	assert.Equal(t, ok, true)
	assert.Equal(t, env["name"].(string), "NCCL_SOCKET_IFNAME")
	assert.Equal(t, env["value"].(string), "eth1")
	env, ok = envs[1].(map[string]interface{})
	assert.Equal(t, ok, true)
	assert.Equal(t, env["name"].(string), "GLOO_SOCKET_IFNAME")
	assert.Equal(t, env["value"].(string), "eth0")
}

func TestCreateK8sJob(t *testing.T) {
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")

	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.GroupVersionKind = v1.GroupVersionKind{
		Version: "v1",
		Kind:    common.JobKind,
	}
	workload.Spec.Workspace = corev1.NamespaceDefault
	workload.Spec.CustomerLabels = map[string]string{
		v1.K8sHostName: "node1",
	}
	workload.Spec.Resource.Replica = 2
	v1.SetAnnotation(workload, v1.UserNameAnnotation, common.UserSystem)
	v1.SetLabel(workload, v1.OpsJobTypeLabel, string(v1.OpsJobPreflightType))

	configmap, err := parseConfigmap(TestJobTemplateConfig)
	assert.NilError(t, err)
	metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap, jobutils.TestJobTemplate).WithScheme(scheme).Build()

	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)
	// fmt.Println(unstructuredutils.ToString(obj))

	templates := jobutils.TestJobTemplate.Spec.ResourceSpecs
	checkResources(t, obj, workload, &templates[0], workload.Spec.Resource.Replica)
	checkPorts(t, obj, workload, &templates[0])
	checkNodeSelectorTerms(t, obj, workload, &templates[0])
	checkEnvs(t, obj, workload, &templates[0])
	checkImage(t, obj, workload.Spec.Image, &templates[0])
	checkLabels(t, obj, workload, &templates[0])
	checkHostNetwork(t, obj, workload, &templates[0])
	checkHostPid(t, obj, workload, &templates[0])
	checkPriorityClass(t, obj, workload, &templates[0])
	checkSecurityContext(t, obj, workload, &templates[0])
}

func TestCreateCICDScaleSet(t *testing.T) {
	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.GroupVersionKind = v1.GroupVersionKind{
		Version: "v1",
		Kind:    common.CICDScaleRunnerSetKind,
	}
	workload.Spec.Env[common.GithubConfigUrl] = "test-url"
	workload.Spec.Env[common.AdminControlPlane] = "10.0.0.1"
	workload.Spec.Env[common.GithubSecretId] = "test-secret"
	workload.Spec.Workspace = workspace.Name
	workload.Spec.EntryPoint = stringutil.Base64Encode("bash test.sh")

	configmap, err := parseConfigmap(TestCICDScaleSetTemplateConfig)
	assert.NilError(t, err)
	metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap,
		jobutils.TestCICDScaleSetTemplate, workspace).WithScheme(scheme).Build()

	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)
	// fmt.Println(unstructuredutils.ToString(obj))

	templates := jobutils.TestJobTemplate.Spec.ResourceSpecs
	checkGithubConfig(t, obj)
	checkNodeSelectorTerms(t, obj, workload, &templates[0])
	checkLabels(t, obj, workload, &templates[0])
	checkSecurityContext(t, obj, workload, &templates[0])
	checkEnvs(t, obj, workload, &templates[0])
	checkImage(t, obj, workload.Spec.Image, &templates[0])
	checkHostNetwork(t, obj, workload, &templates[0])
	envs := getEnvs(t, obj, &templates[0])
	checkCICDEnvs(t, envs, workload)

	assert.Equal(t, getContainer(obj, "runner", &templates[0]) != nil, true)
	assert.Equal(t, getContainer(obj, "unified_job", &templates[0]) != nil, false)
}

func TestCICDScaleSetWithUnifiedJob(t *testing.T) {
	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.GroupVersionKind = v1.GroupVersionKind{
		Version: "v1",
		Kind:    common.CICDScaleRunnerSetKind,
	}
	workload.Spec.Resource.Replica = 2
	workload.Spec.Env[common.GithubConfigUrl] = "test-url"
	workload.Spec.Env[common.GithubSecretId] = "test-secret"
	workload.Spec.Env[common.AdminControlPlane] = "10.0.0.1"
	workload.Spec.Env[common.UnifiedJobEnable] = v1.TrueStr
	workload.Spec.Workspace = workspace.Name

	configmap, err := parseConfigmap(TestCICDScaleSetTemplateConfig)
	assert.NilError(t, err)
	v1.SetAnnotation(workload, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap,
		jobutils.TestCICDScaleSetTemplate, workspace).WithScheme(scheme).Build()

	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)
	// fmt.Println(unstructuredutils.ToString(obj))

	templates := jobutils.TestJobTemplate.Spec.ResourceSpecs
	checkNodeSelectorTerms(t, obj, workload, &templates[0])
	checkLabels(t, obj, workload, &templates[0])
	checkSecurityContext(t, obj, workload, &templates[0])
	checkEnvs(t, obj, workload, &templates[0])
	checkHostNetwork(t, obj, workload, &templates[0])

	checkCICDContainer(t, obj, workload, &templates[0],
		"runner", workload.Spec.Image)
	checkCICDContainer(t, obj, workload, &templates[0],
		"unified_job", "docker.io/primussafe/cicd-unified-job-proxy:latest")
}

func checkGithubConfig(t *testing.T, obj *unstructured.Unstructured) {
	specObject, found, err := unstructured.NestedMap(obj.Object, []string{"spec"}...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, len(specObject) == 0, false)

	val, found := specObject["githubConfigSecret"]
	assert.Equal(t, found, true)
	assert.Equal(t, val.(string), "test-secret")

	val, found = specObject["githubConfigUrl"]
	assert.Equal(t, found, true)
	assert.Equal(t, val.(string), "test-url")
}

func checkCICDContainer(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload,
	resourceSpec *v1.ResourceSpec, containerName, containerImage string) {
	container := getContainer(obj, containerName, resourceSpec)
	assert.Equal(t, container != nil, true)
	image, found, err := unstructured.NestedString(container, []string{"image"}...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, image, containerImage)
	envs, found, err := unstructured.NestedSlice(container, []string{"env"}...)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	checkCICDEnvs(t, envs, workload)
}

func checkCICDEnvs(t *testing.T, envs []interface{}, workload *v1.Workload) {
	var ok bool
	ok = findEnv(envs, common.ScaleRunnerSetID, workload.Name)
	assert.Equal(t, ok, true)
	ok = findEnv(envs, common.AdminControlPlane, "10.0.0.1")
	assert.Equal(t, ok, true)
	ok = findEnv(envs, "APISERVER_NODE_PORT", "32495")
	assert.Equal(t, ok, true)

	val, ok := workload.Spec.Env[common.UnifiedJobEnable]
	if ok && val == v1.TrueStr {
		ok = findEnv(envs, common.UnifiedJobEnable, v1.TrueStr)
		assert.Equal(t, ok, true)
		ok = findEnv(envs, jobutils.NfsInputEnv, UnifiedJobInput)
		assert.Equal(t, ok, true)
		ok = findEnv(envs, jobutils.NfsOutputEnv, UnifiedJobOutput)
		assert.Equal(t, ok, true)
	}
}

func TestUpdateContainerEnv(t *testing.T) {
	tests := []struct {
		name            string
		envs            map[string]string
		container       map[string]interface{}
		toBeRemovedKeys []string
		expectedEnvs    []map[string]interface{}
		expectNoChange  bool
	}{
		{
			name:            "empty envs and toBeRemovedKeys should not change container",
			envs:            map[string]string{},
			container:       map[string]interface{}{},
			toBeRemovedKeys: []string{},
			expectedEnvs:    nil,
			expectNoChange:  true,
		},
		{
			name: "add new envs to container with no existing envs",
			envs: map[string]string{
				"KEY1": "value1",
				"KEY2": "value2",
			},
			container:       map[string]interface{}{},
			toBeRemovedKeys: []string{},
			expectedEnvs: []map[string]interface{}{
				{"name": "KEY1", "value": "value1"},
				{"name": "KEY2", "value": "value2"},
			},
		},
		{
			name: "add new envs to container with existing envs",
			envs: map[string]string{
				"KEY3": "value3",
			},
			container: map[string]interface{}{
				"env": []interface{}{
					map[string]interface{}{"name": "KEY1", "value": "value1"},
					map[string]interface{}{"name": "KEY2", "value": "value2"},
				},
			},
			toBeRemovedKeys: []string{},
			expectedEnvs: []map[string]interface{}{
				{"name": "KEY1", "value": "value1"},
				{"name": "KEY2", "value": "value2"},
				{"name": "KEY3", "value": "value3"},
			},
		},
		{
			name: "update existing env value",
			envs: map[string]string{
				"KEY1": "new_value1",
			},
			container: map[string]interface{}{
				"env": []interface{}{
					map[string]interface{}{"name": "KEY1", "value": "old_value1"},
					map[string]interface{}{"name": "KEY2", "value": "value2"},
				},
			},
			toBeRemovedKeys: []string{},
			expectedEnvs: []map[string]interface{}{
				{"name": "KEY1", "value": "new_value1"},
				{"name": "KEY2", "value": "value2"},
			},
		},
		{
			name: "remove env vars",
			envs: map[string]string{},
			container: map[string]interface{}{
				"env": []interface{}{
					map[string]interface{}{"name": "KEY1", "value": "value1"},
					map[string]interface{}{"name": "KEY2", "value": "value2"},
					map[string]interface{}{"name": "KEY3", "value": "value3"},
				},
			},
			toBeRemovedKeys: []string{"KEY2"},
			expectedEnvs: []map[string]interface{}{
				{"name": "KEY1", "value": "value1"},
				{"name": "KEY3", "value": "value3"},
			},
		},
		{
			name: "combined add update and remove",
			envs: map[string]string{
				"KEY1": "updated_value1",
				"KEY4": "value4",
			},
			container: map[string]interface{}{
				"env": []interface{}{
					map[string]interface{}{"name": "KEY1", "value": "value1"},
					map[string]interface{}{"name": "KEY2", "value": "value2"},
					map[string]interface{}{"name": "KEY3", "value": "value3"},
				},
			},
			toBeRemovedKeys: []string{"KEY2"},
			expectedEnvs: []map[string]interface{}{
				{"name": "KEY1", "value": "updated_value1"},
				{"name": "KEY3", "value": "value3"},
				{"name": "KEY4", "value": "value4"},
			},
		},
		{
			name: "no changes when existing values match new values",
			envs: map[string]string{
				"KEY1": "value1",
			},
			container: map[string]interface{}{
				"env": []interface{}{
					map[string]interface{}{"name": "KEY1", "value": "value1"},
				},
			},
			toBeRemovedKeys: []string{},
			expectedEnvs:    nil,
			expectNoChange:  true,
		},
		{
			name: "skip malformed env entry without name",
			envs: map[string]string{
				"KEY2": "value2",
			},
			container: map[string]interface{}{
				"env": []interface{}{
					map[string]interface{}{"value": "value1"},
					map[string]interface{}{"name": "KEY3", "value": "value3"},
				},
			},
			toBeRemovedKeys: []string{},
			expectedEnvs: []map[string]interface{}{
				{"name": "KEY3", "value": "value3"},
				{"name": "KEY2", "value": "value2"},
			},
		},
		{
			name: "skip non-map env entry",
			envs: map[string]string{
				"KEY2": "value2",
			},
			container: map[string]interface{}{
				"env": []interface{}{
					"invalid_entry",
					map[string]interface{}{"name": "KEY3", "value": "value3"},
				},
			},
			toBeRemovedKeys: []string{},
			expectedEnvs: []map[string]interface{}{
				{"name": "KEY3", "value": "value3"},
				{"name": "KEY2", "value": "value2"},
			},
		},
		{
			name: "remove multiple keys",
			envs: map[string]string{},
			container: map[string]interface{}{
				"env": []interface{}{
					map[string]interface{}{"name": "KEY1", "value": "value1"},
					map[string]interface{}{"name": "KEY2", "value": "value2"},
					map[string]interface{}{"name": "KEY3", "value": "value3"},
				},
			},
			toBeRemovedKeys: []string{"KEY1", "KEY3"},
			expectedEnvs: []map[string]interface{}{
				{"name": "KEY2", "value": "value2"},
			},
		},
		{
			name: "env entry without value field should be preserved when not updated",
			envs: map[string]string{
				"KEY2": "new_value2",
			},
			container: map[string]interface{}{
				"env": []interface{}{
					map[string]interface{}{"name": "KEY1", "valueFrom": map[string]interface{}{"secretKeyRef": "secret"}},
					map[string]interface{}{"name": "KEY2", "value": "value2"},
				},
			},
			toBeRemovedKeys: []string{},
			expectedEnvs: []map[string]interface{}{
				{"name": "KEY1", "valueFrom": map[string]interface{}{"secretKeyRef": "secret"}},
				{"name": "KEY2", "value": "new_value2"},
			},
		},
		{
			name: "update env that has valueFrom to value",
			envs: map[string]string{
				"KEY1": "new_value1",
			},
			container: map[string]interface{}{
				"env": []interface{}{
					map[string]interface{}{"name": "KEY1", "valueFrom": map[string]interface{}{"secretKeyRef": "secret"}},
				},
			},
			toBeRemovedKeys: []string{},
			expectedEnvs:    nil,
			expectNoChange:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			containerCopy := make(map[string]interface{})
			for k, v := range tt.container {
				containerCopy[k] = v
			}

			updateContainerEnv(tt.envs, containerCopy, tt.toBeRemovedKeys)

			if tt.expectNoChange {
				if tt.container["env"] == nil {
					_, exists := containerCopy["env"]
					assert.Equal(t, exists, false, "env should not be added when no changes")
				}
				return
			}

			envs, ok := containerCopy["env"].([]interface{})
			assert.Equal(t, ok, true, "env should exist in container")

			assert.Equal(t, len(envs), len(tt.expectedEnvs), "env count mismatch")

			for _, expectedEnv := range tt.expectedEnvs {
				found := false
				expectedName := expectedEnv["name"].(string)
				for _, env := range envs {
					envMap := env.(map[string]interface{})
					if envMap["name"] == expectedName {
						found = true
						if expectedVal, hasVal := expectedEnv["value"]; hasVal {
							assert.Equal(t, envMap["value"], expectedVal, "value mismatch for "+expectedName)
						}
						if expectedValFrom, hasValFrom := expectedEnv["valueFrom"]; hasValFrom {
							assert.DeepEqual(t, envMap["valueFrom"], expectedValFrom)
						}
						break
					}
				}
				assert.Equal(t, found, true, "expected env "+expectedName+" not found")
			}
		})
	}
}
