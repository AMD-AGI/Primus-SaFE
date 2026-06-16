/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
)

func k8sNode(name, clusterId string) *corev1.Node {
	n := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if clusterId != "" {
		v1.SetLabel(n, v1.ClusterIdLabel, clusterId)
	}
	return n
}

func TestNodeEventHandlerFull(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	factory := commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs)
	r := &NodeK8sReconciler{
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[*nodeQueueMessage](),
			workqueue.TypedRateLimitingQueueConfig[*nodeQueueMessage]{Name: "test"}),
	}
	handler := r.nodeEventHandler(factory).(cache.ResourceEventHandlerFuncs)

	// Add event for a node belonging to c1
	handler.AddFunc(k8sNode("n1", "c1"))
	// Add event for a node NOT belonging to c1 (filtered out)
	handler.AddFunc(k8sNode("other", "c2"))
	// Managed: old not in c1, new in c1
	handler.UpdateFunc(k8sNode("n2", "c2"), k8sNode("n2", "c1"))
	// Unmanaged: old in c1, new not
	handler.UpdateFunc(k8sNode("n3", "c1"), k8sNode("n3", "c2"))
	// Delete event
	handler.DeleteFunc(k8sNode("n1", "c1"))

	assert.Positive(t, r.queue.Len())
}

func TestWatchErrorHandlerFull(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	factory := commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs)
	factory.SetValid(true, "")
	h := watchErrorHandler(context.Background(), factory)
	h(&cache.Reflector{}, errors.New("boom"))
	assert.False(t, factory.IsValid())
}
