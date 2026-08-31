/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"
	"time"

	testifyassert "github.com/stretchr/testify/assert"

	"github.com/agiledragon/gomonkey/v2"
	"golang.org/x/crypto/ssh"
	"gotest.tools/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/pointer"
	"k8s.io/utils/ptr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

func genMockScheme() (*runtime.Scheme, error) {
	result := runtime.NewScheme()
	err := v1.AddToScheme(result)
	if err != nil {
		return nil, err
	}
	err = corev1.AddToScheme(result)
	if err != nil {
		return nil, err
	}
	err = appsv1.AddToScheme(result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func genMockCluster() *v1.Cluster {
	return &v1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1.ClusterKind,
			APIVersion: "amd.com/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName("cluster"),
		},
	}
}

func genMockAdminNode(name, clusterName string, nf *v1.NodeFlavor) *v1.Node {
	n := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				v1.DisplayNameLabel:  name,
				v1.ClusterIdLabel:    clusterName,
				v1.NodeFlavorIdLabel: nf.Name,
			},
		},
		Spec: v1.NodeSpec{
			NodeFlavor: commonutils.GenObjectReference(nf.TypeMeta, nf.ObjectMeta),
			Cluster:    pointer.String(clusterName),
			Port:       pointer.Int32(22),
		},
		Status: v1.NodeStatus{
			MachineStatus: v1.MachineStatus{
				Phase:    v1.NodeReady,
				HostName: name,
			},
		},
	}
	if clusterName != "" {
		n.Status.ClusterStatus = v1.NodeClusterStatus{
			Phase:   v1.NodeManaged,
			Cluster: pointer.String(clusterName),
		}
	}
	return n
}

func genMockNodeFlavor() *v1.NodeFlavor {
	memQuantity, _ := resource.ParseQuantity("1024Gi")
	return &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName("nodeFlavor"),
		},
		Spec: v1.NodeFlavorSpec{
			Cpu: v1.CpuChip{
				Product:  "AMD 9554",
				Quantity: *resource.NewQuantity(256, resource.DecimalSI),
			},
			Memory: memQuantity,
			Gpu: &v1.GpuChip{
				ResourceName: common.AmdGpu,
				Product:      "MI300X",
				Quantity:     *resource.NewQuantity(8, resource.DecimalSI),
			},
		},
	}
}

func genMockK8sNode(nodeName, clusterName, nodeFlavor, workspace string) *corev1.Node {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Labels: map[string]string{
				v1.ClusterIdLabel:    clusterName,
				v1.NodeFlavorIdLabel: nodeFlavor,
			},
		},
	}
	if workspace != "" {
		node.Labels[v1.WorkspaceIdLabel] = workspace
	}
	return node
}

func genMockSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-secret", Namespace: common.PrimusSafeNamespace},
		Data:       map[string][]byte{"user": []byte(`user-name`), "password": []byte(`user-password`)},
	}
}

func genMockNamespace() *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: common.PrimusSafeNamespace,
		},
	}
}

func newMockNodeReconciler(adminClient client.Client) NodeReconciler {
	return NodeReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: adminClient,
		},
		clientManager: commonutils.NewObjectManagerSingleton(),
	}
}

func TestGetK8sNode(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode := genMockAdminNode("node1", clusterName, nodeFlavor)
	adminClient := fake.NewClientBuilder().WithObjects(adminNode).WithScheme(scheme.Scheme).Build()
	k8sNode := genMockK8sNode(adminNode.Name, clusterName, nodeFlavor.Name, "")
	k8sClient := k8sfake.NewClientset(k8sNode)

	r := newMockNodeReconciler(adminClient)
	k8sClients := commonclient.NewClientFactoryWithOnlyClient(context.Background(), clusterName, k8sClient)
	r.clientManager.AddOrReplace(clusterName, k8sClients)
	node, _, err := r.getK8sNode(context.Background(), adminNode)
	assert.NilError(t, err)
	assert.Equal(t, node != nil, true)
	assert.Equal(t, node.Name, k8sNode.Name)
}

func TestObserveNode(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode := genMockAdminNode("node1", clusterName, nodeFlavor)
	// Set UpdateTime to recent time so shouldSyncMachineStatus returns false
	now := metav1.Now()
	adminNode.Status.MachineStatus.UpdateTime = &now
	k8sNode := genMockK8sNode(adminNode.Name, clusterName, nodeFlavor.Name, "")

	r := newMockNodeReconciler(nil)
	resp, err := r.observe(context.Background(), adminNode, k8sNode)
	assert.NilError(t, err)
	assert.Equal(t, resp, true)
}

func TestObserveNodeTaints(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode := genMockAdminNode("node1", clusterName, nodeFlavor)
	adminNode.Spec.Taints = []corev1.Taint{{
		Key: commonfaults.GenerateTaintKey("001"),
	}}
	adminNode.Status.Taints = []corev1.Taint{{
		Key: commonfaults.GenerateTaintKey("001"),
	}}
	r := newMockNodeReconciler(nil)
	resp, err := r.observeTaints(context.Background(), adminNode, nil)
	assert.NilError(t, err)
	assert.Equal(t, resp, true)

	adminNode.Spec.Taints = []corev1.Taint{{
		Key: commonfaults.GenerateTaintKey("001"),
	}}
	adminNode.Status.Taints = []corev1.Taint{{
		Key: "001",
	}}
	adminNode.Status.Taints = []corev1.Taint{}
	resp, err = r.observeTaints(context.Background(), adminNode, nil)
	assert.NilError(t, err)
	assert.Equal(t, resp, false)
}

func TestObserveNodeAction(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode := genMockAdminNode("node1", clusterName, nodeFlavor)

	r := newMockNodeReconciler(nil)
	resp, _ := r.observeLabelAction(context.Background(), adminNode, nil)
	assert.Equal(t, resp, true)
	resp, _ = r.observeAnnotationAction(context.Background(), adminNode, nil)
	assert.Equal(t, resp, true)

	metav1.SetMetaDataAnnotation(&adminNode.ObjectMeta, v1.NodeLabelAction,
		string(jsonutils.MarshalSilently(map[string]string{"test.key": v1.NodeActionRemove})))
	resp, _ = r.observeLabelAction(context.Background(), adminNode, nil)
	assert.Equal(t, resp, false)
	resp, _ = r.observeAnnotationAction(context.Background(), adminNode, nil)
	assert.Equal(t, resp, true)
}

func TestObserveNodeWorkspace(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode := genMockAdminNode("node1", clusterName, nodeFlavor)

	r := newMockNodeReconciler(nil)
	resp, _ := r.observeWorkspace(context.Background(), adminNode, nil)
	assert.Equal(t, resp, true)
	adminNode.Spec.Workspace = ptr.To("workspace")
	resp, _ = r.observeWorkspace(context.Background(), adminNode, nil)
	assert.Equal(t, resp, false)
}

func TestSyncMachineStatus(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode := genMockAdminNode("node1", clusterName, nodeFlavor)
	adminNode.Status.MachineStatus = v1.MachineStatus{
		Phase: NodeNotReady,
	}
	secret := genMockSecret()
	adminNode.Spec.SSHSecret = commonutils.GenObjectReference(secret.TypeMeta, secret.ObjectMeta)

	// Mock GetSSHClient to avoid SSH connection issues
	patches1 := gomonkey.ApplyFunc(utils.GetSSHClient, func(ctx context.Context, cli client.Client, node *v1.Node) (*ssh.Client, error) {
		// Return nil to simulate successful connection that will be handled in defer
		return nil, nil
	})
	defer patches1.Reset()
	patches2 := gomonkey.ApplyFunc(getHostname, func(conn *ssh.Client) (string, error) {
		return adminNode.Name, nil
	})
	defer patches2.Reset()

	mockScheme, err := genMockScheme()
	adminClient := fake.NewClientBuilder().WithObjects(adminNode, secret).WithStatusSubresource(adminNode).WithScheme(mockScheme).Build()
	r := newMockNodeReconciler(adminClient)

	err = r.syncMachineStatus(context.Background(), adminNode)
	assert.NilError(t, err)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode.Name}, adminNode)
	assert.NilError(t, err)
	assert.Equal(t, adminNode.IsMachineReady(), true)
	assert.Equal(t, adminNode.GetK8sNodeName(), adminNode.Name)
}

func TestUpdateK8sNode(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode := genMockAdminNode("node1", clusterName, nodeFlavor)
	metav1.SetMetaDataLabel(&adminNode.ObjectMeta, "test-key", "test-value")
	metav1.SetMetaDataAnnotation(&adminNode.ObjectMeta, v1.NodeLabelAction,
		string(jsonutils.MarshalSilently(map[string]string{"test-key": v1.NodeActionAdd})))
	adminClient := fake.NewClientBuilder().WithObjects(adminNode).
		WithStatusSubresource(adminNode).WithScheme(scheme.Scheme).Build()

	k8sNode := genMockK8sNode(adminNode.Name, clusterName, nodeFlavor.Name, "")
	k8sClient := k8sfake.NewClientset(k8sNode)
	r := newMockNodeReconciler(adminClient)
	k8sClients := commonclient.NewClientFactoryWithOnlyClient(context.Background(), clusterName, k8sClient)
	r.clientManager.AddOrReplace(clusterName, k8sClients)
	assert.Equal(t, v1.GetNodeLabelAction(adminNode) != "", true)

	_, err := r.updateK8sNode(context.Background(), adminNode, k8sNode)
	assert.NilError(t, err)

	k8sNode2, err := k8sClient.CoreV1().Nodes().Get(context.Background(), k8sNode.Name, metav1.GetOptions{})
	assert.NilError(t, err)
	assert.Equal(t, k8sNode2.Labels["test-key"], "test-value")
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode.Name}, adminNode)
	assert.NilError(t, err)
	assert.Equal(t, v1.GetNodeLabelAction(adminNode) != "", false)
}

func TestUpdateK8sNodeTaints(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode := genMockAdminNode("node1", clusterName, nodeFlavor)
	adminNode.Spec.Taints = []corev1.Taint{{Key: commonfaults.GenerateTaintKey("001"), Effect: corev1.TaintEffectNoSchedule}}
	k8sNode := genMockK8sNode(adminNode.Name, clusterName, nodeFlavor.Name, "")
	k8sNode.Spec.Taints = []corev1.Taint{{Key: NodeNotReady, Effect: corev1.TaintEffectNoSchedule}}

	r := newMockNodeReconciler(nil)
	resp := r.updateK8sNodeTaints(adminNode, k8sNode)
	assert.Equal(t, resp, true)
	assert.Equal(t, len(k8sNode.Spec.Taints), 2)
	assert.Equal(t, k8sNode.Spec.Taints[0].Key, NodeNotReady)
	assert.Equal(t, k8sNode.Spec.Taints[1].Key, "primus-safe.001")
}

func TestUpdateK8sNodeLabel(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode := genMockAdminNode("node1", clusterName, nodeFlavor)
	metav1.SetMetaDataLabel(&adminNode.ObjectMeta, "test-key", "test-value")
	metav1.SetMetaDataAnnotation(&adminNode.ObjectMeta, v1.NodeLabelAction,
		string(jsonutils.MarshalSilently(
			map[string]string{"test-key": v1.NodeActionAdd, "test-key2": v1.NodeActionRemove})))
	k8sNode := genMockK8sNode(adminNode.Name, clusterName, nodeFlavor.Name, "")
	metav1.SetMetaDataLabel(&k8sNode.ObjectMeta, "test-key2", "test-value2")
	metav1.SetMetaDataAnnotation(&k8sNode.ObjectMeta, "test-key2", "test-value2")

	r := newMockNodeReconciler(nil)
	resp := r.updateK8sNodeLabels(adminNode, k8sNode)
	assert.Equal(t, resp, true)
	assert.Equal(t, k8sNode.Labels["test-key"], "test-value")
	assert.Equal(t, k8sNode.Labels["test-key2"], "")
	assert.Equal(t, k8sNode.Annotations["test-key2"], "test-value2")
}

func TestUpdateK8sNodeAnnotation(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode := genMockAdminNode("node1", clusterName, nodeFlavor)
	metav1.SetMetaDataAnnotation(&adminNode.ObjectMeta, "test-key", "test-value")
	metav1.SetMetaDataAnnotation(&adminNode.ObjectMeta, v1.NodeAnnotationAction,
		string(jsonutils.MarshalSilently(
			map[string]string{"test-key": v1.NodeActionAdd, "test-key2": v1.NodeActionRemove})))
	k8sNode := genMockK8sNode(adminNode.Name, clusterName, nodeFlavor.Name, "")
	metav1.SetMetaDataLabel(&k8sNode.ObjectMeta, "test-key2", "test-value2")
	metav1.SetMetaDataAnnotation(&k8sNode.ObjectMeta, "test-key2", "test-value2")

	r := newMockNodeReconciler(nil)
	resp := r.updateK8sNodeAnnotations(adminNode, k8sNode)
	assert.Equal(t, resp, true)
	assert.Equal(t, k8sNode.Annotations["test-key"], "test-value")
	assert.Equal(t, k8sNode.Labels["test-key2"], "test-value2")
	assert.Equal(t, k8sNode.Annotations["test-key2"], "")
}

func TestUpdateK8sWorkspace(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode := genMockAdminNode("node1", clusterName, nodeFlavor)
	adminNode.Spec.Workspace = ptr.To("workspace")
	k8sNode := genMockK8sNode(adminNode.Name, clusterName, nodeFlavor.Name, "")

	r := newMockNodeReconciler(nil)
	resp := r.updateK8sNodeWorkspace(adminNode, k8sNode)
	assert.Equal(t, resp, true)
	assert.Equal(t, v1.GetWorkspaceId(k8sNode), "workspace")

	adminNode.Spec.Workspace = nil
	resp = r.updateK8sNodeWorkspace(adminNode, k8sNode)
	assert.Equal(t, resp, true)
	assert.Equal(t, v1.GetWorkspaceId(k8sNode), "")
}

func TestClearConditions(t *testing.T) {
	monitorId001 := "001"
	monitorId002 := "002"
	taintKey001 := commonfaults.GenerateTaintKey(monitorId001)
	taintKey002 := commonfaults.GenerateTaintKey(monitorId002)
	nodeName := "node1"

	adminNode := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
	}

	k8sNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeConditionType(taintKey001),
					Status: corev1.ConditionTrue,
				},
				{
					Type:   corev1.NodeConditionType(taintKey002),
					Status: corev1.ConditionTrue,
				},
				{
					Type:   corev1.NodeConditionType("Ready"),
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	fault001 := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonfaults.GenerateFaultId(nodeName, monitorId001),
			Labels: map[string]string{
				v1.NodeIdLabel: nodeName,
			},
		},
		Spec: v1.FaultSpec{
			MonitorId: monitorId001,
		},
	}

	mockScheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithScheme(mockScheme).WithObjects(fault001).Build()
	reconciler := newMockNodeReconciler(adminClient)

	k8sClient := k8sfake.NewClientset(k8sNode)
	err = reconciler.clearConditions(context.Background(), adminNode, k8sClient, k8sNode)
	assert.NilError(t, err)

	k8sNode2, err := k8sClient.CoreV1().Nodes().Get(context.Background(), k8sNode.Name, metav1.GetOptions{})
	assert.NilError(t, err)
	// Should keep 2 conditions:
	// - taintKey001 (Primus condition with existing fault)
	// - Ready (non-Primus condition)
	// Should remove:
	// - taintKey002 (Primus condition without fault)
	assert.Equal(t, len(k8sNode2.Status.Conditions), 2)
	assert.Equal(t, k8sNode2.Status.Conditions[0].Type, corev1.NodeConditionType(taintKey001))
	assert.Equal(t, k8sNode2.Status.Conditions[1].Type, corev1.NodeConditionType("Ready"))
}

func TestManageNodeSuccessfully(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	cluster := genMockCluster()
	adminNode := genMockAdminNode("node1", "", nodeFlavor)
	secret := genMockSecret()
	secret.Name = cluster.Name
	adminNode.Spec.SSHSecret = commonutils.GenObjectReference(secret.TypeMeta, secret.ObjectMeta)
	adminNode.Spec.Cluster = ptr.To(cluster.Name)
	now := metav1.Now()
	adminNode.Status.MachineStatus.UpdateTime = &now
	adminNode.Status.ClusterStatus.CommandStatus = []v1.CommandStatus{
		{Name: utils.Authorize, Phase: v1.CommandSucceeded},
		{Name: HarborCA, Phase: v1.CommandSucceeded},
	}

	mockScheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(adminNode, secret, cluster).
		WithStatusSubresource(adminNode).WithScheme(mockScheme).Build()
	k8sNode := genMockK8sNode(adminNode.Name, "", "", "")
	k8sClient := k8sfake.NewClientset(k8sNode)
	r := newMockNodeReconciler(adminClient)
	k8sClients := commonclient.NewClientFactoryWithOnlyClient(context.Background(), cluster.Name, k8sClient)
	r.clientManager.AddOrReplace(cluster.Name, k8sClients)

	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	patches := gomonkeyApplyGetSSHClient(sshClient)
	defer patches.Reset()

	assert.Equal(t, v1.GetClusterId(k8sNode), "")
	assert.Equal(t, v1.GetNodeFlavorId(k8sNode), "")
	assert.Equal(t, adminNode.IsManaged(), false)
	ok := isCommandSuccessful(adminNode.Status.ClusterStatus.CommandStatus, utils.Authorize)
	assert.Equal(t, ok, true)
	assert.Equal(t, adminNode.Status.ClusterStatus.Cluster == nil, true)

	_, err = r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: adminNode.Name}})
	assert.NilError(t, err)

	k8sNode2, err := k8sClient.CoreV1().Nodes().Get(context.Background(), k8sNode.Name, metav1.GetOptions{})
	assert.NilError(t, err)
	assert.Equal(t, v1.GetClusterId(k8sNode2), cluster.Name)
	assert.Equal(t, v1.GetNodeFlavorId(k8sNode2), nodeFlavor.Name)

	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode.Name}, adminNode)
	assert.NilError(t, err)
	assert.Equal(t, adminNode.IsManaged(), true)
	ok = isCommandSuccessful(adminNode.Status.ClusterStatus.CommandStatus, utils.Authorize)
	assert.Equal(t, ok, true)
	assert.Equal(t, adminNode.Status.ClusterStatus.Cluster == nil, false)
	assert.Equal(t, *adminNode.Status.ClusterStatus.Cluster, cluster.Name)
}

func TestManagingNode(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	cluster := genMockCluster()
	adminNode := genMockAdminNode("node1", "", nodeFlavor)
	secret := genMockSecret()
	secret.Name = cluster.Name
	adminNode.Spec.SSHSecret = commonutils.GenObjectReference(secret.TypeMeta, secret.ObjectMeta)
	adminNode.Spec.Cluster = ptr.To(cluster.Name)
	adminNode.Status.ClusterStatus.CommandStatus = []v1.CommandStatus{{
		Name:  utils.Authorize,
		Phase: v1.CommandSucceeded,
	}}

	mockScheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(adminNode, secret, cluster).
		WithStatusSubresource(adminNode).WithScheme(mockScheme).Build()
	r := newMockNodeReconciler(adminClient)

	err = r.Update(context.Background(), adminNode)
	assert.NilError(t, err)
}

func TestManagingControlPlaneNode(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	cluster := genMockCluster()
	adminNode := genMockAdminNode("node1", "", nodeFlavor)
	adminNode.Spec.Cluster = ptr.To(cluster.Name)
	adminNode.Labels[v1.KubernetesControlPlane] = "true"
	now := metav1.Now()
	adminNode.Status.MachineStatus.UpdateTime = &now
	adminNode.Status.ClusterStatus.CommandStatus = []v1.CommandStatus{{
		Name:  utils.Authorize,
		Phase: v1.CommandSucceeded,
	}, {
		Name:  HarborCA,
		Phase: v1.CommandSucceeded,
	}}

	mockScheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(adminNode, cluster).
		WithStatusSubresource(adminNode).WithScheme(mockScheme).Build()
	r := newMockNodeReconciler(adminClient)
	k8sClient := k8sfake.NewClientset()
	k8sClients := commonclient.NewClientFactoryWithOnlyClient(context.Background(), cluster.Name, k8sClient)
	r.clientManager.AddOrReplace(cluster.Name, k8sClients)

	_, err = r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: adminNode.Name}})
	assert.NilError(t, err)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode.Name}, adminNode)
	assert.NilError(t, err)
	assert.Equal(t, adminNode.Status.ClusterStatus.Phase, v1.NodeManaging)
}

func TestUnmanageNodeSuccessfully(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	cluster := genMockCluster()
	secret := genMockSecret()
	secret.Name = cluster.Name
	adminNode := genMockAdminNode("node1", cluster.Name, nodeFlavor)
	adminNode.Labels[v1.NodeUnmanageNoRebootLabel] = v1.TrueStr
	adminNode.Spec.SSHSecret = commonutils.GenObjectReference(secret.TypeMeta, secret.ObjectMeta)
	adminNode.Spec.Cluster = nil
	now := metav1.Now()
	adminNode.Status.MachineStatus.UpdateTime = &now
	adminNode.Status.ClusterStatus = v1.NodeClusterStatus{
		Cluster: ptr.To(cluster.Name),
		Phase:   v1.NodeManaged,
	}

	mockScheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(adminNode, secret, cluster).
		WithStatusSubresource(adminNode).WithScheme(mockScheme).Build()
	r := newMockNodeReconciler(adminClient)
	k8sClient := k8sfake.NewClientset()
	k8sClients := commonclient.NewClientFactoryWithOnlyClient(context.Background(), cluster.Name, k8sClient)
	r.clientManager.AddOrReplace(cluster.Name, k8sClients)

	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	patches := gomonkeyApplyGetSSHClient(sshClient)
	defer patches.Reset()

	_, err = r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: adminNode.Name}})
	assert.NilError(t, err)

	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode.Name}, adminNode)
	assert.NilError(t, err)
	assert.Equal(t, adminNode.Status.ClusterStatus.Cluster == nil, true)
	assert.Equal(t, adminNode.Status.ClusterStatus.Phase, v1.NodeUnmanaged)
}

func TestUnmanagingNode(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	ns := genMockNamespace()
	cluster := genMockCluster()
	secret := genMockSecret()
	secret.Name = cluster.Name
	adminNode := genMockAdminNode("node1", cluster.Name, nodeFlavor)
	adminNode.Spec.SSHSecret = commonutils.GenObjectReference(secret.TypeMeta, secret.ObjectMeta)
	adminNode.Spec.Cluster = nil
	adminNode.Status.ClusterStatus = v1.NodeClusterStatus{
		Cluster: ptr.To(cluster.Name),
		Phase:   v1.NodeManaged,
	}

	mockScheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(ns, adminNode, secret, cluster).
		WithStatusSubresource(adminNode).WithScheme(mockScheme).Build()
	k8sNode := genMockK8sNode(adminNode.Name, "", "", "")

	k8sClient := k8sfake.NewClientset(k8sNode, ns)
	r := newMockNodeReconciler(adminClient)
	k8sClients := commonclient.NewClientFactoryWithOnlyClient(context.Background(), cluster.Name, k8sClient)
	r.clientManager.AddOrReplace(cluster.Name, k8sClients)

	_, err = r.updateK8sNode(context.Background(), adminNode, k8sNode)
	time.Sleep(time.Millisecond * 200)
	assert.NilError(t, err)
}

// --- merged from node_observe_full_test.go ---

func newNodeReconcilerFull(t *testing.T, cs *k8sfake.Clientset, objs ...ctrlclient.Object) *NodeReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	testifyassert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1.Node{}).WithObjects(objs...).Build()
	mgr := commonutils.NewObjectManager()
	_ = mgr.Add("c1", commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs))
	return &NodeReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl, clientSet: cs},
		clientManager:         mgr,
	}
}

func TestNodeObserveCleanChain(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	r := newNodeReconcilerFull(t, cs)
	adminNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	_, err := r.observe(context.Background(), adminNode, nil)
	testifyassert.NoError(t, err)

	// drive the individual observe helpers directly for a clean node
	clean := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2"}}
	for _, f := range []func(context.Context, *v1.Node, *corev1.Node) (bool, error){
		r.observeTaints, r.observeLabelAction, r.observeAnnotationAction, r.observeWorkspace, r.observeCluster,
	} {
		ok, err := f(context.Background(), clean, nil)
		testifyassert.NoError(t, err)
		testifyassert.True(t, ok)
	}
}

func TestDeleteK8sNodeFull(t *testing.T) {
	ctx := context.Background()

	// empty cluster/k8s name -> no-op
	cs0 := k8sfake.NewSimpleClientset()
	r0 := newNodeReconcilerFull(t, cs0)
	_, err := r0.deleteK8sNode(ctx, &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n0"}})
	testifyassert.NoError(t, err)

	// with a valid factory and k8s node name -> deletes the node
	k8sNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "kn1"}}
	cs := k8sfake.NewSimpleClientset(k8sNode)
	adminNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	v1.SetLabel(adminNode, v1.ClusterIdLabel, "c1")
	adminNode.Status.MachineStatus.HostName = "kn1"
	r := newNodeReconcilerFull(t, cs, adminNode)
	_, err = r.deleteK8sNode(ctx, adminNode)
	testifyassert.NoError(t, err)
	_, err = cs.CoreV1().Nodes().Get(ctx, "kn1", metav1.GetOptions{})
	testifyassert.Error(t, err)
}

// --- merged from node_predicate_test.go ---

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
	testifyassert.False(t, r.isNodeRelevantFieldChanged(old, same))

	// Machine phase changed.
	changed := old.DeepCopy()
	changed.Status.MachineStatus.Phase = v1.NodePhase("Other")
	testifyassert.True(t, r.isNodeRelevantFieldChanged(old, changed))

	// Deletion timestamp set.
	deleting := old.DeepCopy()
	now := metav1.Now()
	deleting.DeletionTimestamp = &now
	testifyassert.True(t, r.isNodeRelevantFieldChanged(old, deleting))
}

func TestNodeRelevantChangePredicate(t *testing.T) {
	r := newMockNodeReconciler(nil)
	p := r.relevantChangePredicate()
	old := &v1.Node{}
	changed := &v1.Node{}
	changed.Status.MachineStatus.Phase = v1.NodeReady
	testifyassert.True(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: changed}))
	testifyassert.False(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: old.DeepCopy()}))
}

func TestNodeHandlePodEvent(t *testing.T) {
	r := newMockNodeReconciler(nil)
	h := r.handlePodEvent()
	// Just ensure the handler is constructed and callbacks don't panic on non-pod.
	testifyassert.NotNil(t, h)
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
	testifyassert.Contains(t, out, "base64 -d | bash")
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
	testifyassert.NoError(t, forceDeleteK8sNode(context.Background(), cs, "n1"))
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
	testifyassert.NoError(t, err)
	testifyassert.Len(t, pods, 1)
	testifyassert.NoError(t, r.deletePods(context.Background(), "c1", "n1", "scale-up"))
}

func TestNodeInstallAddonsNoTemplate(t *testing.T) {
	scheme, _ := genMockScheme()
	cl := ctrlfakeNewClient(scheme)
	r := newMockNodeReconciler(cl)
	// No node template -> no-op.
	testifyassert.NoError(t, r.installAddons(context.Background(), &v1.Node{}))
}

func TestNodeExecuteSSHCommand(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	r := newMockNodeReconciler(nil)
	testifyassert.NoError(t, r.executeSSHCommand(sshClient, "echo hi"))
}

func TestNodeInstallHarborCertNoSecret(t *testing.T) {
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	// No harbor-tls secret -> returns false, nil.
	ok, err := r.installHarborCert(context.Background(), sshClient)
	testifyassert.NoError(t, err)
	testifyassert.False(t, ok)
}

func TestCleanupNodeAfterUnmanage(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.ClusterIdLabel: "c1", v1.WorkspaceIdLabel: "ws1"},
	}}
	cl := ctrlfakeNewClient(scheme, node)
	r := newMockNodeReconciler(cl)
	testifyassert.NoError(t, r.cleanupNodeAfterUnmanage(context.Background(), node))
	assert.Equal(t, "", v1.GetClusterId(node))

	// No change -> nil.
	clean := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2"}}
	testifyassert.NoError(t, r.cleanupNodeAfterUnmanage(context.Background(), clean))
}

// A node leaving the cluster is not on its way to any workspace. Left on, the reservation
// comes back with the node and keeps every workspace but one off it, with nothing left in
// the system to say why.
func TestCleanupNodeAfterUnmanageDropsTheMigrationReservation(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.ClusterIdLabel: "c1", v1.WorkspaceIdLabel: "ws1"},
	}}
	v1.SetNodeMigrateInfo(node, &v1.NodeMigrateInfo{From: "ws1", Target: "ws2"})
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme, node))

	testifyassert.NoError(t, r.cleanupNodeAfterUnmanage(context.Background(), node))
	testifyassert.Nil(t, v1.GetNodeMigrateInfo(node))
}

func TestProcessNodeManagementCleanup(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Status.MachineStatus.Phase = v1.NodeReady
	cl := ctrlfakeNewClient(scheme, node)
	r := newMockNodeReconciler(cl)
	// No spec cluster, no status cluster, no k8sNode, machine ready -> cleanup path.
	res, err := r.processNodeManagement(context.Background(), node, nil)
	testifyassert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestProcessNodeManagementNotReady(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	cl := ctrlfakeNewClient(scheme, node)
	r := newMockNodeReconciler(cl)
	// Machine not ready -> requeue 30s.
	res, err := r.processNodeManagement(context.Background(), node, nil)
	testifyassert.NoError(t, err)
	testifyassert.True(t, res.RequeueAfter > 0)
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
	testifyassert.NoError(t, r.syncClusterStatus(context.Background(), node))
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
	testifyassert.NoError(t, err)
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
	testifyassert.NoError(t, r.resetNode(context.Background(), node))
}

func TestModifyResolvConfViaSSH(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}

	patches := gomonkeyApplyGetSSHClient(sshClient)
	defer patches.Reset()
	testifyassert.NoError(t, r.modifyResolvConf(context.Background(), node))
}

func TestUnmanageControlPlaneNode(t *testing.T) {
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.KubernetesControlPlane: ""},
	}}
	res, err := r.unmanage(context.Background(), node, nil)
	testifyassert.NoError(t, err)
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
	testifyassert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestAuthorizeClusterAccessNoCluster(t *testing.T) {
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	// No spec cluster -> nil.
	testifyassert.NoError(t, r.authorizeClusterAccess(context.Background(), &v1.Node{}, sshClient))
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
	testifyassert.NoError(t, r.authorizeClusterAccess(context.Background(), node, sshClient))
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
	testifyassert.NoError(t, err)
	testifyassert.True(t, res.RequeueAfter > 0)
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
	testifyassert.NoError(t, err)
	testifyassert.True(t, res.RequeueAfter > 0)
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
	testifyassert.NoError(t, err)
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
	testifyassert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
	assert.Equal(t, v1.NodeManaged, node.Status.ClusterStatus.Phase)
}

func TestUpdateMachineStatus(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1.Node{}).WithObjects(node).Build()
	r := newMockNodeReconciler(cl)
	err := r.updateMachineStatus(context.Background(), node, "host1", v1.NodeReady)
	testifyassert.NoError(t, err)
	assert.Equal(t, v1.NodeReady, node.Status.MachineStatus.Phase)
	// No change -> no-op.
	testifyassert.NoError(t, r.updateMachineStatus(context.Background(), node, "host1", v1.NodeReady))
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
	testifyassert.NoError(t, err)
	assert.Equal(t, v1.NodeHostnameFailed, node.Status.MachineStatus.Phase)
}

func TestCleanupTimeoutPodsNoOp(t *testing.T) {
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	// Phase not managing/unmanaging -> no-op.
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	testifyassert.NoError(t, r.cleanupTimeoutPods(context.Background(), node))
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
	testifyassert.NoError(t, err)
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
	testifyassert.NoError(t, r.clearConditions(context.Background(), adminNode, cs, k8sNode))
}

func TestRemoveResolvConfLockViaSSH(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	patches := gomonkeyApplyGetSSHClient(sshClient)
	defer patches.Reset()
	testifyassert.NoError(t, r.removeResolvConfLock(context.Background(), node))
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
	testifyassert.NoError(t, err)
	testifyassert.True(t, ok)
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
	testifyassert.NoError(t, err)
	testifyassert.False(t, ok)
}

func TestInstallAddonsCreatesJob(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Spec.Cluster = ptrString("c1")
	node.Spec.NodeTemplate = &corev1.ObjectReference{Name: "tmpl1"}
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme, node))
	err := r.installAddons(context.Background(), node)
	testifyassert.NoError(t, err)
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
	testifyassert.NoError(t, err)
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
	testifyassert.NoError(t, err)
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
	testifyassert.NoError(t, r.cleanupTimeoutPods(context.Background(), node))
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
	testifyassert.NoError(t, err)
	assert.Equal(t, v1.NodeUnmanaging, node.Status.ClusterStatus.Phase)
}

func TestManageNotReady(t *testing.T) {
	scheme, _ := genMockScheme()
	r := newMockNodeReconciler(ctrlfakeNewClient(scheme))
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Spec.Cluster = ptrString("c1")
	// Machine not ready -> requeue 30s.
	res, err := r.manage(context.Background(), node, nil)
	testifyassert.NoError(t, err)
	testifyassert.True(t, res.RequeueAfter > 0)
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
	testifyassert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestSyncControlPlaneNodeStatus(t *testing.T) {
	scheme, _ := genMockScheme()
	cl := ctrlfakeNewClient(scheme)
	r := newMockNodeReconciler(cl)
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Spec.Cluster = ptrString("c1")
	// No create pods -> managing phase.
	testifyassert.NoError(t, r.syncControlPlaneNodeStatus(context.Background(), node))
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
	testifyassert.NoError(t, err)
	updated, _ := cs.CoreV1().Nodes().Get(context.Background(), "n1", metav1.GetOptions{})
	assert.Equal(t, "v", updated.Labels["custom"])
}

// --- merged from node_reconcile_extra_test.go ---

func TestNodeReconcileNotFound(t *testing.T) {
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	r := newMockNodeReconciler(cl)
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	testifyassert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestNodeReconcileNoCluster(t *testing.T) {
	// Node without cluster: getK8sNode returns nothing, reconcile completes with a requeue.
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(node).WithStatusSubresource(node).Build()
	r := newMockNodeReconciler(cl)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "n1"}})
	testifyassert.NoError(t, err)
}

func TestNodeReconcileDelete(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:       "n1",
		Finalizers: []string{v1.NodeFinalizer},
	}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(node).WithStatusSubresource(node).Build()
	// Trigger deletion (object keeps existing because of finalizer).
	testifyassert.NoError(t, cl.Delete(context.Background(), node))
	r := newMockNodeReconciler(cl)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "n1"}})
	testifyassert.NoError(t, err)
	// Finalizer removed -> node fully gone.
	err = cl.Get(context.Background(), client.ObjectKey{Name: "n1"}, &v1.Node{})
	testifyassert.Error(t, err)
}

func TestNodeDeleteK8sNodeNoCluster(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	r := newMockNodeReconciler(cl)
	res, err := r.deleteK8sNode(context.Background(), node)
	testifyassert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}
