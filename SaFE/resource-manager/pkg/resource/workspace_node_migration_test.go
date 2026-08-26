/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"
	"time"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
)

// markMigrating puts a node in the state the source workspace leaves it in: released, and
// carrying the target that is expected to pick it up.
func markMigrating(node *v1.Node, from, target string) {
	node.Spec.Workspace = nil
	delete(node.Labels, v1.WorkspaceIdLabel)
	v1.SetNodeMigrateInfo(node, &v1.NodeMigrateInfo{
		From:      from,
		Target:    target,
		StartTime: &metav1.Time{Time: time.Now().UTC()},
	})
}

func TestIsNodeEligibleForScalingUp(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	workspace := genMockWorkspace(clusterName, nodeFlavor.Name, 1)

	cases := []struct {
		name  string
		mutil func(node *v1.Node)
		want  bool
	}{
		{name: "free node of a matching flavor", want: true},
		{name: "machine not ready", want: false, mutil: func(node *v1.Node) {
			node.Status.MachineStatus.Phase = v1.NodeSSHFailed
		}},
		{name: "not managed", want: false, mutil: func(node *v1.Node) {
			node.Status.ClusterStatus.Phase = v1.NodeManaging
		}},
		{name: "bound by spec", want: false, mutil: func(node *v1.Node) {
			node.Spec.Workspace = ptr.To("other")
		}},
		{name: "bound by label", want: false, mutil: func(node *v1.Node) {
			metav1.SetMetaDataLabel(&node.ObjectMeta, v1.WorkspaceIdLabel, "other")
		}},
		{name: "flavor mismatch", want: false, mutil: func(node *v1.Node) {
			metav1.SetMetaDataLabel(&node.ObjectMeta, v1.NodeFlavorIdLabel, "another-flavor")
		}},
		{name: "reserved by a migration to another workspace", want: false, mutil: func(node *v1.Node) {
			markMigrating(node, "ws-a", "ws-b")
		}},
		{name: "reserved by a migration to this workspace", want: true, mutil: func(node *v1.Node) {
			markMigrating(node, "ws-a", workspace.Name)
		}},
		{name: "unreadable migration payload does not park the node", want: true, mutil: func(node *v1.Node) {
			metav1.SetMetaDataAnnotation(&node.ObjectMeta, v1.NodeMigrateAnnotation, "{")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := genMockAdminNode("node1", clusterName, nodeFlavor)
			node.Status.ClusterStatus.Phase = v1.NodeManaged
			if tc.mutil != nil {
				tc.mutil(node)
			}
			assert.Equal(t, isNodeEligibleForScalingUp(node, workspace), tc.want)
		})
	}
}

// A migration leaves the node unbound and of a matching flavor for as long as it takes the
// target to claim it, which is everything an unrelated workspace looks for when it is short
// of a replica. It must not take it.
func TestScaleUpLeavesAMigratingNodeToItsTarget(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	target := genMockWorkspace(clusterName, nodeFlavor.Name, 1)
	bystander := genMockWorkspace(clusterName, nodeFlavor.Name, 1)

	adminNode := genMockAdminNode("node1", clusterName, nodeFlavor)
	adminNode.Status.ClusterStatus.Phase = v1.NodeManaged
	markMigrating(adminNode, "ws-source", target.Name)

	adminClient := fake.NewClientBuilder().WithObjects(adminNode, target, bystander).
		WithStatusSubresource(target, bystander).WithScheme(scheme.Scheme).Build()
	k8sClient := k8sfake.NewClientset(genMockK8sNode(adminNode.Name, clusterName, nodeFlavor.Name, ""))
	k8sClientFactory := commonclient.NewClientFactoryWithOnlyClient(context.Background(), clusterName, k8sClient)
	r := newMockWorkspaceReconciler(adminClient)

	_, err := r.scaleUp(context.Background(), bystander, k8sClientFactory, 1)
	assert.NilError(t, err)
	assert.NilError(t, adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode.Name}, adminNode))
	assert.Equal(t, adminNode.GetSpecWorkspace(), "")

	// The target itself is still free to take it, so a lost handover does not park the node.
	_, err = r.scaleUp(context.Background(), target, k8sClientFactory, 1)
	assert.NilError(t, err)
	assert.NilError(t, adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode.Name}, adminNode))
	assert.Equal(t, adminNode.GetSpecWorkspace(), target.Name)
}
