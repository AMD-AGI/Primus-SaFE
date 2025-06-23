/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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
	OpsJobTimeout   OpsJobPhase = "Timeout"
	OpsJobPending   OpsJobPhase = "Pending"

	OpsJobAddonType OpsJobType = "addon"

	ParameterNode          = "node"
	ParameterNodeTemplate  = "node.template"
	ParameterAddonTemplate = "addon.template"
)

type Parameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type OpsJobSpec struct {
	// the type of ops job, valid values include: addon
	Type OpsJobType `json:"type"`
	// the cluster which the ops job belongs to
	Cluster string `json:"cluster"`
	// The resource objects to be processed, e.g., node. Multiple entries will be processed sequentially.
	Inputs []Parameter `json:"inputs"`
	// the lifecycle of ops job
	TTLSecondsAfterFinished int `json:"ttlSecondsAfterFinished,omitempty"`
	// ops job Timeout (in seconds), Less than or equal to 0 means no timeout
	TimeoutSecond int `json:"timeoutSecond,omitempty"`
}

type OpsJobStatus struct {
	// ops job's start time
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
	// ops job's end time
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`
	// ops job's conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ops job's phase
	Phase OpsJobPhase `json:"phase,omitempty"`
	// error message
	Message string `json:"message,omitempty"`
	// ops job's output
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

func (job *OpsJob) IsEnd() bool {
	if job.IsFinished() {
		return true
	}
	if !job.GetDeletionTimestamp().IsZero() {
		return true
	}
	return false
}

func (job *OpsJob) IsPending() bool {
	if job.Status.Phase == OpsJobPending || job.Status.Phase == "" {
		return true
	}
	return false
}

func (job *OpsJob) IsTimeout() bool {
	if job.Spec.TimeoutSecond <= 0 {
		return false
	}
	costTime := time.Now().Unix() - job.CreationTimestamp.Unix()
	return int(costTime) >= job.Spec.TimeoutSecond
}

func (job *OpsJob) IsFinished() bool {
	if job.Status.FinishedAt != nil {
		return true
	}
	return false
}

func (job *OpsJob) GetParameter(name string) *Parameter {
	for i, param := range job.Spec.Inputs {
		if param.Name == name {
			return &job.Spec.Inputs[i]
		}
	}
	return nil
}

func (job *OpsJob) GetParameters(name string) []*Parameter {
	var result []*Parameter
	for i, param := range job.Spec.Inputs {
		if param.Name == name {
			result = append(result, &job.Spec.Inputs[i])
		}
	}
	return result
}

func CvtParamToString(p *Parameter) string {
	return p.Name + ":" + p.Value
}

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
