/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestClientSets() *ClusterClientSets {
	return &ClusterClientSets{
		name:              "c1",
		resourceInformers: commonutils.NewObjectManager(),
	}
}

// syncerTestCert returns a base64-encoded self-signed cert/key pair accepted by tls.X509KeyPair,
// which lets data-plane client construction run without a real apiserver.
func syncerTestCert(t *testing.T) (certData, keyData string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	assert.NilError(t, err)
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "safe-unit-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	assert.NilError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	assert.NilError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return base64.StdEncoding.EncodeToString(certPEM), base64.StdEncoding.EncodeToString(keyPEM)
}

// newProbeAPIServer starts a TLS server that answers the ServerVersion reachability probe.
func newProbeAPIServer(t *testing.T) int32 {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"major":"1","minor":"30"}`))
	}))
	t.Cleanup(srv.Close)
	addr, ok := srv.Listener.Addr().(*net.TCPAddr)
	assert.Assert(t, ok)
	return int32(addr.Port)
}

// newReadyClusterEnv builds a ready cluster fronted by an admin Service whose ClusterIP points at
// a local probe server, so Service-mode factories can be created and refreshed in tests.
func newReadyClusterEnv(t *testing.T) (*v1.Cluster, ctrlclient.Client) {
	t.Helper()
	certData, keyData := syncerTestCert(t)
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Status: v1.ClusterStatus{ControlPlaneStatus: v1.ControlPlaneStatus{
			Phase:    v1.ReadyPhase,
			CertData: certData,
			KeyData:  keyData,
		}},
	}
	mockScheme := scheme.Scheme
	assert.NilError(t, corev1.AddToScheme(mockScheme))
	assert.NilError(t, v1.AddToScheme(mockScheme))
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Spec: corev1.ServiceSpec{
			ClusterIP: "127.0.0.1",
			Ports:     []corev1.ServicePort{{Port: newProbeAPIServer(t)}},
		},
	}
	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}},
		}},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(mockScheme).
		WithObjects(cluster, service, endpoints).Build()
	return cluster, cl
}

func TestNewClusterClientSets(t *testing.T) {
	cluster, cl := newReadyClusterEnv(t)
	cs, err := newClusterClientSets(context.Background(), cluster, cl, func(*resourceMessage) {})
	assert.NilError(t, err)
	assert.Equal(t, cs.name, "c1")
	assert.Equal(t, cs.dataClientFactory.BackendFingerprint(), "10.0.0.1")
	assert.NilError(t, cs.Release())
}

func TestNewClusterClientSetsWithoutEndpoint(t *testing.T) {
	mockScheme := scheme.Scheme
	assert.NilError(t, v1.AddToScheme(mockScheme))
	cl := ctrlfake.NewClientBuilder().WithScheme(mockScheme).Build()
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Status:     v1.ClusterStatus{ControlPlaneStatus: v1.ControlPlaneStatus{Phase: v1.ReadyPhase}},
	}
	_, err := newClusterClientSets(context.Background(), cluster, cl, func(*resourceMessage) {})
	assert.Assert(t, err != nil)
}

func TestRecreateClientFactory(t *testing.T) {
	cluster, cl := newReadyClusterEnv(t)
	cs := newTestClientSets()
	previous := commonclient.NewClientFactoryForTest("c1", "10.96.9.9:6443")
	cs.dataClientFactory = previous

	assert.NilError(t, cs.recreateClientFactory(context.Background(), cluster, cl))
	assert.Assert(t, cs.dataClientFactory != previous)
	// Informers must be dropped so they are rebuilt against the new client.
	assert.Equal(t, cs.informerCount(), 0)
	assert.NilError(t, cs.dataClientFactory.Release())
}

func TestEnsureClusterClientSetsNotReady(t *testing.T) {
	r := &SyncerReconciler{clusterClientSets: commonutils.NewObjectManager()}
	retry := r.ensureClusterClientSets(context.Background(),
		&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}, &v1.ResourceTemplateList{})
	assert.Assert(t, !retry)
	assert.Equal(t, r.clusterClientSets.Len(), 0)
}

func TestEnsureClusterClientSetsCreatesThenRefreshes(t *testing.T) {
	cluster, cl := newReadyClusterEnv(t)
	ctx := context.Background()
	r := &SyncerReconciler{ctx: ctx, Client: cl, clusterClientSets: commonutils.NewObjectManager()}
	rtList := &v1.ResourceTemplateList{}

	assert.Assert(t, !r.ensureClusterClientSets(ctx, cluster, rtList))
	clientSets, err := GetClusterClientSets(r.clusterClientSets, "c1")
	assert.NilError(t, err)
	first := clientSets.dataClientFactory

	// A healthy factory is reused instead of rebuilt.
	assert.Assert(t, !r.ensureClusterClientSets(ctx, cluster, rtList))
	assert.Assert(t, clientSets.dataClientFactory == first)

	// An invalidated factory is rebuilt because the probe target is reachable.
	first.SetValid(false, "watch error")
	assert.Assert(t, !r.ensureClusterClientSets(ctx, cluster, rtList))
	assert.Assert(t, clientSets.dataClientFactory != first)
	assert.NilError(t, clientSets.dataClientFactory.Release())
}

func TestEnsureAllClusterClientSets(t *testing.T) {
	_, cl := newReadyClusterEnv(t)
	ctx := context.Background()
	r := &SyncerReconciler{ctx: ctx, Client: cl, clusterClientSets: commonutils.NewObjectManager()}

	r.ensureAllClusterClientSets(ctx)
	clientSets, err := GetClusterClientSets(r.clusterClientSets, "c1")
	assert.NilError(t, err)
	assert.NilError(t, clientSets.dataClientFactory.Release())
}

func TestClusterClientSetsGettersSetters(t *testing.T) {
	c := newTestClientSets()
	c.SetName("c2")
	assert.Equal(t, c.name, "c2")

	// ClientFactory getter returns whatever was set (nil here).
	c.SetClientFactory(nil)
	assert.Assert(t, c.ClientFactory() == nil)
}

func TestClusterClientSetsGetResourceInformerMissing(t *testing.T) {
	c := newTestClientSets()
	gvk := schema.GroupVersionKind{Group: "g", Version: "v", Kind: "Pod"}

	// Internal getter returns nil when not present.
	assert.Assert(t, c.getResourceInformer(gvk) == nil)

	// Public getter returns an error when not present.
	_, err := c.GetResourceInformer(context.Background(), gvk)
	assert.Assert(t, err != nil)
}

func TestClusterClientSetsReleaseAndDelTemplate(t *testing.T) {
	c := newTestClientSets()
	gvk := schema.GroupVersionKind{Group: "g", Version: "v", Kind: "Pod"}
	// delResourceTemplate on an empty manager is a no-op (logs only).
	c.delResourceTemplate(gvk)
	// Release clears informers without error.
	assert.NilError(t, c.Release())
}

func TestGetClusterClientSets(t *testing.T) {
	mgr := commonutils.NewObjectManager()

	// Missing entry -> error.
	_, err := GetClusterClientSets(mgr, "missing")
	assert.Assert(t, err != nil)

	// Present entry -> returned.
	c := newTestClientSets()
	assert.NilError(t, mgr.Add("c1", c))
	got, err := GetClusterClientSets(mgr, "c1")
	assert.NilError(t, err)
	assert.Equal(t, got.name, "c1")
}

func TestClusterClientSetsNeedsInformerRetry(t *testing.T) {
	c := newTestClientSets()
	rtList := &v1.ResourceTemplateList{
		Items: []v1.ResourceTemplate{
			{Spec: v1.ResourceTemplateSpec{
				GroupVersionKind: v1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			}},
		},
	}
	assert.Equal(t, c.needsInformerRetry(rtList), true)
}

func TestClusterClientSetsInformerCount(t *testing.T) {
	c := newTestClientSets()
	assert.Equal(t, c.informerCount(), 0)
}

func TestHandleResourceWrongType(t *testing.T) {
	called := false
	c := &ClusterClientSets{
		name:    "cl",
		handler: ResourceHandler(func(m *resourceMessage) { called = true }),
	}
	// Not an *unstructured.Unstructured -> ignored.
	c.handleResource(context.Background(), nil, &corev1.Pod{}, ResourceAdd)
	assert.Equal(t, called, false)
}

func TestHandleResourceNoWorkloadId(t *testing.T) {
	called := false
	c := &ClusterClientSets{
		name:    "cl",
		handler: ResourceHandler(func(m *resourceMessage) { called = true }),
	}
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetKind("Pod")
	u.SetName("p1")
	// No workload-id label and no mesh label -> not a managed object, ignored.
	c.handleResource(context.Background(), nil, u, ResourceAdd)
	assert.Equal(t, called, false)
}

func TestToUnstructuredTombstone(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetKind("Sandbox")
	u.SetName("sb")
	got, ok := toUnstructured(cache.DeletedFinalStateUnknown{Obj: u})
	assert.Assert(t, ok)
	assert.Equal(t, got.GetName(), "sb")
}

func TestHandleResourceDeleteTombstone(t *testing.T) {
	var captured *resourceMessage
	c := &ClusterClientSets{
		name:    "cl",
		handler: ResourceHandler(func(m *resourceMessage) { captured = m }),
	}
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetKind("Sandbox")
	u.SetName("sb")
	u.SetNamespace("ws")
	u.SetLabels(map[string]string{v1.WorkloadIdLabel: "w"})
	tombstone := cache.DeletedFinalStateUnknown{Obj: u}
	c.handleResource(context.Background(), tombstone, tombstone, ResourceDel)
	assert.Assert(t, captured != nil)
	assert.Equal(t, captured.action, ResourceDel)
	assert.Equal(t, captured.workloadId, "w")
	assert.Equal(t, captured.name, "sb")
}

func TestHandleResourceMeshPodDelete(t *testing.T) {
	var captured *resourceMessage
	c := &ClusterClientSets{
		name:    "cl",
		handler: ResourceHandler(func(m *resourceMessage) { captured = m }),
	}
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetKind("Pod")
	u.SetName("mj-mesh-0-worker-0")
	u.SetNamespace("ws")
	u.SetLabels(map[string]string{monarchMeshLabel: "mj-mesh-0"})
	c.handleResource(context.Background(), nil, u, ResourceDel)
	assert.Assert(t, captured != nil)
	assert.Equal(t, captured.workloadId, "")
	assert.Equal(t, captured.meshName, "mj-mesh-0")
	assert.Equal(t, captured.action, ResourceDel)
}

func TestHandleResourceMeshEventCachesWorkloadForPodDelete(t *testing.T) {
	var captured *resourceMessage
	c := &ClusterClientSets{
		name:    "cl",
		handler: ResourceHandler(func(m *resourceMessage) { captured = m }),
	}
	mesh := &unstructured.Unstructured{Object: map[string]interface{}{}}
	mesh.SetKind(common.MonarchMesh)
	mesh.SetName("mj-mesh-0")
	mesh.SetNamespace("ws")
	mesh.SetLabels(map[string]string{v1.WorkloadIdLabel: "w"})
	c.handleResource(context.Background(), nil, mesh, ResourceDel)

	pod := &unstructured.Unstructured{Object: map[string]interface{}{}}
	pod.SetKind("Pod")
	pod.SetName("mj-mesh-0-worker-0")
	pod.SetNamespace("ws")
	pod.SetLabels(map[string]string{monarchMeshLabel: "mj-mesh-0"})
	c.handleResource(context.Background(), nil, pod, ResourceDel)
	assert.Assert(t, captured != nil)
	assert.Equal(t, captured.meshName, "mj-mesh-0")
	assert.Equal(t, captured.workloadId, "w")
}

func TestHandleResourceManaged(t *testing.T) {
	var captured *resourceMessage
	c := &ClusterClientSets{
		name:    "cl",
		handler: ResourceHandler(func(m *resourceMessage) { captured = m }),
	}
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetKind("Job")
	u.SetName("obj")
	u.SetNamespace("ns")
	u.SetLabels(map[string]string{
		v1.WorkloadIdLabel:          "w",
		v1.WorkloadDispatchCntLabel: "3",
	})
	c.handleResource(context.Background(), nil, u, ResourceAdd)
	assert.Assert(t, captured != nil)
	assert.Equal(t, captured.workloadId, "w")
	assert.Equal(t, captured.dispatchCount, 3)
	assert.Equal(t, captured.cluster, "cl")
}

func TestNeedsClientFactoryRefreshInvalid(t *testing.T) {
	cs := newTestClientSets()
	factory := commonclient.NewClientFactoryForTest("c1", "https://10.0.0.1:6443")
	factory.SetValid(false, "watch error")
	cs.dataClientFactory = factory
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Status: v1.ClusterStatus{
			ControlPlaneStatus: v1.ControlPlaneStatus{Phase: v1.ReadyPhase},
		},
	}
	assert.Assert(t, cs.needsClientFactoryRefresh(context.Background(), cluster, nil))
}

func TestNeedsClientFactoryRefreshBackendIPsChanged(t *testing.T) {
	ctx := context.Background()
	mockScheme := scheme.Scheme
	_ = corev1.AddToScheme(mockScheme)
	_ = v1.AddToScheme(mockScheme)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Spec:       corev1.ServiceSpec{ClusterIP: "10.96.1.1", Ports: []corev1.ServicePort{{Port: 6443}}},
	}
	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}},
		}},
	}
	adminClient := ctrlfake.NewClientBuilder().WithScheme(mockScheme).WithObjects(service, endpoints).Build()
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Status:     v1.ClusterStatus{ControlPlaneStatus: v1.ControlPlaneStatus{Phase: v1.ReadyPhase}},
	}
	cs := newTestClientSets()
	factory := commonclient.NewClientFactoryForTest("c1", "10.96.1.1:6443")
	factory.SetBackendFingerprint("10.0.0.1,10.0.0.2")
	cs.dataClientFactory = factory
	assert.Assert(t, cs.needsClientFactoryRefresh(ctx, cluster, adminClient))
}

func TestSyncGithubAnnotationsWrongKind(t *testing.T) {
	c := &ClusterClientSets{}
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetKind("Pod")
	// Wrong kind -> no-op, no panic (adminClient unused).
	c.syncGithubAnnotations(u)
}

func TestSyncGithubAnnotationsNoWorkloadId(t *testing.T) {
	c := &ClusterClientSets{}
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetKind(common.CICDEphemeralRunnerKind)
	// No workload id label -> no-op.
	c.syncGithubAnnotations(u)
}

func TestSyncGithubAnnotationsUpdates(t *testing.T) {
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:      "wl-1",
		Namespace: common.PrimusSafeNamespace,
	}}
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(wl).Build()
	c := &ClusterClientSets{adminClient: cl}

	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"workflowRunId":     int64(42),
			"jobRepositoryName": "org/repo",
		},
	}}
	u.SetKind(common.CICDEphemeralRunnerKind)
	u.SetLabels(map[string]string{v1.WorkloadIdLabel: "wl-1"})

	c.syncGithubAnnotations(u)

	got := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(),
		ctrlclient.ObjectKey{Name: "wl-1", Namespace: common.PrimusSafeNamespace}, got))
	assert.Equal(t, got.GetAnnotations()["actions.github.com/run-id"], "42")
	assert.Equal(t, got.GetAnnotations()["actions.github.com/repository"], "org/repo")
}

func TestSyncGithubAnnotationsWorkloadNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).Build()
	c := &ClusterClientSets{adminClient: cl}

	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{"jobId": int64(7)},
	}}
	u.SetKind(common.CICDEphemeralRunnerKind)
	u.SetLabels(map[string]string{v1.WorkloadIdLabel: "missing"})
	// Workload not found -> no-op, no panic.
	c.syncGithubAnnotations(u)
}
