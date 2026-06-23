/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
)

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

	cs := &syncer.ClusterClientSets{}
	cs.SetClientFactory(commonclient.NewClientFactoryWithOnlyClient(
		context.Background(), "c", k8sfake.NewSimpleClientset()))

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
}

func TestCreateIngressNoService(t *testing.T) {
	r := &DispatcherReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	// No Service spec -> ingress creation is skipped.
	res, err := r.createIngress(context.Background(), w, nil, nil)
	assert.NilError(t, err)
	assert.Equal(t, res.RequeueAfter.Nanoseconds(), int64(0))
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