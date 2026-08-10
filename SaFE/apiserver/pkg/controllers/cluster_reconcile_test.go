/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package controllers

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

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// probeClusterCert returns a base64-encoded self-signed cert/key pair accepted by tls.X509KeyPair,
// which lets data-plane client construction run against a local probe server.
func probeClusterCert(t *testing.T) (certData, keyData string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	assert.NoError(t, err)
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "safe-unit-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	assert.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	assert.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return base64.StdEncoding.EncodeToString(certPEM), base64.StdEncoding.EncodeToString(keyPEM)
}

// newProbeAPIServer starts a TLS server answering the ServerVersion reachability probe.
func newProbeAPIServer(t *testing.T) int32 {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"major":"1","minor":"30"}`))
	}))
	t.Cleanup(srv.Close)
	addr, ok := srv.Listener.Addr().(*net.TCPAddr)
	assert.True(t, ok)
	return int32(addr.Port)
}

func TestAddClientFactoryBuildsAndReusesFactory(t *testing.T) {
	const clusterName = "probe-cluster"
	certData, keyData := probeClusterCert(t)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: clusterName}}
	cluster.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	cluster.Status.ControlPlaneStatus.CertData = certData
	cluster.Status.ControlPlaneStatus.KeyData = keyData

	scheme := ctrlScheme(t)
	assert.NoError(t, corev1.AddToScheme(scheme))
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: common.PrimusSafeNamespace},
		Spec: corev1.ServiceSpec{
			ClusterIP: "127.0.0.1",
			Ports:     []corev1.ServicePort{{Port: newProbeAPIServer(t)}},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, service).Build()
	r := &ClusterReconciler{Client: cl, ctx: context.Background()}

	clientManager := commonutils.NewObjectManagerSingleton()
	t.Cleanup(func() { _ = clientManager.Delete(clusterName) })

	assert.NoError(t, r.addClientFactory(context.Background(), cluster))
	obj, ok := clientManager.Get(clusterName)
	assert.True(t, ok)
	first := obj.(*commonclient.ClientFactory)

	// The apiserver reaches the data plane, so the existing factory is kept and stays valid.
	assert.NoError(t, r.addClientFactory(context.Background(), cluster))
	obj, ok = clientManager.Get(clusterName)
	assert.True(t, ok)
	assert.Same(t, first, obj.(*commonclient.ClientFactory))
	assert.True(t, first.IsValid())
}

func TestInvalidateUnreachableClientFactory(t *testing.T) {
	factory := commonclient.NewClientFactoryForTest("c1", "10.96.1.1:6443")
	factory.AttachRestConfigForTest(&rest.Config{
		Host:    "https://127.0.0.1:1",
		Timeout: time.Millisecond * 100,
	})
	invalidateUnreachableClientFactory(factory)
	assert.False(t, factory.IsValid())
}

func TestInvalidateUnreachableClientFactoryWithoutRestConfig(t *testing.T) {
	factory := commonclient.NewClientFactoryForTest("c1", "10.96.1.1:6443")
	invalidateUnreachableClientFactory(factory)
	assert.True(t, factory.IsValid())

	factory.SetValid(false, "already invalid")
	invalidateUnreachableClientFactory(factory)
	assert.Equal(t, "already invalid", factory.GetInvalidReason())
}

func TestClusterReconcileNotFound(t *testing.T) {
	cl := fake.NewClientBuilder().WithScheme(ctrlScheme(t)).Build()
	r := &ClusterReconciler{Client: cl, ctx: context.Background()}
	res, err := r.Reconcile(context.Background(), reconcileReq("missing"))
	assert.NoError(t, err)
	assert.Equal(t, time.Duration(0), res.RequeueAfter)
}

func TestClusterReconcileDeleting(t *testing.T) {
	now := metav1.Now()
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{
		Name:              "c1",
		DeletionTimestamp: &now,
		Finalizers:        []string{"x"},
	}}
	cl := fake.NewClientBuilder().WithScheme(ctrlScheme(t)).WithObjects(cluster).Build()
	r := &ClusterReconciler{Client: cl, ctx: context.Background()}
	// Deleting path calls deleteClientFactory; for an unregistered cluster the
	// singleton manager returns a not-found style error, which is tolerated.
	_, _ = r.Reconcile(context.Background(), reconcileReq("c1"))
}

func TestClusterReconcileNotReady(t *testing.T) {
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	cl := fake.NewClientBuilder().WithScheme(ctrlScheme(t)).WithObjects(cluster).Build()
	r := &ClusterReconciler{Client: cl, ctx: context.Background()}
	// Not-ready cluster: addClientFactory returns nil immediately.
	res, err := r.Reconcile(context.Background(), reconcileReq("c1"))
	assert.NoError(t, err)
	assert.Equal(t, time.Duration(0), res.RequeueAfter)
}

func TestShouldPeriodicRefreshClientFactory(t *testing.T) {
	ready := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	ready.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	assert.True(t, shouldPeriodicRefreshClientFactory(ready))

	notReady := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c2"}}
	assert.False(t, shouldPeriodicRefreshClientFactory(notReady))

	deleting := ready.DeepCopy()
	now := metav1.Now()
	deleting.DeletionTimestamp = &now
	assert.False(t, shouldPeriodicRefreshClientFactory(deleting))
}

func TestClusterReconcileReadyEndpointError(t *testing.T) {
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	cluster.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	cl := fake.NewClientBuilder().WithScheme(ctrlScheme(t)).WithObjects(cluster).Build()
	r := &ClusterReconciler{Client: cl, ctx: context.Background()}
	// Ready cluster with no endpoint data -> addClientFactory returns an error.
	_, err := r.Reconcile(context.Background(), reconcileReq("c1"))
	assert.Error(t, err)
}
