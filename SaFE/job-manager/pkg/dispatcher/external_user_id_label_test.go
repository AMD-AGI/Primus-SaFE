/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const (
	// creatorUserId is the id the Workload carries as primus-safe.user.id, set by the apiserver
	// from the authenticated user.
	creatorUserId = "0123456789abcdef0123456789abcdef"
	// forgedUserId is another user's id, written by a template where SaFE stamps Pod labels.
	forgedUserId = "fedcba9876543210fedcba9876543210"
)

// kubeSchedulerPlacement marks a workload as admitted on the kube-scheduler path, the
// placement every external workspace uses. A multi-node RDMA PyTorchJob is a gang, which is
// admitted against a ProvisioningRequest.
func kubeSchedulerPlacement(w *v1.Workload) {
	w.Status.ExternalExecution = &v1.WorkloadExternalExecution{
		PlacementMode: v1.ExternalPlacementKubeScheduler, DispatchGeneration: 1, ProvisioningRequest: "pr-1",
	}
}

// userIdLabelCase is one kind SaFE renders, with the template and resource template it is
// rendered from and what the workload needs besides its kind.
type userIdLabelCase struct {
	// name tells cases of one kind apart; it is the kind when empty.
	name     string
	kind     string
	template string
	rt       *v1.ResourceTemplate
	mutate   func(*v1.Workload)
	// config is set for the render and unset after it.
	config map[string]string
}

func userIdLabelCases(t *testing.T) []userIdLabelCase {
	pytorch := func(kind string) userIdLabelCase {
		rt := jobutils.TestPytorchResourceTemplate.DeepCopy()
		v1.SetAnnotation(rt, v1.WorkloadKindLabel, kind)
		return userIdLabelCase{kind: kind, template: withTemplateKind(t, TestPytorchJobTemplateConfig, kind), rt: rt,
			mutate: func(w *v1.Workload) {
				// A master and a worker, so both replica templates are rendered.
				w.Spec.Resources = append(w.Spec.Resources, *w.Spec.Resources[0].DeepCopy())
				w.Spec.Resources[1].Replica = 2
				w.Spec.Images = append(w.Spec.Images, w.Spec.Images[0])
				w.Spec.EntryPoints = append(w.Spec.EntryPoints, w.Spec.EntryPoints[0])
			}}
	}
	// A PyTorchJob an Agent Sandbox grant created: rendered with the envd injector and the
	// session identity, and with its training hostPaths dropped.
	agentSandboxPyTorch := pytorch(common.PytorchJobKind)
	agentSandboxPyTorch.name = "AgentSandboxPyTorchJob"
	pytorchMutate := agentSandboxPyTorch.mutate
	agentSandboxPyTorch.mutate = func(w *v1.Workload) {
		pytorchMutate(w)
		v1.SetLabel(w, grantIDLabel, "gr-01")
		v1.SetLabel(w, sandboxIDLabel, "sbx-1")
		w.Spec.Env[sandboxAuthPublicKeyEnvName] = "test-public-key"
	}
	agentSandboxPyTorch.config = map[string]string{"sandbox.envd_injector_image": "example/envd-injector:test"}
	return []userIdLabelCase{
		pytorch(common.PytorchJobKind),
		pytorch(common.AuthoringKind),
		pytorch(common.UnifiedJobKind),
		agentSandboxPyTorch,
		{kind: common.JobKind, template: TestJobTemplateConfig, rt: jobutils.TestJobResourceTemplate},
		{kind: common.StatefulSetKind, template: withTemplateKind(t,
			strings.ReplaceAll(TestDeploymentTemplateConfig, "kind: Deployment", "kind: StatefulSet"),
			common.StatefulSetKind), rt: jobutils.TestStatefulSetResourceTemplate,
			mutate: func(w *v1.Workload) { w.Spec.GroupVersionKind.Group = "apps" }},
		{kind: common.DeploymentKind, template: TestDeploymentTemplateConfig, rt: jobutils.TestDeploymentResourceTemplate,
			mutate: func(w *v1.Workload) { w.Spec.GroupVersionKind.Group = "apps" }},
		{kind: common.SandboxKind, template: TestSandboxConfig, rt: jobutils.TestSandboxResourceTemplate,
			mutate: func(w *v1.Workload) {
				w.Spec.GroupVersionKind.Version = common.DefaultVersion
				w.Spec.JobPort = 0
				w.Spec.EntryPoints = nil
				w.Spec.Resources[0].RdmaResource = ""
				w.Spec.Env[sandboxAuthPublicKeyEnvName] = "test-public-key"
			}},
		{kind: common.MonarchClient, template: TestMonarchClientTemplateConfig, rt: jobutils.TestMonarchClientResourceTemplate,
			mutate: func(w *v1.Workload) {
				w.Spec.GroupVersionKind.Version = common.DefaultVersion
				w.Spec.Resources[0].GPU = ""
				w.Spec.Resources[0].RdmaResource = ""
				w.Spec.JobPort = 0
				w.Spec.Env[common.MonarchPort] = strconv.Itoa(common.MonarchMeshPortNum)
			}},
		{kind: common.MonarchMesh, template: TestMonarchMeshTemplateConfig, rt: jobutils.TestMonarchMeshResourceTemplate,
			mutate: func(w *v1.Workload) {
				w.Spec.GroupVersionKind.Version = common.DefaultVersion
				w.Spec.JobPort = 0
				w.Spec.Env[common.MonarchPort] = strconv.Itoa(common.MonarchMeshPortNum)
				v1.SetLabel(w, v1.GroupIdLabel, "0")
				v1.SetAnnotation(w, v1.ResourceIdAnnotation, "1")
			}},
		{kind: common.RayJobKind, template: TestRayJobTemplateConfig, rt: jobutils.TestRayJobResourceTemplate},
		{kind: common.CICDScaleRunnerSetKind, template: TestCICDScaleSetTemplateConfig,
			rt: jobutils.TestCICDScaleSetResourceTemplate, mutate: func(w *v1.Workload) {
				w.Spec.Env[common.GithubConfigUrl] = "test-url"
				v1.SetAnnotation(w, v1.AdminControlPlaneAnnotation, "10.0.0.1")
				v1.SetAnnotation(w, v1.GithubSecretIdAnnotation, "test-secret")
				w.Spec.EntryPoints = []string{stringutil.Base64Encode("bash test.sh")}
			}},
	}
}

// withTemplateKind returns a workload template ConfigMap that serves kind instead of the kind
// it declares.
func withTemplateKind(t *testing.T, content, kind string) string {
	t.Helper()
	cm, err := parseConfigmap(content)
	assert.NilError(t, err)
	cm.Labels[v1.WorkloadKindLabel] = kind
	out, err := json.Marshal(cm)
	assert.NilError(t, err)
	return string(out)
}

// renderUserIdLabelCase renders c through generateK8sObject, the dispatcher's create path, for
// a workload whose creator is creatorUserId. forged, when set, is written as primus-safe.user.id
// into the template at the object labels and at every Pod template's labels.
func renderUserIdLabelCase(t *testing.T, c userIdLabelCase, forged string,
	mutate func(*v1.Workload)) (*unstructured.Unstructured, *v1.Workload, error) {
	t.Helper()
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")
	for key, value := range c.config {
		commonconfig.SetValue(key, value)
		defer commonconfig.UnsetValue(key)
	}
	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestWorkloadData.DeepCopy()
	if c.kind == common.RayJobKind {
		workload = jobutils.TestRayWorkloadData.DeepCopy()
	}
	workload.Spec.Workspace = workspace.Name
	workload.Spec.GroupVersionKind = v1.GroupVersionKind{Version: "v1", Kind: c.kind}
	if c.mutate != nil {
		c.mutate(workload)
	}
	metav1.SetMetaDataLabel(&workload.ObjectMeta, v1.UserIdLabel, creatorUserId)
	if mutate != nil {
		mutate(workload)
	}
	configmap, err := parseConfigmap(c.template)
	assert.NilError(t, err)
	if forged != "" {
		forgeTemplateUserId(t, configmap, workload, c.rt, forged)
	}
	metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap, c.rt, workspace).WithScheme(scheme).Build()
	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	return obj, workload, err
}

// forgeTemplateUserId writes primus-safe.user.id=value into the workload template of cm, at the
// object labels and at the labels of every Pod template of the resource template that the
// template has.
func forgeTemplateUserId(t *testing.T, cm *corev1.ConfigMap, workload *v1.Workload, rt *v1.ResourceTemplate,
	value string) {
	t.Helper()
	tmpl, err := jsonutils.ParseYamlToJson(cm.Data["template"])
	assert.NilError(t, err)
	labels := tmpl.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[v1.UserIdLabel] = value
	tmpl.SetLabels(labels)
	for _, path := range podTemplateLabelPaths(tmpl, workload, rt) {
		assert.NilError(t, unstructured.SetNestedField(tmpl.Object, value, append(path, v1.UserIdLabel)...))
	}
	out, err := json.Marshal(tmpl.Object)
	assert.NilError(t, err)
	cm.Data["template"] = string(out)
}

// podTemplateLabelPaths returns the metadata labels path of each Pod template of the resource
// template that obj has. A MonarchMesh has none: its Pods take the object's labels.
func podTemplateLabelPaths(obj *unstructured.Unstructured, workload *v1.Workload, rt *v1.ResourceTemplate) [][]string {
	if commonworkload.IsMonarchMesh(workload) {
		return nil
	}
	var paths [][]string
	for i, spec := range rt.Spec.ResourceSpecs {
		if i >= len(workload.Spec.Resources) {
			break
		}
		if len(spec.PrePaths) > 0 {
			if _, found, _ := unstructured.NestedFieldNoCopy(obj.Object, spec.PrePaths...); !found {
				continue
			}
		}
		paths = append(paths, append(append([]string{}, spec.TemplatePath()...), "metadata", "labels"))
	}
	return paths
}

// podLabelSets returns, keyed by where they are, the label maps of a rendered object that its
// Pods take their labels from: the object's own labels, and the metadata labels of each Pod
// template. It fails when a Pod template has no labels, so an absent label is never read as
// a missing template.
func podLabelSets(t *testing.T, obj *unstructured.Unstructured, workload *v1.Workload,
	rt *v1.ResourceTemplate) map[string]map[string]string {
	t.Helper()
	sets := map[string]map[string]string{"object": obj.GetLabels()}
	for _, path := range podTemplateLabelPaths(obj, workload, rt) {
		labels, found, err := unstructured.NestedStringMap(obj.Object, path...)
		assert.NilError(t, err)
		assert.Assert(t, found, "%s has no labels at %v", workload.SpecKind(), path)
		sets[strings.Join(path, ".")] = labels
	}
	return sets
}

func (c userIdLabelCase) String() string {
	if c.name != "" {
		return c.name
	}
	return c.kind
}

// assertUserIdLabel asserts that every label set has primus-safe.user.id, with the value want.
func assertUserIdLabel(t *testing.T, sets map[string]map[string]string, want string) {
	t.Helper()
	for where, labels := range sets {
		got, ok := labels[v1.UserIdLabel]
		assert.Assert(t, ok, "no %s at %s", v1.UserIdLabel, where)
		assert.Equal(t, got, want, "%s at %s", v1.UserIdLabel, where)
	}
}

// Every kind SaFE renders for an external workload carries the creator's id as
// primus-safe.user.id on the object and on every Pod template: the label the provider holds a
// hostPath below a per-user root to.
func TestExternalPodsCarryTheCreatorUserId(t *testing.T) {
	for _, c := range userIdLabelCases(t) {
		t.Run(c.String(), func(t *testing.T) {
			obj, workload, err := renderUserIdLabelCase(t, c, "", kubeSchedulerPlacement)
			assert.NilError(t, err)
			sets := podLabelSets(t, obj, workload, c.rt)
			if !commonworkload.IsMonarchMesh(workload) {
				assert.Assert(t, len(sets) >= 2, "%s rendered no Pod template: %v", c, sets)
			}
			assertUserIdLabel(t, sets, creatorUserId)
		})
	}
}

// A template that carries primus-safe.user.id, on the object or on a Pod template, cannot name
// the user of an external Pod: SaFE writes the creator's id over it. The same template rendered
// for Pods on the cluster's own nodes keeps the value, which shows the forged label does reach
// the rendered object and is overwritten rather than lost.
func TestExternalPodsOverwriteATemplateUserId(t *testing.T) {
	for _, c := range userIdLabelCases(t) {
		t.Run(c.String(), func(t *testing.T) {
			obj, workload, err := renderUserIdLabelCase(t, c, forgedUserId, kubeSchedulerPlacement)
			assert.NilError(t, err)
			assertUserIdLabel(t, podLabelSets(t, obj, workload, c.rt), creatorUserId)

			obj, workload, err = renderUserIdLabelCase(t, c, forgedUserId, nil)
			assert.NilError(t, err)
			assertUserIdLabel(t, podLabelSets(t, obj, workload, c.rt), forgedUserId)
		})
	}
}

// Pods on the cluster's own nodes are rendered as before: SaFE adds no primus-safe.user.id.
func TestPodsOnOwnNodesGetNoUserIdLabel(t *testing.T) {
	for _, c := range userIdLabelCases(t) {
		t.Run(c.String(), func(t *testing.T) {
			obj, workload, err := renderUserIdLabelCase(t, c, "", nil)
			assert.NilError(t, err)
			for where, labels := range podLabelSets(t, obj, workload, c.rt) {
				_, ok := labels[v1.UserIdLabel]
				assert.Assert(t, !ok, "%s at %s: %v", v1.UserIdLabel, where, labels)
			}
		})
	}
}

// An external workload without a creator id renders primus-safe.user.id empty, which names no
// user, rather than leaving a template's value in place.
func TestExternalPodWithoutCreatorCarriesNoTemplateUserId(t *testing.T) {
	for _, c := range userIdLabelCases(t) {
		t.Run(c.String(), func(t *testing.T) {
			obj, workload, err := renderUserIdLabelCase(t, c, forgedUserId, func(w *v1.Workload) {
				kubeSchedulerPlacement(w)
				delete(w.Labels, v1.UserIdLabel)
			})
			assert.NilError(t, err)
			assertUserIdLabel(t, podLabelSets(t, obj, workload, c.rt), "")
		})
	}
}

// An Agent Sandbox rendered from the chart's Sandbox template for a grant on an external node
// carries the creator's id, and a primus-safe.user.id in that template's Pod template does not
// survive.
func TestExternalAgentSandboxFromChartTemplateCarriesTheCreatorUserId(t *testing.T) {
	c := userIdLabelCase{name: "AgentSandbox", kind: common.SandboxKind, template: renderChartSandboxTemplate(t),
		rt: jobutils.TestSandboxResourceTemplate, mutate: func(w *v1.Workload) {
			w.Spec.GroupVersionKind.Version = common.DefaultVersion
			w.Spec.JobPort = 0
			w.Spec.EntryPoints = nil
			w.Spec.Resources[0].RdmaResource = ""
			w.Spec.Env[sandboxAuthPublicKeyEnvName] = "test-public-key"
			v1.SetLabel(w, grantIDLabel, "gr-01")
		}}
	obj, workload, err := renderUserIdLabelCase(t, c, forgedUserId, kubeSchedulerPlacement)
	assert.NilError(t, err)
	sets := podLabelSets(t, obj, workload, c.rt)
	assert.Equal(t, len(sets), 2)
	assertUserIdLabel(t, sets, creatorUserId)
	// The sandbox runtime's own copy of the user stays as it was.
	assert.Equal(t, sets["spec.podTemplate.metadata.labels"]["runtime.agent-sandbox.io/user.id"], creatorUserId)
}

// An InferaDeployment's Pods take their labels from each role's podLabels, so that is where the
// creator's id goes, over whatever the template put there; a role on the cluster's own nodes
// keeps its podLabels.
func TestNormalizeInferaIDEPStampsTheCreatorUserId(t *testing.T) {
	for _, external := range []bool{true, false} {
		workload := newInferaWorkload([]string{common.DynamoRoleFrontend, common.DynamoRolePrefill}, nil, []int{1, 1})
		v1.SetLabel(workload, v1.UserIdLabel, creatorUserId)
		if external {
			kubeSchedulerPlacement(workload)
		}
		obj := inferaObject(nil, nil)
		for _, role := range []string{"role0", "role1"} {
			assert.NilError(t, unstructured.SetNestedField(obj.Object, forgedUserId,
				"spec", "services", role, "podLabels", v1.UserIdLabel))
		}
		assert.NilError(t, normalizeInferaIDEP(obj, workload))
		want := forgedUserId
		if external {
			want = creatorUserId
		}
		for _, role := range []string{"role0", "role1"} {
			got, found, err := unstructured.NestedString(obj.Object, "spec", "services", role, "podLabels", v1.UserIdLabel)
			assert.NilError(t, err)
			assert.Assert(t, found, "external=%v %s", external, role)
			assert.Equal(t, got, want, "external=%v %s", external, role)
		}
	}
}

// A DynamoGraphDeployment's Pods take their labels from each service's labels, so that is where
// the creator's id goes, over whatever the template put there.
func TestNormalizeDynamoDGDStampsTheCreatorUserId(t *testing.T) {
	for _, external := range []bool{true, false} {
		workload := buildDynamoTestWorkload("frontend,worker", nil)
		v1.SetLabel(workload, v1.UserIdLabel, creatorUserId)
		if external {
			kubeSchedulerPlacement(workload)
		}
		obj := buildFakeDGD(2)
		for _, role := range []string{"role0", "role1"} {
			assert.NilError(t, unstructured.SetNestedField(obj.Object, forgedUserId,
				"spec", "services", role, "labels", v1.UserIdLabel))
		}
		assert.NilError(t, normalizeDynamoDGD(obj, workload))
		want := forgedUserId
		if external {
			want = creatorUserId
		}
		for _, role := range []string{"role0", "role1"} {
			got, found, err := unstructured.NestedString(obj.Object, "spec", "services", role, "labels", v1.UserIdLabel)
			assert.NilError(t, err)
			assert.Assert(t, found, "external=%v %s", external, role)
			assert.Equal(t, got, want, "external=%v %s", external, role)
		}
	}
}
