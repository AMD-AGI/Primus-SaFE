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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/pointer"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// ---- fault_helper ----

func TestFaultConfigIsEnable(t *testing.T) {
	assert.True(t, (&FaultConfig{Toggle: ToggleOn}).IsEnable())
	assert.False(t, (&FaultConfig{Toggle: "off"}).IsEnable())
}

func TestFaultConfigIsAutoRepairEnabled(t *testing.T) {
	assert.False(t, (&FaultConfig{}).IsAutoRepairEnabled())
	assert.True(t, (&FaultConfig{IsAutoRepair: pointer.Bool(true)}).IsAutoRepairEnabled())
	assert.False(t, (&FaultConfig{IsAutoRepair: pointer.Bool(false)}).IsAutoRepairEnabled())
}

func TestParseFaultConfig(t *testing.T) {
	cm := &corev1.ConfigMap{Data: map[string]string{
		"a": `{"id":"f1","toggle":"on","action":"restart"}`,
		"b": `{"id":"f2","toggle":"off"}`,        // disabled -> skipped
		"c": `{"toggle":"on"}`,                   // no id -> skipped
		"d": `not-json`,                          // invalid -> skipped
	}}
	result := parseFaultConfig(cm)
	assert.Len(t, result, 1)
	assert.NotNil(t, result["f1"])
	assert.True(t, result["f1"].IsAutoRepairEnabled()) // defaults to true
}

func TestShouldCreateFault(t *testing.T) {
	// k8s NodeReady not true -> create.
	assert.True(t, shouldCreateFault(corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse}))
	assert.False(t, shouldCreateFault(corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue}))
	// k8s pressure condition true -> create.
	assert.True(t, shouldCreateFault(corev1.NodeCondition{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue}))
}

func TestIsK8sCondition(t *testing.T) {
	assert.True(t, isK8sCondition(corev1.NodeReady))
	assert.False(t, isK8sCondition(corev1.NodeConditionType("Custom")))
}

func TestGetIdByConditionType(t *testing.T) {
	assert.Equal(t, NodeNotReady, getIdByConditionType(corev1.NodeReady))
	assert.Equal(t, string(corev1.NodeDiskPressure), getIdByConditionType(corev1.NodeDiskPressure))
}

func TestGenerateFaultOnCreation(t *testing.T) {
	node := &v1.FaultNode{AdminName: "n1", ClusterName: "c1"}
	cm := map[string]*FaultConfig{NodeNotReady: {Id: NodeNotReady, Action: "restart"}}
	cond := corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Message: "down"}
	fault := generateFaultOnCreation(node, cond, cm)
	assert.NotNil(t, fault)
	assert.Equal(t, NodeNotReady, fault.Spec.MonitorId)

	// No matching config -> nil.
	assert.Nil(t, generateFaultOnCreation(node, cond, map[string]*FaultConfig{}))
}

func TestGenerateFaultOnDeletion(t *testing.T) {
	node := &v1.FaultNode{AdminName: "n1", ClusterName: "c1"}
	cond := corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue}
	fault := generateFaultOnDeletion(node, cond, map[string]*FaultConfig{})
	assert.NotNil(t, fault)

	// Non-fault condition -> nil.
	custom := corev1.NodeCondition{Type: corev1.NodeConditionType("Other")}
	assert.Nil(t, generateFaultOnDeletion(node, custom, map[string]*FaultConfig{}))
}

func TestIsValidFault(t *testing.T) {
	node := &v1.Node{Status: v1.NodeStatus{Conditions: []corev1.NodeCondition{
		{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue},
	}}}
	fault := &v1.Fault{Spec: v1.FaultSpec{MonitorId: string(corev1.NodeDiskPressure)}}
	assert.True(t, isValidFault(fault, node))

	other := &v1.Fault{Spec: v1.FaultSpec{MonitorId: "unknown"}}
	assert.False(t, isValidFault(other, node))
}

func TestGetFaultConfigmap(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: common.PrimusFault, Namespace: common.PrimusSafeNamespace},
		Data:       map[string]string{"a": `{"id":"f1","toggle":"on"}`},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	configs, err := GetFaultConfigmap(context.Background(), cl)
	assert.NoError(t, err)
	assert.Len(t, configs, 1)

	// Missing configmap -> empty map, no error.
	emptyCl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	configs, err = GetFaultConfigmap(context.Background(), emptyCl)
	assert.NoError(t, err)
	assert.Empty(t, configs)
}

func TestCreateDeleteFault(t *testing.T) {
	scheme, _ := genMockScheme()
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	fault := &v1.Fault{ObjectMeta: metav1.ObjectMeta{Name: "f1"}, Spec: v1.FaultSpec{MonitorId: "m1"}}
	assert.NoError(t, createFault(context.Background(), cl, fault))
	// Create again with a fresh object -> already exists handled.
	dup := &v1.Fault{ObjectMeta: metav1.ObjectMeta{Name: "f1"}, Spec: v1.FaultSpec{MonitorId: "m1"}}
	assert.NoError(t, createFault(context.Background(), cl, dup))
	assert.NoError(t, deleteFault(context.Background(), cl, fault))
	// Delete again -> not found ignored.
	assert.NoError(t, deleteFault(context.Background(), cl, fault))
}

func TestListFaults(t *testing.T) {
	scheme, _ := genMockScheme()
	fault := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{Name: "f1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(fault).Build()
	faults, err := listFaults(context.Background(), cl, labels.SelectorFromSet(map[string]string{v1.ClusterIdLabel: "c1"}))
	assert.NoError(t, err)
	assert.Len(t, faults, 1)
}

// ---- node_helper ----

func TestGetKubeSprayScaleCMDs(t *testing.T) {
	up := getKubeSprayScaleUpCMD("u", "n1", "env")
	assert.Contains(t, up, "scale.yml")
	assert.Contains(t, up, "n1")
	down := getKubeSprayScaleDownCMD("u", "n1", "env")
	assert.Contains(t, down, "remove-node.yml")
	assert.Contains(t, down, "n1")
}

func TestIsCommandSuccessful(t *testing.T) {
	status := []v1.CommandStatus{{Name: "c1", Phase: v1.CommandSucceeded}}
	assert.True(t, isCommandSuccessful(status, "c1"))
	assert.False(t, isCommandSuccessful(status, "c2"))
}

func TestSetCommandStatus(t *testing.T) {
	var status []v1.CommandStatus
	status = setCommandStatus(status, "c1", v1.CommandSucceeded)
	assert.Len(t, status, 1)
	// Update existing.
	status = setCommandStatus(status, "c1", v1.CommandFailed)
	assert.Len(t, status, 1)
	assert.Equal(t, v1.CommandFailed, status[0].Phase)
}

func TestIsK8sNodeReady(t *testing.T) {
	ready := &corev1.Node{Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
		{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
	}}}
	assert.True(t, isK8sNodeReady(ready))
	notReady := &corev1.Node{Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
		{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
	}}}
	assert.False(t, isK8sNodeReady(notReady))
}

func TestIsConditionsChanged(t *testing.T) {
	old := []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}
	same := []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}
	assert.False(t, isConditionsChanged(old, same))

	diffLen := []corev1.NodeCondition{}
	assert.True(t, isConditionsChanged(old, diffLen))

	diffStatus := []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}
	assert.True(t, isConditionsChanged(old, diffStatus))
}
