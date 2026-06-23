/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func newClusterReconcilerWithClientSet(t *testing.T, cs *k8sfake.Clientset, objs ...client.Object) *ClusterReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return &ClusterReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl, clientSet: cs},
	}
}

func clusterEndpoints(name string) *corev1.Endpoints {
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: common.PrimusSafeNamespace},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}},
		}},
	}
}

func TestGetClusterEndpoint(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(clusterEndpoints("c1"))
	r := newClusterReconcilerWithClientSet(t, cs)
	addrs, err := r.getClusterEndpoint(context.Background(), testCluster("c1"))
	assert.NoError(t, err)
	assert.Len(t, addrs, 1)

	// Missing endpoints -> error.
	r2 := newClusterReconcilerWithClientSet(t, k8sfake.NewSimpleClientset())
	_, err = r2.getClusterEndpoint(context.Background(), testCluster("c1"))
	assert.Error(t, err)
}

func TestGuaranteeForwardEndpointsCreate(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(clusterEndpoints("c1"))
	cluster := testCluster("c1")
	r := newClusterReconcilerWithClientSet(t, cs, cluster)
	assert.NoError(t, r.guaranteeForwardEndpoints(context.Background(), cluster))
	_, err := cs.CoreV1().Endpoints(common.PrimusSafeNamespace).Get(context.Background(), "c1-forward", metav1.GetOptions{})
	assert.NoError(t, err)
	// Idempotent (already exists, no change).
	assert.NoError(t, r.guaranteeForwardEndpoints(context.Background(), cluster))
}

func TestGuaranteeForwardService(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(clusterEndpoints("c1"))
	cluster := testCluster("c1")
	r := newClusterReconcilerWithClientSet(t, cs, cluster)
	assert.NoError(t, r.guaranteeForwardService(context.Background(), cluster))
	_, err := cs.CoreV1().Services(common.PrimusSafeNamespace).Get(context.Background(), "c1-forward", metav1.GetOptions{})
	assert.NoError(t, err)
	// Idempotent.
	assert.NoError(t, r.guaranteeForwardService(context.Background(), cluster))
}

func TestGuaranteeForwardIngressDisabled(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := testCluster("c1")
	r := newClusterReconcilerWithClientSet(t, cs, cluster)
	// Ingress class not higress by default -> no-op.
	assert.NoError(t, r.guaranteeForwardIngress(context.Background(), cluster))
}

func TestGetAdminClusterRole(t *testing.T) {
	role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "role1"}}
	scheme, _ := genMockScheme()
	_ = rbacv1.AddToScheme(scheme)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(role).Build()
	r := &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl}}
	got, err := r.getAdminClusterRole(context.Background(), "role1")
	assert.NoError(t, err)
	assert.Equal(t, "role1", got.Name)

	// Missing -> nil, nil (IgnoreNotFound).
	got, err = r.getAdminClusterRole(context.Background(), "missing")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestGuaranteeCICDClusterRoleDisabled(t *testing.T) {
	r := newPlaneReconciler(t)
	// CI/CD disabled by default -> no-op.
	assert.NoError(t, r.guaranteeCICDClusterRole(context.Background(), testCluster("c1")))
	assert.NoError(t, r.deleteCICDClusterRole(context.Background(), testCluster("c1")))
	assert.NoError(t, r.guaranteeMonarchClusterRole(context.Background(), testCluster("c1")))
	assert.NoError(t, r.deleteMonarchClusterRole(context.Background(), testCluster("c1")))
}

var _ = v1.ClusterKind
