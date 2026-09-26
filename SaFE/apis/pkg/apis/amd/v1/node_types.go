/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NodePhase string

// NodeLifecycleMode selects how a node is provisioned and reclaimed.
type NodeLifecycleMode string

const (
	// NodeLifecycleExternal marks a node backed by an external execution provider. There is
	// no physical host to manage: no SSH, no hostname or DNS change, no addon install, no
	// kubespray, no kubeadm reset and no reboot. An empty value keeps the managed lifecycle.
	NodeLifecycleExternal NodeLifecycleMode = "external"
)

// DefaultExternalObservationMaxAge caps how long a provider observation stays usable after
// the provider stops reporting. It backstops ValidUntil, which the provider itself chooses.
const DefaultExternalObservationMaxAge = 120 * time.Second

var (
	// nowFunc is replaced in tests to exercise the freshness boundaries.
	nowFunc = time.Now
	// externalObservationMaxAge is the effective backstop for provider observations.
	externalObservationMaxAge = DefaultExternalObservationMaxAge
)

// SetExternalObservationMaxAge tightens the freshness backstop to match the provider
// reporting interval. Non-positive values are ignored.
func SetExternalObservationMaxAge(d time.Duration) {
	if d > 0 {
		externalObservationMaxAge = d
	}
}

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

	// the phase reported for an external node whose provider observation went stale
	NodeExternalStale NodePhase = "ExternalStale"
)

type CommandPhase string

const (
	CommandSucceeded CommandPhase = "Succeeded"
	CommandFailed    CommandPhase = "Failed"
)

type CommandStatus struct {
	// Operational command, e.g. authorize
	Name string `json:"name,omitempty"`
	// Operation result. e.g. Succeeded and Failed
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
	// SSH port，default 22
	Port *int32 `json:"port,omitempty"`
	// The taint will be automatically synchronized to the Kubernetes node.
	Taints []corev1.Taint `json:"taints,omitempty"`
	// Secret for ssh
	SSHSecret *corev1.ObjectReference `json:"secret"`
	// Lifecycle mode of the node. Empty keeps the managed physical-host lifecycle.
	LifecycleMode NodeLifecycleMode `json:"lifecycleMode,omitempty"`
	// Provider allocation backing this node. Required and immutable when lifecycleMode
	// is external, and rejected otherwise. Filled by SaFE when admitting a virtual node;
	// the provider never writes this object.
	ExternalRef *NodeExternalRef `json:"externalRef,omitempty"`
}

// NodeExternalRef identifies the provider allocation backing a virtual node.
type NodeExternalRef struct {
	// The external execution provider that owns the allocation
	Provider string `json:"provider"`
	// The provider-side allocation holding the host
	AllocationId string `json:"allocationId"`
	// Distinguishes reuses of the same allocation id
	Generation int64 `json:"generation"`
	// Identifies the verified physical host behind the allocation. Empty until the
	// provider freezes the first observation; omitted rather than written as "".
	HostKey string `json:"hostKey,omitempty"`
}

type NodeClusterStatus struct {
	// The status of nodes in the cluster, e.g. Ready, Managing, Managed, ManagedFailed, Unmanaging, Unmanaged, UnmanagedFailed
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
	// The status of the physical node, e.g. Ready, SSHFailed, HostnameFailed
	Phase NodePhase `json:"phase,omitempty"`
	// The internalIP of k8s node
	PrivateIP string `json:"privateIP,omitempty"`
	// Reserved field, currently unused.
	CommandStatus []CommandStatus `json:"commandStatus,omitempty"`
	// Last update time
	UpdateTime *metav1.Time `json:"updateTime,omitempty"`
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

// IsAvailable returns true if the node is ready and available for workloads.
func (n *Node) IsAvailable(ignoreTaint bool) bool {
	ok, _ := n.CheckAvailable(ignoreTaint)
	return ok
}

// CheckAvailable checks if the node is available and returns the reason if not.
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
			if isIgnorableAvailabilityTaint(t.Key) {
				continue
			}
			taints = append(taints, fmt.Sprintf("%s=%s", t.Key, t.Value))
		}
		if len(taints) > 0 {
			b, _ := json.Marshal(taints)
			return false, fmt.Sprintf("node has taints: %s", string(b))
		}
	}
	return true, ""
}

// isIgnorableAvailabilityTaint reports taints that select pods but do not mean the node is
// unhealthy. Provider identity taints and the sticky-nodes monitor are in this set.
func isIgnorableAvailabilityTaint(key string) bool {
	if key == ExternalVirtualKubeletTaint {
		return true
	}
	return GetIdByTaintKey(key) == StickyNodesMonitorId
}

// IsExternal reports whether the node is owned by an external execution provider.
//
// Both halves are required. Admission rejects an external node without a reference, but
// this predicate also runs on objects that never reached admission -- a request body being
// validated, a decoded payload -- and callers read the reference straight off the back of
// it. Treating a half-built object as external would hand them a nil pointer.
func (n *Node) IsExternal() bool {
	return n != nil && n.Spec.LifecycleMode == NodeLifecycleExternal && n.Spec.ExternalRef != nil
}

// DeclaresExternalLifecycle reports the requested mode alone, before the reference that has
// to accompany it is known to be there. Admission uses it to tell "external but incomplete"
// apart from "not external", which IsExternal deliberately cannot distinguish.
func (n *Node) DeclaresExternalLifecycle() bool {
	return n != nil && n.Spec.LifecycleMode == NodeLifecycleExternal
}

// IsMachineReady returns true if the underlying machine is ready. An external node has no
// machine to probe over SSH: readiness is the Ready condition synced from the virtual node
// plus freshness of the provider observation annotations on that node.
func (n *Node) IsMachineReady() bool {
	if n == nil {
		return false
	}
	if n.IsExternal() {
		return n.hasReadyCondition() && n.hasFreshExternalObservation()
	}
	return n.Status.MachineStatus.Phase == NodeReady
}

// hasReadyCondition reports whether the synced Ready condition is True.
func (n *Node) hasReadyCondition() bool {
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// hasFreshExternalObservation reports whether the provider observation annotations still
// hold. Both bounds must pass when present: ValidUntil is the provider's own claim, and the
// max age caps how long that claim survives once the provider stops reporting. Missing
// observation annotations are treated as stale so capacity is not counted before the
// provider has published freshness.
func (n *Node) hasFreshExternalObservation() bool {
	observedRaw := GetAnnotation(n, ExternalObservedAtAnnotation)
	validRaw := GetAnnotation(n, ExternalValidUntilAnnotation)
	if observedRaw == "" || validRaw == "" {
		return false
	}
	observedAt, err := time.Parse(time.RFC3339Nano, observedRaw)
	if err != nil {
		observedAt, err = time.Parse(time.RFC3339, observedRaw)
		if err != nil {
			return false
		}
	}
	validUntil, err := time.Parse(time.RFC3339Nano, validRaw)
	if err != nil {
		validUntil, err = time.Parse(time.RFC3339, validRaw)
		if err != nil {
			return false
		}
	}
	now := nowFunc()
	return now.Before(validUntil) && now.Sub(observedAt) < externalObservationMaxAge
}

// IsManaged returns true if the node is managed by the system. An external node never joins
// through kubespray, so cluster ownership is the only condition it can satisfy.
func (n *Node) IsManaged() bool {
	if n == nil {
		return false
	}
	if n.IsExternal() {
		return GetClusterId(n) != ""
	}
	return n.Status.ClusterStatus.Phase == NodeManaged && GetClusterId(n) != ""
}

// GetSpecCluster returns the cluster ID specified in the node spec.
func (n *Node) GetSpecCluster() string {
	if n == nil || n.Spec.Cluster == nil {
		return ""
	}
	return *n.Spec.Cluster
}

// GetSpecWorkspace returns the workspace ID specified in the node spec.
func (n *Node) GetSpecWorkspace() string {
	if n == nil || n.Spec.Workspace == nil {
		return ""
	}
	return *n.Spec.Workspace
}

// GetSpecHostName returns the hostname specified in the node spec.
func (n *Node) GetSpecHostName() string {
	if n == nil || n.Spec.Hostname == nil {
		return ""
	}
	return *n.Spec.Hostname
}

// GetSpecNodeFlavor returns the node flavor name.
func (n *Node) GetSpecNodeFlavor() string {
	if n == nil || n.Spec.NodeFlavor == nil {
		return ""
	}
	return n.Spec.NodeFlavor.Name
}

// GetSpecPort returns the SSH port specified in the node spec.
func (n *Node) GetSpecPort() int32 {
	if n == nil || n.Spec.Port == nil {
		return 0
	}
	return *n.Spec.Port
}

// GetK8sNodeName returns the corresponding Kubernetes node name.
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

// GetPhase returns the current phase of the resource.
func (n *Node) GetPhase() NodePhase {
	if n == nil {
		return ""
	}
	if n.IsExternal() {
		// MachineStatus is never written for external nodes, so reporting it would show an
		// empty phase. Freshness of the provider observation is what decides availability.
		if n.IsMachineReady() {
			return NodeReady
		}
		return NodeExternalStale
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

// GetIdByTaintKey extracts the ID from a taint key by removing the PrimusSafe prefix.
func GetIdByTaintKey(taintKey string) string {
	if len(taintKey) <= len(PrimusSafePrefix) {
		return ""
	}
	return taintKey[len(PrimusSafePrefix):]
}
