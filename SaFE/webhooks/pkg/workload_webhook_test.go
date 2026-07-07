/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"
	"time"

	"gotest.tools/assert"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// wlResource builds a valid workload resource.
func wlResource() v1.WorkloadResource {
	return v1.WorkloadResource{Replica: 1, CPU: "1", Memory: "2Gi", EphemeralStorage: "3Gi"}
}

// validWorkload builds a workload that passes required-params validation.
func validWorkload() *v1.Workload {
	w := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "w1"},
		Spec: v1.WorkloadSpec{
			Workspace:        "ws1",
			GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind, Version: "v1"},
			Resources:        []v1.WorkloadResource{wlResource()},
		},
	}
	v1.SetLabel(w, v1.ClusterIdLabel, "cluster1")
	v1.SetLabel(w, v1.DisplayNameLabel, "my-wl")
	return w
}

// TestWorkloadMutateGvk verifies default kind/version assignment.
func TestWorkloadMutateGvk(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{}
	m.mutateGvk(w)
	assert.Equal(t, w.Spec.Kind, common.PytorchJobKind)
	assert.Equal(t, w.Spec.Version, common.DefaultVersion)
}

// TestWorkloadMutatePriority verifies priority clamping.
func TestWorkloadMutatePriority(t *testing.T) {
	m := &WorkloadMutator{}
	high := &v1.Workload{Spec: v1.WorkloadSpec{Priority: 9999}}
	m.mutatePriority(high)
	assert.Equal(t, high.Spec.Priority, common.HighPriorityInt)

	low := &v1.Workload{Spec: v1.WorkloadSpec{Priority: -5}}
	m.mutatePriority(low)
	assert.Equal(t, low.Spec.Priority, common.LowPriorityInt)
}

// TestWorkloadMutateHostPath verifies workspace hostpath deduplication.
func TestWorkloadMutateHostPath(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Hostpath: []string{"/a", "/a", "/b"}}}
	ws := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Type: v1.HOSTPATH, HostPath: "/b"},
	}}}
	m.mutateHostPath(w, ws)
	assert.Equal(t, len(w.Spec.Hostpath), 1)
	assert.Equal(t, w.Spec.Hostpath[0], "/a")
}

// TestWorkloadMutateHealthCheck verifies health check defaults and clearing.
func TestWorkloadMutateHealthCheck(t *testing.T) {
	m := &WorkloadMutator{}
	app := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.DeploymentKind},
		Readiness:        &v1.HealthCheck{Path: "/healthz", Port: 8080},
		Liveness:         &v1.HealthCheck{Path: "/healthz", Port: 8080},
	}}
	m.mutateHealthCheck(app)
	assert.Equal(t, app.Spec.Readiness.InitialDelaySeconds, DefaultInitialDelaySeconds)

	job := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
		Readiness:        &v1.HealthCheck{Path: "/x"},
	}}
	m.mutateHealthCheck(job)
	assert.Assert(t, job.Spec.Readiness == nil)
}

// TestWorkloadMutateService verifies service protocol and defaults.
func TestWorkloadMutateService(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Service: &v1.Service{TargetPort: 8080}}}
	m.mutateService(w)
	assert.Equal(t, w.Spec.Service.Protocol, corev1.ProtocolTCP)
	assert.Equal(t, w.Spec.Service.Port, 8080)
	assert.Assert(t, w.Spec.Service.Extends != nil)
}

// TestWorkloadMutateDeployment verifies deployment-specific resets.
func TestWorkloadMutateDeployment(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{IsSupervised: true, MaxRetry: 5}}
	m.mutateDeployment(w)
	assert.Assert(t, !w.Spec.IsSupervised)
	assert.Equal(t, w.Spec.MaxRetry, 0)
}

// TestWorkloadMutateAuthoring verifies authoring-specific mutations.
func TestWorkloadMutateAuthoring(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Resources: []v1.WorkloadResource{wlResource(), wlResource()}}}
	m.mutateAuthoring(w)
	assert.Equal(t, len(w.Spec.Resources), 1)
	assert.Equal(t, len(w.Spec.EntryPoints), 1)
}

// TestWorkloadMutateCICDScaleSet verifies cicd scale set mutations.
func TestWorkloadMutateCICDScaleSet(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{IsSupervised: true, Resources: []v1.WorkloadResource{wlResource(), wlResource()}}}
	m.mutateCICDScaleSet(w)
	assert.Assert(t, !w.Spec.IsSupervised)
	assert.Equal(t, len(w.Spec.Resources), 1)
}

// TestWorkloadMutateTorchFT verifies torchFT env defaulting.
func TestWorkloadMutateTorchFT(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Env: map[string]string{}}}
	m.mutateTorchFT(w)
	assert.Equal(t, w.Spec.Env[common.MinReplicaCount], "1")
}

// TestWorkloadMutateMonarchJob verifies monarch job mutations.
func TestWorkloadMutateMonarchJob(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Env: map[string]string{}, Resources: []v1.WorkloadResource{wlResource()}}}
	m.mutateMonarchJob(w)
	assert.Equal(t, w.Spec.Resources[0].Replica, 1)
}

// TestWorkloadMutateSandbox verifies sandbox mutations.
func TestWorkloadMutateSandbox(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{IsSupervised: true, Resources: []v1.WorkloadResource{wlResource(), wlResource()}}}
	m.mutateSandbox(w)
	assert.Assert(t, !w.Spec.IsSupervised)
	assert.Equal(t, len(w.Spec.Resources), 1)
}

// TestWorkloadMutateDynamoDeployment verifies dynamo annotation/env defaults.
func TestWorkloadMutateDynamoDeployment(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w1"}}
	m.mutateDynamoDeployment(w)
	assert.Equal(t, v1.GetAnnotation(w, v1.DynamoBackendFrameworkAnnotation), common.DynamoDefaultBackendFramework)
	assert.Equal(t, w.Spec.Env["DYN_NAMESPACE"], "w1")
}

// TestWorkloadMutateInferaDeployment verifies infera annotation/env defaults.
func TestWorkloadMutateInferaDeployment(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w1"}}
	m.mutateInferaDeployment(w)
	assert.Equal(t, v1.GetAnnotation(w, v1.InferaBackendFrameworkAnnotation), common.InferaDefaultBackendFramework)
	assert.Assert(t, w.Spec.Env["NATS_SERVER"] != "")
}

// TestWorkloadMutateImages verifies image trimming.
func TestWorkloadMutateImages(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Images: []string{"  img:1  "}}}
	m.mutateImages(w)
	assert.Equal(t, w.Spec.Images[0], "img:1")
}

// TestWorkloadMutateRayJob verifies ray job submitter injection.
func TestWorkloadMutateRayJob(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{
		Images:      []string{"img"},
		EntryPoints: []string{"cmd"},
		Resources:   []v1.WorkloadResource{{Replica: 1, CPU: "4", Memory: "8Gi"}},
	}}
	m.mutateRayJob(w)
	assert.Equal(t, len(w.Spec.Resources), 2)
}

// TestWorkloadMutateMaxRetry verifies max retry clamping.
func TestWorkloadMutateMaxRetry(t *testing.T) {
	m := &WorkloadMutator{}
	high := &v1.Workload{Spec: v1.WorkloadSpec{MaxRetry: 9999}}
	m.mutateMaxRetry(high)
	assert.Equal(t, high.Spec.MaxRetry, DefaultMaxFailover)

	low := &v1.Workload{Spec: v1.WorkloadSpec{MaxRetry: -1}}
	m.mutateMaxRetry(low)
	assert.Equal(t, low.Spec.MaxRetry, 0)
}

// TestWorkloadMutateEnv verifies env trimming and removal annotation.
func TestWorkloadMutateEnv(t *testing.T) {
	m := &WorkloadMutator{}
	oldW := &v1.Workload{Spec: v1.WorkloadSpec{Env: map[string]string{"OLD": "v"}}}
	newW := &v1.Workload{Spec: v1.WorkloadSpec{Env: map[string]string{" NEW ": "v"}}}
	m.mutateEnv(oldW, newW)
	_, ok := newW.Spec.Env["NEW"]
	assert.Assert(t, ok)
	assert.Assert(t, v1.HasAnnotation(newW, v1.EnvToBeRemovedAnnotation))
}

// TestWorkloadMutateTTLSeconds verifies default TTL assignment.
func TestWorkloadMutateTTLSeconds(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{}
	m.mutateTTLSeconds(w)
	assert.Assert(t, w.Spec.TTLSecondsAfterFinished != nil)
}

// TestWorkloadMutateEntryPoints verifies entry point base64 encoding.
func TestWorkloadMutateEntryPoints(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
		EntryPoints:      []string{"echo hi"},
	}}
	m.mutateEntryPoints(w)
	assert.Assert(t, w.Spec.EntryPoints[0] != "echo hi")
}

// TestWorkloadMutateRdmaResource verifies no-op without node flavor.
func TestWorkloadMutateRdmaResource(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkloadMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Resources: []v1.WorkloadResource{wlResource()}}}
	m.mutateRdmaResource(context.Background(), w)
	assert.Equal(t, w.Spec.Resources[0].RdmaResource, "")
}

// TestWorkloadMutateCustomerLabels verifies empty customer label removal.
func TestWorkloadMutateCustomerLabels(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{CustomerLabels: map[string]string{"k": "v", "empty": ""}}}
	m.mutateCustomerLabels(w)
	_, ok := w.Spec.CustomerLabels["empty"]
	assert.Assert(t, !ok)
}

// TestWorkloadMutateCronJobs verifies default cron action.
func TestWorkloadMutateCronJobs(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{CronJobs: []v1.CronJob{{Schedule: "x"}}}}
	m.mutateCronJobs(w)
	assert.Equal(t, w.Spec.CronJobs[0].Action, v1.CronStart)
}

// TestWorkloadMutateSecrets verifies image secret inheritance from workspace.
func TestWorkloadMutateSecrets(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkloadMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	w := &v1.Workload{}
	ws := &v1.Workspace{Spec: v1.WorkspaceSpec{ImageSecrets: []corev1.ObjectReference{{Name: "sec1"}}}}
	m.mutateSecrets(context.Background(), w, ws)
	assert.Equal(t, len(w.Spec.Secrets), 1)
}

// TestWorkloadMutateTimeout verifies timeout assignment from workspace max runtime.
func TestWorkloadMutateTimeout(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind}}}
	ws := &v1.Workspace{Spec: v1.WorkspaceSpec{MaxRuntime: map[v1.WorkspaceScope]int{v1.TrainScope: 2}}}
	m.mutateTimeout(w, ws)
	assert.Assert(t, w.Spec.Timeout != nil)
}

// TestWorkloadMutateMeta verifies labels and finalizer on workload.
func TestWorkloadMutateMeta(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkloadMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "W1"}, Spec: v1.WorkloadSpec{
		Workspace:        "ws1",
		GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind, Version: "v1"},
	}}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	m.mutateMeta(context.Background(), w, ws)
	assert.Equal(t, v1.GetWorkspaceId(w), "ws1")
	assert.Equal(t, v1.GetClusterId(w), "cluster1")
}

// TestWorkloadMutateOwnerReference verifies default owner reference assignment.
func TestWorkloadMutateOwnerReference(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkloadMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w1"}, Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
	}}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1", UID: "uid-1"}}
	m.mutateOwnerReference(context.Background(), w, ws)
	assert.Assert(t, len(w.OwnerReferences) > 0)
}

// TestWorkloadMutateOnCreation verifies the full create mutation path.
func TestWorkloadMutateOnCreation(t *testing.T) {
	scheme := newScheme(t)
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	m := &WorkloadMutator{Client: k8sClient}
	assert.NilError(t, m.mutateOnCreation(context.Background(), validWorkload()))
}

// TestWorkloadMutateOnUpdate verifies the update mutation path.
func TestWorkloadMutateOnUpdate(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &WorkloadMutator{Client: k8sClient}
	assert.NilError(t, m.mutateOnUpdate(context.Background(), validWorkload(), validWorkload()))
}

// TestWorkloadMutatorHandle verifies the workload mutator admission handler.
func TestWorkloadMutatorHandle(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &WorkloadMutator{Client: k8sClient, decoder: newDecoder(t)}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, validWorkload(), nil))
	assert.Assert(t, resp.Allowed)

	resp = m.Handle(context.Background(), newRequest(t, admissionv1.Delete, validWorkload(), nil))
	assert.Assert(t, resp.Allowed)
}

// TestWorkloadValidateResource verifies workload resource validation.
func TestWorkloadValidateResource(t *testing.T) {
	assert.NilError(t, validateResource(nil, "ws1"))
	assert.NilError(t, validateResource(&v1.WorkloadResource{}, corev1.NamespaceDefault))
	r := wlResource()
	assert.NilError(t, validateResource(&r, "ws1"))
	assert.Assert(t, validateResource(&v1.WorkloadResource{}, "ws1") != nil)
}

// TestWorkloadValidateService verifies service validation.
func TestWorkloadValidateService(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	assert.NilError(t, v.validateService(context.Background(), &v1.Workload{}))

	ok := &v1.Workload{Spec: v1.WorkloadSpec{Service: &v1.Service{
		Port: 80, TargetPort: 8080, Protocol: corev1.ProtocolTCP, ServiceType: corev1.ServiceTypeClusterIP,
	}}}
	assert.NilError(t, v.validateService(context.Background(), ok))
}

// TestWorkloadValidateHealthCheck verifies health check validation.
func TestWorkloadValidateHealthCheck(t *testing.T) {
	v := &WorkloadValidator{}
	assert.NilError(t, v.validateHealthCheck(&v1.Workload{}))

	bad := &v1.Workload{Spec: v1.WorkloadSpec{Liveness: &v1.HealthCheck{}}}
	assert.Assert(t, v.validateHealthCheck(bad) != nil)

	ok := &v1.Workload{Spec: v1.WorkloadSpec{Readiness: &v1.HealthCheck{Path: "/h", Port: 80}}}
	assert.NilError(t, v.validateHealthCheck(ok))
}

// TestWorkloadValidateRequiredParams verifies required parameter validation.
func TestWorkloadValidateRequiredParams(t *testing.T) {
	v := &WorkloadValidator{}
	assert.NilError(t, v.validateRequiredParams(validWorkload()))
	assert.Assert(t, v.validateRequiredParams(&v1.Workload{}) != nil)
}

// TestWorkloadValidateAuthoring verifies authoring validation.
func TestWorkloadValidateAuthoring(t *testing.T) {
	v := &WorkloadValidator{}
	assert.NilError(t, v.validateAuthoring(&v1.Workload{}))
}

// TestWorkloadValidateSandbox verifies sandbox validation.
func TestWorkloadValidateSandbox(t *testing.T) {
	v := &WorkloadValidator{}
	assert.NilError(t, v.validateSandbox(validWorkload()))
}

// replicaEnv returns env with valid replica count settings.
func replicaEnv() map[string]string {
	return map[string]string{
		common.ReplicaCount:    "2",
		common.MaxReplicaCount: "4",
		common.MinReplicaCount: "1",
	}
}

// TestWorkloadValidateCICDScalingRunnerSet verifies cicd validation.
func TestWorkloadValidateCICDScalingRunnerSet(t *testing.T) {
	v := &WorkloadValidator{}
	assert.Assert(t, v.validateCICDScalingRunnerSet(&v1.Workload{}) != nil)

	ok := &v1.Workload{Spec: v1.WorkloadSpec{
		Workspace: "ws1",
		Env: map[string]string{
			ResourcesEnv:          `{"replica":1,"cpu":"1","memory":"2Gi","ephemeralStorage":"3Gi"}`,
			EntrypointEnv:         "cmd",
			ImageEnv:              "img",
			common.GithubConfigUrl: "http://x",
		},
	}}
	assert.NilError(t, v.validateCICDScalingRunnerSet(ok))
}

// TestWorkloadValidateTorchFT verifies torchFT validation.
func TestWorkloadValidateTorchFT(t *testing.T) {
	v := &WorkloadValidator{}
	assert.Assert(t, v.validateTorchFT(&v1.Workload{}, nil) != nil)

	ok := &v1.Workload{Spec: v1.WorkloadSpec{
		Resources: []v1.WorkloadResource{wlResource(), {Replica: 4, CPU: "1", Memory: "2Gi", EphemeralStorage: "3Gi"}},
		Images:    []string{"a", "b"},
		Env:       replicaEnv(),
	}}
	assert.NilError(t, v.validateTorchFT(ok, nil))
}

// TestWorkloadValidateRayJob verifies rayJob validation.
func TestWorkloadValidateRayJob(t *testing.T) {
	v := &WorkloadValidator{}
	assert.Assert(t, v.validateRayJob(&v1.Workload{}, nil) != nil)

	ok := &v1.Workload{Spec: v1.WorkloadSpec{
		Resources: []v1.WorkloadResource{wlResource(), wlResource()},
		Images:    []string{"a", "b"},
		Env:       map[string]string{common.RayJobEntrypoint: "python main.py"},
	}}
	assert.NilError(t, v.validateRayJob(ok, nil))
}

// TestWorkloadValidateMonarchJob verifies monarch validation.
func TestWorkloadValidateMonarchJob(t *testing.T) {
	v := &WorkloadValidator{}
	assert.Assert(t, v.validateMonarchJob(&v1.Workload{}, nil) != nil)

	ok := &v1.Workload{Spec: v1.WorkloadSpec{
		Resources:   []v1.WorkloadResource{wlResource(), {Replica: 4, CPU: "1", Memory: "2Gi", EphemeralStorage: "3Gi"}},
		EntryPoints: []string{"cmd"},
		Env:         replicaEnv(),
	}}
	assert.NilError(t, v.validateMonarchJob(ok, nil))
}

// TestWorkloadValidateReplicaCount verifies replica count validation.
func TestWorkloadValidateReplicaCount(t *testing.T) {
	v := &WorkloadValidator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Resources: []v1.WorkloadResource{wlResource(), wlResource()}}}
	assert.Assert(t, v.validateReplicaCount(w, nil) != nil)

	ok := &v1.Workload{Spec: v1.WorkloadSpec{
		Resources: []v1.WorkloadResource{wlResource(), {Replica: 4}},
		Env:       replicaEnv(),
	}}
	assert.NilError(t, v.validateReplicaCount(ok, nil))
}

// TestWorkloadValidateDynamoDeployment verifies dynamo validation.
func TestWorkloadValidateDynamoDeployment(t *testing.T) {
	v := &WorkloadValidator{}
	assert.Assert(t, v.validateDynamoDeployment(&v1.Workload{}) != nil)

	w := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.DynamoDeploymentKind},
		Resources:        []v1.WorkloadResource{wlResource()},
	}}
	v1.SetAnnotation(w, v1.DynamoBackendFrameworkAnnotation, "sglang")
	v1.SetAnnotation(w, v1.DynamoKVTransferBackendAnnotation, common.DynamoKVBackendNixl)
	v1.SetAnnotation(w, v1.DynamoServiceRolesAnnotation, common.DynamoRoleFrontend)
	assert.NilError(t, v.validateDynamoDeployment(w))
}

// TestWorkloadValidateInferaDeployment verifies infera validation.
func TestWorkloadValidateInferaDeployment(t *testing.T) {
	v := &WorkloadValidator{}
	assert.Assert(t, v.validateInferaDeployment(&v1.Workload{}) != nil)

	w := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.InferaDeploymentKind},
		Resources:        []v1.WorkloadResource{wlResource()},
	}}
	v1.SetAnnotation(w, v1.InferaBackendFrameworkAnnotation, "sglang")
	v1.SetAnnotation(w, v1.InferaKVTransferBackendAnnotation, common.DynamoKVBackendNixl)
	v1.SetAnnotation(w, v1.InferaServiceRolesAnnotation, common.DynamoRoleFrontend)
	assert.NilError(t, v.validateInferaDeployment(w))
}

// TestWorkloadValidateWorkspace verifies workspace existence validation.
func TestWorkloadValidateWorkspace(t *testing.T) {
	scheme := newScheme(t)
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Replica: 10}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	v := &WorkloadValidator{Client: k8sClient}
	assert.NilError(t, v.validateWorkspace(context.Background(), validWorkload()))

	missing := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	assert.Assert(t, missing.validateWorkspace(context.Background(), validWorkload()) != nil)
}

// TestWorkloadValidateResourceEnough verifies node flavor resource validation.
func TestWorkloadValidateResourceEnough(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	// no flavor id -> nil flavor and nil error returned, total replica > 0
	assert.NilError(t, v.validateResourceEnough(context.Background(), validWorkload()))
}

// TestWorkloadValidateTemplate verifies template existence validation error path.
func TestWorkloadValidateTemplate(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	assert.Assert(t, v.validateTemplate(context.Background(), validWorkload()) != nil)
}

// TestWorkloadValidateImmutableFields verifies immutable field checks.
func TestWorkloadValidateImmutableFields(t *testing.T) {
	v := &WorkloadValidator{}
	oldW := validWorkload()
	assert.NilError(t, v.validateImmutableFields(validWorkload(), oldW))

	changed := validWorkload()
	changed.Spec.Workspace = "other"
	assert.Assert(t, v.validateImmutableFields(changed, oldW) != nil)
}

// TestWorkloadValidateScope verifies scope validation.
func TestWorkloadValidateScope(t *testing.T) {
	scheme := newScheme(t)
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	v := &WorkloadValidator{Client: k8sClient}
	assert.NilError(t, v.validateScope(context.Background(), validWorkload()))
}

// TestWorkloadValidateSpecChanged verifies dispatched spec change validation.
func TestWorkloadValidateSpecChanged(t *testing.T) {
	v := &WorkloadValidator{}
	assert.NilError(t, v.validateSpecChanged(validWorkload(), validWorkload()))
}

// TestWorkloadValidateOwnerWorkload verifies owner workload validation.
func TestWorkloadValidateOwnerWorkload(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	assert.NilError(t, v.validateOwnerWorkload(context.Background(), validWorkload()))

	selfRef := validWorkload()
	v1.SetLabel(selfRef, v1.OwnerLabel, selfRef.Name)
	assert.Assert(t, v.validateOwnerWorkload(context.Background(), selfRef) != nil)
}

// TestGetWorkload verifies workload retrieval helper.
func TestGetWorkload(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(validWorkload()).Build()
	got, err := getWorkload(ctx, k8sClient, "w1")
	assert.NilError(t, err)
	assert.Assert(t, got != nil)
}

// TestWorkloadValidatorHandle verifies the workload validator admission handler.
func TestWorkloadValidatorHandle(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	v := &WorkloadValidator{Client: k8sClient, decoder: newDecoder(t)}
	// missing template -> validation fails, but handler returns a response
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, validWorkload(), nil))
	assert.Assert(t, !resp.Allowed)
}

// fullWorkloadEnvClient builds a client with all objects required for full workload validation.
func fullWorkloadEnvClient(t *testing.T) client.Client {
	scheme := newScheme(t)
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Replica: 100}}
	flavor := gpuFlavor("flavor1")
	flavor.Spec.Gpu = nil
	rt := &v1.ResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "rt1",
			Labels:      map[string]string{v1.WorkloadVersionLabel: "v1"},
			Annotations: map[string]string{v1.WorkloadKindLabel: common.PytorchJobKind},
		},
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wt1",
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				v1.WorkloadVersionLabel: "v1",
				v1.WorkloadKindLabel:    common.PytorchJobKind,
			},
		},
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws, flavor, rt, cm).Build()
}

// fullValidWorkload builds a workload that passes full create validation.
func fullValidWorkload() *v1.Workload {
	w := validWorkload()
	v1.SetLabel(w, v1.NodeFlavorIdLabel, "flavor1")
	return w
}

// TestWorkloadValidateFullChain verifies the full create/update validation chain succeeds.
func TestWorkloadValidateFullChain(t *testing.T) {
	c := fullWorkloadEnvClient(t)
	v := &WorkloadValidator{Client: c}
	assert.NilError(t, v.validateOnCreation(context.Background(), fullValidWorkload()))
	assert.NilError(t, v.validateOnUpdate(context.Background(), fullValidWorkload(), fullValidWorkload()))
}

// TestWorkloadValidatorHandleFull verifies the validator handler with a complete environment.
func TestWorkloadValidatorHandleFull(t *testing.T) {
	c := fullWorkloadEnvClient(t)
	v := &WorkloadValidator{Client: c, decoder: newDecoder(t)}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, fullValidWorkload(), nil))
	assert.Assert(t, resp.Allowed)

	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Update, fullValidWorkload(), fullValidWorkload()))
	assert.Assert(t, resp.Allowed)
}

func TestValidateCronJobs(t *testing.T) {
	nowTime := time.Now().UTC()
	tests := []struct {
		name   string
		t      time.Time
		result bool
	}{
		{"Past time", nowTime.Add(-time.Hour), false},
		{"Future 1 minute", nowTime.Add(time.Minute), true},
		{"Future 6 months", nowTime.AddDate(0, 6, 0), true},
		{"Almost 1 year but less 1 minute", nowTime.AddDate(1, 0, 0).Add(-time.Minute), true},
		{"Exactly 1 year", nowTime.AddDate(1, 0, 0), false},
		{"Over 1 year", nowTime.AddDate(1, 0, 0).Add(time.Minute), false},
		{"now", nowTime, false},
	}

	var validator WorkloadValidator
	for _, tt := range tests {
		workload := &v1.Workload{
			Spec: v1.WorkloadSpec{
				CronJobs: []v1.CronJob{{
					Schedule: tt.t.Format(timeutil.TimeRFC3339Milli),
					Action:   v1.CronStart,
				}},
			},
		}
		err := validator.validateCronJobs(workload)
		assert.Equal(t, tt.result, err == nil)
	}
}

func TestMutateResources(t *testing.T) {
	gpuResourceName := "amd.com/gpu"
	workspaceWithGpu := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				v1.GpuResourceNameAnnotation: gpuResourceName,
			},
		},
	}

	tests := []struct {
		name              string
		workload          *v1.Workload
		workspace         *v1.Workspace
		expectedChanged   bool
		expectedResources []v1.WorkloadResource
	}{
		{
			name: "Replica 0 is filtered out",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Resources: []v1.WorkloadResource{
						{Replica: 0, CPU: "8", Memory: "64Gi"},
						{Replica: 1, CPU: "16", Memory: "128Gi"},
					},
				},
			},
			workspace:       nil,
			expectedChanged: true,
			expectedResources: []v1.WorkloadResource{
				{Replica: 1, CPU: "16", Memory: "128Gi", SharedMemory: "64Gi", EphemeralStorage: DefaultEphemeralStorage},
			},
		},
		{
			name: "GPU '0' cleared and GPUName set from workspace",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Resources: []v1.WorkloadResource{
						{Replica: 1, CPU: "8", GPU: "0", Memory: "64Gi"},
						{Replica: 1, CPU: "8", GPU: "4", Memory: "64Gi"},
					},
				},
			},
			workspace:       workspaceWithGpu,
			expectedChanged: true,
			expectedResources: []v1.WorkloadResource{
				{Replica: 1, CPU: "8", GPU: "", Memory: "64Gi", SharedMemory: "32Gi", EphemeralStorage: DefaultEphemeralStorage},
				{Replica: 1, CPU: "8", GPU: "4", GPUName: gpuResourceName, Memory: "64Gi", SharedMemory: "32Gi", EphemeralStorage: DefaultEphemeralStorage},
			},
		},
		{
			name: "SharedMemory and EphemeralStorage get defaults",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Resources: []v1.WorkloadResource{
						{Replica: 1, CPU: "8", Memory: "100Gi"},
					},
				},
			},
			workspace:       nil,
			expectedChanged: true,
			expectedResources: []v1.WorkloadResource{
				{Replica: 1, CPU: "8", Memory: "100Gi", SharedMemory: "50Gi", EphemeralStorage: DefaultEphemeralStorage},
			},
		},
		{
			name: "SharedMemory and EphemeralStorage not overwritten if set",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Resources: []v1.WorkloadResource{
						{Replica: 1, CPU: "8", Memory: "64Gi", SharedMemory: "16Gi", EphemeralStorage: "200Gi"},
					},
				},
			},
			workspace:       nil,
			expectedChanged: false,
			expectedResources: []v1.WorkloadResource{
				{Replica: 1, CPU: "8", Memory: "64Gi", SharedMemory: "16Gi", EphemeralStorage: "200Gi"},
			},
		},
		{
			name: "Multiple resources with mixed scenarios",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Resources: []v1.WorkloadResource{
						{Replica: 0, CPU: "4", Memory: "32Gi"},                                                            // filtered out
						{Replica: 2, CPU: "8", GPU: "4", Memory: "64Gi"},                                                  // GPU + defaults
						{Replica: 1, CPU: "16", GPU: "0", Memory: "128Gi", SharedMemory: "64Gi", EphemeralStorage: "1Ti"}, // GPU=0 cleared
					},
				},
			},
			workspace:       workspaceWithGpu,
			expectedChanged: true,
			expectedResources: []v1.WorkloadResource{
				{Replica: 2, CPU: "8", GPU: "4", GPUName: gpuResourceName, Memory: "64Gi", SharedMemory: "32Gi", EphemeralStorage: DefaultEphemeralStorage},
				{Replica: 1, CPU: "16", GPU: "", Memory: "128Gi", SharedMemory: "64Gi", EphemeralStorage: "1Ti"},
			},
		},
	}

	var mutator WorkloadMutator
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutator.mutateResources(context.Background(), tt.workload, tt.workspace)
			assert.Equal(t, len(tt.expectedResources), len(tt.workload.Spec.Resources), "resources count mismatch")
			for i, expected := range tt.expectedResources {
				actual := tt.workload.Spec.Resources[i]
				assert.Equal(t, expected.Replica, actual.Replica, "Replica mismatch at index %d", i)
				assert.Equal(t, expected.CPU, actual.CPU, "CPU mismatch at index %d", i)
				assert.Equal(t, expected.GPU, actual.GPU, "GPU mismatch at index %d", i)
				assert.Equal(t, expected.GPUName, actual.GPUName, "GPUName mismatch at index %d", i)
				assert.Equal(t, expected.Memory, actual.Memory, "Memory mismatch at index %d", i)
				assert.Equal(t, expected.SharedMemory, actual.SharedMemory, "SharedMemory mismatch at index %d", i)
				assert.Equal(t, expected.EphemeralStorage, actual.EphemeralStorage, "EphemeralStorage mismatch at index %d", i)
			}
		})
	}
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	s := runtime.NewScheme()
	err := v1.AddToScheme(s)
	assert.NilError(t, err)
	return s
}

func newWorkloadWithOwner(name, workspace, ownerId string) *v1.Workload {
	w := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v1.WorkloadSpec{Workspace: workspace},
	}
	if ownerId != "" {
		v1.SetLabel(w, v1.OwnerLabel, ownerId)
	}
	return w
}

func TestValidateOwnerWorkload(t *testing.T) {
	ctx := context.TODO()
	scheme := newTestScheme(t)

	tests := []struct {
		name      string
		workload  *v1.Workload
		objects   []client.Object
		expectErr bool
	}{
		{
			name:      "no owner label is allowed",
			workload:  newWorkloadWithOwner("child", "ws1", ""),
			expectErr: false,
		},
		{
			name:      "self reference is rejected",
			workload:  newWorkloadWithOwner("child", "ws1", "child"),
			expectErr: true,
		},
		{
			// Regression test for issue #588: the apiserver creates an
			// owner-labeled preheat child before its owner workload is
			// persisted, so a missing owner must not block admission.
			name:      "missing owner is tolerated",
			workload:  newWorkloadWithOwner("child", "ws1", "owner-not-yet-created"),
			expectErr: false,
		},
		{
			name:     "existing owner in same workspace is allowed",
			workload: newWorkloadWithOwner("child", "ws1", "owner"),
			objects: []client.Object{
				newWorkloadWithOwner("owner", "ws1", ""),
			},
			expectErr: false,
		},
		{
			name:     "owner in different workspace is rejected",
			workload: newWorkloadWithOwner("child", "ws1", "owner"),
			objects: []client.Object{
				newWorkloadWithOwner("owner", "ws2", ""),
			},
			expectErr: true,
		},
		{
			name:     "cycle is rejected",
			workload: newWorkloadWithOwner("child", "ws1", "owner"),
			objects: []client.Object{
				newWorkloadWithOwner("owner", "ws1", "child"),
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...).Build()
			v := &WorkloadValidator{Client: k8sClient}
			err := v.validateOwnerWorkload(ctx, tt.workload)
			assert.Equal(t, tt.expectErr, err != nil)
		})
	}
}

// TestValidateOwnerWorkload_LookupError ensures that a non-NotFound owner
// lookup error (e.g. RBAC/connection) fails closed instead of silently
// skipping the workspace/cycle checks.
func TestValidateOwnerWorkload_LookupError(t *testing.T) {
	ctx := context.TODO()
	scheme := newTestScheme(t)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
				return apierrors.NewServiceUnavailable("apiserver unreachable")
			},
		}).
		Build()
	v := &WorkloadValidator{Client: k8sClient}

	err := v.validateOwnerWorkload(ctx, newWorkloadWithOwner("child", "ws1", "owner"))
	assert.Assert(t, err != nil)
}

func TestMutateStickyNodes_EnablePreempt(t *testing.T) {
	ctx := context.TODO()
	scheme := newTestScheme(t)

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "w1",
			Annotations: map[string]string{
				v1.RetryOnOriginalNodesAnnotation: v1.TrueStr,
				v1.NodesAffinityAnnotation:        common.NodesAffinityRequired,
			},
		},
		Spec: v1.WorkloadSpec{
			MaxRetry:         1,
			GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
		},
	}

	workspace := &v1.Workspace{
		Spec: v1.WorkspaceSpec{
			EnablePreempt: true,
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &WorkloadMutator{Client: k8sClient}

	m.mutateStickNodes(ctx, workload, workspace)

	// Should remove sticky nodes annotation when preempt is enabled
	assert.Equal(t, v1.GetAnnotation(workload, v1.RetryOnOriginalNodesAnnotation), "")
}

func TestMutateStickyNodes_UnsupportedKind(t *testing.T) {
	ctx := context.TODO()
	scheme := newTestScheme(t)

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "w1",
			Annotations: map[string]string{
				v1.RetryOnOriginalNodesAnnotation: v1.TrueStr,
				v1.NodesAffinityAnnotation:        common.NodesAffinityRequired,
			},
		},
		Spec: v1.WorkloadSpec{
			MaxRetry:         1,
			GroupVersionKind: v1.GroupVersionKind{Kind: "Deployment"}, // unsupported kind
		},
	}

	workspace := &v1.Workspace{
		Spec: v1.WorkspaceSpec{
			EnablePreempt: false,
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &WorkloadMutator{Client: k8sClient}

	m.mutateStickNodes(ctx, workload, workspace)

	// Should remove sticky nodes annotation for unsupported kind
	assert.Equal(t, v1.GetAnnotation(workload, v1.RetryOnOriginalNodesAnnotation), "")
}

func TestMutateStickyNodes_GpuCountMismatch(t *testing.T) {
	ctx := context.TODO()
	scheme := newTestScheme(t)

	nodeFlavor := &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nf1",
		},
		Spec: v1.NodeFlavorSpec{
			Gpu: &v1.GpuChip{
				Quantity: resource.MustParse("8"),
			},
		},
	}

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "w1",
			Labels: map[string]string{
				v1.NodeFlavorIdLabel: "nf1",
			},
			Annotations: map[string]string{
				v1.RetryOnOriginalNodesAnnotation: v1.TrueStr,
				v1.NodesAffinityAnnotation:        common.NodesAffinityRequired,
			},
		},
		Spec: v1.WorkloadSpec{
			MaxRetry:         1,
			GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
			Resources: []v1.WorkloadResource{
				{GPU: "4"}, // mismatch: 4 != 8
			},
		},
	}

	workspace := &v1.Workspace{
		Spec: v1.WorkspaceSpec{
			EnablePreempt: false,
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeFlavor).Build()
	m := &WorkloadMutator{Client: k8sClient}

	m.mutateStickNodes(ctx, workload, workspace)

	// Should remove sticky nodes annotation when GPU count mismatch
	assert.Equal(t, v1.GetAnnotation(workload, v1.RetryOnOriginalNodesAnnotation), "")
}

func TestMutateStickyNodes_AllConditionsPass(t *testing.T) {
	ctx := context.TODO()
	scheme := newTestScheme(t)

	nodeFlavor := &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nf1",
		},
		Spec: v1.NodeFlavorSpec{
			Gpu: &v1.GpuChip{
				Quantity: resource.MustParse("8"),
			},
		},
	}

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "w1",
			Labels: map[string]string{
				v1.NodeFlavorIdLabel: "nf1",
			},
			Annotations: map[string]string{
				v1.NodesAffinityAnnotation: common.NodesAffinityRequired,
			},
		},
		Spec: v1.WorkloadSpec{
			MaxRetry:         1,
			GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
			Resources: []v1.WorkloadResource{
				{GPU: "8"}, // matches node flavor GPU count
			},
		},
	}

	workspace := &v1.Workspace{
		Spec: v1.WorkspaceSpec{
			EnablePreempt: false,
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeFlavor).Build()
	m := &WorkloadMutator{Client: k8sClient}

	m.mutateStickNodes(ctx, workload, workspace)

	// Should keep sticky nodes annotation when all conditions pass
	assert.Equal(t, v1.GetAnnotation(workload, v1.RetryOnOriginalNodesAnnotation), v1.TrueStr)
}

func TestValidateResourceEnough_CpuFlavorWithGpuRequest(t *testing.T) {
	// NodeFlavor: CPU-only (no GPU)
	nf := &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: "amd-cpu"},
		Spec: v1.NodeFlavorSpec{
			Cpu:    v1.CpuChip{Quantity: resource.MustParse("32")},
			Memory: resource.MustParse("256Gi"),
			ExtendResources: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceEphemeralStorage: resource.MustParse("990Gi"),
			},
		},
	}

	tests := []struct {
		name    string
		res     *v1.WorkloadResource
		wantErr bool
	}{
		{
			name: "gpu request on cpu-only flavor should fail",
			res: &v1.WorkloadResource{
				CPU:              "1",
				GPU:              "1",
				GPUName:          "amd.com/gpu",
				Memory:           "2Gi",
				SharedMemory:     "1Gi",
				EphemeralStorage: "3Gi",
				Replica:          1,
			},
			wantErr: true,
		},
		{
			name: "cpu-only request on cpu flavor should pass",
			res: &v1.WorkloadResource{
				CPU:              "1",
				Memory:           "2Gi",
				EphemeralStorage: "3Gi",
				Replica:          1,
			},
			wantErr: false,
		},
		{
			name: "cpu request exceeding flavor should fail",
			res: &v1.WorkloadResource{
				CPU:              "64",
				Memory:           "2Gi",
				EphemeralStorage: "3Gi",
				Replica:          1,
			},
			wantErr: true,
		},
		{
			name: "memory request exceeding flavor should fail",
			res: &v1.WorkloadResource{
				CPU:              "1",
				Memory:           "512Gi",
				EphemeralStorage: "3Gi",
				Replica:          1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResourceEnough(nf, tt.res)
			if tt.wantErr {
				assert.Assert(t, err != nil, "expected error but got nil")
			} else {
				assert.NilError(t, err)
			}
		})
	}
}
