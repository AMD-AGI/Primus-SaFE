/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NodePhase string

const (
	NodeKind = "Node"

	NodeManaging        NodePhase = "Managing"
	NodeManaged         NodePhase = "Managed"
	NodeManagedFailed   NodePhase = "ManagedFailed"
	NodeReady           NodePhase = "Ready"
	NodeNotReady        NodePhase = "NotReady"
	NodeUnmanaging      NodePhase = "Unmanaging"
	NodeUnmanaged       NodePhase = "Unmanaged"
	NodeUnmanagedFailed NodePhase = "UnmanagedFailed"
	NodeSSHFailed       NodePhase = "SSHFailed"
	NodeHostnameFailed  NodePhase = "HostnameFailed"
)

type CommandPhase string

const (
	CommandSucceeded CommandPhase = "Succeeded"
	CommandFailed    CommandPhase = "Failed"
)

type CommandStatus struct {
	Name  string       `json:"name,omitempty"`
	Phase CommandPhase `json:"phase,omitempty"`
}

type NodeSpec struct {
	// The name of the cluster that the node belongs to
	Cluster *string `json:"cluster,omitempty"`
	// The name of the workspace that the node belongs to
	Workspace *string `json:"workspace,omitempty"`
	// node flavor reference
	NodeFlavor *corev1.ObjectReference `json:"nodeFlavor"`
	// node template reference
	NodeTemplate *corev1.ObjectReference `json:"nodeTemplate"`
	// node hostname
	Hostname  *string `json:"hostname,omitempty"`
	PrivateIP string  `json:"privateIP,omitempty"`
	// option
	PublicIP string `json:"publicIP,omitempty"`
	// SSH port，default 22
	Port *int32 `json:"port,omitempty"`
	// The taint will be automatically synchronized to the Kubernetes node.
	Taints []corev1.Taint `json:"taints,omitempty"`
	// secret for ssh
	SSHSecret *corev1.ObjectReference `json:"secret"`
}

type NodeClusterStatus struct {
	Phase         NodePhase       `json:"phase,omitempty"`
	Cluster       *string         `json:"cluster,omitempty"`
	CommandStatus []CommandStatus `json:"commandStatus,omitempty"`
}

type MachineStatus struct {
	// HostName is the hostname of the machine node.
	HostName string    `json:"hostName,omitempty"`
	Phase    NodePhase `json:"phase,omitempty"`
	// PrivateIP is the private ip address of the machine node.
	PrivateIP string `json:"privateIP,omitempty"`
	// PublicIP specifies the public IP.
	PublicIP      string          `json:"publicIP,omitempty"`
	CommandStatus []CommandStatus `json:"commandStatus,omitempty"`
}

// NodeStatus defines the observed state of Node.
type NodeStatus struct {
	MachineStatus MachineStatus     `json:"machineStatus,omitempty"`
	ClusterStatus NodeClusterStatus `json:"clusterStatus,omitempty"`
	Unschedulable bool              `json:"unschedulable,omitempty"`
	// taint automatically synchronized from the Kubernetes node
	Taints []corev1.Taint `json:"taints,omitempty"`
	// all the resource of node
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
	if n == nil {
		return false
	}
	if !n.IsReady() {
		return false
	}
	if !n.IsManaged() {
		return false
	}
	if !n.GetDeletionTimestamp().IsZero() {
		return false
	}
	if n.Status.Unschedulable {
		return false
	}
	if !ignoreTaint && len(n.Status.Taints) > 0 {
		return false
	}
	return true
}

func (n *Node) IsReady() bool {
	return n != nil && n.Status.MachineStatus.Phase == NodeReady
}

func (n *Node) IsManaged() bool {
	return n != nil && n.Status.ClusterStatus.Phase == NodeManaged
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
