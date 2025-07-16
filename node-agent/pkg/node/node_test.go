/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package node

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

func genNode() *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				common.AMDGpuIdentification: "true",
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				common.AmdGpu: resource.MustParse("8"),
			},
		},
	}
}

func newNode(t *testing.T) (*Node, *fake.Clientset) {
	testNode := genNode()
	// create fake clientSet
	fakeClientSet := fake.NewClientset(testNode)
	opts := &types.Options{
		NodeName: testNode.Name,
	}
	sleepTime = time.Millisecond * 100
	n, err := NewNodeWithClientSet(context.Background(), opts, fakeClientSet)
	assert.NilError(t, err)
	return n, fakeClientSet
}

func TestWatchNode(t *testing.T) {
	n, fakeClientSet := newNode(t)
	err := n.Start()
	assert.NilError(t, err)

	data, _ := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"test.key": "test.val",
			},
		},
	})
	_, err = fakeClientSet.CoreV1().Nodes().Patch(context.Background(), n.GetK8sNode().Name,
		apitypes.MergePatchType, data, metav1.PatchOptions{})
	assert.NilError(t, err)

	time.Sleep(time.Millisecond * 200)
	val, _ := n.GetK8sNode().Labels["test.key"]
	assert.Equal(t, val, "test.val")
}

func TestGetGpuQuantity(t *testing.T) {
	n, _ := newNode(t)
	quantity := n.GetGpuQuantity()
	assert.Equal(t, quantity.Value(), int64(8))
	assert.Equal(t, n.IsMatchGpuChip(string(v1.AmdGpuChip)), true)
	assert.Equal(t, n.IsMatchGpuChip(string(v1.NvidiaGpuChip)), false)
}

func TestUpdateCondition(t *testing.T) {
	n, _ := newNode(t)
	condition := corev1.NodeCondition{
		Type:   "safe.101",
		Status: "True",
	}
	resp := n.FindConditionByType(string(condition.Type))
	assert.Equal(t, resp != nil, false)
	err := n.UpdateConditions([]corev1.NodeCondition{condition})
	assert.NilError(t, err)
	resp = n.FindConditionByType(string(condition.Type))
	assert.Equal(t, resp != nil, true)
}

func TestUpdateStartTime(t *testing.T) {
	n, _ := newNode(t)
	nowTime := time.Now()
	err := n.updateNodeStartTime(nowTime)
	assert.NilError(t, err)
	nowTimeStr := strconv.FormatInt(nowTime.Unix(), 10)
	assert.Equal(t, v1.GetNodeStartupTime(n.GetK8sNode()), nowTimeStr)
}
