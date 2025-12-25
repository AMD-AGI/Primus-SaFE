/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	OpsJobKind = "OpsJob"
)

type OpsJobPhase string
type OpsJobType string

const (
	OpsJobSucceeded OpsJobPhase = "Succeeded"
	OpsJobFailed    OpsJobPhase = "Failed"
	OpsJobRunning   OpsJobPhase = "Running"
	OpsJobPending   OpsJobPhase = "Pending"

	OpsJobAddonType       OpsJobType = "addon"
	OpsJobDumpLogType     OpsJobType = "dumplog"
	OpsJobPreflightType   OpsJobType = "preflight"
	OpsJobRebootType      OpsJobType = "reboot"
	OpsJobExportImageType OpsJobType = "exportimage"
	OpsJobPrewarmType     OpsJobType = "prewarm"
	OpsJobDownloadType    OpsJobType = "download"

	ParameterNode          = "node"
	ParameterNodeTemplate  = "node.template"
	ParameterAddonTemplate = "addon.template"
	ParameterWorkload      = "workload"
	ParameterWorkspace     = "workspace"
	ParameterCluster       = "cluster"
	ParameterEndpoint      = "endpoint"
	ParameterImage         = "image"
	ParameterSecret        = "secret"
	ParameterDestPath      = "dest.path"
)

type Parameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type OpsJobSpec struct {
	// The type of ops-job, valid values include: addon/preflight/dumplog
	Type OpsJobType `json:"type"`
	// Opsjob resource requirements, only for preflight
	Resource *WorkloadResource `json:"resource,omitempty"`
	// Opsjob image address, only for preflight
	Image *string `json:"image,omitempty"`
	// Opsjob entryPoint(startup command), required in base64, only for preflight
	EntryPoint *string `json:"entryPoint,omitempty"`
	// The resource objects to be processed, e.g., {{"name": "node", "value": "tus1-p8-g6"}}.
	// Multiple entries will be processed sequentially.
	Inputs []Parameter `json:"inputs"`
	// The lifecycle of ops-job after it finishes
	TTLSecondsAfterFinished int `json:"ttlSecondsAfterFinished,omitempty"`
	// Opsjob Timeout (in seconds), Less than or equal to 0 means no timeout
	TimeoutSecond int `json:"timeoutSecond,omitempty"`
	// Environment variables
	Env map[string]string `json:"env,omitempty"`
	// Indicates whether the job tolerates node taints. for preflight, default false
	IsTolerateAll bool `json:"isTolerateAll"`
	// The hostpath for opsjob mounting.
	Hostpath []string `json:"hostpath,omitempty"`
	// The nodes to be excluded, for preflight/addon
	ExcludedNodes []string `json:"excludedNodes,omitempty"`
}

type OpsJobStatus struct {
	// Opsjob start time
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
	// Opsjob end time
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`
	// Description of the job execution process
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// The job status: Succeeded/Failed/Running/Pending
	Phase OpsJobPhase `json:"phase,omitempty"`
	// Opsjob output. For example, the download log URL or the preflight check results.
	Outputs []Parameter `json:"outputs,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:webhook:path=/mutate-amd-primus-safe-v1-opsjob,mutating=true,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=opsjobs,verbs=create;update,versions=v1,name=mopsjob.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:webhook:path=/validate-amd-primus-safe-v1-opsjob,mutating=false,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=opsjobs,verbs=create;update,versions=v1,name=vopsjob.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:rbac:groups=amd.com,resources=opsjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=amd.com,resources=opsjobs/status,verbs=get;update;patch

type OpsJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpsJobSpec   `json:"spec,omitempty"`
	Status OpsJobStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

type OpsJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpsJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpsJob{}, &OpsJobList{})
}

// IsEnd returns true if the fault has ended (completed or failed).
func (job *OpsJob) IsEnd() bool {
	if job.IsFinished() {
		return true
	}
	if !job.GetDeletionTimestamp().IsZero() {
		return true
	}
	return false
}

// IsPending returns true if the operations job is pending execution.
func (job *OpsJob) IsPending() bool {
	if job.Status.Phase == OpsJobPending || job.Status.Phase == "" {
		return true
	}
	return false
}

// IsTimeout returns true if the operations job has timed out.
func (job *OpsJob) IsTimeout() bool {
	if job.Spec.TimeoutSecond <= 0 {
		return false
	}
	elapsedSeconds := time.Now().Unix() - job.CreationTimestamp.Unix()
	return int(elapsedSeconds) >= job.Spec.TimeoutSecond
}

// GetLeftTime returns the remaining time in seconds before timeout.
func (job *OpsJob) GetLeftTime() int64 {
	if job.Spec.TimeoutSecond <= 0 {
		return -1
	}
	leftTime := job.CreationTimestamp.Unix() + int64(job.Spec.TimeoutSecond) - time.Now().Unix()
	return leftTime
}

// IsFinished returns true if the operations job has finished execution.
func (job *OpsJob) IsFinished() bool {
	if job.Status.FinishedAt != nil {
		return true
	}
	return false
}

// GetParameter retrieves a single parameter by name.
func (job *OpsJob) GetParameter(name string) *Parameter {
	for i, param := range job.Spec.Inputs {
		if param.Name == name {
			return &job.Spec.Inputs[i]
		}
	}
	return nil
}

// GetParameters retrieves all parameters with the specified name.
func (job *OpsJob) GetParameters(name string) []*Parameter {
	var result []*Parameter
	for i, param := range job.Spec.Inputs {
		if param.Name == name {
			result = append(result, &job.Spec.Inputs[i])
		}
	}
	return result
}

// CvtParamToString converts data to the target format.
func CvtParamToString(p *Parameter) string {
	return p.Name + ":" + p.Value
}

// CvtStringToParam converts data to the target format.
func CvtStringToParam(str string) *Parameter {
	splitArray := strings.Split(str, ":")
	if len(splitArray) != 2 {
		return nil
	}
	return &Parameter{
		Name:  splitArray[0],
		Value: splitArray[1],
	}
}
