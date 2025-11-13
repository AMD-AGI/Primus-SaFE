package reconciler

import (
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestIsGPUNode(t *testing.T) {
	n := &NodeReconciler{}

	tests := []struct {
		name     string
		node     *corev1.Node
		expected bool
	}{
		{
			name: "有AMD GPU的节点",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu-node-1",
				},
				Status: corev1.NodeStatus{
					Capacity: corev1.ResourceList{
						corev1.ResourceName(metadata.GetResourceName(metadata.GpuVendorAMD)): resource.MustParse("8"),
					},
				},
			},
			expected: true,
		},
		{
			name: "无GPU的节点",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cpu-node-1",
				},
				Status: corev1.NodeStatus{
					Capacity: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("64"),
						corev1.ResourceMemory: resource.MustParse("256Gi"),
					},
				},
			},
			expected: false,
		},
		{
			name: "空Capacity的节点",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "empty-node",
				},
				Status: corev1.NodeStatus{
					Capacity: corev1.ResourceList{},
				},
			},
			expected: false,
		},
		{
			name: "有多种资源包括GPU的节点",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mixed-node",
				},
				Status: corev1.NodeStatus{
					Capacity: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("32"),
						corev1.ResourceMemory: resource.MustParse("128Gi"),
						corev1.ResourceName(metadata.GetResourceName(metadata.GpuVendorAMD)): resource.MustParse("4"),
					},
				},
			},
			expected: true,
		},
		{
			name: "GPU数量为0的节点",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "zero-gpu-node",
				},
				Status: corev1.NodeStatus{
					Capacity: corev1.ResourceList{
						corev1.ResourceName(metadata.GetResourceName(metadata.GpuVendorAMD)): resource.MustParse("0"),
					},
				},
			},
			expected: true, // 只要有这个资源键就返回 true
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := n.isGPUNode(tt.node)
			assert.Equal(t, tt.expected, result, "GPU node detection mismatch")
		})
	}
}

func TestGetNodeAddress(t *testing.T) {
	n := &NodeReconciler{}

	tests := []struct {
		name     string
		node     *corev1.Node
		expected string
	}{
		{
			name: "有内部IP地址",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: "192.168.1.100",
						},
					},
				},
			},
			expected: "192.168.1.100",
		},
		{
			name: "有外部IP地址",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-2",
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeExternalIP,
							Address: "203.0.113.50",
						},
					},
				},
			},
			expected: "203.0.113.50",
		},
		{
			name: "有多个地址-返回第一个",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-3",
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: "10.0.0.5",
						},
						{
							Type:    corev1.NodeExternalIP,
							Address: "198.51.100.10",
						},
						{
							Type:    corev1.NodeHostName,
							Address: "node-3.example.com",
						},
					},
				},
			},
			expected: "10.0.0.5",
		},
		{
			name: "空地址列表",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-4",
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{},
				},
			},
			expected: "",
		},
		{
			name: "nil地址列表",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-5",
				},
				Status: corev1.NodeStatus{},
			},
			expected: "",
		},
		{
			name: "只有主机名",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-6",
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeHostName,
							Address: "node-6.local",
						},
					},
				},
			},
			expected: "node-6.local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := n.getNodeAddress(tt.node)
			assert.Equal(t, tt.expected, result, "Node address mismatch")
		})
	}
}

func TestConvertTaintsToExtType(t *testing.T) {
	n := &NodeReconciler{}

	now := metav1.Now()

	tests := []struct {
		name     string
		taints   []corev1.Taint
		validate func(t *testing.T, result model.ExtType)
	}{
		{
			name:   "空taints",
			taints: []corev1.Taint{},
			validate: func(t *testing.T, result model.ExtType) {
				assert.Empty(t, result, "Result should be empty for empty taints")
			},
		},
		{
			name:   "nil taints",
			taints: nil,
			validate: func(t *testing.T, result model.ExtType) {
				assert.Empty(t, result, "Result should be empty for nil taints")
			},
		},
		{
			name: "单个taint-无TimeAdded",
			taints: []corev1.Taint{
				{
					Key:    "node.kubernetes.io/not-ready",
					Value:  "true",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
			validate: func(t *testing.T, result model.ExtType) {
				assert.NotEmpty(t, result)
				taintsList, ok := result["taints"].([]map[string]interface{})
				assert.True(t, ok, "taints should be a slice of maps")
				assert.Len(t, taintsList, 1)
				
				taint := taintsList[0]
				assert.Equal(t, "node.kubernetes.io/not-ready", taint["key"])
				assert.Equal(t, "true", taint["value"])
				assert.Equal(t, string(corev1.TaintEffectNoSchedule), taint["effect"])
				_, hasTimeAdded := taint["timeAdded"]
				assert.False(t, hasTimeAdded, "Should not have timeAdded field")
			},
		},
		{
			name: "单个taint-有TimeAdded",
			taints: []corev1.Taint{
				{
					Key:       "node.kubernetes.io/memory-pressure",
					Value:     "true",
					Effect:    corev1.TaintEffectNoExecute,
					TimeAdded: &now,
				},
			},
			validate: func(t *testing.T, result model.ExtType) {
				assert.NotEmpty(t, result)
				taintsList, ok := result["taints"].([]map[string]interface{})
				assert.True(t, ok)
				assert.Len(t, taintsList, 1)
				
				taint := taintsList[0]
				assert.Equal(t, "node.kubernetes.io/memory-pressure", taint["key"])
				assert.Equal(t, "true", taint["value"])
				assert.Equal(t, string(corev1.TaintEffectNoExecute), taint["effect"])
				timeAdded, hasTimeAdded := taint["timeAdded"]
				assert.True(t, hasTimeAdded, "Should have timeAdded field")
				assert.Equal(t, now.Time, timeAdded)
			},
		},
		{
			name: "多个taints",
			taints: []corev1.Taint{
				{
					Key:    "node.kubernetes.io/not-ready",
					Value:  "true",
					Effect: corev1.TaintEffectNoSchedule,
				},
				{
					Key:       "node.kubernetes.io/disk-pressure",
					Value:     "true",
					Effect:    corev1.TaintEffectNoExecute,
					TimeAdded: &now,
				},
				{
					Key:    "dedicated",
					Value:  "gpu-workload",
					Effect: corev1.TaintEffectPreferNoSchedule,
				},
			},
			validate: func(t *testing.T, result model.ExtType) {
				assert.NotEmpty(t, result)
				taintsList, ok := result["taints"].([]map[string]interface{})
				assert.True(t, ok)
				assert.Len(t, taintsList, 3)
				
				// 验证第一个 taint
				assert.Equal(t, "node.kubernetes.io/not-ready", taintsList[0]["key"])
				assert.Equal(t, string(corev1.TaintEffectNoSchedule), taintsList[0]["effect"])
				
				// 验证第二个 taint（带 TimeAdded）
				assert.Equal(t, "node.kubernetes.io/disk-pressure", taintsList[1]["key"])
				assert.Equal(t, string(corev1.TaintEffectNoExecute), taintsList[1]["effect"])
				_, hasTimeAdded := taintsList[1]["timeAdded"]
				assert.True(t, hasTimeAdded)
				
				// 验证第三个 taint
				assert.Equal(t, "dedicated", taintsList[2]["key"])
				assert.Equal(t, "gpu-workload", taintsList[2]["value"])
				assert.Equal(t, string(corev1.TaintEffectPreferNoSchedule), taintsList[2]["effect"])
			},
		},
		{
			name: "空值的taint",
			taints: []corev1.Taint{
				{
					Key:    "test-key",
					Value:  "",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
			validate: func(t *testing.T, result model.ExtType) {
				taintsList := result["taints"].([]map[string]interface{})
				assert.Len(t, taintsList, 1)
				assert.Equal(t, "", taintsList[0]["value"])
			},
		},
		{
			name: "所有Effect类型",
			taints: []corev1.Taint{
				{
					Key:    "effect-noschedule",
					Effect: corev1.TaintEffectNoSchedule,
				},
				{
					Key:    "effect-prefernoschedule",
					Effect: corev1.TaintEffectPreferNoSchedule,
				},
				{
					Key:    "effect-noexecute",
					Effect: corev1.TaintEffectNoExecute,
				},
			},
			validate: func(t *testing.T, result model.ExtType) {
				taintsList := result["taints"].([]map[string]interface{})
				assert.Len(t, taintsList, 3)
				assert.Equal(t, string(corev1.TaintEffectNoSchedule), taintsList[0]["effect"])
				assert.Equal(t, string(corev1.TaintEffectPreferNoSchedule), taintsList[1]["effect"])
				assert.Equal(t, string(corev1.TaintEffectNoExecute), taintsList[2]["effect"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := n.convertTaintsToExtType(tt.taints)
			tt.validate(t, result)
		})
	}
}

func TestDesiredKubeletService(t *testing.T) {
	n := &NodeReconciler{}

	svc := n.desiredKubeletService()

	// 验证基本属性
	assert.NotNil(t, svc, "Service should not be nil")
	assert.Equal(t, "primus-lens-kubelet-service", svc.Name)
	assert.Equal(t, "kube-system", svc.Namespace)

	// 验证标签
	assert.Equal(t, "primus-lens", svc.Labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "kubelet", svc.Labels["app.kubernetes.io/name"])
	assert.Equal(t, "kubelet", svc.Labels["k8s-app"])

	// 验证 Spec
	assert.Equal(t, "None", svc.Spec.ClusterIP)
	assert.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type)

	// 验证端口配置
	assert.Len(t, svc.Spec.Ports, 3, "Should have 3 ports")

	// 验证 https-metrics 端口
	httpsPort := findServicePort(svc.Spec.Ports, "https-metrics")
	assert.NotNil(t, httpsPort, "https-metrics port should exist")
	assert.Equal(t, int32(10250), httpsPort.Port)
	assert.Equal(t, corev1.ProtocolTCP, httpsPort.Protocol)
	assert.Equal(t, intstr.FromInt(10250), httpsPort.TargetPort)

	// 验证 http-metrics 端口
	httpPort := findServicePort(svc.Spec.Ports, "http-metrics")
	assert.NotNil(t, httpPort, "http-metrics port should exist")
	assert.Equal(t, int32(10255), httpPort.Port)
	assert.Equal(t, corev1.ProtocolTCP, httpPort.Protocol)

	// 验证 cadvisor 端口
	cadvisorPort := findServicePort(svc.Spec.Ports, "cadvisor")
	assert.NotNil(t, cadvisorPort, "cadvisor port should exist")
	assert.Equal(t, int32(4194), cadvisorPort.Port)
	assert.Equal(t, corev1.ProtocolTCP, cadvisorPort.Protocol)
}

func TestDesireKubeletServiceEndpoint(t *testing.T) {
	n := &NodeReconciler{}

	tests := []struct {
		name     string
		nodes    *corev1.NodeList
		validate func(t *testing.T, ep *corev1.Endpoints)
	}{
		{
			name: "空节点列表",
			nodes: &corev1.NodeList{
				Items: []corev1.Node{},
			},
			validate: func(t *testing.T, ep *corev1.Endpoints) {
				assert.NotNil(t, ep)
				assert.Equal(t, "primus-lens-kubelet-service", ep.Name)
				assert.Equal(t, "kube-system", ep.Namespace)
				assert.Len(t, ep.Subsets, 1)
				assert.Empty(t, ep.Subsets[0].Addresses)
				assert.Len(t, ep.Subsets[0].Ports, 3)
			},
		},
		{
			name: "单个节点-有内部IP",
			nodes: &corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
							UID:  "uid-1",
						},
						Status: corev1.NodeStatus{
							Addresses: []corev1.NodeAddress{
								{
									Type:    corev1.NodeInternalIP,
									Address: "192.168.1.10",
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, ep *corev1.Endpoints) {
				assert.Len(t, ep.Subsets, 1)
				assert.Len(t, ep.Subsets[0].Addresses, 1)
				
				addr := ep.Subsets[0].Addresses[0]
				assert.Equal(t, "192.168.1.10", addr.IP)
				assert.Equal(t, "node-1", *addr.NodeName)
				assert.Equal(t, "Node", addr.TargetRef.Kind)
				assert.Equal(t, "node-1", addr.TargetRef.Name)
			},
		},
		{
			name: "单个节点-有外部IP",
			nodes: &corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-2",
							UID:  "uid-2",
						},
						Status: corev1.NodeStatus{
							Addresses: []corev1.NodeAddress{
								{
									Type:    corev1.NodeExternalIP,
									Address: "203.0.113.20",
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, ep *corev1.Endpoints) {
				assert.Len(t, ep.Subsets[0].Addresses, 1)
				assert.Equal(t, "203.0.113.20", ep.Subsets[0].Addresses[0].IP)
			},
		},
		{
			name: "单个节点-有内部IP和外部IP",
			nodes: &corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-3",
							UID:  "uid-3",
						},
						Status: corev1.NodeStatus{
							Addresses: []corev1.NodeAddress{
								{
									Type:    corev1.NodeInternalIP,
									Address: "10.0.0.5",
								},
								{
									Type:    corev1.NodeExternalIP,
									Address: "198.51.100.30",
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, ep *corev1.Endpoints) {
				// 应该包含两个地址
				assert.Len(t, ep.Subsets[0].Addresses, 2)
				assert.Equal(t, "10.0.0.5", ep.Subsets[0].Addresses[0].IP)
				assert.Equal(t, "198.51.100.30", ep.Subsets[0].Addresses[1].IP)
			},
		},
		{
			name: "多个节点",
			nodes: &corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
							UID:  "uid-1",
						},
						Status: corev1.NodeStatus{
							Addresses: []corev1.NodeAddress{
								{Type: corev1.NodeInternalIP, Address: "192.168.1.10"},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-2",
							UID:  "uid-2",
						},
						Status: corev1.NodeStatus{
							Addresses: []corev1.NodeAddress{
								{Type: corev1.NodeInternalIP, Address: "192.168.1.11"},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-3",
							UID:  "uid-3",
						},
						Status: corev1.NodeStatus{
							Addresses: []corev1.NodeAddress{
								{Type: corev1.NodeInternalIP, Address: "192.168.1.12"},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, ep *corev1.Endpoints) {
				assert.Len(t, ep.Subsets[0].Addresses, 3)
				
				ips := []string{}
				for _, addr := range ep.Subsets[0].Addresses {
					ips = append(ips, addr.IP)
				}
				assert.Contains(t, ips, "192.168.1.10")
				assert.Contains(t, ips, "192.168.1.11")
				assert.Contains(t, ips, "192.168.1.12")
			},
		},
		{
			name: "节点无有效地址",
			nodes: &corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-no-addr",
							UID:  "uid-no-addr",
						},
						Status: corev1.NodeStatus{
							Addresses: []corev1.NodeAddress{
								{
									Type:    corev1.NodeHostName,
									Address: "node-no-addr.local",
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, ep *corev1.Endpoints) {
				// 只有 HostName，不应该被包含
				assert.Empty(t, ep.Subsets[0].Addresses)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep := n.desireKubeletServiceEndpoint(tt.nodes)
			
			// 通用验证
			assert.NotNil(t, ep, "Endpoints should not be nil")
			assert.Equal(t, "primus-lens-kubelet-service", ep.Name)
			assert.Equal(t, "kube-system", ep.Namespace)
			
			// 验证端口配置
			assert.Len(t, ep.Subsets[0].Ports, 3)
			
			ports := ep.Subsets[0].Ports
			httpsPort := findEndpointPort(ports, "https-metrics")
			assert.NotNil(t, httpsPort)
			assert.Equal(t, int32(10250), httpsPort.Port)
			
			httpPort := findEndpointPort(ports, "http-metrics")
			assert.NotNil(t, httpPort)
			assert.Equal(t, int32(10255), httpPort.Port)
			
			cadvisorPort := findEndpointPort(ports, "cadvisor")
			assert.NotNil(t, cadvisorPort)
			assert.Equal(t, int32(4194), cadvisorPort.Port)
			
			// 自定义验证
			tt.validate(t, ep)
		})
	}
}

// 辅助函数：查找 Service Port
func findServicePort(ports []corev1.ServicePort, name string) *corev1.ServicePort {
	for i := range ports {
		if ports[i].Name == name {
			return &ports[i]
		}
	}
	return nil
}

// 辅助函数：查找 Endpoint Port
func findEndpointPort(ports []corev1.EndpointPort, name string) *corev1.EndpointPort {
	for i := range ports {
		if ports[i].Name == name {
			return &ports[i]
		}
	}
	return nil
}

