/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestGetChartName(t *testing.T) {
	r := &AddonTemplateController{}

	// OCI URL -> name is the URL itself.
	ociTmpl := &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "t1"},
		Spec:       v1.AddonTemplateSpec{URL: "oci://registry/chart"},
	}
	installClient := &action.Install{}
	name := r.getChartName(ociTmpl, installClient)
	assert.Equal(t, "oci://registry/chart", name)
	assert.Equal(t, "", installClient.ChartPathOptions.RepoURL)

	// HTTP repo URL -> name is template name, RepoURL set.
	httpTmpl := &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "t2"},
		Spec:       v1.AddonTemplateSpec{URL: "https://charts.example.com"},
	}
	installClient2 := &action.Install{}
	name = r.getChartName(httpTmpl, installClient2)
	assert.Equal(t, "t2", name)
	assert.Equal(t, "https://charts.example.com", installClient2.ChartPathOptions.RepoURL)
}

func TestAddonTemplateReconcileNotFound(t *testing.T) {
	scheme, _ := genMockScheme()
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	r := &AddonTemplateController{Client: cl}
	res, err := r.Reconcile(context.Background(), reconcileRequest("missing"))
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestAddonTemplateReconcileNoURL(t *testing.T) {
	scheme, _ := genMockScheme()
	tmpl := &v1.AddonTemplate{ObjectMeta: metav1.ObjectMeta{Name: "t1"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(tmpl).Build()
	r := &AddonTemplateController{Client: cl}
	// Empty URL -> early return.
	_, err := r.Reconcile(context.Background(), reconcileRequest("t1"))
	assert.NoError(t, err)
}

func TestAddonTemplateReconcileAlreadySynced(t *testing.T) {
	scheme, _ := genMockScheme()
	tmpl := &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "t1"},
		Spec:       v1.AddonTemplateSpec{URL: "https://charts"},
		Status:     v1.AddonTemplateStatus{HelmStatus: v1.HelmStatus{Values: "already"}},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(tmpl).Build()
	r := &AddonTemplateController{Client: cl}
	// Status.Values already set -> early return.
	_, err := r.Reconcile(context.Background(), reconcileRequest("t1"))
	assert.NoError(t, err)
}

func TestUpdateTemplateStatus(t *testing.T) {
	scheme, _ := genMockScheme()
	tmpl := &v1.AddonTemplate{ObjectMeta: metav1.ObjectMeta{Name: "t1"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.AddonTemplate{}).
		WithObjects(tmpl).
		Build()
	r := &AddonTemplateController{Client: cl}

	ch := &chart.Chart{
		Values: map[string]interface{}{"replicas": 3},
		Raw:    []*chart.File{{Name: "values.yaml", Data: []byte("replicas: 3")}},
	}
	err := r.updateTemplateStatus(context.Background(), tmpl, ch)
	assert.NoError(t, err)
	assert.Contains(t, tmpl.Status.HelmStatus.Values, "replicas")
	assert.Equal(t, "replicas: 3", tmpl.Status.HelmStatus.ValuesYAML)
}
