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
	CommandPending   CommandPhase = "Pending"
)

type CommandStatus struct {
	Name  string       `json:"name,omitempty"`
	Phase CommandPhase `json:"phase,omitempty"`
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NodeSpec defines the desired state of Node.
type NodeSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of Node
	// Important: Run "make" to regenerate code after modifying this file
	// node flavor id
	NodeFlavor *corev1.ObjectReference `json:"nodeFlavor,omitempty"`
	//
	Hostname *string `json:"hostname,omitempty"`
	// 私有IP
	PrivateIP string `json:"privateIP"`
	// 公共IP
	PublicIP string `json:"publicIP,omitempty"`
	// SSH 端口，默认22
	Port *int32 `json:"port,omitempty"`
	// SSH 登录节点
	SSHSecret *corev1.ObjectReference `json:"secret"`
	// 节点模板
	NodeTemplate *corev1.ObjectReference `json:"nodeTemplate,omitempty"`
	// 解纳管后操作 discarded
	KubernetesUnmanaged *corev1.ObjectReference `json:"kubernetesUnmanaged,omitempty"`

	Cluster *string `json:"cluster,omitempty"`
}

type NodeClusterStatus struct {
	Phase         NodePhase       `json:"phase,omitempty"`
	Cluster       *string         `json:"cluster,omitempty"`
	CommandStatus []CommandStatus `json:"commandStatus,omitempty"`
}

type MachineStatus struct {
	// HostName is the hostname of the machinenode.
	HostName string    `json:"hostName,omitempty"`
	Phase    NodePhase `json:"phase,omitempty"`
	// PrivateIP is the private ip address of the machinenode.
	PrivateIP string `json:"privateIP,omitempty"`
	// PublicIP specifies the public IP.
	PublicIP        string          `json:"publicIP,omitempty"`
	CommandStatus   []CommandStatus `json:"commandStatus,omitempty"`
	UnmanagedStatus []CommandStatus `json:"unmanagedStatus,omitempty"`
}

// NodeStatus defines the observed state of Node.
type NodeStatus struct {
	MachineStatus MachineStatus     `json:"machineStatus,omitempty"`
	ClusterStatus NodeClusterStatus `json:"clusterStatus,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// Node is the Schema for the Nodes API.
type Node struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeSpec   `json:"spec,omitempty"`
	Status NodeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// NodeList contains a list of Node.
type NodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Node `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Node{}, &NodeList{})
}
