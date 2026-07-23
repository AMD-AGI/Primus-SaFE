/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

const (
	testSlurmCluster   = "c1"
	testSlurmWorkspace = "ws1"
	testSlurmFlavor    = "f1"
)

// genSlurmWorkspace builds a workspace with the Slurm scope (and optional
// volumes) that createSlurmCluster requires.
func genSlurmWorkspace(name, cluster string, vols []v1.WorkspaceVolume) *v1.Workspace {
	return &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{v1.WorkspaceIdLabel: name},
		},
		Spec: v1.WorkspaceSpec{
			Cluster:    cluster,
			NodeFlavor: testSlurmFlavor,
			Scopes:     []v1.WorkspaceScope{v1.SlurmScope},
			Volumes:    vols,
		},
	}
}

// genSlurmAddonTemplate builds the helm AddonTemplate the handler looks up.
func genSlurmAddonTemplate() *v1.AddonTemplate {
	return &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: slurmChartTemplate},
		Spec: v1.AddonTemplateSpec{
			Type:    v1.AddonTemplateHelm,
			URL:     "oci://ghcr.io/slinkyproject/charts/slurm",
			Version: "1.2.0",
		},
	}
}

// genSlurmAddon builds an Addon representing an existing Slurm cluster with the
// given persisted spec (labels + spec annotation) so get/patch/delete/stop can
// be exercised.
func genSlurmAddon(cluster, ws, name string, spec slurmSpec) *v1.Addon {
	specJSON, _ := json.Marshal(spec)
	release := slurmReleaseName(name)
	return &v1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: slurmAddonName(cluster, ws, name),
			Labels: map[string]string{
				v1.DisplayNameLabel: name,
				v1.WorkspaceIdLabel: ws,
				slurmClusterLabel:   v1.TrueStr,
			},
			Annotations: map[string]string{slurmSpecAnnotation: string(specJSON)},
		},
		Spec: v1.AddonSpec{
			Cluster: &corev1.ObjectReference{Name: cluster},
			AddonSource: v1.AddonSource{
				HelmRepository: &v1.HelmRepository{
					ReleaseName: release,
					Namespace:   ws,
				},
			},
		},
	}
}

// slurmHandlerWithDataplane builds a Handler backed by a control-plane fake
// client (seeded with the admin user/role plus objs) and an optional data-plane
// clientset keyed by cluster.
func slurmHandlerWithDataplane(cs kubernetes.Interface, cluster string, objs ...client.Object) (*Handler, *v1.User) {
	mockUser := genMockUser()
	mockRole := genMockRole()
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	all := append([]client.Object{mockUser, mockRole}, objs...)
	ctrlClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(all...).Build()

	om := commonutils.NewObjectManager()
	if cs != nil {
		_ = om.Add(cluster, k8sclient.NewClientFactoryWithOnlyClient(context.Background(), cluster, cs))
	}
	return &Handler{
		Client:           ctrlClient,
		accessController: authority.NewAccessController(ctrlClient),
		clientManager:    om,
	}, mockUser
}

// slurmTestCtx builds a gin test context with the common cluster/user set.
func slurmTestCtx(method, target string, body []byte, userId string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	c.Request = httptest.NewRequest(method, target, r)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, userId)
	c.Set(common.Name, testSlurmCluster)
	c.Params = params
	return c, rsp
}

func slurmNameParam(name string) gin.Params {
	return gin.Params{{Key: common.SlurmClusterName, Value: name}}
}

// ---------------------------------------------------------------------------
// Group A: pure / deterministic functions
// ---------------------------------------------------------------------------

func mustUnmarshalValues(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	m := map[string]interface{}{}
	assert.NoError(t, yaml.Unmarshal([]byte(raw), &m))
	return m
}

func TestRenderSlurmValuesBasic(t *testing.T) {
	spec := slurmSpec{
		Pools: []view.NodePool{
			{Name: "pool1", Nodes: 2, GPU: 8, CPU: "8", Memory: "32Gi"},
			{Name: "pool2", Nodes: 1},
		},
	}
	out, err := renderSlurmValues(spec, "slurm-c1")
	assert.NoError(t, err)
	m := mustUnmarshalValues(t, out)

	nodesets := m["nodesets"].(map[string]interface{})
	pool1 := nodesets["pool1"].(map[string]interface{})
	assert.Equal(t, true, pool1["enabled"])
	assert.EqualValues(t, 2, pool1["replicas"])
	slurmd := pool1["slurmd"].(map[string]interface{})
	limits := slurmd["resources"].(map[string]interface{})["limits"].(map[string]interface{})
	assert.Equal(t, "8", limits["cpu"])
	assert.Equal(t, "32Gi", limits["memory"])
	assert.EqualValues(t, 8, limits[amdGPUResourceName])

	// pool2 has no resource limits.
	pool2 := nodesets["pool2"].(map[string]interface{})
	_, hasSlurmd := pool2["slurmd"]
	assert.False(t, hasSlurmd)

	// First partition is the default; the second is not.
	partitions := m["partitions"].(map[string]interface{})
	p1cfg := partitions["pool1"].(map[string]interface{})["configMap"].(map[string]interface{})
	assert.Equal(t, "YES", p1cfg["Default"])
	p2cfg := partitions["pool2"].(map[string]interface{})["configMap"].(map[string]interface{})
	_, hasDefault := p2cfg["Default"]
	assert.False(t, hasDefault)

	// Not stopped: restapi + login both at one replica.
	assert.EqualValues(t, 1, m["restapi"].(map[string]interface{})["replicas"])
	login := m["loginsets"].(map[string]interface{})["slinky"].(map[string]interface{})
	assert.EqualValues(t, 1, login["replicas"])

	// Accounting disabled by default.
	assert.Equal(t, false, m["accounting"].(map[string]interface{})["enabled"])
}

func TestRenderSlurmValuesStopped(t *testing.T) {
	spec := slurmSpec{
		Pools:   []view.NodePool{{Name: "pool1", Nodes: 3}},
		Stopped: true,
	}
	out, err := renderSlurmValues(spec, "slurm-c1")
	assert.NoError(t, err)
	m := mustUnmarshalValues(t, out)

	pool1 := m["nodesets"].(map[string]interface{})["pool1"].(map[string]interface{})
	assert.EqualValues(t, 0, pool1["replicas"])
	login := m["loginsets"].(map[string]interface{})["slinky"].(map[string]interface{})
	assert.EqualValues(t, 0, login["replicas"])
	// restapi is kept up during phase 1 (RestapiDown false).
	assert.EqualValues(t, 1, m["restapi"].(map[string]interface{})["replicas"])
}

func TestRenderSlurmValuesRestapiDown(t *testing.T) {
	spec := slurmSpec{
		Pools:       []view.NodePool{{Name: "pool1", Nodes: 3}},
		Stopped:     true,
		RestapiDown: true,
	}
	out, err := renderSlurmValues(spec, "slurm-c1")
	assert.NoError(t, err)
	m := mustUnmarshalValues(t, out)
	assert.EqualValues(t, 0, m["restapi"].(map[string]interface{})["replicas"])
}

func TestRenderSlurmValuesAccounting(t *testing.T) {
	spec := slurmSpec{
		Pools:             []view.NodePool{{Name: "pool1", Nodes: 1}},
		AccountingEnabled: true,
	}
	out, err := renderSlurmValues(spec, "slurm-c1")
	assert.NoError(t, err)
	m := mustUnmarshalValues(t, out)

	acct := m["accounting"].(map[string]interface{})
	assert.Equal(t, true, acct["enabled"])
	storage := acct["storageConfig"].(map[string]interface{})
	assert.Equal(t, mariadbServiceName("slurm-c1"), storage["host"])
	ref := storage["passwordKeyRef"].(map[string]interface{})
	assert.Equal(t, mariadbSecretName("slurm-c1"), ref["name"])
	assert.Equal(t, mariadbPasswordKey, ref["key"])
}

func TestRenderSlurmValuesImageTag(t *testing.T) {
	spec := slurmSpec{
		Pools:    []view.NodePool{{Name: "pool1", Nodes: 1}},
		ImageTag: "24.11-custom",
	}
	out, err := renderSlurmValues(spec, "slurm-c1")
	assert.NoError(t, err)
	m := mustUnmarshalValues(t, out)

	ctrlTag := m["controller"].(map[string]interface{})["slurmctld"].(map[string]interface{})["image"].(map[string]interface{})["tag"]
	assert.Equal(t, "24.11-custom", ctrlTag)
	restTag := m["restapi"].(map[string]interface{})["slurmrestd"].(map[string]interface{})["image"].(map[string]interface{})["tag"]
	assert.Equal(t, "24.11-custom", restTag)
	pool1 := m["nodesets"].(map[string]interface{})["pool1"].(map[string]interface{})
	slurmdTag := pool1["slurmd"].(map[string]interface{})["image"].(map[string]interface{})["tag"]
	assert.Equal(t, "24.11-custom", slurmdTag)
}

func TestRenderSlurmValuesVolumes(t *testing.T) {
	spec := slurmSpec{
		Pools: []view.NodePool{{Name: "pool1", Nodes: 1}},
		Volumes: []v1.WorkspaceVolume{
			{Id: 1, Type: v1.HOSTPATH, HostPath: "/shared", MountPath: "/shared"},
		},
	}
	out, err := renderSlurmValues(spec, "slurm-c1")
	assert.NoError(t, err)
	m := mustUnmarshalValues(t, out)

	login := m["loginsets"].(map[string]interface{})["slinky"].(map[string]interface{})
	loginVols := login["podSpec"].(map[string]interface{})["volumes"].([]interface{})
	assert.Len(t, loginVols, 1)
	loginMounts := login["login"].(map[string]interface{})["volumeMounts"].([]interface{})
	assert.Len(t, loginMounts, 1)

	defaults := m["nodesetDefaults"].(map[string]interface{})
	workerVols := defaults["podSpec"].(map[string]interface{})["volumes"].([]interface{})
	assert.Len(t, workerVols, 1)
	workerMounts := defaults["slurmd"].(map[string]interface{})["volumeMounts"].([]interface{})
	assert.Len(t, workerMounts, 1)
}

func TestSlurmWorkspaceVolumes(t *testing.T) {
	vols := []v1.WorkspaceVolume{
		{Id: 1, Type: v1.HOSTPATH, HostPath: "/data", MountPath: "/data", SubPath: "sub", AccessMode: corev1.ReadOnlyMany},
		{Id: 2, Type: v1.PFS, MountPath: "/pfs"},
		{Id: 3, Type: v1.HOSTPATH, HostPath: "/nomount"}, // no MountPath -> volume only
	}
	volumes, mounts := slurmWorkspaceVolumes(vols)
	assert.Len(t, volumes, 3)
	assert.Len(t, mounts, 2)

	// HostPath volume.
	hp := volumes[0].(map[string]interface{})
	assert.Equal(t, v1.GenFullVolumeId(v1.HOSTPATH, 1), hp["name"])
	assert.Equal(t, "/data", hp["hostPath"].(map[string]interface{})["path"])
	assert.Equal(t, "DirectoryOrCreate", hp["hostPath"].(map[string]interface{})["type"])

	hpMount := mounts[0].(map[string]interface{})
	assert.Equal(t, "/data", hpMount["mountPath"])
	assert.Equal(t, true, hpMount["readOnly"])
	assert.Equal(t, "sub", hpMount["subPath"])

	// PFS volume references the pfs-<id> PVC.
	pfs := volumes[1].(map[string]interface{})
	assert.Equal(t, v1.GenFullVolumeId(v1.PFS, 2), pfs["persistentVolumeClaim"].(map[string]interface{})["claimName"])
	pfsMount := mounts[1].(map[string]interface{})
	assert.Equal(t, false, pfsMount["readOnly"])
	_, hasSubPath := pfsMount["subPath"]
	assert.False(t, hasSubPath)

	// Empty input.
	v2, m2 := slurmWorkspaceVolumes(nil)
	assert.Nil(t, v2)
	assert.Nil(t, m2)
}

func TestValidateSlurmClusterName(t *testing.T) {
	assert.NoError(t, validateSlurmClusterName("short", "ws1"))
	// A long name pushes the derived "<ws>_slurm-<name>" past the 40-char cap.
	err := validateSlurmClusterName("this-is-a-very-long-slurm-cluster-name-indeed", "workspace-abc")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too long")
}

func TestValidateNodePools(t *testing.T) {
	assert.NoError(t, validateNodePools([]view.NodePool{{Name: "a", Nodes: 1}, {Name: "b", Nodes: 0}}))

	assert.Error(t, validateNodePools([]view.NodePool{{Name: ""}}))
	assert.Error(t, validateNodePools([]view.NodePool{{Name: "a"}, {Name: "a"}}))
	assert.Error(t, validateNodePools([]view.NodePool{{Name: "a", Nodes: -1}}))
}

func TestSlurmNameHelpers(t *testing.T) {
	assert.Equal(t, "slurm-x", slurmReleaseName("x"))
	assert.Equal(t, "x", trimSlurmPrefix("slurm-x"))
	assert.Equal(t, "no-prefix", trimSlurmPrefix("no-prefix"))
	assert.Equal(t, "slurm-", trimSlurmPrefix("slurm-")) // too short to trim
	assert.Equal(t, genAddonName("c1", "ws1", slurmReleaseName("x")), slurmAddonName("c1", "ws1", "x"))
}

func TestMapAddonPhaseToSlurmPhase(t *testing.T) {
	cases := map[v1.AddonPhaseType]string{
		v1.AddonRunning:  "Deployed",
		v1.AddonDeployed: "Deployed",
		v1.AddonFailed:   "Failed",
		v1.AddonError:    "Failed",
		v1.AddonDeleting: "Deleting",
		"":               "Pending",
		v1.AddonPhaseType("Something"): "Something",
	}
	for phase, want := range cases {
		addon := &v1.Addon{Status: v1.AddonStatus{Phase: phase}}
		assert.Equal(t, want, mapAddonPhaseToSlurmPhase(addon))
	}
}

func TestReadSlurmSpec(t *testing.T) {
	spec := slurmSpec{AccountingEnabled: true, Pools: []view.NodePool{{Name: "p", Nodes: 2}}}
	addon := genSlurmAddon(testSlurmCluster, testSlurmWorkspace, "x", spec)
	got := readSlurmSpec(addon)
	assert.True(t, got.AccountingEnabled)
	assert.Len(t, got.Pools, 1)

	// Missing annotation -> zero value.
	empty := readSlurmSpec(&v1.Addon{})
	assert.False(t, empty.AccountingEnabled)
	assert.Nil(t, empty.Pools)
}

func TestSlurmPodRole(t *testing.T) {
	release := "slurm-c1"
	// Explicit component label wins.
	labeled := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{"app.kubernetes.io/component": "login"},
	}}
	assert.Equal(t, "login", slurmPodRole(labeled, release))

	byName := func(n string) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: n}}
	}
	assert.Equal(t, "controller", slurmPodRole(byName(release+"-controller-0"), release))
	assert.Equal(t, "login", slurmPodRole(byName(release+"-login-abc"), release))
	assert.Equal(t, "restapi", slurmPodRole(byName(release+"-restapi-abc"), release))
	assert.Equal(t, "accounting", slurmPodRole(byName(release+"-accounting-0"), release))
	assert.Equal(t, "accounting-db", slurmPodRole(byName(release+"-mariadb-0"), release))
	assert.Equal(t, "worker", slurmPodRole(byName(release+"-pool1-0"), release))
}

// ---------------------------------------------------------------------------
// Client-arg helpers
// ---------------------------------------------------------------------------

func TestListSlurmPods(t *testing.T) {
	ns := testSlurmWorkspace
	release := "slurm-c1"
	pods := []runtime.Object{
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name:      release + "-controller-0",
			Namespace: ns,
			Labels:    map[string]string{"app.kubernetes.io/instance": release + "-controller"},
		}, Spec: corev1.PodSpec{NodeName: "n1"}, Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "1.2.3.4"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name:      release + "-pool1-0",
			Namespace: ns,
			Labels:    map[string]string{"app.kubernetes.io/instance": release + "-pool1", "app.kubernetes.io/component": "worker"},
		}, Status: corev1.PodStatus{Phase: corev1.PodPending}},
		// Unrelated pod (different release) is skipped.
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name:      "other-thing",
			Namespace: ns,
			Labels:    map[string]string{"app.kubernetes.io/instance": "other"},
		}},
	}
	cs := k8sfake.NewSimpleClientset(pods...)
	out := listSlurmPods(context.Background(), cs, ns, release)
	assert.Len(t, out, 2)
	// Sorted by role: "controller" < "worker".
	assert.Equal(t, "controller", out[0].Role)
	assert.Equal(t, "1.2.3.4", out[0].PodIP)
	assert.Equal(t, "worker", out[1].Role)
}

func newSlurmDynamicClient() *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		slurmNodeSetGVR: "NodeSetList",
		statefulSetGVR:  "StatefulSetList",
	})
}

func TestReadNodeSetStatus(t *testing.T) {
	ctx := context.Background()
	ns := testSlurmWorkspace
	release := "slurm-c1"
	dc := newSlurmDynamicClient()

	mkNodeSet := func(name string, ready, desired int64) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "slinky.slurm.net/v1beta1",
			"kind":       "NodeSet",
			"metadata":   map[string]interface{}{"name": name, "namespace": ns},
			"status":     map[string]interface{}{"readyReplicas": ready, "desired": desired},
		}}
	}
	_, err := dc.Resource(slurmNodeSetGVR).Namespace(ns).Create(ctx, mkNodeSet(release+"-pool1", 2, 3), metav1.CreateOptions{})
	assert.NoError(t, err)
	// A NodeSet not belonging to the release is ignored.
	_, err = dc.Resource(slurmNodeSetGVR).Namespace(ns).Create(ctx, mkNodeSet("other-pool", 5, 5), metav1.CreateOptions{})
	assert.NoError(t, err)
	// A NodeSet with no status.desired falls back to spec.replicas.
	specOnly := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "slinky.slurm.net/v1beta1",
		"kind":       "NodeSet",
		"metadata":   map[string]interface{}{"name": release + "-pool2", "namespace": ns},
		"spec":       map[string]interface{}{"replicas": int64(4)},
		"status":     map[string]interface{}{"readyReplicas": int64(1)},
	}}
	_, err = dc.Resource(slurmNodeSetGVR).Namespace(ns).Create(ctx, specOnly, metav1.CreateOptions{})
	assert.NoError(t, err)

	ready, desired := readNodeSetStatus(ctx, dc, ns, release)
	assert.Equal(t, 3, ready)   // 2 + 1
	assert.Equal(t, 7, desired) // 3 (status.desired) + 4 (spec.replicas fallback)
}

func TestControllerReady(t *testing.T) {
	ctx := context.Background()
	ns := testSlurmWorkspace
	release := "slurm-c1"
	dc := newSlurmDynamicClient()

	sts := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "StatefulSet",
		"metadata":   map[string]interface{}{"name": release + "-controller", "namespace": ns},
		"status":     map[string]interface{}{"readyReplicas": int64(1)},
	}}
	_, err := dc.Resource(statefulSetGVR).Namespace(ns).Create(ctx, sts, metav1.CreateOptions{})
	assert.NoError(t, err)

	assert.True(t, controllerReady(ctx, dc, ns, release))
	// Missing controller -> not ready.
	assert.False(t, controllerReady(ctx, dc, ns, "slurm-absent"))
}

func TestCvtAddonToSlurmCluster(t *testing.T) {
	spec := slurmSpec{
		AccountingEnabled: true,
		Pools:             []view.NodePool{{Name: "pool1", Nodes: 2}},
		ImageTag:          "tag1",
	}
	addon := genSlurmAddon(testSlurmCluster, testSlurmWorkspace, "mycluster", spec)
	addon.Status.Phase = v1.AddonDeployed

	// With a nil dynamic client the live-status enrichment is skipped.
	h := &Handler{}
	item := h.cvtAddonToSlurmCluster(context.Background(), nil, addon)
	assert.Equal(t, "mycluster", item.Name)
	assert.Equal(t, testSlurmWorkspace, item.Workspace)
	assert.Equal(t, testSlurmCluster, item.Cluster)
	assert.Equal(t, "Deployed", item.Phase)
	assert.True(t, item.AccountingEnabled)
	assert.Equal(t, "tag1", item.ImageTag)
	assert.Equal(t, []string{"pool1"}, item.Partitions)

	// Stopped cluster with no ready workers reports "Stopped".
	stoppedSpec := slurmSpec{Stopped: true, Pools: []view.NodePool{{Name: "p", Nodes: 1}}}
	stoppedAddon := genSlurmAddon(testSlurmCluster, testSlurmWorkspace, "stopped", stoppedSpec)
	stoppedItem := h.cvtAddonToSlurmCluster(context.Background(), nil, stoppedAddon)
	assert.Equal(t, "Stopped", stoppedItem.Phase)
	assert.True(t, stoppedItem.Stopped)

	// With a live dynamic client: NodeSet status + a ready controller flip the
	// phase to "Running" and populate the node counts.
	ctx := context.Background()
	release := slurmReleaseName("mycluster")
	dc := newSlurmDynamicClient()
	nodeSet := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "slinky.slurm.net/v1beta1",
		"kind":       "NodeSet",
		"metadata":   map[string]interface{}{"name": release + "-pool1", "namespace": testSlurmWorkspace},
		"status":     map[string]interface{}{"readyReplicas": int64(2), "desired": int64(2)},
	}}
	_, err := dc.Resource(slurmNodeSetGVR).Namespace(testSlurmWorkspace).Create(ctx, nodeSet, metav1.CreateOptions{})
	assert.NoError(t, err)
	sts := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "StatefulSet",
		"metadata":   map[string]interface{}{"name": release + "-controller", "namespace": testSlurmWorkspace},
		"status":     map[string]interface{}{"readyReplicas": int64(1)},
	}}
	_, err = dc.Resource(statefulSetGVR).Namespace(testSlurmWorkspace).Create(ctx, sts, metav1.CreateOptions{})
	assert.NoError(t, err)

	liveItem := h.cvtAddonToSlurmCluster(ctx, dc, addon)
	assert.Equal(t, 2, liveItem.NodesReady)
	assert.Equal(t, 2, liveItem.NodesDesired)
	assert.Equal(t, "Running", liveItem.Phase)
}

// ---------------------------------------------------------------------------
// Group B: handlers
// ---------------------------------------------------------------------------

func TestCreateSlurmCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: testSlurmCluster}}
	ws := genSlurmWorkspace(testSlurmWorkspace, testSlurmCluster, nil)
	tmpl := genSlurmAddonTemplate()
	cs := k8sfake.NewSimpleClientset()
	h, user := slurmHandlerWithDataplane(cs, testSlurmCluster, cluster, ws, tmpl)

	body, _ := json.Marshal(view.CreateSlurmClusterRequest{
		WorkspaceId:       testSlurmWorkspace,
		Name:              "mycluster",
		AccountingEnabled: true,
		Pools:             []view.NodePool{{Name: "pool1", Nodes: 2, GPU: 8}},
	})
	c, _ := slurmTestCtx(http.MethodPost, "/", body, user.Name, nil)
	res, err := h.createSlurmCluster(c)
	assert.NoError(t, err)
	item := res.(view.SlurmClusterResponseItem)
	assert.Equal(t, "mycluster", item.Name)

	// The Addon was created on the control plane.
	addon, err := h.getAdminAddon(context.Background(), slurmAddonName(testSlurmCluster, testSlurmWorkspace, "mycluster"))
	assert.NoError(t, err)
	assert.NotNil(t, addon.Spec.AddonSource.HelmRepository)

	// Accounting provisioned the MariaDB secret on the data plane.
	_, err = cs.CoreV1().Secrets(testSlurmWorkspace).Get(context.Background(), mariadbSecretName(slurmReleaseName("mycluster")), metav1.GetOptions{})
	assert.NoError(t, err)
}

func TestCreateSlurmClusterValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: testSlurmCluster}}
	ws := genSlurmWorkspace(testSlurmWorkspace, testSlurmCluster, nil)
	noScopeWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-noscope"}, Spec: v1.WorkspaceSpec{Cluster: testSlurmCluster}}
	tmpl := genSlurmAddonTemplate()
	h, user := slurmHandlerWithDataplane(nil, testSlurmCluster, cluster, ws, noScopeWs, tmpl)

	// Missing name.
	body, _ := json.Marshal(view.CreateSlurmClusterRequest{WorkspaceId: testSlurmWorkspace, Pools: []view.NodePool{{Name: "p", Nodes: 1}}})
	c, _ := slurmTestCtx(http.MethodPost, "/", body, user.Name, nil)
	_, err := h.createSlurmCluster(c)
	assert.Error(t, err)

	// No pools.
	body, _ = json.Marshal(view.CreateSlurmClusterRequest{WorkspaceId: testSlurmWorkspace, Name: "x"})
	c, _ = slurmTestCtx(http.MethodPost, "/", body, user.Name, nil)
	_, err = h.createSlurmCluster(c)
	assert.Error(t, err)

	// Name too long.
	body, _ = json.Marshal(view.CreateSlurmClusterRequest{
		WorkspaceId: testSlurmWorkspace,
		Name:        "this-is-a-really-long-cluster-name-that-exceeds-the-limit",
		Pools:       []view.NodePool{{Name: "p", Nodes: 1}},
	})
	c, _ = slurmTestCtx(http.MethodPost, "/", body, user.Name, nil)
	_, err = h.createSlurmCluster(c)
	assert.Error(t, err)

	// Workspace without the Slurm scope.
	body, _ = json.Marshal(view.CreateSlurmClusterRequest{WorkspaceId: "ws-noscope", Name: "x", Pools: []view.NodePool{{Name: "p", Nodes: 1}}})
	c, _ = slurmTestCtx(http.MethodPost, "/", body, user.Name, nil)
	_, err = h.createSlurmCluster(c)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Slurm scope")
}

func TestPatchSlurmCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ws := genSlurmWorkspace(testSlurmWorkspace, testSlurmCluster, nil)
	spec := slurmSpec{AccountingEnabled: true, Pools: []view.NodePool{{Name: "pool1", Nodes: 1}}}
	addon := genSlurmAddon(testSlurmCluster, testSlurmWorkspace, "mycluster", spec)
	cs := k8sfake.NewSimpleClientset()
	h, user := slurmHandlerWithDataplane(cs, testSlurmCluster, ws, addon)

	// Change pools and disable accounting (exercises deleteMariaDB path).
	disabled := false
	body, _ := json.Marshal(view.PatchSlurmClusterRequest{
		Pools:             []view.NodePool{{Name: "pool1", Nodes: 4}, {Name: "pool2", Nodes: 1}},
		AccountingEnabled: &disabled,
	})
	c, _ := slurmTestCtx(http.MethodPatch, "/?workspaceId="+testSlurmWorkspace, body, user.Name, slurmNameParam("mycluster"))
	res, err := h.patchSlurmCluster(c)
	assert.NoError(t, err)
	item := res.(view.SlurmClusterResponseItem)
	assert.Len(t, item.Pools, 2)
	assert.False(t, item.AccountingEnabled)

	// The persisted spec was updated.
	updated, err := h.getAdminAddon(context.Background(), addon.Name)
	assert.NoError(t, err)
	got := readSlurmSpec(updated)
	assert.Equal(t, 4, got.Pools[0].Nodes)
}

func TestListAndGetSlurmCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ws := genSlurmWorkspace(testSlurmWorkspace, testSlurmCluster, nil)
	a := genSlurmAddon(testSlurmCluster, testSlurmWorkspace, "aaa", slurmSpec{Pools: []view.NodePool{{Name: "p", Nodes: 1}}})
	b := genSlurmAddon(testSlurmCluster, testSlurmWorkspace, "bbb", slurmSpec{Pools: []view.NodePool{{Name: "p", Nodes: 1}}})
	cs := k8sfake.NewSimpleClientset()
	h, user := slurmHandlerWithDataplane(cs, testSlurmCluster, ws, a, b)

	// List.
	c, _ := slurmTestCtx(http.MethodGet, "/?workspaceId="+testSlurmWorkspace, nil, user.Name, nil)
	res, err := h.listSlurmCluster(c)
	assert.NoError(t, err)
	list := res.(view.ListSlurmClusterResponse)
	assert.Equal(t, 2, list.TotalCount)
	assert.Equal(t, "aaa", list.Items[0].Name)

	// List requires workspaceId.
	c, _ = slurmTestCtx(http.MethodGet, "/", nil, user.Name, nil)
	_, err = h.listSlurmCluster(c)
	assert.Error(t, err)

	// Get one.
	c, _ = slurmTestCtx(http.MethodGet, "/?workspaceId="+testSlurmWorkspace, nil, user.Name, slurmNameParam("aaa"))
	res, err = h.getSlurmCluster(c)
	assert.NoError(t, err)
	item := res.(view.SlurmClusterResponseItem)
	assert.Equal(t, "aaa", item.Name)

	// Get requires a name.
	c, _ = slurmTestCtx(http.MethodGet, "/?workspaceId="+testSlurmWorkspace, nil, user.Name, nil)
	_, err = h.getSlurmCluster(c)
	assert.Error(t, err)
}

func TestDeleteSlurmCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ws := genSlurmWorkspace(testSlurmWorkspace, testSlurmCluster, nil)
	spec := slurmSpec{AccountingEnabled: true, Pools: []view.NodePool{{Name: "p", Nodes: 1}}}
	addon := genSlurmAddon(testSlurmCluster, testSlurmWorkspace, "mycluster", spec)
	release := slurmReleaseName("mycluster")

	// Seed data-plane leftovers that delete should clean up.
	statesave := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
		Name:      "statesave-" + release + "-controller-0",
		Namespace: testSlurmWorkspace,
		Labels: map[string]string{
			"app.kubernetes.io/instance": release,
			"app.kubernetes.io/name":     "slurmctld",
		},
	}}
	cs := k8sfake.NewSimpleClientset(statesave)
	h, user := slurmHandlerWithDataplane(cs, testSlurmCluster, ws, addon)

	c, _ := slurmTestCtx(http.MethodDelete, "/?workspaceId="+testSlurmWorkspace, nil, user.Name, slurmNameParam("mycluster"))
	_, err := h.deleteSlurmCluster(c)
	assert.NoError(t, err)

	// Addon removed.
	_, err = h.getAdminAddon(context.Background(), addon.Name)
	assert.Error(t, err)
	_ = statesave // the fake clientset's DeleteCollection is a no-op; the
	// statesave cleanup path is asserted directly in TestDeleteSlurmStatesave.
}

func TestStopResumeSlurmCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ws := genSlurmWorkspace(testSlurmWorkspace, testSlurmCluster, nil)
	addon := genSlurmAddon(testSlurmCluster, testSlurmWorkspace, "mycluster", slurmSpec{Pools: []view.NodePool{{Name: "p", Nodes: 2}}})
	cs := k8sfake.NewSimpleClientset()
	h, user := slurmHandlerWithDataplane(cs, testSlurmCluster, ws, addon)

	// Stop.
	c, _ := slurmTestCtx(http.MethodPost, "/?workspaceId="+testSlurmWorkspace, nil, user.Name, slurmNameParam("mycluster"))
	res, err := h.stopSlurmCluster(c)
	assert.NoError(t, err)
	assert.True(t, res.(view.SlurmClusterResponseItem).Stopped)

	updated, _ := h.getAdminAddon(context.Background(), addon.Name)
	assert.True(t, readSlurmSpec(updated).Stopped)

	// Resume.
	c, _ = slurmTestCtx(http.MethodPost, "/?workspaceId="+testSlurmWorkspace, nil, user.Name, slurmNameParam("mycluster"))
	res, err = h.resumeSlurmCluster(c)
	assert.NoError(t, err)
	assert.False(t, res.(view.SlurmClusterResponseItem).Stopped)
}

// TestSlurmPublicHandlers exercises the thin gin wrappers so their routing lines
// are covered too.
func TestSlurmPublicHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: testSlurmCluster}}
	ws := genSlurmWorkspace(testSlurmWorkspace, testSlurmCluster, nil)
	tmpl := genSlurmAddonTemplate()
	addon := genSlurmAddon(testSlurmCluster, testSlurmWorkspace, "aaa", slurmSpec{Pools: []view.NodePool{{Name: "p", Nodes: 1}}})
	cs := k8sfake.NewSimpleClientset()
	h, user := slurmHandlerWithDataplane(cs, testSlurmCluster, cluster, ws, tmpl, addon)

	// List.
	c, rsp := slurmTestCtx(http.MethodGet, "/?workspaceId="+testSlurmWorkspace, nil, user.Name, nil)
	h.ListSlurmCluster(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	// Get.
	c, rsp = slurmTestCtx(http.MethodGet, "/?workspaceId="+testSlurmWorkspace, nil, user.Name, slurmNameParam("aaa"))
	h.GetSlurmCluster(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	// Create.
	body, _ := json.Marshal(view.CreateSlurmClusterRequest{
		WorkspaceId: testSlurmWorkspace,
		Name:        "wrapped",
		Pools:       []view.NodePool{{Name: "p", Nodes: 1}},
	})
	c, rsp = slurmTestCtx(http.MethodPost, "/", body, user.Name, nil)
	h.CreateSlurmCluster(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	// Patch.
	body, _ = json.Marshal(view.PatchSlurmClusterRequest{Pools: []view.NodePool{{Name: "p", Nodes: 2}}})
	c, rsp = slurmTestCtx(http.MethodPatch, "/?workspaceId="+testSlurmWorkspace, body, user.Name, slurmNameParam("aaa"))
	h.PatchSlurmCluster(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	// Stop then Resume.
	c, rsp = slurmTestCtx(http.MethodPost, "/?workspaceId="+testSlurmWorkspace, nil, user.Name, slurmNameParam("aaa"))
	h.StopSlurmCluster(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	c, rsp = slurmTestCtx(http.MethodPost, "/?workspaceId="+testSlurmWorkspace, nil, user.Name, slurmNameParam("aaa"))
	h.ResumeSlurmCluster(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	// Delete.
	c, rsp = slurmTestCtx(http.MethodDelete, "/?workspaceId="+testSlurmWorkspace, nil, user.Name, slurmNameParam("aaa"))
	h.DeleteSlurmCluster(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestGetSlurmClusterLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ws := genSlurmWorkspace(testSlurmWorkspace, testSlurmCluster, nil)

	// SSH disabled (default): reports Enabled=false and returns early.
	viper.Set("ssh.enable", false)
	h, user := slurmHandlerWithDataplane(k8sfake.NewSimpleClientset(), testSlurmCluster, ws)
	c, _ := slurmTestCtx(http.MethodGet, "/?workspaceId="+testSlurmWorkspace, nil, user.Name, slurmNameParam("mycluster"))
	res, err := h.getSlurmClusterLogin(c)
	assert.NoError(t, err)
	assert.False(t, res.(*view.SlurmLoginResponse).Enabled)

	// SSH enabled with a running login pod: produces the ssh command.
	viper.Set("ssh.enable", true)
	viper.Set("ssh.server_port", 2222)
	viper.Set("global.domain", "amd.com")
	viper.Set("global.sub_domain", "test")
	defer func() {
		viper.Set("ssh.enable", false)
		viper.Set("global.domain", "")
		viper.Set("global.sub_domain", "")
	}()

	loginPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      slurmReleaseName("mycluster") + "-login-0",
			Namespace: testSlurmWorkspace,
			Labels:    map[string]string{"app.kubernetes.io/instance": slurmReleaseName("mycluster") + "-login-slinky"},
		},
		Spec:   corev1.PodSpec{Containers: []corev1.Container{{Name: "login"}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	cs := k8sfake.NewSimpleClientset(loginPod)
	h, user = slurmHandlerWithDataplane(cs, testSlurmCluster, ws)
	c, _ = slurmTestCtx(http.MethodGet, "/?workspaceId="+testSlurmWorkspace, nil, user.Name, slurmNameParam("mycluster"))
	res, err = h.getSlurmClusterLogin(c)
	assert.NoError(t, err)
	resp := res.(*view.SlurmLoginResponse)
	assert.True(t, resp.Enabled)
	assert.True(t, resp.Ready)
	assert.Contains(t, resp.SSHCommand, "login")
	assert.Equal(t, loginPod.Name, resp.PodName)

	// Public wrapper.
	c, rsp := slurmTestCtx(http.MethodGet, "/?workspaceId="+testSlurmWorkspace, nil, user.Name, slurmNameParam("mycluster"))
	h.GetSlurmClusterLogin(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}
