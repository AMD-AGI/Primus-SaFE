/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

func ptrString(s string) *string { return &s }

func gomonkeyApplyGetSSHClient(_ *ssh.Client) *gomonkey.Patches {
	// Dial a fresh client per call: production code closes the client via defer
	// after every SSH operation, so returning a single shared client breaks
	// multi-step flows.
	return gomonkey.ApplyFunc(utils.GetSSHClient,
		func(_ context.Context, _ client.Client, _ *v1.Node) (*ssh.Client, error) {
			return dialResourceSSH()
		})
}

func timeNowResource() time.Time { return time.Now() }

func gomonkeyApplyGetK8sFactory(cs *k8sfake.Clientset) *gomonkey.Patches {
	return gomonkey.ApplyFunc(utils.GetK8sClientFactory,
		func(_ *commonutils.ObjectManager, _ string) (*commonclient.ClientFactory, error) {
			return commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs), nil
		})
}

func k8sfakeClientset() *k8sfake.Clientset { return k8sfake.NewSimpleClientset() }

func ctrlfakeNewClient(scheme *runtime.Scheme, objs ...client.Object) client.Client {
	return ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func TestIsNodeRelevantFieldChanged(t *testing.T) {
	r := newMockNodeReconciler(nil)

	old := &v1.Node{}
	old.Status.MachineStatus.Phase = v1.NodeReady
	same := old.DeepCopy()
	assert.False(t, r.isNodeRelevantFieldChanged(old, same))

	// Machine phase changed.
	changed := old.DeepCopy()
	changed.Status.MachineStatus.Phase = v1.NodePhase("Other")
	assert.True(t, r.isNodeRelevantFieldChanged(old, changed))

	// Deletion timestamp set.
	deleting := old.DeepCopy()
	now := metav1.Now()
	deleting.DeletionTimestamp = &now
	assert.True(t, r.isNodeRelevantFieldChanged(old, deleting))
}

func TestNodeRelevantChangePredicate(t *testing.T) {
	r := newMockNodeReconciler(nil)
	p := r.relevantChangePredicate()
	old := &v1.Node{}
	changed := &v1.Node{}
	changed.Status.MachineStatus.Phase = v1.NodeReady
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: changed}))
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: old.DeepCopy()}))
}

func TestNodeHandlePodEvent(t *testing.T) {
	r := newMockNodeReconciler(nil)
	h := r.handlePodEvent()
	// Just ensure the handler is constructed and callbacks don't panic on non-pod.
	assert.NotNil(t, h)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "p1",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1.SchemeGroupVersion.String(),
				Kind:       v1.NodeKind,
				Name:       "n1",
			}},
		},
	}
	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer q.ShutDown()
	h.Create(context.Background(), event.CreateEvent{Object: pod}, q)
	assert.Equal(t, 1, q.Len())
}

func TestBashRemoteScript(t *testing.T) {
	out := bashRemoteScript("echo hi")
	assert.Contains(t, out, "base64 -d | bash")
}

func TestGetClusterIdHelper(t *testing.T) {
	node := &v1.Node{}
	node.Spec.Cluster = ptrString("c1")
	assert.Equal(t, "c1", getClusterId(node))
}

func TestForceDeleteK8sNode(t *testing.T) {
	cs := k8sfakeClientset()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	_, _ = cs.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
	assert.NoError(t, forceDeleteK8sNode(context.Background(), cs, "n1"))
}

func TestNodeListAndDeletePods(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      "p1",
		Namespace: "primus-safe",
		Labels: map[string]string{
			v1.ClusterManageClusterLabel: "c1",
			v1.ClusterManageNodeLabel:    "n1",
			v1.ClusterManageActionLabel:  "scale-up",
		},
	}}
	scheme, _ := genMockScheme()
	cl := ctrlfakeNewClient(scheme, pod)
	r := newMockNodeReconciler(cl)
	pods, err := r.listPod(context.Background(), "c1", "n1", "scale-up")
	assert.NoError(t, err)
	assert.Len(t, pods, 1)
	assert.NoError(t, r.deletePods(context.Background(), "c1", "n1", "scale-up"))
}

func TestNodeInstallAddonsNoTemplate(t *testing.T) {
	scheme, _ := genMockScheme()
	cl := ctrlfakeNewClient(scheme)
	r := newMockNodeReconciler(cl)
	// No node template -> no-op.
	assert.NoError(t, r.installAddons(context.Background(), &v1.Node{}))
}

func TestNodeExecuteSSHCommand(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	r := newMockNodeReconciler(nil)
	assert.NoError(t, r.executeSSHCommand(sshClient, "echo hi"))
}

func TestNodeInstallHarborCertNoSecret(t *testing.T) {
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	// No harbor-tls secret -> returns false, nil.
	ok, err := r.installHarborCert(context.Background(), sshClient)
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestCleanupNodeAfterUnmanage(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.ClusterIdLabel: "c1", v1.WorkspaceIdLabel: "ws1"},
	}}
	cl := ctrlfakeNewClient(scheme, node)
	r := newMockNodeReconciler(cl)
	assert.NoError(t, r.cleanupNodeAfterUnmanage(context.Background(), node))
	assert.Equal(t, "", v1.GetClusterId(node))

	// No change -> nil.
	clean := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2"}}
	assert.NoError(t, r.cleanupNodeAfterUnmanage(context.Background(), clean))
}

func TestProcessNodeManagementCleanup(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Status.MachineStatus.Phase = v1.NodeReady
	cl := ctrlfakeNewClient(scheme, node)
	r := newMockNodeReconciler(cl)
	// No spec cluster, no status cluster, no k8sNode, machine ready -> cleanup path.
	res, err := r.processNodeManagement(context.Background(), node, nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestProcessNodeManagementNotReady(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	cl := ctrlfakeNewClient(scheme, node)
	r := newMockNodeReconciler(cl)
	// Machine not ready -> requeue 30s.
	res, err := r.processNodeManagement(context.Background(), node, nil)
	assert.NoError(t, err)
	assert.True(t, res.RequeueAfter > 0)
}

func TestSyncClusterStatusManaged(t *testing.T) {
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.ClusterIdLabel: "c1"},
	}}
	node.Spec.Cluster = ptrString("c1")
	node.Status.ClusterStatus.Phase = v1.NodeManaged
	// Already managed -> nil immediately.
	assert.NoError(t, r.syncClusterStatus(context.Background(), node))
}

func TestSyncOrCreateScaleUpPodExisting(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.ClusterIdLabel: "c1"},
	}}
	node.Spec.Cluster = ptrString("c1")
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "primus-safe",
			Labels: map[string]string{
				v1.ClusterManageClusterLabel: "c1",
				v1.ClusterManageNodeLabel:    "n1",
				v1.ClusterManageActionLabel:  string(v1.ClusterScaleUpAction),
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodFailed},
	}
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme, node, pod))
	_, err := r.syncOrCreateScaleUpPod(context.Background(), node)
	assert.NoError(t, err)
	assert.Equal(t, v1.NodeManagedFailed, node.Status.ClusterStatus.Phase)
}

func TestRebootNodeViaSSH(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}

	patches := gomonkeyApplyGetSSHClient(sshClient)
	defer patches.Reset()
	// Should not panic.
	r.rebootNode(context.Background(), node)
}

func TestResetNodeViaSSH(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}

	patches := gomonkeyApplyGetSSHClient(sshClient)
	defer patches.Reset()
	assert.NoError(t, r.resetNode(context.Background(), node))
}

func TestModifyResolvConfViaSSH(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}

	patches := gomonkeyApplyGetSSHClient(sshClient)
	defer patches.Reset()
	assert.NoError(t, r.modifyResolvConf(context.Background(), node))
}

func TestUnmanageControlPlaneNode(t *testing.T) {
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.KubernetesControlPlane: ""},
	}}
	res, err := r.unmanage(context.Background(), node, nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestUnmanageWorkspaceBound(t *testing.T) {
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.WorkspaceIdLabel: "ws1"},
	}}
	res, err := r.unmanage(context.Background(), node, nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestAuthorizeClusterAccessNoCluster(t *testing.T) {
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	// No spec cluster -> nil.
	assert.NoError(t, r.authorizeClusterAccess(context.Background(), &v1.Node{}, sshClient))
}

func TestAuthorizeClusterAccessViaSSH(t *testing.T) {
	scheme, _ := genMockScheme()
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Spec.Cluster = ptrString("c1")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "primus-safe"},
		Data:       map[string][]byte{"username": []byte("root"), "authorize.pub": []byte("ssh-rsa AAA")},
	}
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme, cluster, secret))
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	// cluster-level secret present, node not yet authorized -> appends key via SSH.
	assert.NoError(t, r.authorizeClusterAccess(context.Background(), node, sshClient))
}

func TestSyncOrCreateScaleDownPodExisting(t *testing.T) {
	scheme, _ := genMockScheme()
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Status.MachineStatus.HostName = "host1"
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "primus-safe",
			Labels: map[string]string{
				v1.ClusterManageClusterLabel: "c1",
				v1.ClusterManageNodeLabel:    "host1",
				v1.ClusterManageActionLabel:  string(v1.ClusterScaleDownAction),
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
	}
	cl := ctrlfakeNewClient(scheme, cluster, node, pod)
	r := newMockNodeReconciler(cl)
	cs := k8sfakeClientset()
	res, err := r.syncOrCreateScaleDownPod(context.Background(), cs, node, &corev1.Node{}, "c1")
	assert.NoError(t, err)
	assert.True(t, res.RequeueAfter > 0)
	assert.Equal(t, v1.NodeUnmanaging, node.Status.ClusterStatus.Phase)
}

func TestUnmanageK8sNodeNilWithReset(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Status.ClusterStatus.Cluster = ptrString("c1")
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme, node))

	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	patches := gomonkeyApplyGetSSHClient(sshClient)
	defer patches.Reset()

	res, err := r.unmanage(context.Background(), node, nil)
	assert.NoError(t, err)
	assert.True(t, res.RequeueAfter > 0)
	assert.Equal(t, v1.NodeUnmanaged, node.Status.ClusterStatus.Phase)
}

func TestUnmanageK8sNodeNilNoReboot(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.NodeUnmanageNoRebootLabel: v1.TrueStr},
	}}
	node.Status.ClusterStatus.Cluster = ptrString("c1")
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme, node))

	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	patches := gomonkeyApplyGetSSHClient(sshClient)
	defer patches.Reset()

	res, err := r.unmanage(context.Background(), node, nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestManageWithK8sNodeFullPath(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.ClusterIdLabel: "c1"},
	}}
	node.Spec.Cluster = ptrString("c1")
	node.Status.MachineStatus.Phase = v1.NodeReady
	node.Status.MachineStatus.UpdateTime = &metav1.Time{Time: timeNowResource()}
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "primus-safe"},
		Data:       map[string][]byte{"username": []byte("root"), "authorize.pub": []byte("ssh-rsa AAA")},
	}
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme, node, cluster, secret))

	cs := k8sfakeClientset()
	k8sNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "host1"}}
	_, _ = cs.CoreV1().Nodes().Create(context.Background(), k8sNode, metav1.CreateOptions{})

	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	p1 := gomonkeyApplyGetK8sFactory(cs)
	defer p1.Reset()
	p2 := gomonkeyApplyGetSSHClient(sshClient)
	defer p2.Reset()

	// k8sNode present but cluster not yet on it -> sync labels, resolvconf, addons, delete pods, mark managed.
	res, err := r.manage(context.Background(), node, k8sNode)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
	assert.Equal(t, v1.NodeManaged, node.Status.ClusterStatus.Phase)
}

func TestUpdateMachineStatus(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1.Node{}).WithObjects(node).Build()
	r := newMockNodeReconciler(cl)
	err := r.updateMachineStatus(context.Background(), node, "host1", v1.NodeReady)
	assert.NoError(t, err)
	assert.Equal(t, v1.NodeReady, node.Status.MachineStatus.Phase)
	// No change -> no-op.
	assert.NoError(t, r.updateMachineStatus(context.Background(), node, "host1", v1.NodeReady))
}

func TestSyncMachineStatusHostnameFailed(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1.Node{}).WithObjects(node).Build()
	r := newMockNodeReconciler(cl)
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	patches := gomonkeyApplyGetSSHClient(sshClient)
	defer patches.Reset()
	// hostname command returns empty -> hostname failed status.
	err := r.syncMachineStatus(context.Background(), node)
	assert.NoError(t, err)
	assert.Equal(t, v1.NodeHostnameFailed, node.Status.MachineStatus.Phase)
}

func TestCleanupTimeoutPodsNoOp(t *testing.T) {
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	// Phase not managing/unmanaging -> no-op.
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	assert.NoError(t, r.cleanupTimeoutPods(context.Background(), node))
}

func TestNodeUpdateK8sNodeViaFactory(t *testing.T) {
	scheme, _ := genMockScheme()
	adminNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.ClusterIdLabel: "c1"},
	}}
	adminNode.Spec.Cluster = ptrString("c1")
	k8sNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "k8s-n1"}}
	cs := k8sfake.NewSimpleClientset(k8sNode)
	cl := ctrlfakeNewClient(scheme, adminNode)
	r := newMockNodeReconciler(cl)

	patches := gomonkeyApplyGetK8sFactory(cs)
	defer patches.Reset()

	res, err := r.updateK8sNode(context.Background(), adminNode, k8sNode)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestNodeClearConditions(t *testing.T) {
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	cs := k8sfake.NewSimpleClientset()
	adminNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	k8sNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "k8s-n1"},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
			{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
		}},
	}
	// No primus conditions -> nothing to update.
	assert.NoError(t, r.clearConditions(context.Background(), adminNode, cs, k8sNode))
}

func TestRemoveResolvConfLockViaSSH(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	patches := gomonkeyApplyGetSSHClient(sshClient)
	defer patches.Reset()
	assert.NoError(t, r.removeResolvConfLock(context.Background(), node))
}

func TestInstallHarborCertSuccess(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	scheme, _ := genMockScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "harbor-tls", Namespace: "harbor"},
		Data:       map[string][]byte{"ca.crt": []byte("cacert")},
	}
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme, secret))
	ok, err := r.installHarborCert(context.Background(), sshClient)
	assert.NoError(t, err)
	assert.True(t, ok)
}

func TestInstallHarborCertNoCAKey(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	scheme, _ := genMockScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "harbor-tls", Namespace: "harbor"},
		Data:       map[string][]byte{},
	}
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme, secret))
	ok, err := r.installHarborCert(context.Background(), sshClient)
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestInstallAddonsCreatesJob(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Spec.Cluster = ptrString("c1")
	node.Spec.NodeTemplate = &corev1.ObjectReference{Name: "tmpl1"}
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme, node))
	err := r.installAddons(context.Background(), node)
	assert.NoError(t, err)
}

func TestSyncOrCreateScaleUpPodCreates(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Spec.Cluster = ptrString("c1")
	node.Spec.PrivateIP = "10.0.0.1"
	node.Status.MachineStatus.Phase = v1.NodeReady
	node.Status.MachineStatus.HostName = "host1"
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	cluster.Spec.ControlPlane.Nodes = []string{"n1"}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "primus-safe"},
		Data:       map[string][]byte{"username": []byte("root")},
	}
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme, node, cluster, secret))
	patches := gomonkeyApplyGetSSHClient(sshClient)
	defer patches.Reset()
	// No scale-up pods exist -> resetNode (ssh), generate hosts, create pod.
	_, err := r.syncOrCreateScaleUpPod(context.Background(), node)
	assert.NoError(t, err)
	assert.Equal(t, v1.NodeManaging, node.Status.ClusterStatus.Phase)
}

func TestNodeDeleteK8sNodeViaFactory(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.ClusterIdLabel: "c1", v1.NodeHostnameLabel: "host1"},
	}}
	node.Spec.Cluster = ptrString("c1")
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme, node))
	cs := k8sfakeClientset()
	k8sNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: node.GetK8sNodeName()}}
	_, _ = cs.CoreV1().Nodes().Create(context.Background(), k8sNode, metav1.CreateOptions{})
	patches := gomonkeyApplyGetK8sFactory(cs)
	defer patches.Reset()
	res, err := r.deleteK8sNode(context.Background(), node)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestCleanupTimeoutPodsManaging(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Spec.Cluster = ptrString("c1")
	node.Status.ClusterStatus.Phase = v1.NodeManaging
	oldPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:              "p1",
		Namespace:         "primus-safe",
		CreationTimestamp: metav1.NewTime(timeNowResource().Add(-2 * time.Hour)),
		Labels: map[string]string{
			v1.ClusterManageClusterLabel: "c1",
			v1.ClusterManageNodeLabel:    "n1",
		},
	}}
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme, node, oldPod))
	assert.NoError(t, r.cleanupTimeoutPods(context.Background(), node))
}

func TestSyncOrCreateScaleDownPodCreates(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Spec.Cluster = ptrString("c1")
	node.Spec.PrivateIP = "10.0.0.1"
	node.Status.MachineStatus.Phase = v1.NodeReady
	node.Status.MachineStatus.HostName = "host1"
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	cluster.Spec.ControlPlane.Nodes = []string{"n1"}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "primus-safe"},
		Data:       map[string][]byte{"username": []byte("root")},
	}
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme, node, cluster, secret))
	patches := gomonkeyApplyGetSSHClient(sshClient)
	defer patches.Reset()
	cs := k8sfakeClientset()
	k8sNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "host1"}}
	_, err := r.syncOrCreateScaleDownPod(context.Background(), cs, node, k8sNode, "c1")
	assert.NoError(t, err)
	assert.Equal(t, v1.NodeUnmanaging, node.Status.ClusterStatus.Phase)
}

func TestManageNotReady(t *testing.T) {
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Spec.Cluster = ptrString("c1")
	// Machine not ready -> requeue 30s.
	res, err := r.manage(context.Background(), node, nil)
	assert.NoError(t, err)
	assert.True(t, res.RequeueAfter > 0)
}

func TestManageAlreadyManaged(t *testing.T) {
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.ClusterIdLabel: "c1"},
	}}
	node.Spec.Cluster = ptrString("c1")
	node.Status.MachineStatus.Phase = v1.NodeReady
	node.Status.ClusterStatus.Phase = v1.NodeManaged
	k8sNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}},
	}
	res, err := r.manage(context.Background(), node, k8sNode)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestSyncControlPlaneNodeStatus(t *testing.T) {
	scheme, _ := genMockScheme()
	cl := ctrlfakeNewClient(scheme)
	r := newMockNodeReconciler(cl)
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Spec.Cluster = ptrString("c1")
	// No create pods -> managing phase.
	assert.NoError(t, r.syncControlPlaneNodeStatus(context.Background(), node))
	assert.Equal(t, v1.NodeManaging, node.Status.ClusterStatus.Phase)
}

func TestSyncLabelsToK8sNode(t *testing.T) {
	cs := k8sfakeClientset()
	k8sNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	_, _ = cs.CoreV1().Nodes().Create(context.Background(), k8sNode, metav1.CreateOptions{})
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	adminNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "admin1",
		Labels: map[string]string{"custom": "v"},
	}}
	adminNode.Spec.Cluster = ptrString("c1")
	err := r.syncLabelsToK8sNode(context.Background(), cs, adminNode, k8sNode)
	assert.NoError(t, err)
	updated, _ := cs.CoreV1().Nodes().Get(context.Background(), "n1", metav1.GetOptions{})
	assert.Equal(t, "v", updated.Labels["custom"])
}
