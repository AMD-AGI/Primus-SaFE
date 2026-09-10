/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gotest.tools/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	unstructuredutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/unstructured"
	"github.com/agiledragon/gomonkey/v2"
)

func TestValidateGithubRunnerRBAC(t *testing.T) {
	ctx := context.Background()
	namespace := "runner-workspace"
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Name: common.GithubRunnerServiceAccount, Namespace: namespace,
	}}
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: common.GithubRunnerServiceAccount},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{"apps"}, Resources: []string{"statefulsets"}, Verbs: []string{"get"},
		}},
	}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: common.GithubRunnerServiceAccount, Namespace: namespace},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     common.ClusterRoleKind,
			Name:     common.GithubRunnerServiceAccount,
		},
		Subjects: []rbacv1.Subject{{
			Kind: "ServiceAccount", Name: common.GithubRunnerServiceAccount, Namespace: namespace,
		}},
	}

	assert.ErrorContains(t, validateGithubRunnerRBAC(ctx, k8sfake.NewSimpleClientset(), namespace),
		"ServiceAccount")
	assert.ErrorContains(t, validateGithubRunnerRBAC(ctx, k8sfake.NewSimpleClientset(sa), namespace),
		"RoleBinding")
	assert.ErrorContains(t, validateGithubRunnerRBAC(ctx, k8sfake.NewSimpleClientset(sa, binding), namespace),
		"ClusterRole")
	assert.NilError(t, validateGithubRunnerRBAC(ctx, k8sfake.NewSimpleClientset(sa, binding, role), namespace))
}

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
	err = networkingv1.AddToScheme(result)
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

	checkResources(t, obj, workload, &templates[0], 1, 0)
	checkPorts(t, obj, workload, &templates[0], 0)
	checkEnvs(t, obj, workload, &templates[0], 0)
	checkVolumeMounts(t, obj, workload, &templates[0])
	checkVolumes(t, obj, workload, &templates[0], 0)
	checkRequiredNodeSelectorTerms(t, obj, workload, &templates[0])
	checkImage(t, obj, workload, &templates[0], 0)
	checkLabels(t, obj, workload, &templates[0], 0)
	checkHostNetwork(t, obj, workload, &templates[0], 0)
	checkTolerations(t, obj, workload, &templates[0])
	checkPriorityClass(t, obj, workload, &templates[0])
	checkImageSecrets(t, obj, workload, &templates[0])
	_, found, err := jobutils.NestedSlice(obj.Object, templates[1].PrePaths)
	assert.NilError(t, err)
	assert.Equal(t, found, false)

	// enable worker
	workload.Spec.Resources = append(workload.Spec.Resources, *workload.Spec.Resources[0].DeepCopy())
	workload.Spec.Resources[1].Replica = 2
	workload.Spec.Images = append(workload.Spec.Images, workload.Spec.Images[0])
	workload.Spec.EntryPoints = append(workload.Spec.EntryPoints, workload.Spec.EntryPoints[0])
	workload.Spec.IsTolerateAll = true
	obj, err = r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)
	checkResources(t, obj, workload, &templates[1], 2, 1)
	checkEnvs(t, obj, workload, &templates[1], 1)
	checkPorts(t, obj, workload, &templates[1], 1)
	checkVolumeMounts(t, obj, workload, &templates[1])
	checkVolumes(t, obj, workload, &templates[1], 1)
	checkRequiredNodeSelectorTerms(t, obj, workload, &templates[1])
	checkImage(t, obj, workload, &templates[1], 1)
	checkLabels(t, obj, workload, &templates[1], 1)
	checkHostNetwork(t, obj, workload, &templates[1], 1)
	checkTolerations(t, obj, workload, &templates[1])
	checkPriorityClass(t, obj, workload, &templates[1])
	checkImageSecrets(t, obj, workload, &templates[1])
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
	adminClient := fake.NewClientBuilder().WithObjects(configmap, jobutils.TestDeploymentResourceTemplate, workspace).WithScheme(scheme).Build()

	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)
	templates := jobutils.TestDeploymentResourceTemplate.Spec.ResourceSpecs

	checkResources(t, obj, workload, &templates[0], 1, 0)
	checkPorts(t, obj, workload, &templates[0], 0)
	checkEnvs(t, obj, workload, &templates[0], 0)
	checkVolumeMounts(t, obj, workload, &templates[0])
	checkVolumes(t, obj, workload, &templates[0], 0)
	checkRequiredNodeSelectorTerms(t, obj, workload, &templates[0])
	checkImage(t, obj, workload, &templates[0], 0)
	checkLabels(t, obj, workload, &templates[0], 0)
	checkHostNetwork(t, obj, workload, &templates[0], 0)
	checkSelector(t, obj, workload)
	checkStrategy(t, obj, workload)
	// fmt.Println(unstructuredutils.ToString(obj))
}

func TestUpdateDeployment(t *testing.T) {
	workloadObj, err := jsonutils.ParseYamlToJson(jobutils.TestDeploymentData)
	assert.NilError(t, err)
	adminWorkload := jobutils.TestWorkloadData.DeepCopy()
	metav1.SetMetaDataAnnotation(&adminWorkload.ObjectMeta, v1.MainContainerAnnotation, "test")

	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects().WithScheme(scheme).Build()
	r := DispatcherReconciler{Client: adminClient}

	err = r.applyWorkloadSpecToObject(context.Background(), nil, workloadObj, adminWorkload, nil, jobutils.TestDeploymentResourceTemplate, nil)
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
	cmd := buildEntryPoint(adminWorkload, 0)
	assert.Equal(t, deployment.Spec.Template.Spec.Containers[0].Command[2], "exec "+cmd)

	shareMemorySizes, err := jobutils.GetMemoryStorageSize(workloadObj, jobutils.TestDeploymentResourceTemplate, 1)
	assert.NilError(t, err)
	assert.Equal(t, len(shareMemorySizes), 1)
	assert.Equal(t, shareMemorySizes[0], "32Gi")
}

func TestUpdatePytorchJob(t *testing.T) {
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")

	workloadObj, err := jsonutils.ParseYamlToJson(jobutils.TestPytorchData)
	assert.NilError(t, err)
	adminWorkload := jobutils.TestWorkloadData.DeepCopy()

	adminWorkload.Spec.Resources[0] = v1.WorkloadResource{
		Replica:          1,
		CPU:              "64",
		GPU:              "8",
		GPUName:          "amd.com/gpu",
		Memory:           "512Gi",
		SharedMemory:     "512Gi",
		EphemeralStorage: "100Gi",
		RdmaResource:     "1k",
	}
	adminWorkload.Spec.Resources = append(adminWorkload.Spec.Resources, *adminWorkload.Spec.Resources[0].DeepCopy())
	adminWorkload.Spec.Resources[1].Replica = 2

	metav1.SetMetaDataAnnotation(&adminWorkload.ObjectMeta, v1.MainContainerAnnotation, "pytorch")
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects().WithScheme(scheme).Build()
	r := DispatcherReconciler{Client: adminClient}
	err = r.applyWorkloadSpecToObject(context.Background(), nil, workloadObj, adminWorkload, nil, jobutils.TestPytorchResourceTemplate, nil)
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
	adminWorkload.Spec.Resources[0].RdmaResource = ""
	metav1.SetMetaDataAnnotation(&adminWorkload.ObjectMeta, v1.MainContainerAnnotation, "pytorch")
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects().WithScheme(scheme).Build()
	r := DispatcherReconciler{Client: adminClient}
	err = r.applyWorkloadSpecToObject(context.Background(), nil, workloadObj, adminWorkload, nil, jobutils.TestPytorchResourceTemplate, nil)
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
	workloadObj, err := jsonutils.ParseYamlToJson(jobutils.TestPytorchData)
	assert.NilError(t, err)
	adminWorkload := jobutils.TestWorkloadData.DeepCopy()
	metav1.SetMetaDataAnnotation(&adminWorkload.ObjectMeta, v1.MainContainerAnnotation, "pytorch")

	adminWorkload.Spec.Images = []string{"test-image:0.0.1", "docker.io/test-image:0.0.1"}
	adminWorkload.Spec.Resources = append(adminWorkload.Spec.Resources, adminWorkload.Spec.Resources[0])
	ok := isImagesChanged(adminWorkload, workloadObj, jobutils.TestPytorchResourceTemplate)
	assert.Equal(t, ok, false)

	adminWorkload.Spec.Images[0] = "docker.io/test-image:0.0.1"
	adminWorkload.Spec.Resources = adminWorkload.Spec.Resources[0:1]
	ok = isImagesChanged(adminWorkload, workloadObj, jobutils.TestPytorchResourceTemplate)
	assert.Equal(t, ok, true)
}

func TestIsEntrypointChanged(t *testing.T) {
	workloadObj, err := jsonutils.ParseYamlToJson(jobutils.TestStatefulSetData)
	assert.NilError(t, err)
	adminWorkload := jobutils.TestWorkloadData.DeepCopy()
	metav1.SetMetaDataAnnotation(&adminWorkload.ObjectMeta, v1.MainContainerAnnotation, "main")

	adminWorkload.Spec.EntryPoints = []string{"abcd"}
	ok := isEntrypointChanged(adminWorkload, workloadObj, jobutils.TestStatefulSetResourceTemplate)
	assert.Equal(t, ok, false)

	adminWorkload.Spec.EntryPoints = []string{"1234"}
	ok = isEntrypointChanged(adminWorkload, workloadObj, jobutils.TestStatefulSetResourceTemplate)
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

func TestIsCICDSecretChanged(t *testing.T) {
	runnerSetData, err := jsonutils.ParseYamlToJson(jobutils.TestAutoscalingRunnerSetData)
	assert.NilError(t, err)
	adminWorkload := jobutils.TestWorkloadData.DeepCopy()
	adminWorkload.Spec.Kind = common.CICDScaleRunnerSetKind
	v1.SetAnnotation(adminWorkload, v1.GithubSecretIdAnnotation, "primus-safe-cicd")
	ok := isGithubSecretChanged(adminWorkload, runnerSetData, nil)
	assert.Equal(t, ok, false)

	v1.SetAnnotation(adminWorkload, v1.GithubSecretIdAnnotation, "test-cicd")
	ok = isGithubSecretChanged(adminWorkload, runnerSetData, nil)
	assert.Equal(t, ok, true)
}

func TestIsGithubSecretChangedGetEnvError(t *testing.T) {
	adminWorkload := jobutils.TestWorkloadData.DeepCopy()
	adminWorkload.Spec.Kind = common.CICDGithubRunnerKind
	v1.SetAnnotation(adminWorkload, v1.GithubSecretIdAnnotation, "runner-secret")
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	assert.Equal(t, isGithubSecretChanged(adminWorkload, obj, jobutils.TestStatefulSetResourceTemplate), false)
}

func TestIsShareMemoryChanged(t *testing.T) {
	workloadObj, err := jsonutils.ParseYamlToJson(jobutils.TestDeploymentData)
	assert.NilError(t, err)
	adminWorkload := jobutils.TestWorkloadData.DeepCopy()

	adminWorkload.Spec.Resources[0].SharedMemory = "20Gi"
	ok := isSharedMemoryChanged(adminWorkload, workloadObj, jobutils.TestDeploymentResourceTemplate)
	assert.Equal(t, ok, false)

	adminWorkload.Spec.Resources[0].SharedMemory = "30Gi"
	ok = isSharedMemoryChanged(adminWorkload, workloadObj, jobutils.TestDeploymentResourceTemplate)
	assert.Equal(t, ok, true)
}

func TestIsEnvChanged(t *testing.T) {
	workloadObj, err := jsonutils.ParseYamlToJson(jobutils.TestDeploymentData)
	assert.NilError(t, err)
	adminWorkload := jobutils.TestWorkloadData.DeepCopy()
	metav1.SetMetaDataAnnotation(&adminWorkload.ObjectMeta, v1.MainContainerAnnotation, "test")

	ok := isEnvChanged(adminWorkload, workloadObj, jobutils.TestDeploymentResourceTemplate)
	assert.Equal(t, ok, true)

	adminWorkload.Spec.Env = map[string]string{
		"NCCL_SOCKET_IFNAME": "eth0",
	}
	ok = isEnvChanged(adminWorkload, workloadObj, jobutils.TestDeploymentResourceTemplate)
	assert.Equal(t, ok, false)

	adminWorkload.Spec.Env = map[string]string{
		"NCCL_SOCKET_IFNAME": "eth1",
	}
	ok = isEnvChanged(adminWorkload, workloadObj, jobutils.TestDeploymentResourceTemplate)
	assert.Equal(t, ok, true)

	adminWorkload = jobutils.TestWorkloadData.DeepCopy()
	metav1.SetMetaDataAnnotation(&adminWorkload.ObjectMeta, v1.MainContainerAnnotation, "test")
	adminWorkload.Spec.Env = map[string]string{
		"NCCL_SOCKET_IFNAME": "eth0",
		"GLOO_SOCKET_IFNAME": "",
	}
	ok = isEnvChanged(adminWorkload, workloadObj, jobutils.TestDeploymentResourceTemplate)
	assert.Equal(t, ok, true)

	adminWorkload.Spec.Env = map[string]string{
		"NCCL_SOCKET_IFNAME": "eth0",
		"GLOO_SOCKET_IFNAME": "eth0",
		"key":                "val",
	}
	ok = isEnvChanged(adminWorkload, workloadObj, jobutils.TestDeploymentResourceTemplate)
	assert.Equal(t, ok, true)
}

func TestUpdateDeploymentEnv(t *testing.T) {
	workloadObj, err := jsonutils.ParseYamlToJson(jobutils.TestDeploymentData)
	assert.NilError(t, err)
	adminWorkload := jobutils.TestWorkloadData.DeepCopy()
	metav1.SetMetaDataAnnotation(&adminWorkload.ObjectMeta, v1.MainContainerAnnotation, "test")

	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects().WithScheme(scheme).Build()
	r := DispatcherReconciler{Client: adminClient}
	err = r.applyWorkloadSpecToObject(context.Background(), nil, workloadObj, adminWorkload, nil, jobutils.TestDeploymentResourceTemplate, nil)
	assert.NilError(t, err)
	envs, err := jobutils.GetEnv(workloadObj, jobutils.TestDeploymentResourceTemplate, 1)
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

	err = r.applyWorkloadSpecToObject(context.Background(), nil, workloadObj, adminWorkload, nil, jobutils.TestDeploymentResourceTemplate, nil)
	assert.NilError(t, err)
	envs, err = jobutils.GetEnv(workloadObj, jobutils.TestDeploymentResourceTemplate, 1)
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

	err = r.applyWorkloadSpecToObject(context.Background(), nil, workloadObj, adminWorkload, nil, jobutils.TestDeploymentResourceTemplate, nil)
	assert.NilError(t, err)
	envs, err = jobutils.GetEnv(workloadObj, jobutils.TestDeploymentResourceTemplate, 1)
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

func TestCreatePreflightJob(t *testing.T) {
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
	workload.Spec.Resources[0].Replica = 2
	v1.SetAnnotation(workload, v1.UserNameAnnotation, common.UserSystem)
	v1.SetAnnotation(workload, v1.RequireNodeSpreadAnnotation, v1.TrueStr)
	v1.SetLabel(workload, v1.OpsJobTypeLabel, string(v1.OpsJobPreflightType))
	v1.SetAnnotation(workload, v1.WorkloadPrivilegedAnnotation, v1.TrueStr)

	configmap, err := parseConfigmap(TestJobTemplateConfig)
	assert.NilError(t, err)
	metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap, jobutils.TestJobResourceTemplate).WithScheme(scheme).Build()

	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)
	// fmt.Println(unstructuredutils.ToString(obj))

	templates := jobutils.TestJobResourceTemplate.Spec.ResourceSpecs
	checkResources(t, obj, workload, &templates[0], workload.Spec.Resources[0].Replica, 0)
	checkPorts(t, obj, workload, &templates[0], 0)
	checkRequiredNodeSelectorTerms(t, obj, workload, &templates[0])
	checkPodAntiAffinity(t, obj, workload, &templates[0])
	checkEnvs(t, obj, workload, &templates[0], 0)
	checkImage(t, obj, workload, &templates[0], 0)
	checkLabels(t, obj, workload, &templates[0], 0)
	checkHostNetwork(t, obj, workload, &templates[0], 0)
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
	v1.SetAnnotation(workload, v1.AdminControlPlaneAnnotation, "10.0.0.1")
	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, "test-secret")
	workload.Spec.Workspace = workspace.Name
	workload.Spec.EntryPoints = []string{stringutil.Base64Encode("bash test.sh")}

	configmap, err := parseConfigmap(TestCICDScaleSetTemplateConfig)
	assert.NilError(t, err)
	metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap,
		jobutils.TestCICDScaleSetResourceTemplate, workspace).WithScheme(scheme).Build()

	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)
	// fmt.Println(unstructuredutils.ToString(obj))

	templates := jobutils.TestJobResourceTemplate.Spec.ResourceSpecs
	checkGithubConfig(t, obj)
	checkRequiredNodeSelectorTerms(t, obj, workload, &templates[0])
	checkLabels(t, obj, workload, &templates[0], 0)
	checkSecurityContext(t, obj, workload, &templates[0])
	checkEnvs(t, obj, workload, &templates[0], 0)
	checkImage(t, obj, workload, &templates[0], 0)
	checkHostNetwork(t, obj, workload, &templates[0], 0)
	envs := getEnvs(t, obj, workload, &templates[0])
	checkCICDEnvs(t, envs, workload)
	checkCICDRunnerEnvFieldRefs(t, envs)

	assert.Equal(t, getContainer(obj, "runner", workload, &templates[0]) != nil, true)
	assert.Equal(t, getContainer(obj, "unified_job", workload, &templates[0]) != nil, false)
}

func TestCICDScaleSetWithUnifiedJob(t *testing.T) {
	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.GroupVersionKind = v1.GroupVersionKind{
		Version: "v1",
		Kind:    common.CICDScaleRunnerSetKind,
	}
	workload.Spec.Resources[0].Replica = 2
	workload.Spec.Env[common.GithubConfigUrl] = "test-url"
	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, "test-secret")
	v1.SetAnnotation(workload, v1.AdminControlPlaneAnnotation, "10.0.0.1")
	workload.Spec.Env[common.UnifiedJobEnable] = v1.TrueStr
	workload.Spec.Workspace = workspace.Name

	configmap, err := parseConfigmap(TestCICDScaleSetTemplateConfig)
	assert.NilError(t, err)
	v1.SetAnnotation(workload, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap,
		jobutils.TestCICDScaleSetResourceTemplate, workspace).WithScheme(scheme).Build()

	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)
	// fmt.Println(unstructuredutils.ToString(obj))

	templates := jobutils.TestJobResourceTemplate.Spec.ResourceSpecs
	checkRequiredNodeSelectorTerms(t, obj, workload, &templates[0])
	checkLabels(t, obj, workload, &templates[0], 0)
	checkSecurityContext(t, obj, workload, &templates[0])
	checkEnvs(t, obj, workload, &templates[0], 0)
	checkHostNetwork(t, obj, workload, &templates[0], 0)

	checkCICDContainer(t, obj, workload, &templates[0],
		"runner", workload.Spec.Images[0])
	checkCICDContainer(t, obj, workload, &templates[0],
		"unified_job", "docker.io/primussafe/cicd-unified-job-proxy:latest")
}

func checkGithubConfig(t *testing.T, obj *unstructured.Unstructured) {
	specObject, found, err := jobutils.NestedMap(obj.Object, []string{"spec"})
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
	container := getContainer(obj, containerName, workload, resourceSpec)
	assert.Equal(t, container != nil, true)
	image, found, err := jobutils.NestedString(container, []string{"image"})
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, image, containerImage)
	envs, found, err := jobutils.NestedSlice(container, []string{"env"})
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	checkCICDEnvs(t, envs, workload)
	if containerName == "runner" {
		checkCICDRunnerEnvFieldRefs(t, envs)
	}
}

func checkCICDEnvs(t *testing.T, envs []interface{}, workload *v1.Workload) {
	var ok bool
	ok = findEnv(envs, common.ScaleRunnerSetID, workload.Name)
	assert.Equal(t, ok, true)
	ok = findEnv(envs, jobutils.AdminControlPlaneEnv, "10.0.0.1")
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

func checkCICDRunnerEnvFieldRefs(t *testing.T, envs []interface{}) {
	ok := findEnvFieldRef(envs, "POD_NAME", "metadata.name")
	assert.Equal(t, ok, true)
	ok = findEnvFieldRef(envs, "HOSTNAME", "spec.nodeName")
	assert.Equal(t, ok, true)
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
			name: "replace secret reference with a literal value",
			envs: map[string]string{"KEY1": "literal-value"},
			container: map[string]interface{}{
				"env": []interface{}{
					map[string]interface{}{"name": "KEY1", "valueFrom": map[string]interface{}{
						"secretKeyRef": map[string]interface{}{"name": "settings", "key": "key1"},
					}},
				},
			},
			expectedEnvs: []map[string]interface{}{
				{"name": "KEY1", "value": "literal-value"},
			},
		},
		{
			name: "replace config map reference with a literal value",
			envs: map[string]string{"KEY1": "literal-value"},
			container: map[string]interface{}{
				"env": []interface{}{
					map[string]interface{}{"name": "KEY1", "valueFrom": map[string]interface{}{
						"configMapKeyRef": map[string]interface{}{"name": "settings", "key": "key1"},
					}},
				},
			},
			expectedEnvs: []map[string]interface{}{
				{"name": "KEY1", "value": "literal-value"},
			},
		},
		{
			name: "preserve downward API env when a literal value is present",
			envs: map[string]string{
				"HOSTNAME": "literal-hostname",
			},
			container: map[string]interface{}{
				"env": []interface{}{
					map[string]interface{}{"name": "HOSTNAME", "valueFrom": map[string]interface{}{
						"fieldRef": map[string]interface{}{"fieldPath": "spec.nodeName"},
					}},
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
				assert.DeepEqual(t, containerCopy, tt.container)
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
						assert.DeepEqual(t, envMap, expectedEnv)
						break
					}
				}
				assert.Equal(t, found, true, "expected env "+expectedName+" not found")
			}
		})
	}
}

// Test_updateCICDScaleSet tests the updateCICDScaleSet function
func Test_updateCICDScaleSet(t *testing.T) {
	// Create a test workload with CICD configuration
	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.Env[common.GithubConfigUrl] = "https://github.com/test/repo"
	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, "test-github-secret")
	v1.SetAnnotation(workload, v1.AdminControlPlaneAnnotation, "10.0.0.1")
	v1.SetAnnotation(workload, v1.MainContainerAnnotation, "runner")
	workload.Spec.Workspace = workspace.Name

	// Create a test resource template
	rt := jobutils.TestCICDScaleSetResourceTemplate.DeepCopy()

	// Create an unstructured object with spec
	obj := &unstructured.Unstructured{}
	obj.Object = map[string]interface{}{
		"apiVersion": "actions.github.com/v1alpha1",
		"kind":       "AutoscalingRunnerSet",
		"metadata": map[string]interface{}{
			"name":      "test-runner",
			"namespace": workspace.Name,
		},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "runner",
							"image": "test-image",
							"env":   []interface{}{},
						},
					},
				},
			},
		},
	}

	// Call updateCICDScaleSet
	err := updateCICDScaleSet(obj, workload, workspace, rt)
	assert.NilError(t, err, "updateCICDScaleSet should succeed")

	// Verify GitHub configuration was updated
	specObject, found, err := jobutils.NestedMap(obj.Object, []string{"spec"})
	assert.NilError(t, err)
	assert.Equal(t, found, true, "spec should exist")

	githubSecret, found := specObject["githubConfigSecret"]
	assert.Equal(t, found, true, "githubConfigSecret should be set")
	assert.Equal(t, githubSecret.(string), "test-github-secret")

	githubUrl, found := specObject["githubConfigUrl"]
	assert.Equal(t, found, true, "githubConfigUrl should be set")
	assert.Equal(t, githubUrl.(string), "https://github.com/test/repo")

	// Verify container env was updated
	containers, found, err := jobutils.NestedSlice(obj.Object, []string{"spec", "template", "spec", "containers"})
	assert.NilError(t, err)
	assert.Equal(t, found, true, "containers should exist")
	assert.Assert(t, len(containers) > 0, "should have at least one container")
}

func TestGithubRunnerSecretRotationUpdatesPodSpec(t *testing.T) {
	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.Kind = common.CICDGithubRunnerKind
	workload.Spec.Workspace = workspace.Name
	workload.Spec.Secrets = []v1.SecretEntity{
		{Id: "new-secret", Type: v1.SecretGeneral},
		{Id: "image-secret", Type: v1.SecretImage},
	}
	workload.Spec.Env[common.GithubConfigUrl] = "https://github.com/test/repo"
	workload.Spec.Env[common.ProxyUrl] = "http://wstunnel-client.github-proxy.svc.cluster.local:3128"
	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, "new-secret")
	v1.SetAnnotation(workload, v1.UseWorkspaceStorageAnnotation, v1.TrueStr)
	v1.SetAnnotation(workload, v1.MainContainerAnnotation, "runner")

	rt := jobutils.TestStatefulSetResourceTemplate.DeepCopy()
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "StatefulSet",
		"metadata": map[string]interface{}{
			"name":        workload.Name,
			"namespace":   workspace.Name,
			"annotations": map[string]interface{}{v1.MainContainerAnnotation: "runner"},
		},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name": "runner",
							"env": []interface{}{map[string]interface{}{
								"name": jobutils.GithubSecretEnv, "value": "old-secret",
							}},
							"volumeMounts": []interface{}{
								map[string]interface{}{
									"name": "old-secret", "mountPath": common.SecretPath + "/old-secret",
								},
								map[string]interface{}{
									"name": "image-secret", "mountPath": common.SecretPath + "/image-secret",
								},
								map[string]interface{}{
									"name": "template-secret", "mountPath": "/runner-creds",
								},
							},
						},
						map[string]interface{}{
							"name": "sidecar",
							"lifecycle": map[string]interface{}{
								"postStart": map[string]interface{}{"exec": map[string]interface{}{
									"command": []interface{}{"true"},
								}},
							},
						},
					},
					"volumes": []interface{}{
						buildSecretVolume("old-secret"),
						buildSecretVolume("image-secret"),
						buildSecretVolume("template-secret"),
					},
				},
			},
		},
	}}

	assert.Assert(t, isGithubSecretChanged(workload, obj, rt))
	assert.NilError(t, updateGithubRunner(obj, workload, workspace, rt))

	envs, err := jobutils.GetEnv(obj, rt, 1)
	assert.NilError(t, err)
	assert.Equal(t, convertEnvsToStringMap(envs)[jobutils.GithubSecretEnv], "new-secret")

	containers, found, err := jobutils.NestedSlice(obj.Object, []string{"spec", "template", "spec", "containers"})
	assert.NilError(t, err)
	assert.Assert(t, found)
	mounts := containers[0].(map[string]interface{})["volumeMounts"].([]interface{})
	mountNames := map[string]bool{}
	for _, mount := range mounts {
		mountNames[mount.(map[string]interface{})["name"].(string)] = true
	}
	assert.Equal(t, mountNames["new-secret"], true)
	assert.Equal(t, mountNames["image-secret"], true)
	assert.Equal(t, mountNames["old-secret"], false)
	assert.Equal(t, mountNames["template-secret"], true)

	volumes, found, err := jobutils.NestedSlice(obj.Object, []string{"spec", "template", "spec", "volumes"})
	assert.NilError(t, err)
	assert.Assert(t, found)
	volumeNames := map[string]bool{}
	for _, volume := range volumes {
		volumeNames[volume.(map[string]interface{})["name"].(string)] = true
	}
	assert.Equal(t, volumeNames["new-secret"], true)
	assert.Equal(t, volumeNames["image-secret"], true)
	assert.Equal(t, volumeNames["old-secret"], false)
	assert.Equal(t, volumeNames["template-secret"], true)

	envsMap := convertEnvsToStringMap(envs)
	assert.Equal(t, envsMap[common.GithubRunnerStateRoot],
		"/ceph/github-runners/"+workload.Name)
	assert.Equal(t, envsMap[common.ProxyUrl],
		"http://wstunnel-client.github-proxy.svc.cluster.local:3128")
	sa, found, err := unstructured.NestedString(obj.Object, "spec", "template", "spec", "serviceAccountName")
	assert.NilError(t, err)
	assert.Assert(t, found)
	assert.Equal(t, sa, common.GithubRunnerServiceAccount)
	lifecycle := containers[0].(map[string]interface{})["lifecycle"].(map[string]interface{})
	preStop := lifecycle["preStop"].(map[string]interface{})
	execHook := preStop["exec"].(map[string]interface{})
	cmd := execHook["command"].([]interface{})
	assert.Equal(t, cmd[2], commonworkload.GithubRunnerStopScript())
	sidecarLifecycle := containers[1].(map[string]interface{})["lifecycle"].(map[string]interface{})
	_, hasPreStop := sidecarLifecycle["preStop"]
	assert.Equal(t, hasPreStop, false)
	_, hasPostStart := sidecarLifecycle["postStart"]
	assert.Equal(t, hasPostStart, true)
}

func TestGithubRunnerInjectedEnvSurvivesRemoval(t *testing.T) {
	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.Kind = common.CICDGithubRunnerKind
	workload.Spec.Workspace = workspace.Name
	workload.Spec.Secrets = []v1.SecretEntity{{Id: "runner-secret", Type: v1.SecretGeneral}}
	workload.Spec.Env[common.GithubConfigUrl] = "https://github.com/test/repo"
	workload.Spec.Env[common.ProxyUrl] = "http://github-proxy:3128"
	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, "runner-secret")
	v1.SetAnnotation(workload, v1.UseWorkspaceStorageAnnotation, v1.TrueStr)
	v1.SetAnnotation(workload, v1.MainContainerAnnotation, "runner")
	v1.SetAnnotation(workload, v1.EnvToBeRemovedAnnotation, string(jsonutils.MarshalSilently([]string{
		common.ProxyUrl, common.GithubRunnerStateRoot, jobutils.GithubSecretEnv,
	})))

	rt := jobutils.TestStatefulSetResourceTemplate.DeepCopy()
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "StatefulSet",
		"metadata": map[string]interface{}{
			"name":        workload.Name,
			"namespace":   workspace.Name,
			"annotations": map[string]interface{}{v1.MainContainerAnnotation: "runner"},
		},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{map[string]interface{}{
						"name": "runner",
						"env": []interface{}{
							map[string]interface{}{"name": common.ProxyUrl, "value": "stale"},
							map[string]interface{}{"name": jobutils.GithubSecretEnv, "value": "stale"},
						},
					}},
				},
			},
		},
	}}

	r := DispatcherReconciler{}
	assert.NilError(t, r.applyWorkloadSpecToObject(context.Background(), nil, obj, workload, workspace, rt, workload))
	envs, err := jobutils.GetEnv(obj, rt, 1)
	assert.NilError(t, err)
	envsMap := convertEnvsToStringMap(envs)
	assert.Equal(t, envsMap[common.ProxyUrl], "http://github-proxy:3128")
	assert.Equal(t, envsMap[common.GithubRunnerStateRoot], "/ceph/github-runners/"+workload.Name)
	assert.Equal(t, envsMap[jobutils.GithubSecretEnv], "runner-secret")
}

func TestGithubRunnerDirectProxyReachesDind(t *testing.T) {
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.Kind = common.CICDGithubRunnerKind
	workload.Spec.Env[common.ProxyUrl] = "http://proxy.example.com:3128"
	workload.Spec.Env[common.NoProxy] = ".svc,.cluster.local"
	v1.SetAnnotation(workload, v1.CICDProxyManagedAnnotation, v1.TrueStr)
	v1.SetAnnotation(workload, v1.MainContainerAnnotation, "runner")
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{
			"containers":     []interface{}{map[string]interface{}{"name": "runner"}},
			"initContainers": []interface{}{map[string]interface{}{"name": cicdProxyDindContainer}},
		}}},
	}}

	spec := jobutils.TestStatefulSetResourceTemplate.Spec.ResourceSpecs[0]
	assert.NilError(t, applyGithubRunnerDirectProxy(obj, workload, spec))
	for _, field := range []string{"containers", "initContainers"} {
		entries, _, err := jobutils.NestedSlice(obj.Object, podSpecPath(workload, &spec, field))
		assert.NilError(t, err)
		env, _, err := unstructured.NestedSlice(entries[0].(map[string]interface{}), "env")
		assert.NilError(t, err)
		values := convertEnvsToStringMap(env)
		assert.Equal(t, values[cicdProxyHTTPEnv], "http://proxy.example.com:3128")
		assert.Equal(t, values[common.NoProxy], ".svc,.cluster.local")
	}
}

func TestGithubRunnerCreateDoesNotDuplicateSecretMounts(t *testing.T) {
	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.Kind = common.CICDGithubRunnerKind
	workload.Spec.GroupVersionKind = v1.GroupVersionKind{Version: "v1", Kind: common.CICDGithubRunnerKind}
	workload.Spec.Workspace = workspace.Name
	workload.Spec.Secrets = []v1.SecretEntity{{Id: "runner-secret", Type: v1.SecretGeneral}}
	workload.Spec.Env[common.GithubConfigUrl] = "https://github.com/test/repo"
	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, "runner-secret")
	v1.SetAnnotation(workload, v1.UseWorkspaceStorageAnnotation, v1.TrueStr)

	configmap, err := parseConfigmap(TestGithubRunnerTemplateConfig)
	assert.NilError(t, err)
	metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(
		configmap, jobutils.TestGithubRunnerResourceTemplate, workspace).WithScheme(scheme).Build()

	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)

	volumes, found, err := jobutils.NestedSlice(obj.Object, []string{"spec", "template", "spec", "volumes"})
	assert.NilError(t, err)
	assert.Assert(t, found)
	volumeNames := map[string]int{}
	for _, volume := range volumes {
		name, _ := volume.(map[string]interface{})["name"].(string)
		volumeNames[name]++
	}
	assert.Equal(t, volumeNames["runner-secret"], 1)

	containers, found, err := jobutils.NestedSlice(obj.Object, []string{"spec", "template", "spec", "containers"})
	assert.NilError(t, err)
	assert.Assert(t, found)
	mounts := containers[0].(map[string]interface{})["volumeMounts"].([]interface{})
	mountPaths := map[string]int{}
	for _, mount := range mounts {
		mountPath, _ := mount.(map[string]interface{})["mountPath"].(string)
		mountPaths[mountPath]++
	}
	assert.Equal(t, mountPaths[common.SecretPath+"/runner-secret"], 1)

	sa, found, err := unstructured.NestedString(obj.Object, "spec", "template", "spec", "serviceAccountName")
	assert.NilError(t, err)
	assert.Assert(t, found)
	assert.Equal(t, sa, common.GithubRunnerServiceAccount)

	envs, err := jobutils.GetEnv(obj, jobutils.TestGithubRunnerResourceTemplate, 1)
	assert.NilError(t, err)
	assert.Equal(t, convertEnvsToStringMap(envs)[common.GithubRunnerStateRoot],
		"/ceph/github-runners/"+workload.Name)
}

func TestCreateRayJob(t *testing.T) {
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")

	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestRayWorkloadData.DeepCopy()
	workload.Spec.Workspace = workspace.Name

	configmap, err := parseConfigmap(TestRayJobTemplateConfig)
	assert.NilError(t, err)
	metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap, jobutils.TestRayJobResourceTemplate, workspace).WithScheme(scheme).Build()

	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)
	checkRaySubmitterPod(t, obj, workload)

	templates := jobutils.TestRayJobResourceTemplate.Spec.ResourceSpecs
	for id := 0; id < 2; id++ {
		checkResources(t, obj, workload, &templates[id], 0, id)
		checkPorts(t, obj, workload, &templates[id], id)
		checkEnvs(t, obj, workload, &templates[id], id)
		checkVolumeMounts(t, obj, workload, &templates[id])
		checkVolumes(t, obj, workload, &templates[id], id)
		checkRequiredNodeSelectorTerms(t, obj, workload, &templates[id])
		checkImage(t, obj, workload, &templates[id], id)
		checkLabels(t, obj, workload, &templates[id], id)
		checkHostNetwork(t, obj, workload, &templates[id], id)
		checkTolerations(t, obj, workload, &templates[id])
		checkPriorityClass(t, obj, workload, &templates[id])
		checkImageSecrets(t, obj, workload, &templates[id])
		_, found, err := jobutils.NestedSlice(obj.Object, templates[id].PrePaths)
		assert.Equal(t, err != nil, true)
		assert.Equal(t, found, false)
	}

	workload.Spec.Resources = append(workload.Spec.Resources, v1.WorkloadResource{
		Replica:          2,
		CPU:              "64",
		GPU:              "8",
		GPUName:          "amd.com/gpu",
		Memory:           "1Ti",
		SharedMemory:     "512Gi",
		EphemeralStorage: "200Gi",
		RdmaResource:     "1k",
	})
	workload.Spec.Images = append(workload.Spec.Images, "test-image2")
	workload.Spec.EntryPoints = append(workload.Spec.EntryPoints, "sh -c test2.sh")
	obj, err = r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)

	id := 2
	checkResources(t, obj, workload, &templates[id], 2, id)
	checkPorts(t, obj, workload, &templates[id], id)
	checkEnvs(t, obj, workload, &templates[id], id)
	checkVolumeMounts(t, obj, workload, &templates[id])
	checkVolumes(t, obj, workload, &templates[id], id)
	checkRequiredNodeSelectorTerms(t, obj, workload, &templates[id])
	checkImage(t, obj, workload, &templates[id], id)
	checkLabels(t, obj, workload, &templates[id], id)
	checkHostNetwork(t, obj, workload, &templates[id], id)
	checkTolerations(t, obj, workload, &templates[id])
	checkPriorityClass(t, obj, workload, &templates[id])
	checkImageSecrets(t, obj, workload, &templates[id])
	// fmt.Println(unstructuredutils.ToString(obj))
}

func TestCreatePytorchJobWithStickyNodes(t *testing.T) {
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")

	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.Workspace = workspace.Name
	workload.Status.Nodes = [][]string{{"node1", "node2"}}
	workload.Spec.CustomerLabels[common.SpecifiedNodes] = "node1 node2"
	v1.SetLabel(workload, v1.WorkloadDispatchCntLabel, "1")
	v1.SetAnnotation(workload, v1.NodesAffinityAnnotation, common.NodesAffinityPreferred)

	configmap, err := parseConfigmap(TestPytorchJobTemplateConfig)
	assert.NilError(t, err)
	metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap, jobutils.TestPytorchResourceTemplate, workspace).WithScheme(scheme).Build()

	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)
	// fmt.Println(unstructuredutils.ToString(obj))

	templates := jobutils.TestPytorchResourceTemplate.Spec.ResourceSpecs
	checkRequiredNodeSelectorTerms(t, obj, workload, &templates[0])
	checkPreferredNodeSelectorTerms(t, obj, workload, &templates[0])
	checkTolerations(t, obj, workload, &templates[0])
}

func TestCreateMonarchClient(t *testing.T) {
	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.Workspace = workspace.Name
	workload.Spec.GroupVersionKind = v1.GroupVersionKind{
		Version: common.DefaultVersion,
		Kind:    common.MonarchClient,
	}
	workload.Spec.Resources[0].GPU = ""
	workload.Spec.Resources[0].RdmaResource = ""
	workload.Spec.JobPort = 0
	workload.Spec.Env[common.MonarchPort] = strconv.Itoa(common.MonarchMeshPortNum)
	v1.SetAnnotation(workload, v1.ForceHostNetworkAnnotation, v1.TrueStr)

	configmap, err := parseConfigmap(TestMonarchClientTemplateConfig)
	assert.NilError(t, err)
	metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap, jobutils.TestMonarchClientResourceTemplate, workspace).WithScheme(scheme).Build()

	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)
	templates := jobutils.TestMonarchClientResourceTemplate.Spec.ResourceSpecs

	checkResources(t, obj, workload, &templates[0], 1, 0)
	checkPorts(t, obj, workload, &templates[0], 0)
	checkEnvs(t, obj, workload, &templates[0], 0)
	checkVolumeMounts(t, obj, workload, &templates[0])
	checkVolumes(t, obj, workload, &templates[0], 0)
	checkRequiredNodeSelectorTerms(t, obj, workload, &templates[0])
	checkImage(t, obj, workload, &templates[0], 0)
	checkLabels(t, obj, workload, &templates[0], 0)
	checkHostNetwork(t, obj, workload, &templates[0], 0)

	// fmt.Println(unstructuredutils.ToString(obj))
}

func TestCreateMonarchMesh(t *testing.T) {
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")
	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.Workspace = workspace.Name
	workload.Spec.GroupVersionKind = v1.GroupVersionKind{
		Version: common.DefaultVersion,
		Kind:    common.MonarchMesh,
	}
	workload.Spec.Resources[0].Replica = 2
	workload.Spec.JobPort = 0
	workload.Spec.Env[common.MonarchPort] = strconv.Itoa(common.MonarchMeshPortNum)
	v1.SetLabel(workload, v1.GroupIdLabel, "0")
	v1.SetAnnotation(workload, v1.ResourceIdAnnotation, "1")

	configmap, err := parseConfigmap(TestMonarchMeshTemplateConfig)
	assert.NilError(t, err)
	metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap, jobutils.TestMonarchMeshResourceTemplate, workspace).WithScheme(scheme).Build()

	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)

	templates := jobutils.TestMonarchMeshResourceTemplate.Spec.ResourceSpecs
	checkResources(t, obj, workload, &templates[0], 2, 0)
	checkPorts(t, obj, workload, &templates[0], 0)
	checkEnvs(t, obj, workload, &templates[0], 0)
	checkVolumeMounts(t, obj, workload, &templates[0])
	checkVolumes(t, obj, workload, &templates[0], 0)
	checkRequiredNodeSelectorTerms(t, obj, workload, &templates[0])
	checkImage(t, obj, workload, &templates[0], 0)
	checkHostNetwork(t, obj, workload, &templates[0], 0)

	// fmt.Println(unstructuredutils.ToString(obj))
}

func TestCreateSandboxWithResources(t *testing.T) {
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")
	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workspace.Spec.IdleTime = map[v1.WorkspaceScope]string{
		common.SandboxKind: "12h0m0s",
	}
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.Workspace = workspace.Name
	workload.Spec.GroupVersionKind = v1.GroupVersionKind{
		Version: common.DefaultVersion,
		Kind:    common.SandboxKind,
	}
	workload.Spec.JobPort = 0
	workload.Spec.EntryPoints = nil
	workload.Spec.Resources[0].RdmaResource = ""
	workload.Spec.Env[sandboxAuthPublicKeyEnvName] = "test-public-key"

	configmap, err := parseConfigmap(TestSandboxConfig)
	assert.NilError(t, err)
	v1.SetAnnotation(workload, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))

	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap,
		jobutils.TestSandboxResourceTemplate, workspace).WithScheme(scheme).Build()

	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)

	templates := jobutils.TestSandboxResourceTemplate.Spec.ResourceSpecs
	checkResources(t, obj, workload, &templates[0], 1, 0)
	checkPorts(t, obj, workload, &templates[0], 0)
	checkEnvs(t, obj, workload, &templates[0], 0)
	checkVolumeMounts(t, obj, workload, &templates[0])
	checkVolumes(t, obj, workload, &templates[0], 0)
	checkRequiredNodeSelectorTerms(t, obj, workload, &templates[0])
	checkImage(t, obj, workload, &templates[0], 0)
	checkHostNetwork(t, obj, workload, &templates[0], 0)
	checkSandboxTemplateCleaned(t, obj, &templates[0])

	// fmt.Println(unstructuredutils.ToString(obj))
}

// --- merged from dispatcher_dispatched_test.go ---

func TestMarkAsDispatchedRootWorkload(t *testing.T) {
	r := &DispatcherReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "child"}}
	v1.SetLabel(w, v1.RootWorkloadIdLabel, "root")
	// Child workloads of a root are skipped.
	err := r.markAsDispatched(context.Background(), w)
	assert.NilError(t, err)
}

func TestMarkAsDispatched(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &DispatcherReconciler{Client: cl}

	err = r.markAsDispatched(context.Background(), w)
	assert.NilError(t, err)
	assert.Equal(t, v1.IsWorkloadDispatched(w), true)
}

func TestIsResourceChangedErrorReturnsFalse(t *testing.T) {
	w := &v1.Workload{}
	w.Spec.Resources = []v1.WorkloadResource{{Replica: 1, CPU: "2"}}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	rt := &v1.ResourceTemplate{}
	// With an empty object/template, GetResources fails and the change check is false.
	assert.Equal(t, isResourceChanged(w, obj, rt), false)
}

// --- merged from dispatcher_generate_test.go ---

func multiResourceWorkload() *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:   "rw",
		Labels: map[string]string{v1.DisplayNameLabel: "disp"},
	}}
	w.Spec.Resources = []v1.WorkloadResource{
		{Replica: 1, CPU: "8", Memory: "16Gi"},
		{Replica: 4, CPU: "8", Memory: "16Gi"},
	}
	w.Spec.EntryPoints = []string{
		stringutil.Base64Encode("entrypoint-0"),
		stringutil.Base64Encode("entrypoint-1"),
	}
	w.Spec.Env = map[string]string{
		common.ReplicaCount:    "2",
		common.MinReplicaCount: "1",
	}
	return w
}

func newGenReconciler(t *testing.T) *DispatcherReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	return &DispatcherReconciler{Client: cl}
}

func TestGenerateLighthouse(t *testing.T) {
	r := newGenReconciler(t)
	w := multiResourceWorkload()
	w.Spec.Service = &v1.Service{Name: "root-service"}
	v1.SetLabel(w, v1.ServiceNameLabel, "root-service")
	lh := r.generateLighthouse(context.Background(), w)
	assert.Assert(t, lh != nil)
	assert.Equal(t, lh.Name, "rw-0")
	assert.Equal(t, string(lh.Spec.Kind), common.DeploymentKind)
	assert.Assert(t, lh.Spec.Service != nil)
	assert.Equal(t, commonworkload.GetK8sServiceName(lh), "rw-0")
}

func TestGenerateTorchFTWorker(t *testing.T) {
	r := newGenReconciler(t)
	w := multiResourceWorkload()
	w.Spec.Service = &v1.Service{Name: "root-service"}
	v1.SetLabel(w, v1.ServiceNameLabel, "root-service")
	worker := r.generateTorchFTWorker(context.Background(), w, 0, 2, "lighthouse:29400")
	assert.Assert(t, worker != nil)
	assert.Equal(t, worker.Name, "rw-1")
	assert.Equal(t, string(worker.Spec.Kind), common.PytorchJobKind)
	assert.Equal(t, worker.Spec.Env[common.TorchFTLightHouse], "lighthouse:29400")
	assert.Equal(t, commonworkload.GetK8sServiceName(worker), "rw-1")
}

func TestGenerateMonarchClient(t *testing.T) {
	r := newGenReconciler(t)
	w := multiResourceWorkload()
	client := r.generateMonarchClient(context.Background(), w, 2)
	assert.Assert(t, client != nil)
	assert.Equal(t, client.Name, "rw")
	assert.Equal(t, string(client.Spec.Kind), common.MonarchClient)
}

func TestGenerateMonarchMesh(t *testing.T) {
	r := newGenReconciler(t)
	w := multiResourceWorkload()
	w.Spec.Service = &v1.Service{Name: "root-service"}
	v1.SetLabel(w, v1.ServiceNameLabel, "root-service")
	mesh := r.generateMonarchMesh(context.Background(), w, 2, 0)
	assert.Assert(t, mesh != nil)
	assert.Equal(t, string(mesh.Spec.Kind), common.MonarchMesh)
	assert.Equal(t, commonworkload.GetK8sServiceName(mesh), mesh.Name)
}

// --- merged from dispatcher_monkey_test.go ---

func monkeyDispatchClientSets() *syncer.ClusterClientSets {
	c := &syncer.ClusterClientSets{}
	c.SetClientFactory(commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c", nil))
	return c
}

// TestDispatch patches generateK8sObject + CreateObject so dispatch runs its full path;
// a workload without a Service short-circuits createService/createIngress.
func TestDispatch(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyPrivateMethod(reflect.TypeOf(&DispatcherReconciler{}), "generateK8sObject",
		func(_ *DispatcherReconciler, _ context.Context, _ *v1.Workload, _ *syncer.ClusterClientSets) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		})
	patches.ApplyFunc(jobutils.CreateObject,
		func(context.Context, *commonclient.ClientFactory, *unstructured.Unstructured) error {
			return nil
		})

	r := &DispatcherReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = "ws"
	// No Service -> createService/createIngress return early.
	_, err := r.dispatch(context.Background(), w, monkeyDispatchClientSets())
	assert.NilError(t, err)
}

// TestProcessWorkloadDispatchPath patches GetClusterClientSets + GetResourceTemplate +
// GetObject(NotFound) + dispatch + markAsDispatched so processWorkload runs the
// "object not yet created" path.
func TestProcessWorkloadDispatchPath(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	cs := monkeyDispatchClientSets()
	patches.ApplyFunc(syncer.GetClusterClientSets,
		func(*commonutils.ObjectManager, string) (*syncer.ClusterClientSets, error) { return cs, nil })
	patches.ApplyFunc(commonworkload.GetResourceTemplate,
		func(context.Context, ctrlclient.Client, *v1.Workload) (*v1.ResourceTemplate, error) {
			return &v1.ResourceTemplate{}, nil
		})
	patches.ApplyFunc(jobutils.GetObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) (*unstructured.Unstructured, error) {
			return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "jobs"}, "w")
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&DispatcherReconciler{}), "dispatch",
		func(_ *DispatcherReconciler, _ context.Context, _ *v1.Workload, _ *syncer.ClusterClientSets) (ctrlruntime.Result, error) {
			return ctrlruntime.Result{}, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&DispatcherReconciler{}), "markAsDispatched",
		func(_ *DispatcherReconciler, _ context.Context, _ *v1.Workload) error { return nil })

	r := &DispatcherReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = "ws"
	_, err := r.processWorkload(context.Background(), w)
	assert.NilError(t, err)
}

// TestDispatcherReconcileToProcess drives Reconcile through generateJobPort into
// processWorkload (patched) for a dispatched, non-TorchFT workload.
func TestDispatcherReconcileToProcess(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyPrivateMethod(reflect.TypeOf(&DispatcherReconciler{}), "processWorkload",
		func(_ *DispatcherReconciler, _ context.Context, _ *v1.Workload) (ctrlruntime.Result, error) {
			return ctrlruntime.Result{}, nil
		})

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        "w",
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true"},
	}}
	w.Spec.Workspace = "ws"
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(w).Build()
	r := &DispatcherReconciler{Client: cl}

	_, rerr := r.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: ctrlclient.ObjectKey{Name: "w"},
	})
	assert.NilError(t, rerr)
}

// --- merged from dispatcher_pure_test.go ---

func TestGenerateRandomPort(t *testing.T) {
	ports := map[int]struct{}{}
	p := generateRandomPort(ports)
	assert.Assert(t, p >= 20000 && p < 30000)
	// The chosen port is recorded to avoid reuse.
	_, ok := ports[p]
	assert.Equal(t, ok, true)

	// A second call yields a port also recorded in the set.
	p2 := generateRandomPort(ports)
	assert.Assert(t, p2 >= 20000 && p2 < 30000)
}

func TestGenerateMeshNamePrefix(t *testing.T) {
	assert.Equal(t, generateMeshNamePrefix("my-job_name"), "myjobnamemesh")
	assert.Equal(t, generateMeshNamePrefix("abc"), "abcmesh")
}

func TestGenerateServicePorts(t *testing.T) {
	svc := &v1.Service{Protocol: corev1.ProtocolTCP, Port: 8080, TargetPort: 9090}
	ports := generateServicePorts(svc)
	assert.Equal(t, len(ports), 1)
	assert.Equal(t, ports[0].Port, int32(8080))
	assert.Equal(t, ports[0].TargetPort.IntVal, int32(9090))
	assert.Equal(t, string(ports[0].Protocol), string(corev1.ProtocolTCP))
}

func TestShouldDispatch(t *testing.T) {
	// Scheduled but not dispatched -> true.
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Annotations: map[string]string{v1.WorkloadScheduledAnnotation: "true"},
	}}
	assert.Equal(t, shouldDispatch(w), true)

	// Already dispatched -> false.
	w.Annotations[v1.WorkloadDispatchedAnnotation] = "true"
	assert.Equal(t, shouldDispatch(w), false)

	// Not scheduled -> false.
	w2 := &v1.Workload{}
	assert.Equal(t, shouldDispatch(w2), false)
}

func TestBuildServiceSelectorDefault(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl-1"}}
	svc := &v1.Service{ExtraSelectors: map[string]string{
		"role":              "head",
		v1.K8sObjectIdLabel: "should-be-overridden",
	}}
	sel := buildServiceSelector(w, svc)
	// SaFE-managed key wins and equals the workload name.
	assert.Equal(t, sel[v1.K8sObjectIdLabel], "wl-1")
	// User-supplied non-colliding key is preserved.
	assert.Equal(t, sel["role"], "head")
}

// --- merged from dispatcher_reconcile_test.go ---

func TestDispatcherReconcileNotFound(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &DispatcherReconciler{Client: cl}
	_, rerr := r.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: ctrlclient.ObjectKey{Name: "missing"},
	})
	assert.NilError(t, rerr)
}

func TestDispatcherRelevantChangePredicateCreate(t *testing.T) {
	p := relevantChangePredicate{}

	// Non-workload object -> false.
	assert.Equal(t, p.Create(event.CreateEvent{Object: &corev1.Pod{}}), false)

	// Scheduled but not dispatched -> dispatchable -> true.
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Annotations: map[string]string{v1.WorkloadScheduledAnnotation: "true"},
	}}
	assert.Equal(t, p.Create(event.CreateEvent{Object: w}), true)

	// Neither scheduled nor dispatched -> false.
	assert.Equal(t, p.Create(event.CreateEvent{Object: &v1.Workload{}}), false)
}

func TestDispatcherRelevantChangePredicateUpdate(t *testing.T) {
	p := relevantChangePredicate{}

	// Wrong types -> false.
	assert.Equal(t, p.Update(event.UpdateEvent{ObjectOld: &corev1.Pod{}, ObjectNew: &corev1.Pod{}}), false)

	// Transition into dispatchable -> true.
	oldW := &v1.Workload{}
	newW := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Annotations: map[string]string{v1.WorkloadScheduledAnnotation: "true"},
	}}
	assert.Equal(t, p.Update(event.UpdateEvent{ObjectOld: oldW, ObjectNew: newW}), true)
}

func TestProcessTorchFTWorkloadNoLighthouse(t *testing.T) {
	// With no TorchFT lighthouse configured, processing fails fast.
	r := &DispatcherReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	_, err := r.processTorchFTWorkload(context.Background(), w)
	assert.Assert(t, err != nil)
}

func TestProcessWorkloadNoClusterClientSets(t *testing.T) {
	r := &DispatcherReconciler{clusterClientSets: commonutils.NewObjectManager()}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	res, err := r.processWorkload(context.Background(), w)
	assert.NilError(t, err)
	// No cluster client sets -> requeue.
	assert.Assert(t, res.RequeueAfter > 0)
}

func TestGenerateJobPortAlreadyDispatched(t *testing.T) {
	r := &DispatcherReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true"},
	}}
	// Already dispatched -> no-op, returns nil.
	assert.NilError(t, r.generateJobPort(context.Background(), w))
}

// --- merged from dispatcher_service_test.go ---

func TestCreateServiceNoService(t *testing.T) {
	r := &DispatcherReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	// No Service spec -> early return.
	res, err := r.createService(context.Background(), w, nil, nil)
	assert.NilError(t, err)
	assert.Equal(t, res.RequeueAfter.Nanoseconds(), int64(0))
}

func TestCreateService(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &DispatcherReconciler{Client: cl}

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = "ns"
	w.Spec.Service = &v1.Service{
		Protocol:    corev1.ProtocolTCP,
		Port:        80,
		TargetPort:  8080,
		ServiceType: corev1.ServiceTypeClusterIP,
	}

	clientset := k8sfake.NewSimpleClientset()
	cs := &syncer.ClusterClientSets{}
	cs.SetClientFactory(commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c", clientset))

	// Give the owner object a UID + GVK so SetControllerReference works and the
	// dynamic GetObject fallback is skipped.
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	obj.SetName("w")
	obj.SetNamespace("ns")
	obj.SetUID("owner-uid")
	obj.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Pod"))

	res, err := r.createService(context.Background(), w, cs, obj)
	assert.NilError(t, err)
	assert.Equal(t, res.RequeueAfter.Nanoseconds(), int64(0))
	service, err := clientset.CoreV1().Services("ns").Get(context.Background(), "w", metav1.GetOptions{})
	assert.NilError(t, err)
	assert.Equal(t, service.Labels[v1.WorkloadIdLabel], "w")
	assert.Equal(t, service.Labels[v1.ServiceNameLabel], "w")
}

func TestCreateServiceDuplicateNameOwnedByOther(t *testing.T) {
	r := &DispatcherReconciler{}

	existing := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shared-svc",
			Namespace: "ns",
			Labels:    map[string]string{v1.WorkloadIdLabel: "wl-a"},
		},
	}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl-b"}}
	w.Spec.Workspace = "ns"
	w.Spec.Service = &v1.Service{
		Name:        "shared-svc",
		Protocol:    corev1.ProtocolTCP,
		Port:        80,
		TargetPort:  8080,
		ServiceType: corev1.ServiceTypeClusterIP,
	}

	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	obj.SetName("wl-b")
	obj.SetNamespace("ns")
	obj.SetUID("owner-uid")
	obj.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Pod"))

	res, err := r.createService(context.Background(), w, serviceClientSets(existing), obj)
	assert.Assert(t, err != nil)
	assert.ErrorContains(t, err, "already used by workload wl-a")
	assert.Assert(t, !commonerrors.IsBadRequest(err))
	assert.Equal(t, res.RequeueAfter.Nanoseconds(), int64(0))
}

func TestCreateIngressNoService(t *testing.T) {
	r := &DispatcherReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	// No Service spec -> ingress creation is skipped.
	res, err := r.createIngress(context.Background(), w, nil, nil)
	assert.NilError(t, err)
	assert.Equal(t, res.RequeueAfter.Nanoseconds(), int64(0))
}

func TestCreateIngressUsesCustomServiceName(t *testing.T) {
	workload := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:   "workload",
		Labels: map[string]string{v1.ServiceNameLabel: "custom-service"},
	}}
	workload.Spec.Workspace = "ns"
	workload.Spec.Service = &v1.Service{Port: 80}

	ingress := buildIngress(workload)

	assert.Equal(t, ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name, "custom-service")
}

func serviceClientSets(objs ...runtime.Object) *syncer.ClusterClientSets {
	cs := &syncer.ClusterClientSets{}
	cs.SetClientFactory(commonclient.NewClientFactoryWithOnlyClient(
		context.Background(), "c", k8sfake.NewSimpleClientset(objs...)))
	return cs
}

func TestUpdateServiceDeleteWhenNoSpec(t *testing.T) {
	r := &DispatcherReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = "ns"
	// No Service spec -> delete any existing service (absent -> IgnoreNotFound).
	res, err := r.updateService(context.Background(), w, serviceClientSets(), nil)
	assert.NilError(t, err)
	assert.Equal(t, res.RequeueAfter.Nanoseconds(), int64(0))
}

func TestUpdateServiceDoesNotDeleteServiceOwnedByAnotherWorkload(t *testing.T) {
	existing := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      "wl-a",
		Namespace: "ns",
		Labels:    map[string]string{v1.WorkloadIdLabel: "wl-b"},
	}}
	clientset := k8sfake.NewSimpleClientset(existing)
	cs := &syncer.ClusterClientSets{}
	cs.SetClientFactory(commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c", clientset))
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl-a"}}
	w.Spec.Workspace = "ns"

	_, err := (&DispatcherReconciler{}).updateService(context.Background(), w, cs, nil)
	assert.Assert(t, err != nil)
	_, getErr := clientset.CoreV1().Services("ns").Get(context.Background(), "wl-a", metav1.GetOptions{})
	assert.NilError(t, getErr)
}

func TestUpdateServiceChecksOwnerBeforeDeletingLegacyFallback(t *testing.T) {
	custom := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      "custom",
		Namespace: "ns",
		Labels:    map[string]string{v1.WorkloadIdLabel: "wl-a"},
	}}
	legacy := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      "wl-a",
		Namespace: "ns",
		Labels:    map[string]string{v1.WorkloadIdLabel: "wl-b"},
	}}
	clientset := k8sfake.NewSimpleClientset(custom, legacy)
	cs := &syncer.ClusterClientSets{}
	cs.SetClientFactory(commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c", clientset))
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:   "wl-a",
		Labels: map[string]string{v1.ServiceNameLabel: "custom"},
	}}
	w.Spec.Workspace = "ns"

	_, err := (&DispatcherReconciler{}).updateService(context.Background(), w, cs, nil)
	assert.Assert(t, err != nil)
	_, customErr := clientset.CoreV1().Services("ns").Get(context.Background(), "custom", metav1.GetOptions{})
	assert.Assert(t, apierrors.IsNotFound(customErr))
	_, legacyErr := clientset.CoreV1().Services("ns").Get(context.Background(), "wl-a", metav1.GetOptions{})
	assert.NilError(t, legacyErr)
}

func TestUpdateServiceClearsServiceNameLabelAfterDelete(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	workload := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:   "wl-a",
		Labels: map[string]string{v1.ServiceNameLabel: "custom"},
	}}
	workload.Spec.Workspace = "ns"
	adminClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(workload).Build()
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      "custom",
		Namespace: "ns",
		Labels:    map[string]string{v1.WorkloadIdLabel: "wl-a"},
	}}

	_, err = (&DispatcherReconciler{Client: adminClient}).updateService(
		context.Background(), workload, serviceClientSets(service), nil)
	assert.NilError(t, err)
	stored := &v1.Workload{}
	assert.NilError(t, adminClient.Get(context.Background(), ctrlclient.ObjectKey{Name: "wl-a"}, stored))
	assert.Equal(t, v1.GetLabel(stored, v1.ServiceNameLabel), "")
}

func TestUpdateServiceUpdatesExisting(t *testing.T) {
	existing := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "w", Namespace: "ns"},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"old": "sel"},
			Ports:    []corev1.ServicePort{{Port: 1, NodePort: 5}},
		},
	}
	r := &DispatcherReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = "ns"
	w.Spec.Service = &v1.Service{
		Protocol:    corev1.ProtocolTCP,
		Port:        80,
		TargetPort:  8080,
		ServiceType: corev1.ServiceTypeClusterIP,
	}
	// Existing service differs -> update path.
	res, err := r.updateService(context.Background(), w, serviceClientSets(existing), nil)
	assert.NilError(t, err)
	assert.Equal(t, res.RequeueAfter.Nanoseconds(), int64(0))
}

func proxyDispatcherFixture(t *testing.T, kind string, unified bool) (*DispatcherReconciler, *v1.Workload, *v1.Workload, *v1.ResourceTemplate) {
	t.Helper()
	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workspace.Name = "test-workspace"
	parent := jobutils.TestWorkloadData.DeepCopy()
	parent.Name, parent.UID, parent.Spec.Workspace = "proxy-set", "proxy-set-uid", workspace.Name
	parent.Spec.GroupVersionKind = v1.GroupVersionKind{Kind: common.CICDScaleRunnerSetKind, Version: "v1"}
	parent.Spec.Env = map[string]string{common.GithubConfigUrl: "https://github.com/example", common.ProxyUrl: "http://proxy.example.com:8080",
		common.ProxyCredentialSecret: "proxy-auth", common.NoProxy: " localhost, .example.com, "}
	parent.Spec.Images = []string{"example/runner:latest"}
	parent.Spec.Resources = parent.Spec.Resources[:1]
	parent.Spec.EntryPoints = []string{stringutil.Base64Encode("sleep 1")}
	v1.SetAnnotation(parent, v1.MainContainerAnnotation, "runner")
	v1.SetAnnotation(parent, v1.GithubSecretIdAnnotation, "github-auth")
	v1.SetAnnotation(parent, v1.CICDProxyManagedAnnotation, v1.TrueStr)
	if unified {
		parent.Spec.Env[common.UnifiedJobEnable] = v1.TrueStr
	}
	configmap, err := parseConfigmap(TestCICDScaleSetTemplateConfig)
	assert.NilError(t, err)
	rt := jobutils.TestCICDScaleSetResourceTemplate.DeepCopy()
	rt.Name = "proxy-resource-template"
	w := parent
	objects := []ctrlclient.Object{workspace}
	if kind == common.CICDEphemeralRunnerKind {
		w = parent.DeepCopy()
		w.Name, w.UID, w.Spec.Kind = "proxy-child", "proxy-child-uid", kind
		w.Spec.Env = map[string]string{common.GithubConfigUrl: "https://github.com/example", common.ScaleRunnerSetID: parent.Name, "CHILD_SETTING": "retained"}
		v1.RemoveAnnotation(w, v1.CICDProxyManagedAnnotation)
		v1.SetAnnotation(w, v1.CICDScaleSetIdAnnotation, "1")
		w.OwnerReferences = []metav1.OwnerReference{{APIVersion: v1.SchemeGroupVersion.String(), Kind: v1.WorkloadKind, Name: parent.Name, UID: parent.UID, Controller: ptr.To(true)}}
		template := &unstructured.Unstructured{}
		assert.NilError(t, yamlutil.NewYAMLOrJSONDecoder(strings.NewReader(configmap.Data["template"]), 4096).Decode(template))
		pod, _, err := unstructured.NestedMap(template.Object, "spec", "template", "spec")
		assert.NilError(t, err)
		template.SetKind(kind)
		template.Object["spec"] = map[string]interface{}{"spec": pod}
		configmap.Data["template"] = string(jsonutils.MarshalSilently(template.Object))
		configmap.Labels[v1.WorkloadKindLabel] = kind
		rt = jobutils.TestCICDRunnerResourceTemplate.DeepCopy()
		rt.Name = "proxy-resource-template"
		objects = append(objects, parent)
	}
	objects = append(objects, rt, configmap)
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	cli := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1.Workload{}).WithIndex(&v1.Workload{}, cicdProxyOwnerIndex, cicdProxyOwnerUID).WithObjects(objects...).Build()
	assert.NilError(t, cli.Create(context.Background(), w))
	return &DispatcherReconciler{Client: cli}, w, parent, rt
}

func TestCreateCICDScaleSet_Proxy(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	previousNoProxy := commonconfig.GetCICDNoProxy()
	commonconfig.SetValue("cicd.no_proxy", "mirror.example.org")
	t.Cleanup(func() { commonconfig.SetValue("cicd.no_proxy", previousNoProxy) })
	for _, unified := range []bool{false, true} {
		r, w, _, rt := proxyDispatcherFixture(t, common.CICDScaleRunnerSetKind, unified)
		obj, err := r.generateK8sObject(context.Background(), w, nil)
		assert.NilError(t, err)
		assert.Equal(t, obj.GetNamespace(), w.Spec.Workspace)
		assert.Equal(t, v1.GetAnnotation(obj, v1.CICDProxyManagedAnnotation), v1.TrueStr)
		for _, protocol := range []string{"http", "https"} {
			url, _, err := unstructured.NestedString(obj.Object, "spec", "proxy", protocol, "url")
			assert.NilError(t, err)
			assert.Equal(t, url, w.Spec.Env[common.ProxyUrl])
			secret, _, err := unstructured.NestedString(obj.Object, "spec", "proxy", protocol, "credentialSecretRef")
			assert.NilError(t, err)
			assert.Equal(t, secret, "proxy-auth")
		}
		list, _, err := unstructured.NestedStringSlice(obj.Object, "spec", "proxy", "noProxy")
		assert.NilError(t, err)
		assert.DeepEqual(t, list, []string{
			"localhost", "127.0.0.1", "::1", ".svc", ".cluster.local", "10.96.0.1", "mirror.example.org", ".example.com"})
		containers, _, err := getContainers(w, obj, rt.Spec.ResourceSpecs[0])
		assert.NilError(t, err)
		if unified {
			assert.Equal(t, len(containers), 2)
		} else {
			assert.Equal(t, len(containers), 1)
		}
		for _, entry := range containers {
			env, _, err := unstructured.NestedSlice(entry.(map[string]interface{}), "env")
			assert.NilError(t, err)
			assert.Assert(t, findEnv(env, common.ProxyCredentialSecret, "proxy-auth"))
			assert.Assert(t, findEnv(env, common.NoProxy, strings.Join(list, ",")))
		}
		assert.Equal(t, w.Spec.Env[common.NoProxy], " localhost, .example.com, ")
		assert.Assert(t, !strings.Contains(string(jsonutils.MarshalSilently(obj)), `"password"`))
	}
}

func TestCreateCICDScaleSet_NoProxy(t *testing.T) {
	for _, unified := range []bool{false, true} {
		r, w, _, _ := proxyDispatcherFixture(t, common.CICDScaleRunnerSetKind, unified)
		for _, key := range commonworkload.CICDProxyEnvKeys() {
			delete(w.Spec.Env, key)
		}
		obj, err := r.generateK8sObject(context.Background(), w, nil)
		assert.NilError(t, err)
		_, exists, err := unstructured.NestedFieldNoCopy(obj.Object, "spec", "proxy")
		assert.NilError(t, err)
		assert.Assert(t, !exists)
		assert.Assert(t, !v1.HasAnnotation(obj, v1.CICDProxyManagedAnnotation))
		assert.Assert(t, commonworkload.IsCICDProxyManaged(w))
	}
}

func TestCICDEphemeralRunnerProxy_InheritsOwner(t *testing.T) {
	r, w, parent, rt := proxyDispatcherFixture(t, common.CICDEphemeralRunnerKind, false)
	w.Spec.Env[common.ProxyUrl] = "http://stale.example.com"
	refs := append([]metav1.OwnerReference{}, w.OwnerReferences...)
	obj, err := r.generateK8sObject(context.Background(), w, nil)
	assert.NilError(t, err)
	endpoint, _, err := unstructured.NestedString(obj.Object, "spec", "proxy", "http", "url")
	assert.NilError(t, err)
	assert.Equal(t, endpoint, parent.Spec.Env[common.ProxyUrl])
	id, _, err := unstructured.NestedInt64(obj.Object, "spec", "runnerScaleSetId")
	assert.NilError(t, err)
	assert.Equal(t, id, int64(1))
	env := getEnvs(t, obj, w, &rt.Spec.ResourceSpecs[0])
	assert.Assert(t, findEnv(env, common.ProxyUrl, parent.Spec.Env[common.ProxyUrl]))
	noProxy, _, err := unstructured.NestedStringSlice(obj.Object, "spec", "proxy", "noProxy")
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(strings.Join(noProxy, ","), ".cluster.local"))
	assert.Assert(t, findEnv(env, common.NoProxy, strings.Join(noProxy, ",")))
	assert.Assert(t, findEnv(env, "CHILD_SETTING", "retained"))
	assert.DeepEqual(t, w.OwnerReferences, refs)
	assert.Equal(t, w.Spec.Env[common.ProxyUrl], "http://stale.example.com")
	for _, change := range []func(*v1.Workload){
		func(c *v1.Workload) { c.OwnerReferences = nil }, func(c *v1.Workload) { c.OwnerReferences[0].UID = "wrong" },
		func(c *v1.Workload) { c.OwnerReferences[0].Kind = "Pod" }, func(c *v1.Workload) { c.Spec.Workspace = "other-workspace" },
		func(c *v1.Workload) { v1.SetLabel(c, v1.ClusterIdLabel, "other-cluster") }, func(c *v1.Workload) { c.Spec.Env[common.ScaleRunnerSetID] = "missing" },
	} {
		child := w.DeepCopy()
		change(child)
		_, err := commonworkload.ResolveCICDProxySource(context.Background(), r.Client, child)
		assert.Assert(t, err != nil)
	}
}

func TestUpdateCICDEphemeralRunnerDoesNotRefetchExistingOwner(t *testing.T) {
	r, workload, source, rt := proxyDispatcherFixture(t, common.CICDEphemeralRunnerKind, false)
	scaleRunnerID := "deleted-scale-runner"
	v1.SetLabel(workload, v1.CICDScaleRunnerIdLabel, scaleRunnerID)
	for _, key := range commonworkload.CICDProxyEnvKeys() {
		delete(source.Spec.Env, key)
	}
	v1.RemoveAnnotation(source, v1.CICDProxyManagedAnnotation)
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)
	obj.SetOwnerReferences([]metav1.OwnerReference{{Name: scaleRunnerID}})

	gets := 0
	patches := gomonkey.ApplyFunc(jobutils.GetObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) (*unstructured.Unstructured, error) {
			gets++
			return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "autoscalingrunnersets"}, scaleRunnerID)
		})
	defer patches.Reset()

	err = updateCICDEphemeralRunner(context.Background(), &syncer.ClusterClientSets{}, obj, workload, source, rt)
	assert.NilError(t, err)
	assert.Equal(t, gets, 0)
}

func TestCreateCICDEphemeralRunnerWithoutCredentialedProxyRemovesRelay(t *testing.T) {
	for name, removeProxy := range map[string]bool{"no proxy": true, "no credential": false} {
		t.Run(name, func(t *testing.T) {
			r, w, parent, _ := proxyRelayDispatcherFixture(t)
			delete(parent.Spec.Env, common.ProxyCredentialSecret)
			if removeProxy {
				delete(parent.Spec.Env, common.ProxyUrl)
			}
			assert.NilError(t, r.Update(context.Background(), parent))

			obj, err := r.generateK8sObject(context.Background(), w, nil)
			assert.NilError(t, err)
			assertCICDProxyRelayRemoved(t, obj)
		})
	}
}

func TestCreateCICDEphemeralRunnerProxyRelayPreservesRunnerResources(t *testing.T) {
	resources := v1.WorkloadResource{
		Replica: 1, CPU: "8", Memory: "16Gi", GPU: "1", GPUName: common.AmdGpu, EphemeralStorage: "40Gi",
	}
	plainReconciler, plainWorkload, _, _ := proxyRunnerDispatcherFixture(t, false)
	plainWorkload.Spec.Resources = []v1.WorkloadResource{resources}
	plain, err := plainReconciler.generateK8sObject(context.Background(), plainWorkload, nil)
	assert.NilError(t, err)

	relayReconciler, relayWorkload, _, _ := proxyRelayDispatcherFixture(t)
	relayWorkload.Spec.Resources = []v1.WorkloadResource{resources}
	withRelay, err := relayReconciler.generateK8sObject(context.Background(), relayWorkload, nil)
	assert.NilError(t, err)

	plainRunner := findCICDContainer(t, plain, "containers", "runner")
	relayRunner := findCICDContainer(t, withRelay, "containers", "runner")
	assert.Check(t, reflect.DeepEqual(relayRunner["resources"], plainRunner["resources"]),
		"relay changed runner resources: %#v", relayRunner["resources"])
	limits := relayRunner["resources"].(map[string]interface{})["limits"].(map[string]interface{})
	assert.Check(t, reflect.DeepEqual(limits, map[string]interface{}{
		"cpu": "8", "memory": "16Gi", common.AmdGpu: "1", "ephemeral-storage": "40Gi",
	}), "runner limits changed: %#v", limits)

	normalRelay := lookupCICDContainer(t, withRelay, "containers", cicdProxyRelayContainer)
	relay := lookupCICDContainer(t, withRelay, "initContainers", cicdProxyRelayContainer)
	assert.Check(t, normalRelay == nil, "proxy relay rendered as a regular container")
	assert.Check(t, relay != nil, "proxy relay init container not found")
	if relay == nil {
		relay = normalRelay
	}
	assert.Assert(t, relay != nil)
	relayResources := relay["resources"].(map[string]interface{})
	assert.Check(t, reflect.DeepEqual(relayResources["limits"], map[string]interface{}{"cpu": "500m", "memory": "256Mi"}),
		"relay limits changed: %#v", relayResources["limits"])
	_, hasRequests := relayResources["requests"]
	assert.Check(t, !hasRequests, "relay received workload resource requests: %#v", relayResources["requests"])
	_, hasGPU := relayResources["limits"].(map[string]interface{})[common.AmdGpu]
	assert.Check(t, !hasGPU, "relay received a GPU limit")
}

func TestSyncCICDEphemeralRunnerWithoutCredentialedProxyRemovesRelay(t *testing.T) {
	for name, removeProxy := range map[string]bool{"no proxy": true, "no credential": false} {
		t.Run(name, func(t *testing.T) {
			r, w, parent, rt := proxyRelayDispatcherFixture(t)
			obj, err := r.generateK8sObject(context.Background(), w, nil)
			assert.NilError(t, err)
			delete(parent.Spec.Env, common.ProxyCredentialSecret)
			if removeProxy {
				delete(parent.Spec.Env, common.ProxyUrl)
			}
			assert.NilError(t, r.Update(context.Background(), parent))
			cs, read := proxyDynamicClient(t, obj)

			current, _ := read()
			assert.NilError(t, syncProxyFixture(context.Background(), r, w, cs, current, rt))
			current, count := read()
			assert.Equal(t, count, 1)
			assertCICDProxyRelayRemoved(t, current)
		})
	}
}

func proxyDynamicClient(t *testing.T, initial *unstructured.Unstructured,
	objects ...*unstructured.Unstructured) (*syncer.ClusterClientSets, func() (*unstructured.Unstructured, int)) {
	t.Helper()
	current := initial.DeepCopy()
	current.SetResourceVersion("1")
	objectsByName := make(map[string]*unstructured.Unstructured, len(objects))
	for _, obj := range objects {
		objectsByName[obj.GetName()] = obj.DeepCopy()
	}
	updates := 0
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			next := &unstructured.Unstructured{}
			if err := json.NewDecoder(request.Body).Decode(next); err != nil {
				http.Error(writer, "invalid object", 400)
				return
			}
			if next.GetResourceVersion() != current.GetResourceVersion() {
				writer.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(writer).Encode(apierrors.NewConflict(schema.GroupResource{Resource: "runners"}, current.GetName(), fmt.Errorf("stale resource version")).ErrStatus)
				return
			}
			version, _ := strconv.Atoi(current.GetResourceVersion())
			next.SetResourceVersion(strconv.Itoa(version + 1))
			current = next
			updates++
		}
		name := request.URL.Path[strings.LastIndex(request.URL.Path, "/")+1:]
		if request.Method == http.MethodGet && name != current.GetName() {
			obj, found := objectsByName[name]
			if !found {
				writer.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(writer).Encode(apierrors.NewNotFound(schema.GroupResource{Resource: "runners"}, name).ErrStatus)
				return
			}
			_ = json.NewEncoder(writer).Encode(obj)
			return
		}
		_ = json.NewEncoder(writer).Encode(current)
	}))
	t.Cleanup(server.Close)
	dynamicClient, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
	assert.NilError(t, err)
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{initial.GroupVersionKind().GroupVersion()})
	mapper.Add(initial.GroupVersionKind(), meta.RESTScopeNamespace)
	factory := commonclient.NewClientFactoryForTest("test-cluster", server.URL)
	patches := gomonkey.NewPatches()
	t.Cleanup(patches.Reset)
	patches.ApplyMethod(reflect.TypeOf(factory), "DynamicClient", func(*commonclient.ClientFactory) *dynamic.DynamicClient { return dynamicClient })
	patches.ApplyMethod(reflect.TypeOf(factory), "Mapper", func(*commonclient.ClientFactory) meta.RESTMapper { return mapper })
	cs := &syncer.ClusterClientSets{}
	cs.SetClientFactory(factory)
	return cs, func() (*unstructured.Unstructured, int) {
		mu.Lock()
		defer mu.Unlock()
		return current.DeepCopy(), updates
	}
}

func syncProxyFixture(ctx context.Context, r *DispatcherReconciler, w *v1.Workload, cs *syncer.ClusterClientSets, obj *unstructured.Unstructured, rt *v1.ResourceTemplate) error {
	if commonworkload.IsCICDEphemeralRunner(w) {
		source, err := commonworkload.ResolveCICDProxySource(ctx, r.Client, w)
		if err != nil {
			return err
		}
		return r.syncCICDEphemeralRunnerProxy(ctx, w, source, cs, obj, rt)
	}
	return r.syncWorkloadToObject(ctx, w, cs, obj)
}

func TestSyncCICDProxy_Drift(t *testing.T) {
	for _, kind := range []string{common.CICDScaleRunnerSetKind, common.CICDEphemeralRunnerKind} {
		t.Run(kind, func(t *testing.T) {
			r, w, parent, rt := proxyDispatcherFixture(t, kind, false)
			obj, err := r.generateK8sObject(context.Background(), w, nil)
			assert.NilError(t, err)
			assert.NilError(t, unstructured.SetNestedField(obj.Object, "http://drift.example.com", "spec", "proxy", "http", "url"))
			cs, read := proxyDynamicClient(t, obj)
			current, _ := read()
			assert.NilError(t, syncProxyFixture(context.Background(), r, w, cs, current, rt))
			current, count := read()
			assert.Equal(t, count, 1)
			endpoint, _, err := unstructured.NestedString(current.Object, "spec", "proxy", "http", "url")
			assert.NilError(t, err)
			assert.Equal(t, endpoint, parent.Spec.Env[common.ProxyUrl])
		})
	}
}

func TestSyncCICDProxy_Idempotent(t *testing.T) {
	for _, kind := range []string{common.CICDScaleRunnerSetKind, common.CICDEphemeralRunnerKind} {
		t.Run(kind, func(t *testing.T) {
			r, w, _, rt := proxyDispatcherFixture(t, kind, false)
			obj, err := r.generateK8sObject(context.Background(), w, nil)
			assert.NilError(t, err)
			cs, read := proxyDynamicClient(t, obj)
			for i := 0; i < 2; i++ {
				current, _ := read()
				assert.NilError(t, syncProxyFixture(context.Background(), r, w, cs, current, rt))
			}
			_, count := read()
			assert.Equal(t, count, 0)
		})
	}
}

func TestSyncCICDProxy_ConflictRestartRemoval(t *testing.T) {
	for _, kind := range []string{common.CICDScaleRunnerSetKind, common.CICDEphemeralRunnerKind} {
		t.Run(kind, func(t *testing.T) {
			r, w, parent, rt := proxyDispatcherFixture(t, kind, false)
			obj, err := r.generateK8sObject(context.Background(), w, nil)
			assert.NilError(t, err)
			cs, read := proxyDynamicClient(t, obj)
			assert.NilError(t, r.Get(context.Background(), ctrlclient.ObjectKeyFromObject(parent), parent))
			for _, key := range commonworkload.CICDProxyEnvKeys() {
				delete(parent.Spec.Env, key)
			}
			assert.NilError(t, r.Update(context.Background(), parent))
			if kind == common.CICDScaleRunnerSetKind {
				w = parent
			}
			stale, _ := read()
			stale.SetResourceVersion("0")
			err = syncProxyFixture(context.Background(), r, w, cs, stale, rt)
			assert.Assert(t, apierrors.IsConflict(err))
			current, count := read()
			assert.Equal(t, count, 0)
			assert.Equal(t, v1.GetAnnotation(current, v1.CICDProxyManagedAnnotation), v1.TrueStr)
			restarted := &DispatcherReconciler{Client: r.Client}
			assert.NilError(t, syncProxyFixture(context.Background(), restarted, w, cs, current, rt))
			current, count = read()
			assert.Equal(t, count, 1)
			_, found, err := unstructured.NestedFieldNoCopy(current.Object, "spec", "proxy")
			assert.NilError(t, err)
			assert.Assert(t, !found)
			assert.Assert(t, !v1.HasAnnotation(current, v1.CICDProxyManagedAnnotation))
			assert.Assert(t, commonworkload.IsCICDProxyManaged(parent))
			env := getEnvs(t, current, w, &rt.Spec.ResourceSpecs[0])
			for _, key := range commonworkload.CICDProxyEnvKeys() {
				for _, item := range env {
					assert.Assert(t, item.(map[string]interface{})["name"] != key)
				}
			}
		})
	}
}

func TestSyncCICDProxy_InvalidStoredConfig(t *testing.T) {
	for _, kind := range []string{common.CICDScaleRunnerSetKind, common.CICDEphemeralRunnerKind} {
		t.Run(kind, func(t *testing.T) {
			r, w, parent, rt := proxyDispatcherFixture(t, kind, false)
			obj, err := r.generateK8sObject(context.Background(), w, nil)
			assert.NilError(t, err)
			cs, read := proxyDynamicClient(t, obj)
			assert.NilError(t, r.Get(context.Background(), ctrlclient.ObjectKeyFromObject(parent), parent))
			parent.Spec.Env[common.ProxyUrl] = "http://sample@example.com"
			assert.NilError(t, r.Update(context.Background(), parent))
			if kind == common.CICDScaleRunnerSetKind {
				w = parent
			}
			current, _ := read()
			before := current.DeepCopy()
			assert.Assert(t, syncProxyFixture(context.Background(), r, w, cs, current, rt) != nil)
			current, count := read()
			assert.Equal(t, count, 0)
			assert.DeepEqual(t, current.Object, before.Object)
		})
	}
}

func TestSyncCICDProxy_LegacyUnmarked(t *testing.T) {
	for _, kind := range []string{common.CICDScaleRunnerSetKind, common.CICDEphemeralRunnerKind} {
		for _, endpoint := range []string{"arbitrary legacy value", "http://proxy.example.com:3128"} {
			t.Run(kind+endpoint, func(t *testing.T) {
				r, w, parent, rt := proxyDispatcherFixture(t, kind, false)
				assert.NilError(t, r.Get(context.Background(), ctrlclient.ObjectKeyFromObject(parent), parent))
				v1.RemoveAnnotation(parent, v1.CICDProxyManagedAnnotation)
				parent.Spec.Env[common.ProxyUrl] = endpoint
				assert.NilError(t, r.Update(context.Background(), parent))
				if kind == common.CICDScaleRunnerSetKind {
					w = parent
				}
				obj, err := r.generateK8sObject(context.Background(), w, nil)
				assert.NilError(t, err)
				custom := map[string]interface{}{"https": map[string]interface{}{"url": "http://custom.example.com"}}
				assert.NilError(t, unstructured.SetNestedMap(obj.Object, custom, "spec", "proxy"))
				cs, read := proxyDynamicClient(t, obj)
				patches := gomonkey.NewPatches()
				defer patches.Reset()
				patches.ApplyFunc(commonworkload.ParseCICDProxy, func(map[string]string) (*commonworkload.CICDProxyConfig, error) {
					t.Fatal("unmarked source was parsed")
					return nil, nil
				})
				for i := 0; i < 2; i++ {
					current, _ := read()
					assert.NilError(t, syncProxyFixture(context.Background(), r, w, cs, current, rt))
				}
				w.Spec.Images[0] = "example/runner:updated"
				current, _ := read()
				assert.NilError(t, syncProxyFixture(context.Background(), r, w, cs, current, rt))
				current, _ = read()
				actual, _, err := unstructured.NestedMap(current.Object, "spec", "proxy")
				assert.NilError(t, err)
				assert.DeepEqual(t, actual, custom)
				assert.Assert(t, !v1.HasAnnotation(current, v1.CICDProxyManagedAnnotation))
			})
		}
	}
}

func TestCICDEphemeralRunnerProxy_ParentChangeRequeues(t *testing.T) {
	r, w, parent, rt := proxyDispatcherFixture(t, common.CICDEphemeralRunnerKind, false)
	obj, err := r.generateK8sObject(context.Background(), w, nil)
	assert.NilError(t, err)
	cs, read := proxyDynamicClient(t, obj)
	r.clusterClientSets = commonutils.NewObjectManager()
	assert.NilError(t, r.clusterClientSets.Add(v1.GetClusterId(w), cs))
	terminal := w.DeepCopy()
	terminal.Name = "ended-child"
	terminal.UID = "ended-child-uid"
	terminal.ResourceVersion = ""
	terminal.Status.Phase = v1.WorkloadSucceeded
	assert.NilError(t, r.Create(context.Background(), terminal))
	for _, endpoint := range []string{"http://changed.example.com:3128", "", "http://enabled.example.com:3128"} {
		assert.NilError(t, r.Get(context.Background(), ctrlclient.ObjectKeyFromObject(parent), parent))
		old := parent.DeepCopy()
		parent.Spec.Env = map[string]string{common.ProxyUrl: endpoint}
		assert.NilError(t, r.Update(context.Background(), parent))
		assert.Assert(t, cicdProxyParentPredicate().Update(event.UpdateEvent{ObjectOld: old, ObjectNew: parent}))
		requests := r.enqueueCICDProxyChildren(context.Background(), parent)
		assert.Equal(t, len(requests), 1)
		assert.Equal(t, requests[0].Name, w.Name)
		assert.NilError(t, r.Get(context.Background(), ctrlclient.ObjectKeyFromObject(w), w))
		result, err := r.processWorkload(context.Background(), w)
		assert.NilError(t, err)
		assert.Equal(t, result.RequeueAfter, 30*time.Second)
		current, _ := read()
		changed, err := isCICDProxyChanged(parent, current)
		assert.NilError(t, err)
		assert.Assert(t, !changed)
		assert.NilError(t, syncProxyFixture(context.Background(), &DispatcherReconciler{Client: r.Client}, w, cs, current, rt))
	}
	_, before := read()
	_, err = r.processWorkload(context.Background(), terminal)
	assert.NilError(t, err)
	_, after := read()
	assert.Equal(t, before, after)
	assert.Assert(t, (relevantChangePredicate{}).Create(event.CreateEvent{Object: w}))
}

func TestSyncCICDProxy_ValidatedOptIn(t *testing.T) {
	r, w, _, rt := proxyDispatcherFixture(t, common.CICDScaleRunnerSetKind, false)
	v1.RemoveAnnotation(w, v1.CICDProxyManagedAnnotation)
	obj, err := r.generateK8sObject(context.Background(), w, nil)
	assert.NilError(t, err)
	obj.SetAnnotations(map[string]string{v1.CICDProxyManagedAnnotation: v1.TrueStr})
	changed, err := isCICDProxyChanged(w, obj)
	assert.NilError(t, err)
	assert.Assert(t, !changed)
	cs, read := proxyDynamicClient(t, obj)
	v1.SetAnnotation(w, v1.CICDProxyManagedAnnotation, v1.TrueStr)
	current, _ := read()
	assert.NilError(t, syncProxyFixture(context.Background(), r, w, cs, current, rt))
	current, count := read()
	assert.Equal(t, count, 1)
	changed, err = isCICDProxyChanged(w, current)
	assert.NilError(t, err)
	assert.Assert(t, !changed)
}

func TestCICDEphemeralRunnerProxy_DriftClearsSecretRef(t *testing.T) {
	r, w, parent, rt := proxyDispatcherFixture(t, common.CICDEphemeralRunnerKind, false)
	owner := proxyRunnerOwner(rt, w.Spec.Workspace, "scale-runner", "runner-proxy")
	v1.SetLabel(w, v1.CICDScaleRunnerIdLabel, owner.GetName())
	obj, err := r.generateK8sObject(context.Background(), w, nil)
	assert.NilError(t, err)
	assert.NilError(t, unstructured.SetNestedField(obj.Object, "runner-proxy", "spec", "proxySecretRef"))
	assert.NilError(t, r.Get(context.Background(), ctrlclient.ObjectKeyFromObject(parent), parent))
	for _, key := range commonworkload.CICDProxyEnvKeys() {
		delete(parent.Spec.Env, key)
	}
	assert.NilError(t, r.Update(context.Background(), parent))
	cs, read := proxyDynamicClient(t, obj, owner)

	current, _ := read()
	assert.NilError(t, syncProxyFixture(context.Background(), r, w, cs, current, rt))
	current, count := read()
	assert.Equal(t, count, 1)
	_, found, err := unstructured.NestedFieldNoCopy(current.Object, "spec", "proxy")
	assert.NilError(t, err)
	assert.Assert(t, !found)
	_, found, err = unstructured.NestedFieldNoCopy(current.Object, "spec", "proxySecretRef")
	assert.NilError(t, err)
	assert.Assert(t, !found)
}

func TestCICDEphemeralRunnerProxy_LaterInheritsSecretRef(t *testing.T) {
	r, w, _, rt := proxyDispatcherFixture(t, common.CICDEphemeralRunnerKind, false)
	owner := proxyRunnerOwner(rt, w.Spec.Workspace, "scale-runner", "runner-proxy")
	v1.SetLabel(w, v1.CICDScaleRunnerIdLabel, owner.GetName())
	obj, err := r.generateK8sObject(context.Background(), w, nil)
	assert.NilError(t, err)
	_, found, err := unstructured.NestedFieldNoCopy(obj.Object, "spec", "proxySecretRef")
	assert.NilError(t, err)
	assert.Assert(t, !found)
	cs, read := proxyDynamicClient(t, obj, owner)

	current, _ := read()
	assert.NilError(t, syncProxyFixture(context.Background(), r, w, cs, current, rt))
	current, count := read()
	assert.Equal(t, count, 1)
	ref, found, err := unstructured.NestedString(current.Object, "spec", "proxySecretRef")
	assert.NilError(t, err)
	assert.Assert(t, found)
	assert.Equal(t, ref, "runner-proxy")
}

func TestSyncCICDEphemeralRunnerProxy_OwnerNotFound(t *testing.T) {
	ctx := context.Background()
	r, workload, _, rt := proxyDispatcherFixture(t, common.CICDEphemeralRunnerKind, false)
	const scaleRunnerID = "deleted-scale-runner"
	v1.SetLabel(workload, v1.CICDScaleRunnerIdLabel, scaleRunnerID)
	obj, err := r.generateK8sObject(ctx, workload, nil)
	assert.NilError(t, err)
	obj.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: rt.ToSchemaGVK().GroupVersion().String(), Kind: common.CICDScaleRunnerSetKind,
		Name: scaleRunnerID, UID: "deleted-scale-runner-uid", Controller: ptr.To(true),
	}})
	assert.NilError(t, unstructured.SetNestedField(obj.Object, "http://drift.example.com", "spec", "proxy", "http", "url"))
	assert.NilError(t, unstructured.SetNestedField(obj.Object, "runner-proxy", "spec", "proxySecretRef"))
	cs, read := proxyDynamicClient(t, obj)
	original, _ := read()

	for i := 0; i < 2; i++ {
		current, _ := read()
		assert.NilError(t, syncProxyFixture(ctx, r, workload, cs, current, rt))
		assert.DeepEqual(t, current.Object, original.Object)
	}
	current, updates := read()
	assert.Equal(t, updates, 0)
	assert.DeepEqual(t, current.Object, original.Object)
}

func TestSyncCICDEphemeralRunnerProxy_OwnerLookupError(t *testing.T) {
	resource := schema.GroupResource{Group: "actions.github.com", Resource: "autoscalingrunnersets"}
	for _, tc := range []struct {
		name      string
		getErr    error
		isSameErr func(error) bool
	}{
		{
			name:      "forbidden",
			getErr:    apierrors.NewForbidden(resource, "scale-runner", fmt.Errorf("access denied")),
			isSameErr: apierrors.IsForbidden,
		},
		{
			name:      "internal server error",
			getErr:    apierrors.NewInternalError(fmt.Errorf("owner lookup unavailable")),
			isSameErr: apierrors.IsInternalError,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r, workload, _, rt := proxyDispatcherFixture(t, common.CICDEphemeralRunnerKind, false)
			v1.SetLabel(workload, v1.CICDScaleRunnerIdLabel, "scale-runner")
			obj, err := r.generateK8sObject(ctx, workload, nil)
			assert.NilError(t, err)
			cs, read := proxyDynamicClient(t, obj)
			patches := gomonkey.NewPatches()
			t.Cleanup(patches.Reset)
			patches.ApplyFuncReturn(jobutils.GetObject, nil, tc.getErr)

			current, _ := read()
			err = syncProxyFixture(ctx, r, workload, cs, current, rt)
			assert.ErrorContains(t, err, "failed to get owner scale runner")
			assert.Assert(t, tc.isSameErr(err))
			_, updates := read()
			assert.Equal(t, updates, 0)
		})
	}
}

func proxyRunnerOwner(rt *v1.ResourceTemplate, namespace, name, proxySecretRef string) *unstructured.Unstructured {
	owner := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{"proxySecretRef": proxySecretRef},
	}}
	owner.SetGroupVersionKind(rt.ToSchemaGVK())
	owner.SetNamespace(namespace)
	owner.SetName(name)
	return owner
}

func TestInheritCICDProxySecretRef(t *testing.T) {
	owner := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{"proxySecretRef": "scaleset-runner-proxy"}}}
	proxied := map[string]interface{}{
		"spec": map[string]interface{}{"proxy": map[string]interface{}{
			"http": map[string]interface{}{"url": "http://proxy:3128"}}}}

	obj := &unstructured.Unstructured{Object: proxied}
	assert.NilError(t, inheritCICDProxySecretRef(obj, owner))
	ref, found, err := unstructured.NestedString(obj.Object, "spec", "proxySecretRef")
	assert.NilError(t, err)
	assert.Assert(t, found)
	assert.Equal(t, ref, "scaleset-runner-proxy")

	// A ref without spec.proxy is dereferenced unguarded by the controller, so it must never be set.
	bare := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{"proxySecretRef": "stale"}}}
	assert.NilError(t, inheritCICDProxySecretRef(bare, owner))
	_, found, err = unstructured.NestedString(bare.Object, "spec", "proxySecretRef")
	assert.NilError(t, err)
	assert.Assert(t, !found)

	// Owner without a ref of its own leaves the runner unproxied rather than dangling.
	obj = &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"proxy":          map[string]interface{}{"http": map[string]interface{}{"url": "http://proxy:3128"}},
			"proxySecretRef": "stale",
		}}}
	assert.NilError(t, inheritCICDProxySecretRef(obj, &unstructured.Unstructured{Object: map[string]interface{}{}}))
	_, found, err = unstructured.NestedString(obj.Object, "spec", "proxySecretRef")
	assert.NilError(t, err)
	assert.Assert(t, !found)
}

func ephemeralRunnerSpec() v1.ResourceSpec {
	return v1.ResourceSpec{PodSpecPaths: []string{"spec"}, PrePaths: []string{"spec"}}
}

func proxyRelayDispatcherFixture(t *testing.T) (*DispatcherReconciler, *v1.Workload, *v1.Workload, *v1.ResourceTemplate) {
	t.Helper()
	return proxyRunnerDispatcherFixture(t, true)
}

func proxyRunnerDispatcherFixture(t *testing.T, relay bool) (*DispatcherReconciler, *v1.Workload, *v1.Workload, *v1.ResourceTemplate) {
	t.Helper()
	r, w, parent, rt := proxyDispatcherFixture(t, common.CICDEphemeralRunnerKind, false)
	configMap := &corev1.ConfigMap{}
	key := ctrlclient.ObjectKey{Namespace: "primus-safe", Name: "github-scale-set-template"}
	assert.NilError(t, r.Get(context.Background(), key, configMap))
	template := &unstructured.Unstructured{}
	assert.NilError(t, yamlutil.NewYAMLOrJSONDecoder(strings.NewReader(configMap.Data["template"]), 4096).Decode(template))
	containers, _, err := unstructured.NestedSlice(template.Object, "spec", "spec", "containers")
	assert.NilError(t, err)
	runnerFound := false
	for _, entry := range containers {
		if entry.(map[string]interface{})["name"] == "runner" {
			assert.NilError(t, unstructured.SetNestedSlice(template.Object, []interface{}{entry}, "spec", "spec", "containers"))
			runnerFound = true
			break
		}
	}
	assert.Assert(t, runnerFound)
	if !relay {
		configMap.Data["template"] = string(jsonutils.MarshalSilently(template.Object))
		assert.NilError(t, r.Update(context.Background(), configMap))
		return r, w, parent, rt
	}
	path := []string{"spec", "spec", "initContainers"}
	entries, _, err := unstructured.NestedSlice(template.Object, path...)
	assert.NilError(t, err)
	entries = append([]interface{}{proxyRelayContainer()}, entries...)
	assert.NilError(t, unstructured.SetNestedSlice(template.Object, entries, path...))
	volumes, _, err := unstructured.NestedSlice(template.Object, "spec", "spec", "volumes")
	assert.NilError(t, err)
	volumes = append(volumes, proxyCredentialVolume())
	assert.NilError(t, unstructured.SetNestedSlice(template.Object, volumes, "spec", "spec", "volumes"))
	configMap.Data["template"] = string(jsonutils.MarshalSilently(template.Object))
	assert.NilError(t, r.Update(context.Background(), configMap))
	return r, w, parent, rt
}

func proxyRelayContainer() map[string]interface{} {
	return map[string]interface{}{
		"name":          cicdProxyRelayContainer,
		"restartPolicy": "Always",
		"resources": map[string]interface{}{"limits": map[string]interface{}{
			"cpu": "500m", "memory": "256Mi",
		}},
	}
}

func findCICDContainer(t *testing.T, obj *unstructured.Unstructured, field, name string) map[string]interface{} {
	t.Helper()
	container := lookupCICDContainer(t, obj, field, name)
	if container != nil {
		return container
	}
	t.Fatalf("%s %q not found", field, name)
	return nil
}

func lookupCICDContainer(t *testing.T, obj *unstructured.Unstructured, field, name string) map[string]interface{} {
	t.Helper()
	containers, _, err := unstructured.NestedSlice(obj.Object, "spec", "spec", field)
	assert.NilError(t, err)
	for _, entry := range containers {
		container := entry.(map[string]interface{})
		if container["name"] == name {
			return container
		}
	}
	return nil
}

func proxyCredentialVolume() map[string]interface{} {
	return map[string]interface{}{
		"name": cicdProxyCredentialVol,
		"secret": map[string]interface{}{
			"optional": true,
		},
	}
}

func assertCICDProxyRelayRemoved(t *testing.T, obj *unstructured.Unstructured) {
	t.Helper()
	for _, field := range []string{"containers", "initContainers"} {
		containers, _, err := unstructured.NestedSlice(obj.Object, "spec", "spec", field)
		assert.NilError(t, err)
		for _, entry := range containers {
			assert.Assert(t, entry.(map[string]interface{})["name"] != cicdProxyRelayContainer)
		}
	}
	volumes, _, err := unstructured.NestedSlice(obj.Object, "spec", "spec", "volumes")
	assert.NilError(t, err)
	for _, entry := range volumes {
		assert.Assert(t, entry.(map[string]interface{})["name"] != cicdProxyCredentialVol)
	}
	for _, target := range []struct{ field, name string }{
		{"containers", "runner"}, {"initContainers", cicdProxyDindContainer}} {
		container := lookupCICDContainer(t, obj, target.field, target.name)
		if container == nil {
			continue
		}
		envs, _, err := unstructured.NestedSlice(container, "env")
		assert.NilError(t, err)
		for _, entry := range envs {
			name, _ := entry.(map[string]interface{})["name"].(string)
			assert.Assert(t, name != cicdProxyHTTPEnv && name != cicdProxyHTTPSEnv)
		}
	}
}

func relayObject() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{"spec": map[string]interface{}{
			"containers": []interface{}{map[string]interface{}{"name": "runner"}},
			"initContainers": []interface{}{proxyRelayContainer(),
				map[string]interface{}{"name": cicdProxyDindContainer}},
			"volumes": []interface{}{proxyCredentialVolume()},
		}}}}
}

func relaySource(credential string) *v1.Workload {
	w := &v1.Workload{Spec: v1.WorkloadSpec{Env: map[string]string{
		common.ProxyUrl: "http://proxy.example.com:3128"}}}
	w.Spec.GroupVersionKind = v1.GroupVersionKind{Kind: common.CICDEphemeralRunnerKind, Version: "v1"}
	if credential != "" {
		w.Spec.Env[common.ProxyCredentialSecret] = credential
	}
	v1.SetAnnotation(w, v1.CICDProxyManagedAnnotation, v1.TrueStr)
	v1.SetAnnotation(w, v1.MainContainerAnnotation, "runner")
	return w
}

func TestConfigureCICDProxyRelay(t *testing.T) {
	obj, w := relayObject(), relaySource("proxy-auth")
	w.Spec.Env[common.NoProxy] = ".svc,.cluster.local"
	relay, err := configureCICDProxyRelay(obj, w, w, ephemeralRunnerSpec())
	assert.NilError(t, err)
	assert.Assert(t, relay)

	env := map[string]map[string]string{}
	for _, field := range []string{"containers", "initContainers"} {
		containers, _, err := unstructured.NestedSlice(obj.Object, "spec", "spec", field)
		assert.NilError(t, err)
		for _, entry := range containers {
			c := entry.(map[string]interface{})
			values := map[string]string{}
			list, _, _ := unstructured.NestedSlice(c, "env")
			for _, e := range list {
				item := e.(map[string]interface{})
				values[item["name"].(string)], _ = item["value"].(string)
			}
			env[c["name"].(string)] = values
		}
	}
	assert.Equal(t, env[cicdProxyRelayContainer]["PROXY_UPSTREAM_HOST"], "proxy.example.com")
	assert.Equal(t, env[cicdProxyRelayContainer]["PROXY_UPSTREAM_PORT"], "3128")
	// The runner reaches the relay on loopback, so it never holds the credential.
	assert.Equal(t, env["runner"]["http_proxy"], "http://127.0.0.1:3129")
	assert.Equal(t, env["runner"]["https_proxy"], "http://127.0.0.1:3129")
	// The runner delegates every pull and job container to dind over the shared
	// docker.sock, so dockerd needs the relay too or ghcr.io leaves via the node.
	assert.Equal(t, env[cicdProxyDindContainer]["http_proxy"], "http://127.0.0.1:3129")
	assert.Equal(t, env[cicdProxyDindContainer]["https_proxy"], "http://127.0.0.1:3129")
	assert.Equal(t, env[cicdProxyDindContainer][common.NoProxy], ".svc,.cluster.local")
	// The relay must not be pointed at itself.
	assert.Equal(t, env[cicdProxyRelayContainer]["http_proxy"], "")

	volumes, _, err := unstructured.NestedSlice(obj.Object, "spec", "spec", "volumes")
	assert.NilError(t, err)
	secret := volumes[0].(map[string]interface{})["secret"].(map[string]interface{})
	assert.Equal(t, secret["secretName"], "proxy-auth")

	delete(w.Spec.Env, common.ProxyCredentialSecret)
	relay, err = configureCICDProxyRelay(obj, w, w, ephemeralRunnerSpec())
	assert.NilError(t, err)
	assert.Assert(t, !relay)
	assertCICDProxyRelayRemoved(t, obj)
}

func TestUpdateCICDProxyContainerEnvsReachesDind(t *testing.T) {
	// NO_PROXY carries the in-cluster bypasses. dind is pointed at the relay, so it
	// needs them too: with an empty upstream domain list squid has no direct path to
	// fall back to, and an in-cluster registry pull would be forced at the corporate
	// proxy instead. dind is a native sidecar, so it sits in initContainers.
	obj, w := relayObject(), relaySource("proxy-auth")
	w.Spec.Env[common.NoProxy] = ".svc,.cluster.local"
	rt := &v1.ResourceTemplate{Spec: v1.ResourceTemplateSpec{
		ResourceSpecs: []v1.ResourceSpec{ephemeralRunnerSpec()}}}
	assert.NilError(t, updateCICDProxyContainerEnvs(obj, w, w, rt))

	for _, target := range []struct{ field, name string }{
		{"containers", "runner"}, {"initContainers", cicdProxyDindContainer}} {
		container := lookupCICDContainer(t, obj, target.field, target.name)
		assert.Assert(t, container != nil, "missing container %s", target.name)
		values := map[string]string{}
		list, _, _ := unstructured.NestedSlice(container, "env")
		for _, e := range list {
			item := e.(map[string]interface{})
			values[item["name"].(string)], _ = item["value"].(string)
		}
		assert.Equal(t, values[common.NoProxy], ".svc,.cluster.local", "container %s", target.name)
	}
	// The relay talks to the upstream proxy directly; it must not inherit the bypasses.
	relay := lookupCICDContainer(t, obj, "initContainers", cicdProxyRelayContainer)
	assert.Assert(t, relay != nil)
	list, _, _ := unstructured.NestedSlice(relay, "env")
	for _, e := range list {
		assert.Assert(t, e.(map[string]interface{})["name"] != common.NoProxy)
	}
}

func TestConfigureCICDProxyRelayRequiresUpstreamPort(t *testing.T) {
	// A forward proxy has no well-known port, so a portless PROXY_URL must fail
	// rather than have the relay peer at a guessed 3128. ParseCICDProxy rejects it
	// first; the dispatcher's own guard behind it is defense in depth.
	obj, w := relayObject(), relaySource("proxy-auth")
	w.Spec.Env[common.ProxyUrl] = "http://proxy.example.com"
	relay, err := configureCICDProxyRelay(obj, w, w, ephemeralRunnerSpec())
	assert.ErrorContains(t, err, "explicit port 1-65535")
	assert.Assert(t, !relay)
}

func TestConfigureCICDProxyRelaySkipped(t *testing.T) {
	// No credential: nothing to front, so the runner keeps the direct path.
	obj, w := relayObject(), relaySource("")
	relay, err := configureCICDProxyRelay(obj, w, w, ephemeralRunnerSpec())
	assert.NilError(t, err)
	assert.Assert(t, !relay)
	assertCICDProxyRelayRemoved(t, obj)

	// A credential cannot be used safely without the relay that keeps it out of runner env.
	bare := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{"spec": map[string]interface{}{
			"containers": []interface{}{map[string]interface{}{"name": "runner"}}}}}}
	hosted := relaySource("proxy-auth")
	hosted.Spec.GroupVersionKind.Kind = common.CICDGithubRunnerKind
	relay, err = configureCICDProxyRelay(bare, hosted, hosted, ephemeralRunnerSpec())
	assert.ErrorContains(t, err, "proxy-relay")
	assert.Assert(t, !relay)
}
