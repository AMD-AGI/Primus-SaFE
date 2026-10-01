/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"strings"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonquantity "github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
)

const (
	gpuResource = corev1.ResourceName(common.AmdGpu)
	gpuQuota    = corev1.ResourceName("requests." + common.AmdGpu)
)

func qty(list corev1.ResourceList, name corev1.ResourceName) string {
	q := list[name]
	return q.String()
}

func budgetQuota(namespace string, hard, used corev1.ResourceList) *corev1.ResourceQuota {
	return &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: v1.ExternalBudgetQuotaName, Namespace: namespace},
		Spec:       corev1.ResourceQuotaSpec{Hard: hard},
		Status:     corev1.ResourceQuotaStatus{Hard: hard, Used: used},
	}
}

func externalWorkspace(cluster, nodeFlavor string, replica int) *v1.Workspace {
	workspace := genMockWorkspace(cluster, nodeFlavor, replica)
	workspace.Labels[v1.WorkspaceExternalLabel] = v1.TrueStr
	return workspace
}

// newExternalReconciler wires a reconciler whose data plane for cluster is dataPlane. Each
// test uses its own cluster name: the client manager is a process-wide singleton.
func newExternalReconciler(t *testing.T, cluster string, dataPlane *k8sfake.Clientset,
	objs ...client.Object) (*WorkspaceReconciler, client.Client, *record.FakeRecorder) {
	t.Helper()
	builder := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objs...)
	for _, o := range objs {
		if w, ok := o.(*v1.Workspace); ok {
			builder = builder.WithStatusSubresource(w)
		}
	}
	cli := builder.Build()
	r := newMockWorkspaceReconciler(cli)
	recorder := record.NewFakeRecorder(10)
	r.recorder = recorder
	r.clientManager.AddOrReplace(cluster, commonclient.NewClientFactoryWithOnlyClient(
		context.Background(), cluster, dataPlane))
	t.Cleanup(func() { _ = r.clientManager.Delete(cluster) })
	return &r, cli, recorder
}

// The quota name is a contract with the capacity supplier, which knows namespaces but not
// workspaces. Pinned as a literal so a rename of the constant cannot pass unnoticed.
func TestExternalBudgetQuotaName(t *testing.T) {
	assert.Equal(t, v1.ExternalBudgetQuotaName, "external-budget")
}

func TestIsExternalWorkspace(t *testing.T) {
	workspace := genMockWorkspace("c", "f", 0)
	assert.Equal(t, v1.IsExternalWorkspace(workspace), false)
	workspace.Labels[v1.WorkspaceExternalLabel] = "yes"
	assert.Equal(t, v1.IsExternalWorkspace(workspace), false)
	workspace.Labels[v1.WorkspaceExternalLabel] = v1.TrueStr
	assert.Equal(t, v1.IsExternalWorkspace(workspace), true)
}

func TestQuotaHardToResources(t *testing.T) {
	got := quotaHardToResources(corev1.ResourceList{
		gpuQuota:                               resource.MustParse("16"),
		"requests.cpu":                         resource.MustParse("200"),
		corev1.ResourceCPU:                     resource.MustParse("100"), // smaller: the one enforced
		corev1.ResourceMemory:                  resource.MustParse("2Ti"),
		"requests.memory":                      resource.MustParse("3Ti"),
		"requests.ephemeral-storage":           resource.MustParse("8Ti"),
		"limits.cpu":                           resource.MustParse("1"),
		corev1.ResourcePods:                    resource.MustParse("10"),
		"count/deployments.apps":               resource.MustParse("1"),
		corev1.ResourceName("requests.rdma/x"): resource.MustParse("2"),
	})
	want := corev1.ResourceList{
		gpuResource:                     resource.MustParse("16"),
		corev1.ResourceCPU:              resource.MustParse("100"),
		corev1.ResourceMemory:           resource.MustParse("2Ti"),
		corev1.ResourceEphemeralStorage: resource.MustParse("8Ti"),
		"rdma/x":                        resource.MustParse("2"),
	}
	assert.Assert(t, commonquantity.Equal(got, want), "got %v, want %v", got, want)
}

// The budget is spec.hard for both totals. status.used is deliberately not subtracted: the
// job-manager queue subtracts every dispatched workload from AvailableResources itself, so
// subtracting used here would count a running workload twice.
func TestSyncExternalWorkspaceWritesQuotaHard(t *testing.T) {
	const cluster = "ext-budget-hard"
	workspace := externalWorkspace(cluster, "flavor", 0)
	workspace.Status.AvailableReplica = 2
	hard := corev1.ResourceList{
		gpuQuota:                     resource.MustParse("16"),
		"requests.cpu":               resource.MustParse("256"),
		"requests.memory":            resource.MustParse("4Ti"),
		"requests.ephemeral-storage": resource.MustParse("10Ti"),
	}
	used := corev1.ResourceList{gpuQuota: resource.MustParse("8")}
	dataPlane := k8sfake.NewClientset(budgetQuota(workspace.Name, hard, used))
	r, cli, recorder := newExternalReconciler(t, cluster, dataPlane, workspace)

	assert.NilError(t, r.syncWorkspace(context.Background(), workspace))

	stored := storedWorkspace(t, cli, workspace.Name)
	want := quotaHardToResources(hard)
	assert.Equal(t, qty(stored.Status.TotalResources, gpuResource), "16")
	assert.Assert(t, commonquantity.Equal(stored.Status.TotalResources, want))
	assert.Assert(t, commonquantity.Equal(stored.Status.AvailableResources, want))
	assert.Equal(t, len(stored.Status.AbnormalResources), 0)
	assert.Equal(t, stored.Status.AvailableReplica, 0)
	assert.Equal(t, len(recorder.Events), 0)
}

// Raising the quota is picked up by the next sync, with no restart.
func TestSyncExternalWorkspaceFollowsQuotaChange(t *testing.T) {
	const cluster = "ext-budget-change"
	workspace := externalWorkspace(cluster, "flavor", 0)
	dataPlane := k8sfake.NewClientset(budgetQuota(workspace.Name,
		corev1.ResourceList{gpuQuota: resource.MustParse("16")}, nil))
	r, cli, _ := newExternalReconciler(t, cluster, dataPlane, workspace)
	assert.NilError(t, r.syncWorkspace(context.Background(), workspace))

	_, err := dataPlane.CoreV1().ResourceQuotas(workspace.Name).Update(context.Background(),
		budgetQuota(workspace.Name, corev1.ResourceList{gpuQuota: resource.MustParse("24")}, nil),
		metav1.UpdateOptions{})
	assert.NilError(t, err)
	workspace = storedWorkspace(t, cli, workspace.Name)
	assert.NilError(t, r.syncWorkspace(context.Background(), workspace))

	stored := storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, qty(stored.Status.AvailableResources, gpuResource), "24")
	assert.Equal(t, qty(stored.Status.TotalResources, gpuResource), "24")
}

// No quota means no budget: the previous totals are cleared, not kept, and the workspace
// carries an event saying which quota is missing.
func TestSyncExternalWorkspaceWithoutQuotaIsZero(t *testing.T) {
	const cluster = "ext-budget-missing"
	workspace := externalWorkspace(cluster, "flavor", 0)
	workspace.Status.TotalResources = corev1.ResourceList{gpuResource: resource.MustParse("16")}
	workspace.Status.AvailableResources = corev1.ResourceList{gpuResource: resource.MustParse("16")}
	r, cli, recorder := newExternalReconciler(t, cluster, k8sfake.NewClientset(), workspace)

	assert.NilError(t, r.syncWorkspace(context.Background(), workspace))

	stored := storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, len(stored.Status.TotalResources), 0)
	assert.Equal(t, len(stored.Status.AvailableResources), 0)
	assert.Equal(t, len(recorder.Events), 1)
	event := <-recorder.Events
	assert.Assert(t, strings.Contains(event, ExternalBudgetMissingReason), event)
	assert.Assert(t, strings.Contains(event, v1.ExternalBudgetQuotaName), event)
	assert.Assert(t, strings.Contains(event, workspace.Name), event)
}

// A quota that could not be read is not an absent one: the error comes back and the last
// known budget stays, instead of being zeroed by a transient failure.
func TestSyncExternalWorkspaceReadErrorKeepsStatus(t *testing.T) {
	const cluster = "ext-budget-forbidden"
	workspace := externalWorkspace(cluster, "flavor", 0)
	workspace.Status.TotalResources = corev1.ResourceList{gpuResource: resource.MustParse("16")}
	workspace.Status.AvailableResources = corev1.ResourceList{gpuResource: resource.MustParse("16")}
	dataPlane := k8sfake.NewClientset()
	dataPlane.PrependReactor("get", "resourcequotas",
		func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(
				schema.GroupResource{Resource: "resourcequotas"}, v1.ExternalBudgetQuotaName, nil)
		})
	r, cli, recorder := newExternalReconciler(t, cluster, dataPlane, workspace)

	err := r.syncWorkspace(context.Background(), workspace)
	assert.Assert(t, apierrors.IsForbidden(err), "got %v", err)

	stored := storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, qty(stored.Status.TotalResources, gpuResource), "16")
	assert.Equal(t, qty(stored.Status.AvailableResources, gpuResource), "16")
	assert.Equal(t, len(recorder.Events), 0)
}

// An external workspace is not scaled by node: spec.Replica asks for a node and a free one of
// the right flavor is in the fleet, yet nothing is bound. The phase becomes Running and the
// reconcile comes back to re-read the quota.
func TestReconcileExternalWorkspaceSkipsScaling(t *testing.T) {
	const cluster = "ext-budget-noscale"
	nodeFlavor := genMockNodeFlavor()
	workspace := externalWorkspace(cluster, nodeFlavor.Name, 1)
	workspace.Status.Phase = v1.WorkspaceCreating
	free := genMockAdminNode("ext-free-node", cluster, nodeFlavor)
	dataPlane := k8sfake.NewClientset(budgetQuota(workspace.Name,
		corev1.ResourceList{gpuQuota: resource.MustParse("8")}, nil),
		genMockK8sNode(free.Name, cluster, nodeFlavor.Name, workspace.Name))
	r, cli, _ := newExternalReconciler(t, cluster, dataPlane, workspace, free, nodeFlavor)

	result, err := r.processWorkspace(context.Background(), workspace)
	assert.NilError(t, err)

	node := &v1.Node{}
	assert.NilError(t, cli.Get(context.Background(), client.ObjectKey{Name: free.Name}, node))
	assert.Equal(t, node.GetSpecWorkspace(), "")
	stored := storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, stored.Status.Phase, v1.WorkspaceRunning)
	assert.Equal(t, qty(stored.Status.TotalResources, gpuResource), "8")
	assert.Equal(t, result.RequeueAfter, externalBudgetResync)
}
