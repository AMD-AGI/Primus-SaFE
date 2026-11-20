/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package robustclient

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func readyCluster(name string, annotations map[string]string) *v1.Cluster {
	c := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: annotations}}
	c.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	return c
}

// A sync registers every ready cluster at its annotated endpoint or the
// in-cluster default, skips one not ready, and drops one whose CR is gone.
func TestSyncOnce(t *testing.T) {
	s := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(s))
	k8s := ctrlfake.NewClientBuilder().WithScheme(s).WithObjects(
		readyCluster("local", nil),
		readyCluster("remote", map[string]string{annotationRobustEndpoint: "http://10.0.0.1:32626"}),
		&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "pending"}},
	).Build()
	rc := NewClient(ClientConfig{Timeout: time.Second})
	rc.RegisterCluster("gone", "http://old")
	d := NewDiscovery(k8s, rc, time.Minute)
	d.managementCluster = func() string { return "local" }
	resolves := true
	d.lookupHost = func(_ context.Context, host string) ([]string, error) {
		if resolves {
			return []string{"192.168.0.10"}, nil
		}
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}

	d.syncOnce(context.Background())
	assert.Equal(t, robustAPIEndpoint, rc.ForCluster("local").BaseURL())
	assert.Equal(t, "http://10.0.0.1:32626", rc.ForCluster("remote").BaseURL())
	assert.Nil(t, rc.ForCluster("pending"))
	assert.Nil(t, rc.ForCluster("gone"))

	// robust-api goes away: the local cluster moves to robust-analyzer.
	resolves = false
	d.syncOnce(context.Background())
	assert.Equal(t, robustAnalyzerEndpoint, rc.ForCluster("local").BaseURL())
	assert.Equal(t, "http://10.0.0.1:32626", rc.ForCluster("remote").BaseURL())
}

func newTestDiscovery(t *testing.T, management string, objs ...client.Object) (*Discovery, *Client) {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(s))
	require.NoError(t, clientgoscheme.AddToScheme(s))
	k8s := ctrlfake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
	rc := NewClient(ClientConfig{Timeout: time.Second, ServiceToken: func() string { return "global" }})
	d := NewDiscovery(k8s, rc, time.Minute)
	d.managementCluster = func() string { return management }
	d.lookupHost = func(_ context.Context, _ string) ([]string, error) {
		return []string{"192.168.0.10"}, nil
	}
	return d, rc
}

// Only the management cluster may fall back to the in-cluster Robust; another
// cluster without the annotation gets no client, even one registered before.
func TestSyncOnceFallbackOnlyForTheManagementCluster(t *testing.T) {
	d, rc := newTestDiscovery(t, "mgmt", readyCluster("mgmt", nil), readyCluster("crusoe", nil))
	rc.RegisterCluster("crusoe", robustAPIEndpoint)

	d.syncOnce(context.Background())
	assert.Equal(t, robustAPIEndpoint, rc.ForCluster("mgmt").BaseURL())
	assert.Nil(t, rc.ForCluster("crusoe"))
	assert.True(t, d.noEndpointLogged["crusoe"])

	// Annotating it later registers it and clears the report.
	cluster := &v1.Cluster{}
	require.NoError(t, d.k8sClient.Get(context.Background(), client.ObjectKey{Name: "crusoe"}, cluster))
	cluster.Annotations = map[string]string{annotationRobustEndpoint: "http://crusoe:8085"}
	require.NoError(t, d.k8sClient.Update(context.Background(), cluster))
	d.syncOnce(context.Background())
	assert.Equal(t, "http://crusoe:8085", rc.ForCluster("crusoe").BaseURL())
	assert.False(t, d.noEndpointLogged["crusoe"])
}

func TestSyncOnceWithoutConfiguredManagementCluster(t *testing.T) {
	// A single Cluster CR keeps today's behaviour.
	d, rc := newTestDiscovery(t, "", readyCluster("only", nil))
	d.syncOnce(context.Background())
	require.NotNil(t, rc.ForCluster("only"))
	assert.Equal(t, robustAPIEndpoint, rc.ForCluster("only").BaseURL())

	// With several, the one labeled as the control plane cluster is the management cluster.
	mgmt := readyCluster("a", nil)
	mgmt.Labels = map[string]string{v1.ClusterControlPlaneLabel: ""}
	d, rc = newTestDiscovery(t, "", mgmt, readyCluster("b", nil),
		readyCluster("c", map[string]string{annotationRobustEndpoint: "http://c:8085"}))
	d.syncOnce(context.Background())
	require.NotNil(t, rc.ForCluster("a"))
	assert.Equal(t, robustAPIEndpoint, rc.ForCluster("a").BaseURL())
	assert.Nil(t, rc.ForCluster("b"))
	assert.Equal(t, "http://c:8085", rc.ForCluster("c").BaseURL())
	assert.False(t, d.unresolvedLogged)

	// Without any signal every cluster without the annotation falls back, as before, with a warning.
	d, rc = newTestDiscovery(t, "", readyCluster("a", nil), readyCluster("b", nil),
		readyCluster("c", map[string]string{annotationRobustEndpoint: "http://c:8085"}))
	d.syncOnce(context.Background())
	require.NotNil(t, rc.ForCluster("a"))
	assert.Equal(t, robustAPIEndpoint, rc.ForCluster("a").BaseURL())
	require.NotNil(t, rc.ForCluster("b"))
	assert.Equal(t, robustAPIEndpoint, rc.ForCluster("b").BaseURL())
	assert.Equal(t, "http://c:8085", rc.ForCluster("c").BaseURL())
	assert.True(t, d.unresolvedLogged)
}

func TestFallbackCluster(t *testing.T) {
	labeled := func(name string) v1.Cluster {
		c := readyCluster(name, nil)
		c.Labels = map[string]string{v1.ClusterControlPlaneLabel: ""}
		return *c
	}
	one := []v1.Cluster{*readyCluster("x", nil)}
	two := []v1.Cluster{*readyCluster("x", nil), *readyCluster("y", nil)}
	for _, tc := range []struct {
		name       string
		configured string
		clusters   []v1.Cluster
		local      string
		all        bool
	}{
		{"configured among several", "m", two, "m", false},
		{"configured with one", "m", one, "m", false},
		{"single cluster", "", one, "x", false},
		{"no cluster", "", nil, "", false},
		{"one labeled", "", []v1.Cluster{*readyCluster("x", nil), labeled("y")}, "y", false},
		{"none labeled", "", two, "", true},
		{"several labeled", "", []v1.Cluster{labeled("x"), labeled("y")}, "", true},
	} {
		local, all := fallbackCluster(tc.configured, tc.clusters)
		assert.Equal(t, tc.local, local, tc.name)
		assert.Equal(t, tc.all, all, tc.name)
	}
}

func tokenEcho(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(ServiceTokenHeader)
		_, _ = w.Write([]byte(`{"meta":{"code":2000},"data":{}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

// A cluster annotated with a token Secret sends that Secret's token, re-read
// after rotation; a cluster without the annotation sends the global token.
func TestSyncOnceSelectsTheServiceToken(t *testing.T) {
	srv, got := tokenEcho(t)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "primus-safe", Name: "crusoe-token"},
		Data:       map[string][]byte{"service-token": []byte("crusoe-tok\n")},
	}
	d, rc := newTestDiscovery(t, "mgmt",
		readyCluster("mgmt", map[string]string{annotationRobustEndpoint: srv.URL}),
		readyCluster("crusoe", map[string]string{
			annotationRobustEndpoint:    srv.URL,
			annotationRobustTokenSecret: "primus-safe/crusoe-token",
		}),
		readyCluster("broken", map[string]string{
			annotationRobustEndpoint:    srv.URL,
			annotationRobustTokenSecret: "no-slash",
		}),
		secret)
	d.syncOnce(context.Background())
	ctx := context.Background()

	require.NoError(t, rc.ForCluster("mgmt").Get(ctx, "/x", nil, nil))
	assert.Equal(t, "global", *got)

	cc := rc.ForCluster("crusoe")
	assert.Equal(t, "primus-safe/crusoe-token", cc.TokenSecret())
	require.NoError(t, cc.Get(ctx, "/x", nil, nil))
	assert.Equal(t, "crusoe-tok", *got)

	secret.Data["service-token"] = []byte("rotated")
	require.NoError(t, d.k8sClient.Update(ctx, secret))
	require.NoError(t, cc.Get(ctx, "/x", nil, nil))
	assert.Equal(t, "rotated", *got)

	// The Secret going away keeps the last token read.
	require.NoError(t, d.k8sClient.Delete(ctx, secret))
	require.NoError(t, cc.Get(ctx, "/x", nil, nil))
	assert.Equal(t, "rotated", *got)

	// A malformed reference sends no token rather than the global one.
	require.NoError(t, rc.ForCluster("broken").Get(ctx, "/x", nil, nil))
	assert.Equal(t, "", *got)

	// Dropping the annotation moves the cluster back to the global token.
	cluster := &v1.Cluster{}
	require.NoError(t, d.k8sClient.Get(ctx, client.ObjectKey{Name: "crusoe"}, cluster))
	delete(cluster.Annotations, annotationRobustTokenSecret)
	require.NoError(t, d.k8sClient.Update(ctx, cluster))
	d.syncOnce(ctx)
	assert.Equal(t, "", rc.ForCluster("crusoe").TokenSecret())
	require.NoError(t, rc.ForCluster("crusoe").Get(ctx, "/x", nil, nil))
	assert.Equal(t, "global", *got)
}

// A cluster annotated with a CA Secret reaches a Robust API signed by that CA;
// a malformed reference, a missing Secret or a ca.crt without a certificate
// leaves it on the system pool, reported once and without re-registering.
func TestSyncOnceTrustsTheClusterCA(t *testing.T) {
	srv, caPEM := tlsServer(t)
	ca := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "primus-safe", Name: "crusoe-ca"},
		Data:       map[string][]byte{"ca.crt": caPEM},
	}
	empty := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "primus-safe", Name: "empty-ca"},
		Data:       map[string][]byte{"ca.crt": []byte("no certificate here")},
	}
	withCA := func(name, ref string) *v1.Cluster {
		annotations := map[string]string{annotationRobustEndpoint: srv.URL}
		if ref != "" {
			annotations[annotationRobustCASecret] = ref
		}
		return readyCluster(name, annotations)
	}
	d, rc := newTestDiscovery(t, "mgmt",
		withCA("crusoe", "primus-safe/crusoe-ca"),
		withCA("plain", ""),
		withCA("bad-ref", "no-slash"),
		withCA("missing", "primus-safe/nope"),
		withCA("empty", "primus-safe/empty-ca"),
		ca, empty)
	ctx := context.Background()
	d.syncOnce(ctx)

	assert.Equal(t, "primus-safe/crusoe-ca", rc.ForCluster("crusoe").CASecret())
	assert.NoError(t, rc.ForCluster("crusoe").Get(ctx, "/x", nil, nil))
	assert.ErrorContains(t, rc.ForCluster("plain").Get(ctx, "/x", nil, nil), "certificate")
	assert.NotContains(t, d.caErrLogged, "plain")
	assert.NotContains(t, d.caErrLogged, "crusoe")

	degraded := map[string]*ClusterClient{}
	for _, name := range []string{"bad-ref", "missing", "empty"} {
		cc := rc.ForCluster(name)
		require.NotNil(t, cc, name)
		assert.Equal(t, "", cc.CAHash(), name)
		assert.ErrorContains(t, cc.Get(ctx, "/x", nil, nil), "certificate", name)
		assert.Contains(t, d.caErrLogged, name)
		degraded[name] = cc
	}
	assert.Contains(t, d.caErrLogged["bad-ref"], "<namespace>/<name>")
	assert.Contains(t, d.caErrLogged["missing"], "read secret")
	assert.Contains(t, d.caErrLogged["empty"], "ca.crt")

	// Nothing changed: the clients stay as they are.
	crusoe := rc.ForCluster("crusoe")
	d.syncOnce(ctx)
	assert.Same(t, crusoe, rc.ForCluster("crusoe"))
	for name, cc := range degraded {
		assert.Same(t, cc, rc.ForCluster(name), name)
	}

	// Fixing the Secret content re-registers the cluster with the CA.
	empty.Data["ca.crt"] = caPEM
	require.NoError(t, d.k8sClient.Update(ctx, empty))
	d.syncOnce(ctx)
	assert.NotSame(t, degraded["empty"], rc.ForCluster("empty"))
	assert.Equal(t, CAHash(caPEM), rc.ForCluster("empty").CAHash())
	assert.NoError(t, rc.ForCluster("empty").Get(ctx, "/x", nil, nil))
	assert.NotContains(t, d.caErrLogged, "empty")

	// Dropping the annotation moves the cluster back to the system pool.
	cluster := &v1.Cluster{}
	require.NoError(t, d.k8sClient.Get(ctx, client.ObjectKey{Name: "crusoe"}, cluster))
	delete(cluster.Annotations, annotationRobustCASecret)
	require.NoError(t, d.k8sClient.Update(ctx, cluster))
	d.syncOnce(ctx)
	assert.Equal(t, "", rc.ForCluster("crusoe").CASecret())
	assert.ErrorContains(t, rc.ForCluster("crusoe").Get(ctx, "/x", nil, nil), "certificate")

	// Pointing it at another Secret re-registers it too.
	require.NoError(t, d.k8sClient.Get(ctx, client.ObjectKey{Name: "crusoe"}, cluster))
	cluster.Annotations[annotationRobustCASecret] = "primus-safe/empty-ca"
	require.NoError(t, d.k8sClient.Update(ctx, cluster))
	d.syncOnce(ctx)
	assert.Equal(t, "primus-safe/empty-ca", rc.ForCluster("crusoe").CASecret())
	assert.NoError(t, rc.ForCluster("crusoe").Get(ctx, "/x", nil, nil))
}
