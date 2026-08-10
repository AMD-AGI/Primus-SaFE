/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	testifyassert "github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestTryRestartNodeInformer(t *testing.T) {
	called := false
	RegisterNodeInformerRestarter(func(ctx context.Context, cluster *v1.Cluster) error {
		called = cluster != nil && cluster.Name == "c1"
		return nil
	})
	t.Cleanup(func() { RegisterNodeInformerRestarter(nil) })

	err := tryRestartNodeInformer(context.Background(), &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
	})
	testifyassert.NoError(t, err)
	testifyassert.True(t, called)
}

func TestTryRestartNodeInformerNilCluster(t *testing.T) {
	testifyassert.NoError(t, tryRestartNodeInformer(context.Background(), nil))
}

func TestClearNodeInformerForCluster(t *testing.T) {
	cleared := ""
	RegisterNodeInformerClearer(func(clusterName string) {
		cleared = clusterName
	})
	t.Cleanup(func() { RegisterNodeInformerClearer(nil) })

	clearNodeInformerForCluster("c1")
	testifyassert.Equal(t, "c1", cleared)
}
