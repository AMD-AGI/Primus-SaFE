/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package monitors

import (
	"context"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/node"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

// newNode builds a fake Kubernetes node for monitor unit tests.
func newNode(t *testing.T) *node.Node {
	t.Helper()
	testNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				v1.NodeGpuCountLabel:        "8",
				common.AMDGpuIdentification: v1.TrueStr,
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				common.AmdGpu: resource.MustParse("4"),
			},
		},
	}
	fakeClientSet := fake.NewClientset(testNode)
	opts := &types.Options{
		NodeName: testNode.Name,
	}
	n, err := node.NewNodeWithClientSet(context.Background(), opts, fakeClientSet)
	assert.NilError(t, err)
	return n
}

// newMonitorConfig returns a minimal monitor configuration for tests.
func newMonitorConfig(id, script string) *MonitorConfig {
	return &MonitorConfig{
		Id:               id,
		Script:           script,
		Cronjob:          "@every 1s",
		TimeoutSecond:    60,
		ConsecutiveCount: 1,
		Toggle:           "on",
	}
}
