package resource

import (
	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func genMockAdminNode(name, clusterName string, nf *v1.NodeFlavor) *v1.Node {
	return &v1.Node{
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
		},
		Status: v1.NodeStatus{
			MachineStatus: v1.MachineStatus{
				Phase:    v1.NodeReady,
				HostName: name,
			},
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
