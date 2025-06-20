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
	JobKind = "Job"
)

type JobPhase string
type JobType string

const (
	JobSucceeded JobPhase = "Succeeded"
	JobFailed    JobPhase = "Failed"
	JobRunning   JobPhase = "Running"
	JobTimeout   JobPhase = "Timeout"
	JobPending   JobPhase = "Pending"

	JobAddonType JobType = "addon"

	ParameterNode          = "node"
	ParameterNodeTemplate  = "node.template"
	ParameterAddonTemplate = "addon.template"

	NodeJob = "NodeJob"
)

type Parameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type JobSpec struct {
	// the type of job
	Type JobType `json:"type"`
	// the cluster which the job belongs to
	Cluster string `json:"cluster"`
	// The resource objects to be processed, e.g., node. Multiple entries will be processed sequentially.
	Inputs []Parameter `json:"inputs"`
	// the lifecycle of job
	TTLSecondsAfterFinished int `json:"ttlSecondsAfterFinished,omitempty"`
	// job Timeout (in seconds), Less than or equal to 0 means no timeout
	TimeoutSecond int `json:"timeoutSecond,omitempty"`
}

type JobStatus struct {
	// job's start time
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
	// job's end time
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`
	// job's conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// job's phase
	Phase JobPhase `json:"phase,omitempty"`
	// error message
	Message string `json:"message,omitempty"`
	// job's output
	Outputs []Parameter `json:"outputs,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:webhook:path=/mutate-amd-primus-safe-v1-job,mutating=true,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=jobs,verbs=create;update,versions=v1,name=mjob.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:webhook:path=/validate-amd-primus-safe-v1-job,mutating=false,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=jobs,verbs=create;update,versions=v1,name=vjob.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:rbac:groups=amd.com,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=amd.com,resources=jobs/status,verbs=get;update;patch

type Job struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   JobSpec   `json:"spec,omitempty"`
	Status JobStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

type JobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Job `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Job{}, &JobList{})
}

func (job *Job) IsEnd() bool {
	if job.IsFinished() {
		return true
	}
	if !job.GetDeletionTimestamp().IsZero() {
		return true
	}
	return false
}

func (job *Job) IsPending() bool {
	if job.Status.Phase == JobPending || job.Status.Phase == "" {
		return true
	}
	return false
}

func (job *Job) IsTimeout() bool {
	if job.Spec.TimeoutSecond <= 0 {
		return false
	}
	costTime := time.Now().Unix() - job.CreationTimestamp.Unix()
	return int(costTime) >= job.Spec.TimeoutSecond
}

func (job *Job) IsFinished() bool {
	if job.Status.FinishedAt != nil {
		return true
	}
	return false
}

func (job *Job) GetParameter(name string) *Parameter {
	for i, param := range job.Spec.Inputs {
		if param.Name == name {
			return &job.Spec.Inputs[i]
		}
	}
	return nil
}

func (job *Job) GetParameters(name string) []*Parameter {
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
