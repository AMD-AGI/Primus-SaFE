/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WorkspacePhase string
type WorkspaceQueuePolicy string

// +kubebuilder:validation:Enum=Train;Infer;Authoring;VirtualMachine
type WorkspaceScope string
type FileSystemType string
type VolumePurpose int
type WorkspaceType string

const (
	WorkspaceCreating WorkspacePhase = "Creating"
	WorkspaceRunning  WorkspacePhase = "Running"
	WorkspaceAbnormal WorkspacePhase = "Abnormal"
	WorkspaceDeleted  WorkspacePhase = "Deleted"

	DedicatedType WorkspaceType = "dedicated"
	ElasticType   WorkspaceType = "elastic"

	QueueFifoPolicy    WorkspaceQueuePolicy = "fifo"
	QueueBalancePolicy WorkspaceQueuePolicy = "balance"

	// 训练
	TrainScope WorkspaceScope = "Train"
	// 推理
	InferScope WorkspaceScope = "Infer"
	// 开发机
	AuthoringScope WorkspaceScope = "Authoring"
	// 虚拟机
	VMScope WorkspaceScope = "VirtualMachine"

	// 给系统盘使用
	VolumeRoot VolumePurpose = 0
	// 给数据盘使用
	VolumeData VolumePurpose = 1
)

type FlavorReplica struct {
	// 节点规格名，对应NodeFlavor.name
	Flavor string `json:"flavor,omitempty"`

	// 节点副本数
	Replica int16 `json:"replica,omitempty"`
}

type WorkspaceSpec struct {
	// 使用的集群名
	Cluster string `json:"cluster"`
	// 工作空间类型，目前只支持dedicated(专属，默认)和elastic（弹性）
	// +kubebuilder:validation:Enum=dedicated;elastic
	Type WorkspaceType `json:"type,omitempty"`
	// 申请的节点资源
	Nodes []FlavorReplica `json:"nodes,omitempty"`
	// 在该空间提交任务时使用的排队策略（目前所有任务遵循同样的策略，不支持单独设置)，默认fifo
	QueuePolicy WorkspaceQueuePolicy `json:"queuePolicy,omitempty"`
	// 该空间支持的服务模块，如果为空则不做限制
	Scopes []WorkspaceScope `json:"scopes,omitempty"`
	// 该空间指定的volumes
	Volumes []WorkspaceVolume `json:"volumes,omitempty"`
	// 是否开启抢占
	EnablePreempt bool `json:"enablePreempt,omitempty"`
	// 绑定的专属空间列表，该参数只对弹性空间有用，且对弹性空间是必选，弹性空间必须绑定1-n个专属池
	BindDedicatedIds []string `json:"bindDedicatedIds,omitempty"`
	// 空间管理员列表
	Managers []string `json:"managers,omitempty"`
}

type StorageUseType string

type WorkspaceVolume struct {
	FsType StorageUseType `json:"fsType"`
	// 挂载目录，对应volume mount中的mountPath，必选
	MountPath string `json:"mountPath"`
	// 卷内路径，对应volume mount中的subPath。默认空，对应卷内的根路径
	SubPath string `json:"subPath,omitempty"`
	// 是否创建个人用户目录，这里是for 01AI的逻辑，非通用。
	// 在subPath下有userid-username子目录，然后挂载到mountPath/username下
	EnableUserDir bool `json:"enableUserDir,omitempty"`
	// 用途：0给系统盘使用，1给数据盘使用。当前只有vm需要指定
	Purpose VolumePurpose `json:"purpose,omitempty"`

	// 以下是gpfs 必选参数
	// hostPath路径
	HostPath string `json:"hostPath,omitempty"`

	// 以下是非gpfs 必选参数，非gpfs文件系统会自动创建pvc(生命周期同workspace)
	// 容量大小，比如100Gi
	Capacity string `json:"capacity,omitempty"`
	// 创建pvc时，选择的数据面k8s配置的StorageClass
	StorageClass string `json:"storageClass,omitempty"`
	// 访问方式，默认是ReadWriteMany
	AccessMode corev1.PersistentVolumeAccessMode `json:"accessMode,omitempty"`
}

type WorkspaceNodes struct {
	// 集群可用节点数量
	AvailableNodes []FlavorReplica `json:"availableNodes,omitempty"`

	// 集群异常节点数量
	AbnormalNodes []FlavorReplica `json:"abnormalNodes,omitempty"`
}

type WorkspaceStatus struct {
	// 工作空间状态
	Phase WorkspacePhase `json:"phase,omitempty"`
	// 工作空间节点状态
	Nodes WorkspaceNodes `json:"nodes,omitempty"`
	// 工作空间资源量总和, 会去掉比如插件和操作系统等消耗
	TotalResources corev1.ResourceList `json:"totalResources,omitempty"`
	// 可用资源总和
	AvailableResources corev1.ResourceList `json:"availableResources,omitempty"`
	// 上次更新时间
	UpdateTime *metav1.Time `json:"updateTime,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:webhook:path=/mutate-amd.com-v1-workspace,mutating=true,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=workspaces,verbs=create;update,versions=v1,name=mworkspace.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:webhook:path=/validate-amd.com-v1-workspace,mutating=false,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=workspaces,verbs=create;update,versions=v1,name=vworkspace.kb.io,admissionReviewVersions={v1,v1beta1}
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

func (w *Workspace) GetFirstFlavor() string {
	if len(w.Spec.Nodes) == 0 {
		return ""
	}
	return w.Spec.Nodes[0].Flavor
}

func (w *Workspace) IsEnd() bool {
	if w.Status.Phase == WorkspaceRunning || w.Status.Phase == WorkspaceAbnormal {
		return true
	}
	return false
}

func (w *Workspace) IsPending() bool {
	if w.Status.Phase == "" {
		return true
	}
	return false
}

func (w *Workspace) IsEnableFifo() bool {
	if w.Spec.QueuePolicy == "" || w.Spec.QueuePolicy == QueueFifoPolicy {
		return true
	}
	return false
}

func (w *Workspace) IsEnablePreempt() bool {
	return w.Spec.EnablePreempt
}

func (w *Workspace) IsElastic() bool {
	return w.Spec.Type == ElasticType
}

func (w *Workspace) IsDedicate() bool {
	return w.Spec.Type == DedicatedType
}

func (w *Workspace) GetSpecTotalCount() int {
	count := 0
	for _, n := range w.Spec.Nodes {
		count += int(n.Replica)
	}
	return count
}

func (w *Workspace) GetStatusTotalCount() int {
	count := 0
	for _, n := range w.Status.Nodes.AvailableNodes {
		count += int(n.Replica)
	}
	for _, n := range w.Status.Nodes.AbnormalNodes {
		count += int(n.Replica)
	}
	return count
}

func FindFlavor(nodes []FlavorReplica, flavor string) int {
	for i, n := range nodes {
		if n.Flavor == flavor {
			return i
		}
	}
	return -1
}
