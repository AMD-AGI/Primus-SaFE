/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"golang.org/x/crypto/ssh"
	"gotest.tools/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/pointer"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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
				Product:      "AMD MI300X",
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
	resp, err := r.observeTaints(context.Background(), adminNode)
	assert.NilError(t, err)
	assert.Equal(t, resp, true)

	adminNode.Spec.Taints = []corev1.Taint{{
		Key: commonfaults.GenerateTaintKey("001"),
	}}
	adminNode.Status.Taints = []corev1.Taint{{
		Key: "001",
	}}
	adminNode.Status.Taints = []corev1.Taint{}
	resp, err = r.observeTaints(context.Background(), adminNode)
	assert.NilError(t, err)
	assert.Equal(t, resp, false)
}

func TestObserveNodeAction(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode := genMockAdminNode("node1", clusterName, nodeFlavor)

	r := newMockNodeReconciler(nil)
	resp, _ := r.observeLabelAction(context.Background(), adminNode)
	assert.Equal(t, resp, true)
	resp, _ = r.observeAnnotationAction(context.Background(), adminNode)
	assert.Equal(t, resp, true)

	metav1.SetMetaDataAnnotation(&adminNode.ObjectMeta, v1.NodeLabelAction,
		string(jsonutils.MarshalSilently(map[string]string{"test.key": v1.NodeActionRemove})))
	resp, _ = r.observeLabelAction(context.Background(), adminNode)
	assert.Equal(t, resp, false)
	resp, _ = r.observeAnnotationAction(context.Background(), adminNode)
	assert.Equal(t, resp, true)
}

func TestObserveNodeCluster(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode := genMockAdminNode("node1", clusterName, nodeFlavor)

	r := newMockNodeReconciler(nil)
	resp, _ := r.observeCluster(context.Background(), adminNode)
	assert.Equal(t, resp, true)
	adminNode.Spec.Cluster = nil
	resp, _ = r.observeCluster(context.Background(), adminNode)
	assert.Equal(t, resp, false)
}

func TestObserveNodeWorkspace(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	adminNode := genMockAdminNode("node1", clusterName, nodeFlavor)

	r := newMockNodeReconciler(nil)
	resp, _ := r.observeWorkspace(context.Background(), adminNode)
	assert.Equal(t, resp, true)
	adminNode.Spec.Workspace = ptr.To("workspace")
	resp, _ = r.observeWorkspace(context.Background(), adminNode)
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

	patches1 := gomonkey.ApplyFunc(ssh.Dial, func(network string, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
		return &ssh.Client{}, nil
	})
	defer patches1.Reset()
	patches2 := gomonkey.ApplyFunc(getHostname, func(conn *ssh.Client) (string, error) {
		return adminNode.Name, nil
	})
	defer patches2.Reset()

	mockScheme, err := genMockScheme()
	adminClient := fake.NewClientBuilder().WithObjects(adminNode, secret).WithStatusSubresource(adminNode).WithScheme(mockScheme).Build()
	r := newMockNodeReconciler(adminClient)

	_, err = r.syncMachineStatus(context.Background(), adminNode)
	assert.NilError(t, err)
	err = adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode.Name}, adminNode)
	assert.NilError(t, err)
	assert.Equal(t, adminNode.IsReady(), true)
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
	taintKey := commonfaults.GenerateTaintKey("001")
	k8sNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{{Key: taintKey, Effect: corev1.TaintEffectNoSchedule}},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{
				Type:   corev1.NodeConditionType(taintKey),
				Status: corev1.ConditionTrue,
			}, {
				Type:   corev1.NodeConditionType(commonfaults.GenerateTaintKey("002")),
				Status: corev1.ConditionTrue,
			}, {
				Type:   corev1.NodeConditionType("Ready"),
				Status: corev1.ConditionTrue,
			}, {
				Type:   corev1.NodeConditionType(v1.OpsJobKind),
				Status: corev1.ConditionTrue,
				Reason: "test-ops-job",
			}},
		},
	}
	k8sClient := k8sfake.NewClientset(k8sNode)
	err := clearConditions(context.Background(), k8sClient, k8sNode, "test-ops-job")
	assert.NilError(t, err)

	k8sNode2, err := k8sClient.CoreV1().Nodes().Get(context.Background(), k8sNode.Name, metav1.GetOptions{})
	assert.NilError(t, err)
	assert.Equal(t, len(k8sNode2.Status.Conditions), 2)
	assert.Equal(t, k8sNode2.Status.Conditions[0].Type, corev1.NodeConditionType(taintKey))
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

	patches1 := gomonkey.ApplyFunc(ssh.Dial, func(network string, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
		return &ssh.Client{}, nil
	})
	defer patches1.Reset()
	patches2 := gomonkey.ApplyFunc(isAlreadyAuthorized, func(username string, secret *corev1.Secret, sshClient *ssh.Client) (bool, error) {
		return true, nil
	})
	defer patches2.Reset()

	mockScheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(adminNode, secret, cluster).
		WithStatusSubresource(adminNode).WithScheme(mockScheme).Build()
	k8sNode := genMockK8sNode(adminNode.Name, "", "", "")
	k8sClient := k8sfake.NewClientset(k8sNode)
	r := newMockNodeReconciler(adminClient)
	k8sClients := commonclient.NewClientFactoryWithOnlyClient(context.Background(), cluster.Name, k8sClient)
	r.clientManager.AddOrReplace(cluster.Name, k8sClients)

	assert.Equal(t, v1.GetClusterId(k8sNode), "")
	assert.Equal(t, v1.GetNodeFlavorId(k8sNode), "")
	assert.Equal(t, adminNode.IsManaged(), false)
	ok := isCommandSuccessful(adminNode.Status.ClusterStatus.CommandStatus, utils.Authorize)
	assert.Equal(t, ok, false)
	assert.Equal(t, adminNode.Status.ClusterStatus.Cluster == nil, true)

	_, err = r.updateAdminNode(context.Background(), adminNode, k8sNode)
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

	_, err = r.updateAdminNode(context.Background(), adminNode, nil)
	assert.NilError(t, err)

	labelSelector := client.MatchingLabels{v1.ClusterManageClusterLabel: cluster.Name, v1.ClusterManageNodeLabel: adminNode.Name}
	pods, err := r.getPodList(context.Background(), labelSelector)
	assert.NilError(t, err)
	assert.Equal(t, len(pods), 1)
}

func TestManagingControlPlaneNode(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	cluster := genMockCluster()
	adminNode := genMockAdminNode("node1", "", nodeFlavor)
	adminNode.OwnerReferences = addOwnerReferences(adminNode.OwnerReferences, cluster)
	adminNode.Spec.Cluster = ptr.To(cluster.Name)
	adminNode.Status.ClusterStatus.CommandStatus = []v1.CommandStatus{{
		Name:  utils.Authorize,
		Phase: v1.CommandSucceeded,
	}}

	mockScheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(adminNode, cluster).
		WithStatusSubresource(adminNode).WithScheme(mockScheme).Build()
	r := newMockNodeReconciler(adminClient)

	_, err = r.updateAdminNode(context.Background(), adminNode, nil)
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
	adminNode.Spec.SSHSecret = commonutils.GenObjectReference(secret.TypeMeta, secret.ObjectMeta)
	adminNode.Spec.Cluster = nil
	adminNode.Status.ClusterStatus = v1.NodeClusterStatus{
		Cluster: ptr.To(cluster.Name),
		Phase:   v1.NodeManaged,
	}

	mockScheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(adminNode, secret, cluster).
		WithStatusSubresource(adminNode).WithScheme(mockScheme).Build()
	r := newMockNodeReconciler(adminClient)

	_, err = r.updateAdminNode(context.Background(), adminNode, nil)
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

	_, err = r.updateAdminNode(context.Background(), adminNode, k8sNode)
	time.Sleep(time.Millisecond * 200)
	assert.NilError(t, err)

	clusterName := *adminNode.Status.ClusterStatus.Cluster
	labelSelector := client.MatchingLabels{v1.ClusterManageClusterLabel: clusterName, v1.ClusterManageNodeLabel: adminNode.Name}
	pods, err := r.getPodList(context.Background(), labelSelector)
	assert.NilError(t, err)
	assert.Equal(t, len(pods), 1)
}
