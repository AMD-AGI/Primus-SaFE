/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

func wsForHelper(name string) *v1.Workspace {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	ws.Spec.Cluster = "c1"
	return ws
}

func TestServiceAccountLifecycleFull(t *testing.T) {
	patches := gomonkey.NewPatches()
	patches.ApplyFunc(commonconfig.IsCICDEnable, func() bool { return true })
	patches.ApplyFunc(commonconfig.IsMonarchEnable, func() bool { return true })
	patches.ApplyFunc(commonconfig.GetMonarchClientRole, func() string { return "monarch-sa" })
	defer patches.Reset()

	cs := k8sfake.NewSimpleClientset()
	ws := wsForHelper("ws1")
	ctx := context.Background()

	assert.NoError(t, createCICDServiceAccount(ctx, ws, cs))
	assert.NoError(t, createMonarchServiceAccount(ctx, ws, cs))
	// idempotent second call
	assert.NoError(t, createMonarchServiceAccount(ctx, ws, cs))

	rb, err := cs.RbacV1().RoleBindings("ws1").Get(ctx, "monarch-sa", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, "monarch-sa", rb.Name)

	assert.NoError(t, deleteMonarchServiceAccount(ctx, ws, cs))
	assert.NoError(t, deleteCICDServiceAccount(ctx, ws, cs))
}

func TestGetPvTemplateAndCreateDataPlanePv(t *testing.T) {
	pvYaml := `apiVersion: v1
kind: PersistentVolume
metadata:
  name: placeholder
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteMany
  hostPath:
    path: /data
`
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        common.PrimusPvmName,
			Namespace:   common.PrimusSafeNamespace,
			Labels:      map[string]string{v1.DisplayNameLabel: "pvtmpl"},
			Annotations: map[string]string{"primus-safe.workspace.auto-create-pv": v1.TrueStr},
		},
		Data: map[string]string{"template": pvYaml},
	}
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	ws := wsForHelper("ws1")
	v1.SetLabel(ws, v1.DisplayNameLabel, "wsdisp")
	ctx := context.Background()

	tmpl, err := getPvTemplate(ctx, cl, ws)
	assert.NoError(t, err)
	assert.NotNil(t, tmpl)

	cs := k8sfake.NewSimpleClientset()
	assert.NoError(t, createDataPlanePv(ctx, ws, cl, cs))
	pvs, err := cs.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Len(t, pvs.Items, 1)
}

func TestGetPvTemplateNoConfigMap(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	tmpl, err := getPvTemplate(context.Background(), cl, wsForHelper("ws1"))
	assert.NoError(t, err)
	assert.Nil(t, tmpl)
}
