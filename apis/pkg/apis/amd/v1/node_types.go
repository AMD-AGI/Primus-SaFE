/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NodePhase string

const (
	NodeKind = "Node"

	// the phase of NodeClusterStatus
	NodeManaging        NodePhase = "Managing"
	NodeManaged         NodePhase = "Managed"
	NodeManagedFailed   NodePhase = "ManagedFailed"
	NodeUnmanaging      NodePhase = "Unmanaging"
	NodeUnmanaged       NodePhase = "Unmanaged"
	NodeUnmanagedFailed NodePhase = "UnmanagedFailed"

	// the phase of MachineStatus
	NodeReady          NodePhase = "Ready"
	NodeSSHFailed      NodePhase = "SSHFailed"
	NodeHostnameFailed NodePhase = "HostnameFailed"
)

type CommandPhase string

const (
	CommandSucceeded CommandPhase = "Succeeded"
	CommandFailed    CommandPhase = "Failed"
)

type CommandStatus struct {
	// Operational command, such as authorize
	Name string `json:"name,omitempty"`
	// Operation result. such as Succeeded and Failed
	Phase CommandPhase `json:"phase,omitempty"`
}

type NodeSpec struct {
	// The cluster which the node belongs to.
	// If a value is set, it indicates that the node should be managed within the specified cluster,
	// Otherwise, if set to an empty value, it indicates that the node should be unmanaged from the cluster.
	Cluster *string `json:"cluster,omitempty"`
	// The workspace which the node belongs to. This is optional, a node can belong to no workspace.
	// If a value is set, the node will be bound to the specified workspace; otherwise, it will be unbound.
	Workspace *string `json:"workspace,omitempty"`
	// Node flavor reference, required
	NodeFlavor *corev1.ObjectReference `json:"nodeFlavor"`
	// Node template reference, required
	NodeTemplate *corev1.ObjectReference `json:"nodeTemplate"`
	// Node hostname
	Hostname *string `json:"hostname,omitempty"`
	// Node private ip, required
	PrivateIP string `json:"privateIP,omitempty"`
	// Node public IP, accessible from external networks, optional
	PublicIP string `json:"publicIP,omitempty"`
	// SSH port，default is 22
	Port *int32 `json:"port,omitempty"`
	// The taint will be automatically synchronized to the Kubernetes node.
	Taints []corev1.Taint `json:"taints,omitempty"`
	// Secret for ssh
	SSHSecret *corev1.ObjectReference `json:"secret"`
}

type NodeClusterStatus struct {
	// The status of nodes in the cluster, such as Ready, Managing, Managed, ManagedFailed, Unmanaging, Unmanaged, UnmanagedFailed
	Phase NodePhase `json:"phase,omitempty"`
	// The result of cluster binding (note that the cluster in spec represents the desired state,
	// while this field represents the actual outcome of the operation).
	Cluster *string `json:"cluster,omitempty"`
	// The execution result of each install command.
	CommandStatus []CommandStatus `json:"commandStatus,omitempty"`
}

type MachineStatus struct {
	// The hostname of k8s node
	HostName string `json:"hostName,omitempty"`
	// The status of the physical node, such as Ready, SSHFailed, HostnameFailed
	Phase NodePhase `json:"phase,omitempty"`
	// The internalIP of k8s node
	PrivateIP string `json:"privateIP,omitempty"`
	// Reserved field, currently unused.
	CommandStatus []CommandStatus `json:"commandStatus,omitempty"`
}

// NodeStatus defines the observed state of Node.
type NodeStatus struct {
	// The status of the physical node
	MachineStatus MachineStatus `json:"machineStatus,omitempty"`
	// The status of nodes in the cluster
	ClusterStatus NodeClusterStatus `json:"clusterStatus,omitempty"`
	// Indicates whether the node is unschedulable
	Unschedulable bool `json:"unschedulable,omitempty"`
	// Taint automatically synchronized from the Kubernetes node
	Taints []corev1.Taint `json:"taints,omitempty"`
	// All resource information of the node
	Resources corev1.ResourceList `json:"resources,omitempty"`
	// Node condition, automatically synchronized from the Kubernetes node
	Conditions []corev1.NodeCondition `json:"conditions,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:webhook:path=/mutate-amd-primus-safe-v1-node,mutating=true,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=nodes,verbs=create;update,versions=v1,name=mnode.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:webhook:path=/validate-amd-primus-safe-v1-node,mutating=false,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=nodes,verbs=create;update,versions=v1,name=vnode.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:rbac:groups=amd.com,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=amd.com,resources=nodes/status,verbs=get;update;patch

type Node struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeSpec   `json:"spec,omitempty"`
	Status NodeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
type NodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Node `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Node{}, &NodeList{})
}

func (n *Node) IsAvailable(ignoreTaint bool) bool {
	ok, _ := n.CheckAvailable(ignoreTaint)
	return ok
}

func (n *Node) CheckAvailable(ignoreTaint bool) (bool, string) {
	if n == nil {
		return false, "node is empty"
	}
	if !n.IsMachineReady() {
		return false, "node's status is not ready"
	}
	if !n.IsManaged() {
		return false, "node is not managed"
	}
	if !n.GetDeletionTimestamp().IsZero() {
		return false, "node is deleting"
	}
	if n.Status.Unschedulable {
		return false, "node is unschedulable"
	}
	if !ignoreTaint && len(n.Status.Taints) > 0 {
		var taints []string
		for _, t := range n.Status.Taints {
			taints = append(taints, fmt.Sprintf("%s=%s", t.Key, t.Value))
		}
		b, _ := json.Marshal(taints)
		return false, fmt.Sprintf("node has taints: %s", string(b))
	}
	return true, ""
}

func (n *Node) IsMachineReady() bool {
	return n != nil && n.Status.MachineStatus.Phase == NodeReady
}

func (n *Node) IsManaged() bool {
	return n != nil && n.Status.ClusterStatus.Phase == NodeManaged && GetClusterId(n) != ""
}

func (n *Node) GetSpecCluster() string {
	if n == nil || n.Spec.Cluster == nil {
		return ""
	}
	return *n.Spec.Cluster
}

func (n *Node) GetSpecWorkspace() string {
	if n == nil || n.Spec.Workspace == nil {
		return ""
	}
	return *n.Spec.Workspace
}

func (n *Node) GetSpecHostName() string {
	if n == nil || n.Spec.Hostname == nil {
		return ""
	}
	return *n.Spec.Hostname
}

func (n *Node) GetSpecNodeFlavor() string {
	if n == nil || n.Spec.NodeFlavor == nil {
		return ""
	}
	return n.Spec.NodeFlavor.Name
}

func (n *Node) GetSpecPort() int32 {
	if n == nil || n.Spec.Port == nil {
		return 0
	}
	return *n.Spec.Port
}

func (n *Node) GetK8sNodeName() string {
	if n == nil {
		return ""
	}
	if n.Status.MachineStatus.HostName != "" {
		return n.Status.MachineStatus.HostName
	}
	if n.Spec.Hostname != nil {
		return *n.Spec.Hostname
	}
	return ""
}

// Get the node phase, taking into account both machine status and cluster status.
func (n *Node) GetPhase() NodePhase {
	if n == nil {
		return ""
	}
	if !n.IsMachineReady() {
		return n.Status.MachineStatus.Phase
	}
	if n.Status.ClusterStatus.Phase == NodeManagedFailed || n.Status.ClusterStatus.Phase == NodeUnmanagedFailed ||
		n.Status.ClusterStatus.Phase == NodeManaging || n.Status.ClusterStatus.Phase == NodeUnmanaging {
		return n.Status.ClusterStatus.Phase
	}
	return NodeReady
}
