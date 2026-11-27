/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WorkspacePhase string
type WorkspaceQueuePolicy string

// +kubebuilder:validation:Enum=Train;Infer;Authoring;CICD;Ray;Sandbox
type WorkspaceScope string
type FileSystemType string
type VolumePurpose int
type WorkspaceType string
type WorkspaceVolumeType string

// +kubebuilder:validation:Enum=Average;Min
type LowUtilizationMode string

const (
	WorkspaceKind = "Workspace"

	WorkspaceCreating WorkspacePhase = "Creating"
	WorkspaceRunning  WorkspacePhase = "Running"
	WorkspaceAbnormal WorkspacePhase = "Abnormal"
	WorkspaceDeleting WorkspacePhase = "Deleting"

	QueueFifoPolicy    WorkspaceQueuePolicy = "fifo"
	QueueBalancePolicy WorkspaceQueuePolicy = "balance"

	TrainScope     WorkspaceScope = "Train"
	InferScope     WorkspaceScope = "Infer"
	AuthoringScope WorkspaceScope = "Authoring"
	CICDScope      WorkspaceScope = "CICD"
	RayScope       WorkspaceScope = "Ray"
	SandboxScope   WorkspaceScope = "Sandbox"

	HOSTPATH WorkspaceVolumeType = "hostpath"
	PFS      WorkspaceVolumeType = "pfs"

	// The workload is idle when the across-node average stays below the threshold for the whole window.
	LowUtilizationAverage LowUtilizationMode = "Average"
	// The workload is idle when at least one node stays below the threshold for the whole window.
	LowUtilizationMin LowUtilizationMode = "Min"
)

type WorkspaceSpec struct {
	// The cluster that the workspace belongs to
	Cluster string `json:"cluster"`
	// The node flavor id of workspace, A workspace supports only one node flavor
	NodeFlavor string `json:"nodeFlavor,omitempty"`
	// The expected number of nodes in the workspace
	Replica int `json:"replica,omitempty"`
	// Queuing policy for workloads submitted in this workspace.
	// All workloads currently share the same policy, supports fifo (default) and balance.
	// 1. "Fifo" means first-in, first-out: the workload that enters the queue first is served first.
	//    If the front workload does not meet the conditions for dispatch, it will wait indefinitely,
	//    and other tasks in the queue will also be blocked waiting.
	// 2. "Balance" allows any workload that meets the resource conditions to be dispatched,
	//    avoiding blockage by the front workload in the queue. However, it is still subject to priority constraints.
	//    If a higher-priority task cannot be dispatched, lower-priority tasks will wait.
	QueuePolicy WorkspaceQueuePolicy `json:"queuePolicy,omitempty"`
	// Service modules available in this space. support: Train/Infer/Authoring/CICD, No limitation if not specified
	Scopes []WorkspaceScope `json:"scopes,omitempty"`
	// Volumes used in this workspace
	Volumes []WorkspaceVolume `json:"volumes,omitempty"`
	// Whether preemption is enabled. If enabled, higher-priority workload will preempt the lower-priority one
	EnablePreempt bool `json:"enablePreempt,omitempty"`
	// User id of the workspace administrator
	Managers []string `json:"managers,omitempty"`
	// Set the workspace as the default workspace (i.e., all users can access it)
	IsDefault bool `json:"isDefault,omitempty"`
	// Workspace image secret ID, used for downloading images
	ImageSecrets []corev1.ObjectReference `json:"imageSecrets,omitempty"`
	// Trigger workload processing after a period of workspace inactivity. The idletime of each scope, Unit: "12h0m0s"
	// only for sandbox workload
	IdleTime map[WorkspaceScope]string `json:"idleTime,omitempty"`
	// Stop running workloads whose GPU utilization stays low, per scope
	LowUtilization map[WorkspaceScope]LowUtilizationPolicy `json:"lowUtilization,omitempty"`
	// Teams whose members access the workspace
	Teams []WorkspaceTeam `json:"teams,omitempty"`
	// Whether users may request direct access: closed (default) or approval
	AccessPolicy WorkspaceAccessPolicy `json:"accessPolicy,omitempty"`
}

// WorkspaceTeamRole is the role the members of a team hold in a workspace.
type WorkspaceTeamRole string

const (
	WorkspaceTeamMember  WorkspaceTeamRole = "member"
	WorkspaceTeamManager WorkspaceTeamRole = "manager"
)

// WorkspaceAccessPolicy controls whether users may request direct access to a workspace.
type WorkspaceAccessPolicy string

const (
	WorkspaceAccessClosed   WorkspaceAccessPolicy = "closed"
	WorkspaceAccessApproval WorkspaceAccessPolicy = "approval"
)

// TeamGroupPrefix prefixes a team id to form its group name in RBAC subjects and SubjectAccessReviews.
const TeamGroupPrefix = "team:"

// WorkspaceTeam binds a team of the identity service to the workspace.
type WorkspaceTeam struct {
	// Team id in the identity service
	Name string `json:"name"`
	// Role of the team members in the workspace, member by default
	Role WorkspaceTeamRole `json:"role,omitempty"`
}

// EffectiveRole returns the role of the binding, member when it is not set.
func (t WorkspaceTeam) EffectiveRole() WorkspaceTeamRole {
	if t.Role == "" {
		return WorkspaceTeamMember
	}
	return t.Role
}

// LowUtilizationPolicy stops workloads whose GPU utilization stays below a threshold
type LowUtilizationPolicy struct {
	// Legacy single rule: GPU utilization in percent (0-100) below which a workload counts as idle.
	// Only used when rules is empty.
	Threshold int `json:"threshold,omitempty"`
	// Legacy single rule: how long the utilization must stay below the threshold, e.g. 30m (5m to 24h).
	// Only used when rules is empty.
	Window string `json:"window,omitempty"`
	// A workload counts as idle when any rule matches (at most 5, each with its own window)
	Rules []LowUtilizationRule `json:"rules,omitempty"`
	// Average compares the across-node average, Min the least busy node. Required with a rule.
	Mode LowUtilizationMode `json:"mode,omitempty"`
	// Time after the workload starts before it is checked, e.g. 1h
	GracePeriod string `json:"gracePeriod,omitempty"`
	// Time between the low utilization notice and the stop, e.g. 30m
	NotifyBefore string `json:"notifyBefore,omitempty"`
	// Time a detected workload must stay active before its detection is cleared, and the least
	// time between two notices to its owner, e.g. 30m. Default the shortest rule window.
	Cooldown string `json:"cooldown,omitempty"`
	// Detect and notify only, never stop the workload
	DryRun bool `json:"dryRun,omitempty"`
}

// LowUtilizationRule counts a workload as idle when its GPU utilization stays below the threshold for the window
type LowUtilizationRule struct {
	// GPU utilization in percent (0-100) below which a workload counts as idle
	Threshold int `json:"threshold"`
	// How long the utilization must stay below the threshold, e.g. 30m (5m to 24h)
	Window string `json:"window"`
}

type WorkspaceVolume struct {
	// The volume id, which is used to identify the volume. This field is set internally.
	Id int `json:"id"`
	// The volume type, valid values includes: pfs/hostpath
	// If PFS is configured, a PVC will be automatically created in the workspace.
	Type WorkspaceVolumeType `json:"type"`
	// Mount path to be used, equivalent to 'mountPath' in Kubernetes volume mounts.
	// +required
	MountPath string `json:"mountPath"`
	// When enabled, this appends the {user_id} directory to the subPath and mounts it to {mount_path}/{user_id}
	EnableUserDir bool `json:"enableUserDir,omitempty"`
	// access mode, default ReadWriteMany
	AccessMode corev1.PersistentVolumeAccessMode `json:"accessMode,omitempty"`
	// equivalent to 'subPath' in Kubernetes volume mounts
	// +optional
	SubPath string `json:"subPath,omitempty"`

	// Path on the host to mount. Required when volume type is hostpath
	HostPath string `json:"hostPath,omitempty"`

	// The following parameters are used for PVC creation. If using hostPath mounting, they are not required.
	// Capacity size, e.g. 100Gi. This is a required parameter when creating a PVC (PersistentVolumeClaim).
	Capacity string `json:"capacity,omitempty"`
	// selector is a label query over volumes to consider for binding.
	// It cannot be used together with storageClass. If both are set, the selector takes priority
	Selector map[string]string `json:"selector,omitempty"`
	// Responsible for automatic PV creation
	StorageClass string `json:"storageClass,omitempty"`
	// The workload scopes this volume is mounted into. Empty means every scope.
	// A workload whose scope is not listed does not get the volume, even when it asks for workspace storage.
	// +optional
	// +listType=set
	Scopes []WorkspaceScope `json:"scopes,omitempty"`
	// When set, the volume is mounted only into workloads whose creator held the
	// workload/privileged grant in the workspace when the workload was created (built-in:
	// default, workspace-admin, and system-admin). Workloads of users without the grant are still
	// admitted and run without this volume, as if its scopes did not list theirs. A user without
	// the grant whose spec.hostpath names the volume's hostPath, a path under it or a path
	// containing it is refused, with the reason, even when another volume declares that path.
	// +optional
	RequiresPrivilegedGrant bool `json:"requiresPrivilegedGrant,omitempty"`
	// The directories of a hostpath volume that the workspace's users share, such as model and
	// cache directories. They are used only when the job-manager mounts user directories for
	// external Pods (external_execution.user_dir_mounts): an external Pod then mounts its own user
	// directory (with enableUserDir) and these directories, each at the path it has below
	// mountPath, instead of the whole volume. Pods on the cluster's own nodes are not affected.
	// +optional
	// +listType=map
	// +listMapKey=path
	SharedPaths []WorkspaceSharedPath `json:"sharedPaths,omitempty"`
}

// WorkspaceSharedPath is a directory of a hostpath workspace volume that the workspace's users share.
type WorkspaceSharedPath struct {
	// The directory, relative to the volume's root: hostPath (and subPath, when set) on the node,
	// mountPath in the container. For example "models" is mounted from {hostPath}/models at
	// {mountPath}/models. It must be a clean relative path without "..", and with enableUserDir it
	// may not be "users" or lie under it.
	// +required
	Path string `json:"path"`
	// Mount the directory read-only.
	// +optional
	ReadOnly bool `json:"readOnly,omitempty"`
}

type WorkspaceStatus struct {
	// The status of workspace, e.g. Creating, Running, Abnormal, Deleting
	Phase WorkspacePhase `json:"phase,omitempty"`
	// The total resource of workspace
	TotalResources corev1.ResourceList `json:"totalResources,omitempty"`
	// The available resource of workspace
	AvailableResources corev1.ResourceList `json:"availableResources,omitempty"`
	// The abnormal resource of workspace
	AbnormalResources corev1.ResourceList `json:"abnormalResources,omitempty"`
	// The available node count of workspace
	AvailableReplica int `json:"availableReplica,omitempty"`
	// The abnormal node count of workspace
	AbnormalReplica int `json:"abnormalReplica,omitempty"`
	// Last update time
	UpdateTime *metav1.Time `json:"updateTime,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:webhook:path=/mutate-amd-primus-safe-v1-workspace,mutating=true,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=workspaces,verbs=create;update,versions=v1,name=mworkspace.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:webhook:path=/validate-amd-primus-safe-v1-workspace,mutating=false,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=workspaces,verbs=create;update,versions=v1,name=vworkspace.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:rbac:groups=amd.com,resources=workspaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=amd.com,resources=workspaces/status,verbs=get;update;patch

type Workspace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkspaceSpec   `json:"spec,omitempty"`
	Status WorkspaceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
type WorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workspace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Workspace{}, &WorkspaceList{})
}

// IsEnd returns true if the fault has ended (completed or failed).
func (w *Workspace) IsEnd() bool {
	if w.Status.Phase == WorkspaceRunning || w.Status.Phase == WorkspaceAbnormal {
		return true
	}
	return false
}

// IsAbnormal returns true if the workspace is abnormal
func (w *Workspace) IsAbnormal() bool {
	if w.Status.Phase == WorkspaceAbnormal {
		return true
	}
	return false
}

// IsPending returns true if the operations job is pending execution.
func (w *Workspace) IsPending() bool {
	if w.Status.Phase == "" || w.Status.Phase == WorkspaceCreating {
		return true
	}
	return false
}

// IsEnableFifo returns true if the workspace uses FIFO (First In, First Out) queue policy.
// If QueuePolicy is not set, FIFO mode is enabled.
func (w *Workspace) IsEnableFifo() bool {
	if w.Spec.QueuePolicy == "" || w.Spec.QueuePolicy == QueueFifoPolicy {
		return true
	}
	return false
}

// CurrentReplica returns the current number of replicas in the workspace.
func (w *Workspace) CurrentReplica() int {
	return w.Status.AvailableReplica + w.Status.AbnormalReplica
}

func (w *Workspace) HasImageSecret(name string) bool {
	for _, secret := range w.Spec.ImageSecrets {
		if secret.Name == name {
			return true
		}
	}
	return false
}

func (w *Workspace) HasScope(scope WorkspaceScope) bool {
	for _, s := range w.Spec.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// TeamRole returns the role the members of the team hold in the workspace, and whether the team is bound.
// A team bound more than once holds manager when any of its bindings is manager.
func (w *Workspace) TeamRole(team string) (WorkspaceTeamRole, bool) {
	role, bound := WorkspaceTeamRole(""), false
	for _, t := range w.Spec.Teams {
		if t.Name != team || team == "" {
			continue
		}
		bound = true
		if role != WorkspaceTeamManager {
			role = t.EffectiveRole()
		}
	}
	return role, bound
}

// AcceptsSandbox reports whether the workspace admits Sandbox workloads: no scopes means every
// scope, the rule the workload webhook applies. It is what WorkspaceSandboxScopeLabel records.
func (w *Workspace) AcceptsSandbox() bool {
	return len(w.Spec.Scopes) == 0 || w.HasScope(SandboxScope)
}

// SandboxScopeLabelValue is the value WorkspaceSandboxScopeLabel must carry, "" meaning absent.
func (w *Workspace) SandboxScopeLabelValue() string {
	if w.AcceptsSandbox() {
		return TrueStr
	}
	return ""
}

// PendingGrants returns the grants recorded in WorkspaceGrantPending.
func (w *Workspace) PendingGrants() []string {
	val := GetAnnotation(w, WorkspaceGrantPending)
	if val == "" {
		return nil
	}
	return strings.Split(val, ",")
}

// AddPendingGrant records a grant in WorkspaceGrantPending, once.
func (w *Workspace) AddPendingGrant(grant string) {
	grants := w.PendingGrants()
	for _, g := range grants {
		if g == grant {
			return
		}
	}
	SetAnnotation(w, WorkspaceGrantPending, strings.Join(append(grants, grant), ","))
}

func (w *Workspace) GetIdleTime(scope WorkspaceScope) string {
	idleTime, ok := w.Spec.IdleTime[scope]
	if !ok {
		return ""
	}
	return idleTime
}

// GetLowUtilization returns the low-utilization policy of the scope, or nil when it has none.
func (w *Workspace) GetLowUtilization(scope WorkspaceScope) *LowUtilizationPolicy {
	policy, ok := w.Spec.LowUtilization[scope]
	if !ok {
		return nil
	}
	return &policy
}

// EffectiveRules returns the rules of the policy, or the legacy threshold and window as the
// only rule when there are no rules and the window is set.
func (p *LowUtilizationPolicy) EffectiveRules() []LowUtilizationRule {
	if len(p.Rules) > 0 {
		return p.Rules
	}
	if p.Window != "" {
		return []LowUtilizationRule{{Threshold: p.Threshold, Window: p.Window}}
	}
	return nil
}

// AppliesToScope reports whether the volume is mounted into workloads of the given scope.
// A volume with no scopes applies to every scope. A workload without a scope (an ops job, for example)
// is not restricted by the volume's scopes.
func (v *WorkspaceVolume) AppliesToScope(scope WorkspaceScope) bool {
	if len(v.Scopes) == 0 || scope == "" {
		return true
	}
	for _, s := range v.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// UserDirsPath is the directory, relative to the root of a volume with EnableUserDir, that holds the
// user directories: the directory of user {user_id} is {UserDirsPath}/{user_id}.
const UserDirsPath = "users"

// ValidateSharedPath reports why p cannot be one of the volume's shared paths: it must be a clean
// relative path, not "." and without "..", and on a volume with EnableUserDir it may be neither
// UserDirsPath nor below it, because a user directory is only ever mounted for its own user.
func (v *WorkspaceVolume) ValidateSharedPath(p string) error {
	switch {
	case p == "":
		return fmt.Errorf("a shared path of volume(%s) is empty", v.GenFullVolumeId())
	case path.IsAbs(p):
		return fmt.Errorf("shared path %q of volume(%s) must be relative to the volume root", p, v.GenFullVolumeId())
	case p == "." || path.Clean(p) != p:
		return fmt.Errorf("shared path %q of volume(%s) must be a clean path below the volume root", p, v.GenFullVolumeId())
	}
	for _, elem := range strings.Split(p, "/") {
		if elem == ".." {
			return fmt.Errorf("shared path %q of volume(%s) has a \"..\" element", p, v.GenFullVolumeId())
		}
	}
	if v.EnableUserDir && (p == UserDirsPath || strings.HasPrefix(p, UserDirsPath+"/")) {
		return fmt.Errorf("shared path %q of volume(%s) is in the user directories %q", p, v.GenFullVolumeId(), UserDirsPath)
	}
	return nil
}

func (v *WorkspaceVolume) GenFullVolumeId() string {
	return GenFullVolumeId(v.Type, v.Id)
}

func GenFullVolumeId(volumeType WorkspaceVolumeType, id int) string {
	return string(volumeType) + "-" + strconv.Itoa(id)
}
