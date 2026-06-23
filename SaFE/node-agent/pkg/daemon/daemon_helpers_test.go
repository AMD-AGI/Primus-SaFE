/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package daemon

import (
	"context"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/monitors"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/node"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

// newDaemonTestComponents builds monitor manager dependencies for daemon stop tests.
func newDaemonTestComponents(t *testing.T) (*monitors.MonitorManager, *node.Node, types.MonitorQueue) {
	t.Helper()
	testNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "daemon-node",
			Labels: map[string]string{
				common.AMDGpuIdentification: v1.TrueStr,
			},
		},
	}
	fakeClientSet := fake.NewClientset(testNode)
	opts := &types.Options{NodeName: testNode.Name, ConfigMapPath: t.TempDir(), ScriptPath: t.TempDir()}
	n, err := node.NewNodeWithClientSet(context.Background(), opts, fakeClientSet)
	assert.NilError(t, err)

	var queue types.MonitorQueue
	queue = workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[*types.MonitorMessage](),
		workqueue.TypedRateLimitingQueueConfig[*types.MonitorMessage]{Name: "daemon-test"})
	manager := monitors.NewMonitorManager(&queue, opts, n)
	return manager, n, queue
}
