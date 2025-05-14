package resource

import (
	"context"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func genMockAdminNode(name, clusterName string, nf *v1.NodeFlavor) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				v1.DisplayNameLabel:  name,
				v1.NodeFlavorIdLabel: nf.Name,
			},
		},
		Spec: v1.NodeSpec{
			NodeFlavor: commonutils.GenObjectReference(nf.TypeMeta, nf.ObjectMeta),
			Cluster:    pointer.String(clusterName),
		},
	}
}

func genMockNodeFlavor() *v1.NodeFlavor {
	memQuantity, _ := resource.ParseQuantity("1024Gi")
	return &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName("nodeFlavor"),
		},
		Spec: v1.NodeFlavorSpec{
			FlavorType: v1.BareMetal,
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

func getMockAdminNode(name string) (*v1.Node, error) {
	result := &v1.Node{}
	err := mockClient.Get(context.Background(), client.ObjectKey{Name: name}, result)
	return result, err
}

func getMockAdminNodes(clusterName string) ([]v1.Node, error) {
	labelSelector := labels.SelectorFromSet(map[string]string{v1.ClusterIdLabel: clusterName})
	nodeList := &v1.NodeList{}
	err := mockClient.List(context.Background(), nodeList, &client.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

func getMockNodeFlavor(name string) (*v1.NodeFlavor, error) {
	result := &v1.NodeFlavor{}
	err := mockClient.Get(context.Background(), client.ObjectKey{Name: name}, result)
	return result, err
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

func createMockK8sNode(name, cluster, nodeflavor, workspace string) *corev1.Node {
	node := genMockK8sNode(name, cluster, nodeflavor, workspace)
	createObject(node)

	patch := client.MergeFrom(node.DeepCopy())
	node.Spec.Taints = []corev1.Taint{}
	err := mockClient.Patch(context.Background(), node, patch)
	Expect(err).Should(BeNil())

	nf, err := getMockNodeFlavor(nodeflavor)
	Expect(err).Should(BeNil())
	node.Status.Allocatable = corev1.ResourceList{
		corev1.ResourceCPU:    nf.Spec.Cpu.Quantity,
		corev1.ResourceMemory: nf.Spec.Memory,
		common.AmdGpu:         nf.Spec.Gpu.Quantity,
	}
	err = mockClient.Status().Update(context.Background(), node)
	Expect(err).Should(BeNil())

	time.Sleep(time.Millisecond * 200)
	return node
}
