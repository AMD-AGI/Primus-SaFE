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
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
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
	// The exact command, not just its presence: the entrypoint has to keep the
	// container alive without holding a process that ignores signals.
	assert.Equal(t, w.Spec.EntryPoints[0], stringutil.Base64Encode("tail -f /dev/null"))
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
			ResourcesEnv:           `{"replica":1,"cpu":"1","memory":"2Gi","ephemeralStorage":"3Gi"}`,
			EntrypointEnv:          "cmd",
			ImageEnv:               "img",
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

// --- merged from final2_test.go ---

// TestWorkspaceMutatorHandleBranches covers workspace mutator deletion and error branches.
func TestWorkspaceMutatorHandleBranches(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}

	// deletion timestamp -> allowed
	now := metav1.Now()
	deleting := validWorkspace("ws1")
	deleting.DeletionTimestamp = &now
	deleting.Finalizers = []string{"x"}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, deleting, nil))
	assert.Assert(t, resp.Allowed)

	// creation error: cluster lookup fails
	ws := validWorkspace("ws1")
	ws.Spec.Cluster = "missing"
	resp = m.Handle(context.Background(), newRequest(t, admissionv1.Create, ws, nil))
	assert.Assert(t, !resp.Allowed)
}

// TestWorkspaceValidateVolumeRemovedNoWorkload covers volume removal with no running workloads.
func TestWorkspaceValidateVolumeRemovedNoWorkload(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{
		Cluster: "cluster1",
		Volumes: []v1.WorkspaceVolume{
			{Id: 1, Type: v1.HOSTPATH, MountPath: "/h", HostPath: "/h"},
			{Id: 2, Type: v1.PFS, MountPath: "/pfs", StorageClass: "sc", Capacity: "100Gi"},
		},
	}}
	v1.SetLabel(oldWs, v1.ClusterIdLabel, "cluster1")
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{
		Cluster: "cluster1",
		Volumes: []v1.WorkspaceVolume{{Id: 1, Type: v1.HOSTPATH, MountPath: "/h", HostPath: "/h"}},
	}}
	v1.SetLabel(newWs, v1.ClusterIdLabel, "cluster1")
	assert.NilError(t, v.validateVolumeRemoved(context.Background(), newWs, oldWs))
}

// TestNodeMutatorHandleDeletion covers the node mutator deletion-timestamp branch.
func TestNodeMutatorHandleDeletion(t *testing.T) {
	scheme := newScheme(t)
	m := &NodeMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}
	now := metav1.Now()
	node := validNode()
	node.DeletionTimestamp = &now
	node.Finalizers = []string{"x"}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Update, node, node))
	assert.Assert(t, resp.Allowed)
}

// TestNodeMutateLabelsRemoveEmpty covers empty workspace/cluster label removal.
func TestNodeMutateLabelsRemoveEmpty(t *testing.T) {
	scheme := newScheme(t)
	m := &NodeMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
		v1.WorkspaceIdLabel: "",
		v1.ClusterIdLabel:   "",
	}}, Spec: v1.NodeSpec{Hostname: pointer.String("h1")}}
	assert.Assert(t, m.mutateLabels(context.Background(), node))
}

// TestNodeValidateNodeSpecWorkspaceMissing covers node workspace existence error in spec validation.
func TestNodeValidateNodeSpecWorkspaceMissing(t *testing.T) {
	scheme := newScheme(t)
	v := &NodeValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	node := validNode()
	node.Spec.Workspace = pointer.String("missing")
	assert.Assert(t, v.validateNodeSpec(context.Background(), node) != nil)
}

// TestNodeValidateOnUpdateImmutable covers node update immutable validation.
func TestNodeValidateOnUpdateImmutable(t *testing.T) {
	scheme := newScheme(t)
	v := &NodeValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldNode := validNode()
	newNode := validNode()
	newNode.Spec.Hostname = pointer.String("changed")
	assert.Assert(t, v.validateOnUpdate(context.Background(), newNode, oldNode) != nil)
}

// TestUserValidateOnUpdateImmutable covers user update immutable validation.
func TestUserValidateOnUpdateImmutable(t *testing.T) {
	scheme := newScheme(t)
	v := &UserValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldUser := validUser("u1")
	newUser := validUser("u1")
	newUser.Spec.Type = v1.SSOUserType
	assert.Assert(t, v.validateOnUpdate(context.Background(), newUser, oldUser) != nil)
}

// TestFaultValidatorHandleUpdateDeleting covers fault validator update deletion branch.
func TestFaultValidatorHandleUpdateDeleting(t *testing.T) {
	v := &FaultValidator{decoder: newDecoder(t)}
	now := metav1.Now()
	fault := &v1.Fault{ObjectMeta: metav1.ObjectMeta{Name: "f1"}, Spec: v1.FaultSpec{MonitorId: "m1"}}
	fault.DeletionTimestamp = &now
	fault.Finalizers = []string{"x"}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Update, fault, fault))
	assert.Assert(t, resp.Allowed)
}

// --- merged from final3_test.go ---

// TestWorkspaceValidateOnUpdateVolumeRemoved covers the volume-removed update branch.
func TestWorkspaceValidateOnUpdateVolumeRemoved(t *testing.T) {
	scheme := newScheme(t)
	wl := dispatchedWorkload("w1", "cluster1", "ws1", "node1")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wl).Build()
	v := &WorkspaceValidator{Client: c}

	oldWs := validWorkspace("ws1")
	oldWs.Spec.Volumes = []v1.WorkspaceVolume{
		{Id: 1, Type: v1.PFS, MountPath: "/pfs", StorageClass: "sc", Capacity: "100Gi"},
	}
	newWs := validWorkspace("ws1")
	assert.Assert(t, v.validateOnUpdate(context.Background(), newWs, oldWs) != nil)
}

// TestWorkspaceMutateCommonGpuProductError covers the gpu product mutation error branch.
func TestWorkspaceMutateCommonGpuProductError(t *testing.T) {
	scheme := newScheme(t)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	m := &WorkspaceMutator{Client: c}
	ws := validWorkspace("ws1")
	ws.Spec.NodeFlavor = "missing"
	v1.SetAnnotation(ws, v1.GpuResourceNameAnnotation, "amd.com/gpu") // skip mutateByNodeFlavor lookup
	assert.Assert(t, m.mutateOnCreation(context.Background(), ws) != nil)
}

// TestOpsJobValidateOnUpdateImmutable covers the ops job update immutable branch.
func TestOpsJobValidateOnUpdateImmutable(t *testing.T) {
	scheme := newScheme(t)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldJob := opsJobWithDisplayName("job1", v1.OpsJobCDType)
	newJob := opsJobWithDisplayName("job1", v1.OpsJobDownloadType) // type changed
	assert.Assert(t, v.validateOnUpdate(context.Background(), newJob, oldJob) != nil)
}

// --- merged from final4_test.go ---

// TestNodeMutateOnUpdateSubnet covers the subnet annotation action branch.
func TestNodeMutateOnUpdateSubnet(t *testing.T) {
	scheme := newScheme(t)
	m := &NodeMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldNode := validNode()
	newNode := validNode()
	v1.SetAnnotation(newNode, v1.NodeSubnetAnnotation, "10.0.0.0/16")
	assert.Assert(t, m.mutateOnUpdate(context.Background(), newNode, oldNode))
	assert.Assert(t, v1.HasAnnotation(newNode, v1.NodeAnnotationAction))
}

// TestFaultValidateOnUpdateSpecError covers fault spec validation on update.
func TestFaultValidateOnUpdateSpecError(t *testing.T) {
	v := &FaultValidator{}
	newFault := &v1.Fault{ObjectMeta: metav1.ObjectMeta{Name: "f1"}}
	assert.Assert(t, v.validateOnUpdate(newFault, newFault) != nil)
}

// --- merged from final5_test.go ---

// TestWorkspaceMutateOnUpdateScaleDownRoute covers the scale-down routing branch.
func TestWorkspaceMutateOnUpdateScaleDownRoute(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldWs := validWorkspace("ws1")
	oldWs.Spec.Replica = 1
	newWs := validWorkspace("ws1")
	newWs.Spec.Replica = 1
	assert.NilError(t, m.mutateOnUpdate(context.Background(), oldWs, newWs))
}

// TestNodeValidateOnCreationFlavorMissing covers node creation with a missing flavor.
func TestNodeValidateOnCreationFlavorMissing(t *testing.T) {
	scheme := newScheme(t)
	v := &NodeValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	node := validNode()
	assert.Assert(t, v.validateOnCreation(context.Background(), node) != nil)
}

// TestOpsJobValidateOnUpdateRequiredParams covers ops update required-param validation.
func TestOpsJobValidateOnUpdateRequiredParams(t *testing.T) {
	scheme := newScheme(t)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldJob := opsJobWithDisplayName("job1", v1.OpsJobCDType)
	newJob := &v1.OpsJob{} // missing required params
	assert.Assert(t, v.validateOnUpdate(context.Background(), newJob, oldJob) != nil)
}

// --- merged from final_test.go ---

// TestClusterValidateOnCreationBadDisplayName covers the display name error branch.
func TestClusterValidateOnCreationBadDisplayName(t *testing.T) {
	scheme := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyNode("node1")).Build()
	v := &ClusterValidator{Client: c}
	cl := validControlPlaneCluster()
	v1.SetLabel(cl, v1.DisplayNameLabel, "Bad_Name")
	assert.Assert(t, v.validateOnCreation(context.Background(), cl) != nil)
}

// TestClusterValidateControlPlaneNodesInUse covers the nodes-in-use error branch.
func TestClusterValidateControlPlaneNodesInUse(t *testing.T) {
	scheme := newScheme(t)
	other := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "other"},
		Spec: v1.ClusterSpec{ControlPlane: v1.ControlPlane{Nodes: []string{"node1"}}}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(other, readyNode("node1")).Build()
	v := &ClusterValidator{Client: c}
	assert.Assert(t, v.validateControlPlane(context.Background(), validControlPlaneCluster()) != nil)
}

// TestClusterValidateControlPlaneNodesNotReady covers the nodes-not-ready error branch.
func TestClusterValidateControlPlaneNodesNotReady(t *testing.T) {
	scheme := newScheme(t)
	notReady := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(notReady).Build()
	v := &ClusterValidator{Client: c}
	assert.Assert(t, v.validateControlPlane(context.Background(), validControlPlaneCluster()) != nil)
}

// TestClusterValidateOnUpdateBadLabels covers the update label validation branch.
func TestClusterValidateOnUpdateBadLabels(t *testing.T) {
	v := &ClusterValidator{}
	oldCluster := &v1.Cluster{Spec: v1.ClusterSpec{ControlPlane: v1.ControlPlane{Nodes: []string{"node1"}}}}
	newCluster := &v1.Cluster{Spec: v1.ClusterSpec{ControlPlane: v1.ControlPlane{Nodes: []string{"node1"}}}}
	newCluster.Labels = map[string]string{"Bad Key": "v"}
	assert.Assert(t, v.validateOnUpdate(newCluster, oldCluster) != nil)
}

// TestNodeValidateCommonBadDisplayName covers display name validation in node common.
func TestNodeValidateCommonBadDisplayName(t *testing.T) {
	scheme := newScheme(t)
	v := &NodeValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	node := validNode()
	v1.SetLabel(node, v1.DisplayNameLabel, "Bad_Name")
	assert.Assert(t, v.validateCommon(context.Background(), node) != nil)
}

// TestNodeValidateTaintsBadLabelKey covers the taint key validation branch.
func TestNodeValidateTaintsBadLabelKey(t *testing.T) {
	v := &NodeValidator{}
	node := &v1.Node{Spec: v1.NodeSpec{Taints: []corev1.Taint{
		{Key: "Bad Key", Effect: corev1.TaintEffectNoSchedule},
	}}}
	assert.Assert(t, v.validateNodeTaints(node) != nil)
}

// TestNodeValidateImmutablePrivateIP covers the control-plane private IP immutability branch.
func TestNodeValidateImmutablePrivateIP(t *testing.T) {
	v := &NodeValidator{}
	oldNode := &v1.Node{Spec: v1.NodeSpec{Hostname: pointer.String("h1"), PrivateIP: "1.1.1.1"}}
	newNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{v1.KubernetesControlPlane: ""}},
		Spec: v1.NodeSpec{Hostname: pointer.String("h1"), PrivateIP: "2.2.2.2"}}
	assert.Assert(t, v.validateImmutableFields(newNode, oldNode) != nil)
}

// TestNodeValidatorHandleDeletion covers the node validator deletion-timestamp branch.
func TestNodeValidatorHandleDeletion(t *testing.T) {
	scheme := newScheme(t)
	v := &NodeValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}
	now := metav1.Now()
	node := validNode()
	node.DeletionTimestamp = &now
	node.Finalizers = []string{"x"}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Update, node, node))
	assert.Assert(t, resp.Allowed)
}

// TestUserValidatorHandleDeletion covers the user validator deletion-timestamp branch.
func TestUserValidatorHandleDeletion(t *testing.T) {
	scheme := newScheme(t)
	v := &UserValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}
	now := metav1.Now()
	user := validUser("u1")
	user.DeletionTimestamp = &now
	user.Finalizers = []string{"x"}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Update, user, user))
	assert.Assert(t, resp.Allowed)
}

// TestUserValidateOnCreationReserved covers reserved-name validation in user creation.
func TestUserValidateOnCreationReserved(t *testing.T) {
	scheme := newScheme(t)
	v := &UserValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	user := validUser(common.UserSelf)
	assert.Assert(t, v.validateOnCreation(context.Background(), user) != nil)
}

// --- merged from scenarios2_test.go ---

// dispatchedWorkload builds a dispatched, running workload bound to a node.
func dispatchedWorkload(name, cluster, ws, node string) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:   name,
		Labels: map[string]string{v1.ClusterIdLabel: cluster, v1.WorkspaceIdLabel: ws},
	}}
	v1.SetAnnotation(w, v1.WorkloadDispatchedAnnotation, v1.TrueStr)
	w.Status.Pods = []v1.WorkloadPod{{AdminNodeName: node}}
	return w
}

// TestWorkspaceValidateNodesRemoved covers running-workload node removal validation.
func TestWorkspaceValidateNodesRemoved(t *testing.T) {
	scheme := newScheme(t)
	wl := dispatchedWorkload("w1", "cluster1", "ws1", "node1")
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wl).Build()
	v := &WorkspaceValidator{Client: k8sClient}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}

	assert.NilError(t, v.validateNodesRemoved(context.Background(), ws, nil))
	assert.Assert(t, v.validateNodesRemoved(context.Background(), ws, []string{"node1"}) != nil)
}

// TestWorkspaceValidateVolumeRemovedConflict covers pvc-in-use removal validation.
func TestWorkspaceValidateVolumeRemovedConflict(t *testing.T) {
	scheme := newScheme(t)
	wl := dispatchedWorkload("w1", "cluster1", "ws1", "node1")
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wl).Build()
	v := &WorkspaceValidator{Client: k8sClient}

	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{
		Cluster: "cluster1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/pfs", StorageClass: "sc", Capacity: "100Gi"}},
	}}
	v1.SetLabel(oldWs, v1.ClusterIdLabel, "cluster1")
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetLabel(newWs, v1.ClusterIdLabel, "cluster1")
	assert.Assert(t, v.validateVolumeRemoved(context.Background(), newWs, oldWs) != nil)
}

// TestWorkspaceValidateNodesActionRemove covers node remove action validation.
func TestWorkspaceValidateNodesActionRemove(t *testing.T) {
	scheme := newScheme(t)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.NodeSpec{Workspace: pointer.String("ws1")},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	v := &WorkspaceValidator{Client: k8sClient}
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(newWs, v1.WorkspaceNodesAction, `{"node1":"remove"}`)
	assert.NilError(t, v.validateNodesAction(context.Background(), newWs, &v1.Workspace{}))
}

// TestWorkspaceMutateNodesActionRemove covers node remove action mutation.
func TestWorkspaceMutateNodesActionRemove(t *testing.T) {
	scheme := newScheme(t)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{
			v1.ClusterIdLabel:    "cluster1",
			v1.NodeFlavorIdLabel: "flavor1",
		}},
		Spec: v1.NodeSpec{Workspace: pointer.String("ws1")},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	m := &WorkspaceMutator{Client: k8sClient}
	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1", NodeFlavor: "flavor1", Replica: 2}}
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1", NodeFlavor: "flavor1", Replica: 2}}
	v1.SetAnnotation(newWs, v1.WorkspaceNodesAction, `{"node1":"remove"}`)
	assert.NilError(t, m.mutateNodesAction(context.Background(), oldWs, newWs))
	assert.Equal(t, newWs.Spec.Replica, 1)
}

// TestWorkspaceMutateScaleDownSuccess covers successful scale-down node selection.
func TestWorkspaceMutateScaleDownSuccess(t *testing.T) {
	scheme := newScheme(t)
	idleNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "node1",
		Labels: map[string]string{v1.WorkspaceIdLabel: "ws1"},
	}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(idleNode).Build()
	m := &WorkspaceMutator{Client: k8sClient}
	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Replica: 2}}
	oldWs.Status.AvailableReplica = 1
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Replica: 0}}
	assert.NilError(t, m.mutateScaleDown(context.Background(), oldWs, newWs))
	assert.Assert(t, v1.GetWorkspaceNodesAction(newWs) != "")
}

// TestWorkspaceValidateScaleDownSourceWorkload covers scale-down against a source workload.
func TestWorkspaceValidateScaleDownSourceWorkload(t *testing.T) {
	scheme := newScheme(t)
	src := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "src"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(src).Build()
	v := &WorkspaceValidator{Client: k8sClient}
	oldWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Replica: 2}}
	newWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Replica: 1}}
	v1.SetLabel(newWs, v1.SourceWorkloadIdLabel, "src")
	assert.Assert(t, v.validateScaleDown(context.Background(), newWs, oldWs) != nil)
}

// TestWorkloadValidateSpecChangedDispatched covers dispatched spec change rejection.
func TestWorkloadValidateSpecChangedDispatched(t *testing.T) {
	v := &WorkloadValidator{}
	oldW := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
		Resources:        []v1.WorkloadResource{wlResource()},
	}}
	v1.SetAnnotation(oldW, v1.WorkloadDispatchedAnnotation, v1.TrueStr)
	newW := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
		Resources:        []v1.WorkloadResource{{Replica: 2, CPU: "2", Memory: "4Gi", EphemeralStorage: "5Gi"}},
	}}
	v1.SetAnnotation(newW, v1.WorkloadDispatchedAnnotation, v1.TrueStr)
	assert.Assert(t, v.validateSpecChanged(newW, oldW) != nil)
}

// TestOpsJobMutateMetaWithNode covers cluster/flavor label derivation from node.
func TestOpsJobMutateMetaWithNode(t *testing.T) {
	scheme := newScheme(t)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.NodeSpec{NodeFlavor: &corev1.ObjectReference{Name: "flavor1"}},
	}
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node, cluster).Build()
	m := &OpsJobMutator{Client: k8sClient}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job1"},
		Spec: v1.OpsJobSpec{
			Type:   v1.OpsJobPreflightType,
			Inputs: []v1.Parameter{{Name: v1.ParameterNode, Value: "node1"}},
		},
	}
	m.mutateMeta(context.Background(), job)
	assert.Equal(t, v1.GetClusterId(job), "cluster1")
	assert.Equal(t, v1.GetNodeFlavorId(job), "flavor1")
}

// TestOpsJobMutateJobSpecWithResource covers resource gpu/replica mutation.
func TestOpsJobMutateJobSpecWithResource(t *testing.T) {
	scheme := newScheme(t)
	flavor := gpuFlavor("flavor1")
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(flavor).Build()
	m := &OpsJobMutator{Client: k8sClient}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{v1.NodeFlavorIdLabel: "flavor1"}},
		Spec: v1.OpsJobSpec{
			Resource: &v1.WorkloadResource{CPU: "1", GPU: "8", Memory: "2Gi"},
			Inputs:   []v1.Parameter{{Name: v1.ParameterNode, Value: "n1"}},
		},
	}
	m.mutateJobSpec(context.Background(), job)
	assert.Equal(t, job.Spec.Resource.GPUName, common.AmdGpu)
	assert.Equal(t, job.Spec.Resource.Replica, 1)
}

// TestWorkspaceMutatorHandleFull covers the workspace mutator full update path.
func TestWorkspaceMutatorHandleFull(t *testing.T) {
	scheme := newScheme(t)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	m := &WorkspaceMutator{Client: k8sClient, decoder: newDecoder(t)}

	oldWs := validWorkspace("ws1")
	newWs := validWorkspace("ws1")
	newWs.Spec.EnablePreempt = true
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Update, newWs, oldWs))
	assert.Assert(t, resp.Allowed)
}

// TestNodeFlavorValidatorHandleUpdate covers the node flavor validator update path.
func TestNodeFlavorValidatorHandleUpdate(t *testing.T) {
	v := &NodeFlavorValidator{decoder: newDecoder(t)}
	nf := &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: "nf1"},
		Spec: v1.NodeFlavorSpec{
			Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
			Memory: resource.MustParse("16Gi"),
		},
	}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Update, nf, nf))
	assert.Assert(t, resp.Allowed)

	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)
}

// --- merged from scenarios3_test.go ---

// TestWorkloadMutateSecretsClusterConfig covers cluster default image secret injection.
func TestWorkloadMutateSecretsClusterConfig(t *testing.T) {
	commonconfig.SetValue("global.image_secret", "imgsec")
	defer commonconfig.SetValue("global.image_secret", "")
	scheme := newScheme(t)
	m := &WorkloadMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	w := validWorkload()
	m.mutateSecrets(context.Background(), w, nil)
	assert.Equal(t, len(w.Spec.Secrets), 1)
}

// TestWorkloadMutateRdmaResourceEnabled covers the rdma assignment branch.
func TestWorkloadMutateRdmaResourceEnabled(t *testing.T) {
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")
	scheme := newScheme(t)
	flavor := &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: "flavor1"},
		Spec: v1.NodeFlavorSpec{
			Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
			Memory: resource.MustParse("16Gi"),
			Gpu:    &v1.GpuChip{ResourceName: common.AmdGpu, Quantity: resource.MustParse("8")},
			ExtendResources: corev1.ResourceList{
				"rdma/hca": resource.MustParse("4"),
			},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(flavor).Build()
	m := &WorkloadMutator{Client: k8sClient}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Resources: []v1.WorkloadResource{
		{Replica: 2, CPU: "1", GPU: "8", Memory: "2Gi"},
	}}}
	v1.SetLabel(w, v1.NodeFlavorIdLabel, "flavor1")
	m.mutateRdmaResource(context.Background(), w)
	assert.Equal(t, w.Spec.Resources[0].RdmaResource, "4")
}

// TestWorkloadValidateResourceEnoughEphemeral covers the ephemeral storage limit branch.
func TestWorkloadValidateResourceEnoughEphemeral(t *testing.T) {
	commonconfig.SetValue("workload.max_ephemeral_store_percent", "0.99")
	defer commonconfig.SetValue("workload.max_ephemeral_store_percent", "0")
	nf := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{
		Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
		Memory: resource.MustParse("16Gi"),
		ExtendResources: corev1.ResourceList{
			corev1.ResourceEphemeralStorage: resource.MustParse("100Gi"),
		},
	}}
	res := &v1.WorkloadResource{Replica: 1, CPU: "1", Memory: "2Gi", EphemeralStorage: "3Gi"}
	assert.NilError(t, validateResourceEnough(nf, res))
}

// TestClusterValidateControlPlaneErrors covers control plane field error branches.
func TestClusterValidateControlPlaneErrors(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyNode("node1")).Build()
	v := &ClusterValidator{Client: k8sClient}

	noSubnet := validControlPlaneCluster()
	noSubnet.Spec.ControlPlane.KubePodsSubnet = nil
	assert.Assert(t, v.validateControlPlane(context.Background(), noSubnet) != nil)

	noSvc := validControlPlaneCluster()
	noSvc.Spec.ControlPlane.KubeServiceAddress = nil
	assert.Assert(t, v.validateControlPlane(context.Background(), noSvc) != nil)

	noDNS := validControlPlaneCluster()
	noDNS.Spec.ControlPlane.NodeLocalDNSIP = nil
	assert.Assert(t, v.validateControlPlane(context.Background(), noDNS) != nil)

	noImg := validControlPlaneCluster()
	noImg.Spec.ControlPlane.KubeSprayImage = nil
	assert.Assert(t, v.validateControlPlane(context.Background(), noImg) != nil)
}

// TestWorkspaceValidateVolumesErrors covers volume validation error branches.
func TestWorkspaceValidateVolumesErrors(t *testing.T) {
	v := &WorkspaceValidator{}
	badType := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Type: "invalid", MountPath: "/x"},
	}}}
	assert.Assert(t, v.validateVolumes(badType, nil) != nil)

	noStorage := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Type: v1.PFS, MountPath: "/x"},
	}}}
	assert.Assert(t, v.validateVolumes(noStorage, nil) != nil)

	noCapacity := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Type: v1.PFS, MountPath: "/x", StorageClass: "sc"},
	}}}
	assert.Assert(t, v.validateVolumes(noCapacity, nil) != nil)
}

// TestNodeFlavorValidateCommonDisks covers disk and extend-resource validation branches.
func TestNodeFlavorValidateCommonDisks(t *testing.T) {
	v := &NodeFlavorValidator{}
	base := func() *v1.NodeFlavor {
		return &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{
			Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
			Memory: resource.MustParse("16Gi"),
		}}
	}
	badRoot := base()
	badRoot.Spec.RootDisk = &v1.DiskFlavor{Count: 0}
	assert.Assert(t, v.validateCommon(badRoot) != nil)

	badData := base()
	badData.Spec.DataDisk = &v1.DiskFlavor{Count: 0}
	assert.Assert(t, v.validateCommon(badData) != nil)

	badEph := base()
	badEph.Spec.ExtendResources = corev1.ResourceList{corev1.ResourceEphemeralStorage: resource.MustParse("0")}
	assert.Assert(t, v.validateCommon(badEph) != nil)

	okDisk := base()
	okDisk.Spec.RootDisk = &v1.DiskFlavor{Count: 1, Quantity: resource.MustParse("100Gi")}
	okDisk.Spec.DataDisk = &v1.DiskFlavor{Count: 2, Quantity: resource.MustParse("200Gi")}
	assert.NilError(t, v.validateCommon(okDisk))
}

// TestWorkloadValidateServiceErrors covers service validation error branches.
func TestWorkloadValidateServiceErrors(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	badProto := &v1.Workload{Spec: v1.WorkloadSpec{Service: &v1.Service{
		Port: 80, TargetPort: 8080, Protocol: "ICMP", ServiceType: corev1.ServiceTypeClusterIP,
	}}}
	assert.Assert(t, v.validateService(context.Background(), badProto) != nil)

	nodePortMissing := &v1.Workload{Spec: v1.WorkloadSpec{Service: &v1.Service{
		Port: 80, TargetPort: 8080, Protocol: corev1.ProtocolTCP, ServiceType: corev1.ServiceTypeNodePort,
	}}}
	assert.Assert(t, v.validateService(context.Background(), nodePortMissing) != nil)
}

// TestOpsJobValidateOnCreationTypes covers the create validation switch arms.
func TestOpsJobValidateOnCreationTypes(t *testing.T) {
	scheme := newScheme(t)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	dumplog := opsJobWithDisplayName("job1", v1.OpsJobDumpLogType)
	dumplog.Spec.Inputs = []v1.Parameter{{Name: v1.ParameterWorkload, Value: "w1"}}
	assert.NilError(t, v.validateOnCreation(context.Background(), dumplog))

	download := opsJobWithDisplayName("job2", v1.OpsJobDownloadType)
	download.Spec.Inputs = []v1.Parameter{
		{Name: v1.ParameterEndpoint, Value: "http://x"},
		{Name: v1.ParameterDestPath, Value: "/data"},
		{Name: v1.ParameterSecret, Value: "secret"},
		{Name: v1.ParameterWorkspace, Value: "ws1"},
	}
	assert.NilError(t, v.validateOnCreation(context.Background(), download))
}

// TestFaultMutateOnCreationWithOwner covers owner reference assignment from node.
func TestFaultMutateOnCreationWithOwner(t *testing.T) {
	scheme := newScheme(t)
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", UID: "uid-1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	m := &FaultMutator{Client: k8sClient}
	fault := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{Name: "fault1"},
		Spec: v1.FaultSpec{
			MonitorId: "m1",
			Node:      &v1.FaultNode{ClusterName: "cluster1", AdminName: "node1"},
		},
	}
	m.mutateOnCreation(context.Background(), fault)
	assert.Assert(t, len(fault.OwnerReferences) > 0)
}

// TestUserValidateOnUpdateAccessRemoved covers the update validation chain.
func TestUserValidateOnUpdateAccessRemoved(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(defaultRole()).Build()
	v := &UserValidator{Client: k8sClient}
	oldUser := validUser("u1")
	newUser := validUser("u1")
	assert.NilError(t, v.validateOnUpdate(context.Background(), newUser, oldUser))
}

// TestNodeMutatorHandleUpdateChanged covers the node mutator update patch path.
func TestNodeMutatorHandleUpdateChanged(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gpuFlavor("flavor1")).Build()
	m := &NodeMutator{Client: k8sClient, decoder: newDecoder(t)}
	oldNode := validNode()
	newNode := validNode()
	v1.SetLabel(newNode, v1.NodeGpuCountLabel, "8")
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Update, newNode, oldNode))
	assert.Assert(t, resp.Allowed)
}

// TestUserMutatorHandleUpdate covers the user mutator update patch path.
func TestUserMutatorHandleUpdate(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &UserMutator{Client: k8sClient, decoder: newDecoder(t)}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Update, validUser("u1"), validUser("u1")))
	assert.Assert(t, resp.Allowed)
}

// TestFaultValidatorHandleDecodeError covers fault validator decode-error path.
func TestFaultValidatorHandleDecodeError(t *testing.T) {
	v := &FaultValidator{decoder: newDecoder(t)}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)
}

// TestNodeValidateNodeWorkspace covers node workspace existence validation.
func TestNodeValidateNodeWorkspace(t *testing.T) {
	scheme := newScheme(t)
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	v := &NodeValidator{Client: k8sClient}
	node := validNode()
	node.Spec.Workspace = pointer.String("ws1")
	assert.NilError(t, v.validateNodeWorkspace(context.Background(), node))

	missing := validNode()
	missing.Spec.Workspace = pointer.String("missing")
	assert.Assert(t, v.validateNodeWorkspace(context.Background(), missing) != nil)
}

// --- merged from scenarios_test.go ---

// TestClusterValidatorHandleBranches covers update and decode-error handler branches.
func TestClusterValidatorHandleBranches(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyNode("node1")).Build()
	v := &ClusterValidator{Client: k8sClient, decoder: newDecoder(t)}

	oldCluster := validControlPlaneCluster()
	newCluster := validControlPlaneCluster()
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Update, newCluster, oldCluster))
	assert.Assert(t, resp.Allowed)

	// decode error path
	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)
}

// TestNodeHandleBranches covers node handler update and decode-error branches.
func TestNodeHandleBranches(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gpuFlavor("flavor1")).Build()
	m := &NodeMutator{Client: k8sClient, decoder: newDecoder(t)}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Update, validNode(), validNode()))
	assert.Assert(t, resp.Allowed)

	v := &NodeValidator{Client: k8sClient, decoder: newDecoder(t)}
	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Update, validNode(), validNode()))
	assert.Assert(t, resp.Allowed)
	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)
}

// TestOpsJobValidatorHandleBranches covers ops job handler update and decode-error branches.
func TestOpsJobValidatorHandleBranches(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	v := &OpsJobValidator{Client: k8sClient, decoder: newDecoder(t)}
	job := opsJobWithDisplayName("job1", v1.OpsJobCDType)
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Update, job, job))
	assert.Assert(t, resp.Allowed)
	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)
}

// TestWorkspaceHandleDecodeError covers workspace handler decode-error branches.
func TestWorkspaceHandleDecodeError(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &WorkspaceMutator{Client: k8sClient, decoder: newDecoder(t)}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)
}

// richWorkspace builds a workspace that exercises gpu/manager/default mutation branches.
func richWorkspace() *v1.Workspace {
	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec: v1.WorkspaceSpec{
			Cluster:     "cluster1",
			NodeFlavor:  "flavor1",
			Replica:     2,
			QueuePolicy: v1.QueueFifoPolicy,
			IsDefault:   true,
			Managers:    []string{"u1"},
			Volumes: []v1.WorkspaceVolume{
				{Type: v1.PFS, MountPath: "/pfs", StorageClass: "sc", Capacity: "100Gi", AccessMode: corev1.ReadWriteMany},
			},
		},
	}
	v1.SetLabel(ws, v1.ClusterIdLabel, "cluster1")
	v1.SetLabel(ws, v1.DisplayNameLabel, "my-ws")
	return ws
}

// TestWorkspaceRichMutateAndValidate covers gpu/manager/default workspace branches.
func TestWorkspaceRichMutateAndValidate(t *testing.T) {
	scheme := newScheme(t)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	flavor := gpuFlavor("flavor1")
	user := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, flavor, user).Build()

	m := &WorkspaceMutator{Client: k8sClient}
	assert.NilError(t, m.mutateOnCreation(context.Background(), richWorkspace()))

	v := &WorkspaceValidator{Client: k8sClient}
	assert.NilError(t, v.validateOnCreation(context.Background(), richWorkspace()))
}

// TestNodeRichMutate covers node label/subnet/flavor mutation branches.
func TestNodeRichMutate(t *testing.T) {
	scheme := newScheme(t)
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
		Spec: v1.ClusterSpec{ControlPlane: v1.ControlPlane{
			KubePodsSubnet: pointer.String("10.0.0.0/16"),
		}},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, gpuFlavor("flavor1")).Build()
	m := &NodeMutator{Client: k8sClient}
	node := validNode()
	node.Spec.Cluster = pointer.String("cluster1")
	v1.SetLabel(node, v1.NodeFlavorIdLabel, "flavor1")
	assert.Assert(t, m.mutateOnCreation(context.Background(), node))
	assert.Equal(t, v1.GetGpuResourceName(node), common.AmdGpu)
}

// TestWorkloadValidateServiceNodePort covers nodePort service validation branches.
func TestWorkloadValidateServiceNodePort(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Service: &v1.Service{
		Port: 80, TargetPort: 8080, NodePort: 30080,
		Protocol: corev1.ProtocolTCP, ServiceType: corev1.ServiceTypeNodePort,
	}}}
	assert.NilError(t, v.validateService(context.Background(), w))

	badType := &v1.Workload{Spec: v1.WorkloadSpec{Service: &v1.Service{
		Port: 80, TargetPort: 8080, Protocol: corev1.ProtocolTCP, ServiceType: "Bad",
	}}}
	assert.Assert(t, v.validateService(context.Background(), badType) != nil)
}

// TestWorkloadValidateWorkspaceQuota covers the quota-insufficient branch.
func TestWorkloadValidateWorkspaceQuota(t *testing.T) {
	scheme := newScheme(t)
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Replica: 0}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	v := &WorkloadValidator{Client: k8sClient}
	w := validWorkload()
	w.Spec.Resources = []v1.WorkloadResource{{Replica: 5, CPU: "1", Memory: "2Gi", EphemeralStorage: "3Gi"}}
	assert.Assert(t, v.validateWorkspace(context.Background(), w) != nil)
}

// TestWorkloadValidateResourceEnoughWithFlavor covers per-node resource validation with a flavor.
func TestWorkloadValidateResourceEnoughWithFlavor(t *testing.T) {
	scheme := newScheme(t)
	flavor := gpuFlavor("flavor1")
	flavor.Spec.Gpu = nil
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(flavor).Build()
	v := &WorkloadValidator{Client: k8sClient}
	w := validWorkload()
	v1.SetLabel(w, v1.NodeFlavorIdLabel, "flavor1")
	w.Spec.Resources = []v1.WorkloadResource{{Replica: 1, CPU: "1", Memory: "2Gi", SharedMemory: "1Gi", EphemeralStorage: "3Gi"}}
	assert.NilError(t, v.validateResourceEnough(context.Background(), w))
}

// TestWorkloadMutateSecretsClusterSecret covers the secret dedup branch.
func TestWorkloadMutateSecretsClusterSecret(t *testing.T) {
	scheme := newScheme(t)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sec1", Namespace: common.PrimusSafeNamespace}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	m := &WorkloadMutator{Client: k8sClient}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Secrets: []v1.SecretEntity{
		{Id: "sec1"}, {Id: "sec1"}, {Id: "missing"},
	}}}
	m.mutateSecrets(context.Background(), w, nil)
	assert.Equal(t, len(w.Spec.Secrets), 1)
}

// TestWorkloadMutateRdmaResourceWithFlavor covers the rdma loop over gpu resources.
func TestWorkloadMutateRdmaResourceWithFlavor(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gpuFlavor("flavor1")).Build()
	m := &WorkloadMutator{Client: k8sClient}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Resources: []v1.WorkloadResource{
		{Replica: 2, CPU: "1", GPU: "8", Memory: "2Gi"},
	}}}
	v1.SetLabel(w, v1.NodeFlavorIdLabel, "flavor1")
	m.mutateRdmaResource(context.Background(), w)
	assert.Equal(t, w.Spec.Resources[0].RdmaResource, "")
}

// TestWorkspaceMutateScaleDownActual covers the scale-down node selection error branch.
func TestWorkspaceMutateScaleDownActual(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Replica: 3}}
	oldWs.Status.AvailableReplica = 3
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Replica: 1}}
	// not enough nodes available -> error
	assert.Assert(t, m.mutateScaleDown(context.Background(), oldWs, newWs) != nil)
}

// --- merged from workload5_test.go ---

// TestWorkloadValidateCommonStepErrors covers each downstream validation return-error branch.
func TestWorkloadValidateCommonStepErrors(t *testing.T) {
	ctx := context.Background()
	c := fullWorkloadEnvClient(t)
	v := &WorkloadValidator{Client: c}

	// service error
	svc := fullValidWorkload()
	svc.Spec.Service = &v1.Service{Port: 0, TargetPort: 80, Protocol: corev1.ProtocolTCP, ServiceType: corev1.ServiceTypeClusterIP}
	assert.Assert(t, v.validateCommon(ctx, svc, nil) != nil)

	// health check error
	hc := fullValidWorkload()
	hc.Spec.Liveness = &v1.HealthCheck{Port: 80}
	assert.Assert(t, v.validateCommon(ctx, hc, nil) != nil)

	// resource exceeds flavor
	res := fullValidWorkload()
	res.Spec.Resources = []v1.WorkloadResource{{Replica: 1, CPU: "100", Memory: "2Gi", EphemeralStorage: "3Gi"}}
	assert.Assert(t, v.validateCommon(ctx, res, nil) != nil)

	// labels error
	lbl := fullValidWorkload()
	lbl.Spec.CustomerLabels = map[string]string{"Bad Key": "v"}
	assert.Assert(t, v.validateCommon(ctx, lbl, nil) != nil)

	// owner self-reference (a NotFound owner is intentionally tolerated per
	// issue #588, so use a self-referential owner to hit the error branch).
	owner := fullValidWorkload()
	v1.SetLabel(owner, v1.OwnerLabel, owner.Name)
	assert.Assert(t, v.validateCommon(ctx, owner, nil) != nil)
}

// TestWorkloadValidateCommonTemplateMissing covers the template-not-found branch.
func TestWorkloadValidateCommonTemplateMissing(t *testing.T) {
	scheme := newScheme(t)
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Replica: 100}}
	flavor := gpuFlavor("flavor1")
	flavor.Spec.Gpu = nil
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws, flavor).Build()
	v := &WorkloadValidator{Client: c}
	w := fullValidWorkload()
	assert.Assert(t, v.validateCommon(context.Background(), w, nil) != nil)
}

// TestWorkloadValidateOnCreationCronJobs covers the cron job validation branch.
func TestWorkloadValidateOnCreationCronJobs(t *testing.T) {
	c := fullWorkloadEnvClient(t)
	v := &WorkloadValidator{Client: c}
	w := fullValidWorkload()
	w.Spec.CronJobs = []v1.CronJob{{Schedule: "", Action: v1.CronStart}}
	assert.Assert(t, v.validateOnCreation(context.Background(), w) != nil)
}

// TestWorkloadValidateOnUpdateBranches covers immutable and spec-change update branches.
func TestWorkloadValidateOnUpdateBranches(t *testing.T) {
	c := fullWorkloadEnvClient(t)
	v := &WorkloadValidator{Client: c}

	// immutable workspace change
	oldW := fullValidWorkload()
	newW := fullValidWorkload()
	newW.Spec.Workspace = "other"
	assert.Assert(t, v.validateOnUpdate(context.Background(), newW, oldW) != nil)

	// spec changed on dispatched workload
	oldD := fullValidWorkload()
	v1.SetAnnotation(oldD, v1.WorkloadDispatchedAnnotation, v1.TrueStr)
	newD := fullValidWorkload()
	v1.SetAnnotation(newD, v1.WorkloadDispatchedAnnotation, v1.TrueStr)
	newD.Spec.Resources = []v1.WorkloadResource{{Replica: 2, CPU: "2", Memory: "4Gi", EphemeralStorage: "5Gi"}}
	assert.Assert(t, v.validateOnUpdate(context.Background(), newD, oldD) != nil)
}

// TestWorkloadValidateOnUpdateCronJobs covers cron change validation on update.
func TestWorkloadValidateOnUpdateCronJobs(t *testing.T) {
	c := fullWorkloadEnvClient(t)
	v := &WorkloadValidator{Client: c}
	oldW := fullValidWorkload()
	newW := fullValidWorkload()
	newW.Spec.CronJobs = []v1.CronJob{{Schedule: "", Action: v1.CronStart}}
	assert.Assert(t, v.validateOnUpdate(context.Background(), newW, oldW) != nil)
}

// TestWorkloadValidateWorkspaceQuotaOk covers the quota sub-resource success branch.
func TestWorkloadValidateWorkspaceQuotaOk(t *testing.T) {
	scheme := newScheme(t)
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Replica: 0}}
	ws.Status.TotalResources = corev1.ResourceList{
		corev1.ResourceCPU:              resource.MustParse("100"),
		corev1.ResourceMemory:           resource.MustParse("1000Gi"),
		corev1.ResourceEphemeralStorage: resource.MustParse("1000Gi"),
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	v := &WorkloadValidator{Client: c}
	w := validWorkload()
	w.Spec.Resources = []v1.WorkloadResource{{Replica: 1, CPU: "1", Memory: "2Gi", EphemeralStorage: "3Gi"}}
	assert.NilError(t, v.validateWorkspace(context.Background(), w))
}
