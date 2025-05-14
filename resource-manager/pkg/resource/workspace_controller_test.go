/*
   Copyright © 01.AI Co., Ltd. 2023-2024. All rights reserved.
*/

package resource

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

var (
	totalNode = 5
)

func genMockControlPlane() v1.ControlPlaneStatus {
	return v1.ControlPlaneStatus{
		Phase:     v1.ReadyPhase,
		CAData:    stringutil.Base64Encode(string(mockEnv.Config.CAData)),
		KeyData:   stringutil.Base64Encode(string(mockEnv.Config.KeyData)),
		CertData:  stringutil.Base64Encode(string(mockEnv.Config.CertData)),
		Endpoints: []string{mockEnv.Config.Host},
	}
}

func genMockCluster() *v1.Cluster {
	return &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       commonutils.GenerateName("cluster"),
			Finalizers: []string{v1.ClusterFinalizer},
		},
		Status: v1.ClusterStatus{
			ControlPlaneStatus: genMockControlPlane(),
		},
	}
}

func genMockWorkspace(clusterName, nodeFlavor string, replica int) *v1.Workspace {
	result := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName("workspace"),
			Labels: map[string]string{
				v1.ClusterIdLabel: clusterName,
			},
		},
		Spec: v1.WorkspaceSpec{
			Cluster:    clusterName,
			NodeFlavor: nodeFlavor,
			Replica:    replica,
		},
	}
	controllerutil.AddFinalizer(result, v1.WorkspaceFinalizer)
	return result
}

func getMockWorkspace(name string) (*v1.Workspace, error) {
	result := &v1.Workspace{}
	err := mockClient.Get(context.Background(), client.ObjectKey{Name: name}, result)
	return result, err
}

func beforeWorkspaceTest(replica int) (*v1.Cluster, []*v1.Node, []*corev1.Node, *v1.Workspace, *v1.NodeFlavor) {
	cluster := genMockCluster()
	createObject(cluster)
	nodeFlavor := genMockNodeFlavor()
	createObject(nodeFlavor)
	workspace := genMockWorkspace(cluster.Name, nodeFlavor.Name, replica)

	nodeNames := make([]string, totalNode)
	for i := 0; i < totalNode; i++ {
		nodeNames[i] = commonutils.GenerateName(fmt.Sprintf("node-%d", i))
	}
	adminNodes := make([]*v1.Node, totalNode)
	for i := 0; i < totalNode; i++ {
		adminNodes[i] = genMockAdminNode(nodeNames[i], cluster.Name, nodeFlavor)
		if i < replica {
			metav1.SetMetaDataLabel(&adminNodes[i].ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)
			adminNodes[i].Spec.Workspace = pointer.String(workspace.Name)
		}
		metav1.SetMetaDataLabel(&adminNodes[i].ObjectMeta, v1.ClusterIdLabel, cluster.Name)
		controllerutil.AddFinalizer(adminNodes[i], v1.NodeFinalizer)
		createObject(adminNodes[i])
	}

	k8sNodes := make([]*corev1.Node, totalNode)
	for i := 0; i < totalNode; i++ {
		if i < replica {
			k8sNodes[i] = createMockK8sNode(nodeNames[i], cluster.Name, nodeFlavor.Name, workspace.Name)
		} else {
			k8sNodes[i] = createMockK8sNode(nodeNames[i], cluster.Name, nodeFlavor.Name, "")
		}
	}
	createObject(workspace)
	time.Sleep(time.Millisecond * 200)
	return cluster, adminNodes, k8sNodes, workspace, nodeFlavor
}

func afterWorkspaceTest(cluster *v1.Cluster, adminNodes []*v1.Node, k8sNodes []*corev1.Node, workspace *v1.Workspace, nodeFlavor *v1.NodeFlavor) {
	for i := range k8sNodes {
		deleteObject(k8sNodes[i])
	}
	for i := range adminNodes {
		deleteObject(k8sNodes[i])
	}
	if workspace != nil {
		deleteObject(workspace)
	}
	if nodeFlavor != nil {
		deleteObject(nodeFlavor)
	}
	time.Sleep(time.Millisecond * 100)
	deleteObject(cluster)
}

func testCreateWorkspace() {
	replica := 3
	cluster, adminNodes, k8sNodes, workspace, nodeflavor := beforeWorkspaceTest(replica)

	var err error
	workspace, err = getMockWorkspace(workspace.Name)
	Expect(err).Should(BeNil())
	Expect(workspace.Status.Phase).To(Equal(v1.WorkspaceRunning))
	Expect(workspace.Status.AvailableReplica).To(Equal(replica))
	Expect(workspace.Status.AbnormalReplica).To(Equal(0))

	afterWorkspaceTest(cluster, adminNodes, k8sNodes, workspace, nodeflavor)
}

func testScaleUpWorkspace() {
	replica := 3
	cluster, adminNodes, k8sNodes, workspace, nodeflavor := beforeWorkspaceTest(replica)

	var err error
	workspace, err = getMockWorkspace(workspace.Name)
	Expect(err).Should(BeNil())
	Expect(workspace.Status.Phase).To(Equal(v1.WorkspaceRunning))

	workspace.Spec.Replica = totalNode
	err = mockClient.Update(context.Background(), workspace)
	Expect(err).Should(BeNil())
	time.Sleep(time.Millisecond * 200)

	workspace, err = getMockWorkspace(workspace.Name)
	Expect(err).Should(BeNil())
	Expect(workspace.Status.Phase).To(Equal(v1.WorkspaceRunning))
	Expect(workspace.Status.AvailableReplica).To(Equal(workspace.Spec.Replica))
	Expect(workspace.Status.AbnormalReplica).To(Equal(0))

	count := getNodesCount(cluster.Name, workspace.Name)
	Expect(count).Should(Equal(workspace.Spec.Replica))

	afterWorkspaceTest(cluster, adminNodes, k8sNodes, workspace, nodeflavor)
}

func testScaleDownWorkspace() {
	replica := 3
	cluster, adminNodes, k8sNodes, workspace, nodeflavor := beforeWorkspaceTest(replica)

	var err error
	workspace, err = getMockWorkspace(workspace.Name)
	Expect(err).Should(BeNil())
	Expect(workspace.Status.Phase).To(Equal(v1.WorkspaceRunning))

	workspace.Spec.Replica = 1
	err = mockClient.Update(context.Background(), workspace)
	Expect(err).Should(BeNil())
	time.Sleep(time.Millisecond * 200)

	workspace, err = getMockWorkspace(workspace.Name)
	Expect(err).Should(BeNil())
	Expect(workspace.Status.Phase).To(Equal(v1.WorkspaceRunning))
	Expect(workspace.Status.AvailableReplica).To(Equal(workspace.Spec.Replica))
	Expect(workspace.Status.AbnormalReplica).To(Equal(0))

	count := getNodesCount(cluster.Name, workspace.Name)
	Expect(count).Should(Equal(workspace.Spec.Replica))

	afterWorkspaceTest(cluster, adminNodes, k8sNodes, workspace, nodeflavor)
}

func testDeleteWorkspace() {
	replica := 3
	cluster, adminNodes, k8sNodes, workspace, nodeflavor := beforeWorkspaceTest(replica)

	count := getNodesCount(cluster.Name, workspace.Name)
	Expect(count).To(Equal(replica))

	deleteObject(workspace)
	time.Sleep(time.Millisecond * 200)

	count = getNodesCount(cluster.Name, workspace.Name)
	Expect(count).To(Equal(0))

	afterWorkspaceTest(cluster, adminNodes, k8sNodes, nil, nodeflavor)
}

func getNodesCount(clusterName, workspaceName string) int {
	nodes, err := getMockAdminNodes(clusterName)
	Expect(err).Should(BeNil())
	count := 0
	for _, node := range nodes {
		if node.GetSpecWorkspace() == workspaceName {
			count++
		}
	}
	return count
}

func testWorkspaceNodesAction() {
	replica := 3
	cluster, adminNodes, k8sNodes, workspace, nodeflavor := beforeWorkspaceTest(replica)

	var nodeToDelete, nodeToAdd1, nodeToAdd2 *v1.Node
	var count int
	for i, n := range adminNodes {
		if n.GetSpecWorkspace() == workspace.Name {
			if nodeToDelete == nil {
				nodeToDelete = adminNodes[i]
			}
			count++
		} else {
			if nodeToAdd1 == nil {
				nodeToAdd1 = adminNodes[i]
			} else if nodeToAdd2 == nil {
				nodeToAdd2 = adminNodes[i]
			}
		}
	}
	Expect(count).To(Equal(replica))

	actions := make(map[string]string)
	actions[nodeToDelete.Name] = v1.NodeActionRemove
	actions[nodeToAdd1.Name] = v1.NodeActionAdd
	actions[nodeToAdd2.Name] = v1.NodeActionAdd

	var err error
	workspace, err = getMockWorkspace(workspace.Name)
	Expect(err).Should(BeNil())
	Expect(workspace.Status.AvailableReplica).To(Equal(workspace.Spec.Replica))
	workspace.Spec.Replica++
	metav1.SetMetaDataAnnotation(&workspace.ObjectMeta,
		v1.NodesWorkspaceAction, string(jsonutils.MarshalSilently(actions)))
	err = mockClient.Update(context.Background(), workspace)
	Expect(err).Should(BeNil())
	time.Sleep(time.Millisecond * 300)

	workspace, err = getMockWorkspace(workspace.Name)
	Expect(err).Should(BeNil())
	Expect(workspace.Status.AvailableReplica).To(Equal(workspace.Spec.Replica))
	n, err := getMockAdminNode(nodeToDelete.Name)
	Expect(err).Should(BeNil())
	Expect(n.GetSpecWorkspace()).To(Equal(""))
	n, err = getMockAdminNode(nodeToAdd1.Name)
	Expect(err).Should(BeNil())
	Expect(n.GetSpecWorkspace()).To(Equal(workspace.Name))
	n, err = getMockAdminNode(nodeToAdd2.Name)
	Expect(err).Should(BeNil())
	Expect(n.GetSpecWorkspace()).To(Equal(workspace.Name))
	Expect(v1.GetNodesWorkspaceAction(workspace)).To(Equal(""))

	count = getNodesCount(cluster.Name, workspace.Name)
	Expect(count).To(Equal(workspace.Spec.Replica))

	afterWorkspaceTest(cluster, adminNodes, k8sNodes, workspace, nodeflavor)
}

func testWorkspaceWithDeletingNode() {
	cluster, adminNodes, k8sNodes, workspace, nodeflavor := beforeWorkspaceTest(totalNode)
	replica := len(adminNodes)

	var err error
	workspace, err = getMockWorkspace(workspace.Name)
	Expect(err).Should(BeNil())
	Expect(workspace.Status.Phase).To(Equal(v1.WorkspaceRunning))
	count := getNodesCount(cluster.Name, workspace.Name)
	Expect(count).To(Equal(workspace.Spec.Replica))

	deleteObject(adminNodes[0])
	time.Sleep(time.Millisecond * 200)

	workspace, err = getMockWorkspace(workspace.Name)
	Expect(err).Should(BeNil())
	Expect(workspace.Spec.Replica).To(Equal(replica))
	Expect(workspace.Status.Phase).To(Equal(v1.WorkspaceRunning))
	Expect(workspace.Status.AvailableReplica).To(Equal(replica - 1))
	Expect(workspace.Status.AbnormalReplica).To(Equal(1))

	count = getNodesCount(cluster.Name, workspace.Name)
	Expect(count).To(Equal(replica - 1))

	afterWorkspaceTest(cluster, adminNodes, k8sNodes, workspace, nodeflavor)
}

func testDeletingAndScalingDown() {
	cluster, adminNodes, k8sNodes, workspace, nodeflavor := beforeWorkspaceTest(totalNode)
	replica := len(adminNodes)

	var err error
	workspace, err = getMockWorkspace(workspace.Name)
	Expect(err).Should(BeNil())
	Expect(workspace.Status.Phase).To(Equal(v1.WorkspaceRunning))
	Expect(workspace.Spec.Replica).To(Equal(replica))
	Expect(workspace.Status.AvailableReplica).To(Equal(replica))

	deleteObject(adminNodes[0])
	time.Sleep(time.Millisecond * 200)
	workspace, err = getMockWorkspace(workspace.Name)
	Expect(err).Should(BeNil())
	Expect(workspace.Status.Phase).To(Equal(v1.WorkspaceRunning))
	Expect(workspace.Spec.Replica).To(Equal(replica))
	Expect(workspace.Status.AvailableReplica).To(Equal(replica - 1))
	Expect(workspace.Status.AbnormalReplica).To(Equal(1))

	patch := client.MergeFrom(workspace.DeepCopy())
	workspace.Spec.Replica -= 1
	err = mockClient.Patch(context.Background(), workspace, patch)
	Expect(err).Should(BeNil())
	time.Sleep(time.Millisecond * 200)

	workspace, err = getMockWorkspace(workspace.Name)
	Expect(err).Should(BeNil())
	Expect(workspace.Status.Phase).To(Equal(v1.WorkspaceRunning))
	Expect(workspace.Spec.Replica).To(Equal(replica - 1))
	Expect(workspace.Status.AvailableReplica).To(Equal(replica - 1))
	Expect(workspace.Status.AbnormalReplica).To(Equal(0))

	afterWorkspaceTest(cluster, adminNodes, k8sNodes, workspace, nodeflavor)
}

var _ = Describe("Workspace Controller Test", func() {
	Context("", func() {
		It("Test creating workspace", func() {
			testCreateWorkspace()
		})

		It("Test scaling up workspace", func() {
			testScaleUpWorkspace()
		})

		It("Test scaling down workspace", func() {
			testScaleDownWorkspace()
		})

		It("Test deleting workspace", func() {
			testDeleteWorkspace()
		})

		It("Test workspace with nodes action", func() {
			testWorkspaceNodesAction()
		})

		It("Test workspace with deleting node", func() {
			testWorkspaceWithDeletingNode()
		})

		It("Test the workspace by first deleting a node, then performing scale-down", func() {
			testDeletingAndScalingDown()
		})
	})
})
