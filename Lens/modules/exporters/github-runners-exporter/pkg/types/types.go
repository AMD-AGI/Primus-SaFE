package types

import (
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersionResources for GitHub Actions Runner Controller CRDs
var (
	AutoScalingRunnerSetGVR = schema.GroupVersionResource{
		Group:    "actions.github.com",
		Version:  "v1alpha1",
		Resource: "autoscalingrunnersets",
	}

	EphemeralRunnerGVR = schema.GroupVersionResource{
		Group:    "actions.github.com",
		Version:  "v1alpha1",
		Resource: "ephemeralrunners",
	}

	AutoScalingRunnerSetGVK = schema.GroupVersionKind{
		Group:   "actions.github.com",
		Version: "v1alpha1",
		Kind:    "AutoscalingRunnerSet",
	}

	EphemeralRunnerGVK = schema.GroupVersionKind{
		Group:   "actions.github.com",
		Version: "v1alpha1",
		Kind:    "EphemeralRunner",
	}
)

// Annotations on EphemeralRunner that contain GitHub info
const (
	AnnotationRunID      = "actions.github.com/run-id"
	AnnotationRunNumber  = "actions.github.com/run-number"
	AnnotationJobID      = "actions.github.com/job-id"
	AnnotationWorkflow   = "actions.github.com/workflow"
	AnnotationRepository = "actions.github.com/repository"
	AnnotationBranch     = "actions.github.com/branch"
	AnnotationSHA        = "actions.github.com/sha"
)

// Labels on EphemeralRunner
const (
	LabelScaleSetName      = "actions.github.com/scale-set-name"
	LabelScaleSetNamespace = "actions.github.com/scale-set-namespace"
)

// EphemeralRunner status phases
const (
	EphemeralRunnerPhasePending   = "Pending"
	EphemeralRunnerPhaseRunning   = "Running"
	EphemeralRunnerPhaseSucceeded = "Succeeded"
	EphemeralRunnerPhaseFailed    = "Failed"
)

// AutoScalingRunnerSetInfo holds parsed information from an AutoScalingRunnerSet
type AutoScalingRunnerSetInfo struct {
	UID                string
	Name               string
	Namespace          string
	GithubConfigURL    string
	GithubConfigSecret string
	RunnerGroup        string
	GithubOwner        string
	GithubRepo         string
	MinRunners         int
	MaxRunners         int
	CurrentRunners     int
	DesiredRunners     int
	CreationTimestamp  metav1.Time
}

// EphemeralRunnerInfo holds parsed information from an EphemeralRunner
type EphemeralRunnerInfo struct {
	UID               string
	Name              string
	Namespace         string
	Phase             string
	RunnerSetName     string
	GithubRunID       int64
	GithubRunNumber   int
	GithubJobID       int64
	WorkflowName      string
	Repository        string
	Branch            string
	HeadSHA           string
	CreationTimestamp metav1.Time
	CompletionTime    metav1.Time
	IsCompleted       bool
}

// ParseAutoScalingRunnerSet parses an unstructured object into AutoScalingRunnerSetInfo
func ParseAutoScalingRunnerSet(obj *unstructured.Unstructured) *AutoScalingRunnerSetInfo {
	info := &AutoScalingRunnerSetInfo{
		UID:               string(obj.GetUID()),
		Name:              obj.GetName(),
		Namespace:         obj.GetNamespace(),
		CreationTimestamp: obj.GetCreationTimestamp(),
	}

	// Extract spec fields
	spec, found, _ := unstructured.NestedMap(obj.Object, "spec")
	if found {
		if url, ok, _ := unstructured.NestedString(spec, "githubConfigUrl"); ok {
			info.GithubConfigURL = url
			info.GithubOwner, info.GithubRepo = ParseGitHubURL(url)
		}
		if secret, ok, _ := unstructured.NestedString(spec, "githubConfigSecret"); ok {
			info.GithubConfigSecret = secret
		}
		if runnerGroup, ok, _ := unstructured.NestedString(spec, "runnerGroup"); ok {
			info.RunnerGroup = runnerGroup
		}
		if minRunners, ok, _ := unstructured.NestedInt64(spec, "minRunners"); ok {
			info.MinRunners = int(minRunners)
		}
		if maxRunners, ok, _ := unstructured.NestedInt64(spec, "maxRunners"); ok {
			info.MaxRunners = int(maxRunners)
		}
	}

	// Extract status fields
	status, found, _ := unstructured.NestedMap(obj.Object, "status")
	if found {
		if current, ok, _ := unstructured.NestedInt64(status, "currentRunners"); ok {
			info.CurrentRunners = int(current)
		}
		if desired, ok, _ := unstructured.NestedInt64(status, "desiredRunners"); ok {
			info.DesiredRunners = int(desired)
		}
	}

	return info
}

// ParseEphemeralRunner parses an unstructured object into EphemeralRunnerInfo
func ParseEphemeralRunner(obj *unstructured.Unstructured) *EphemeralRunnerInfo {
	info := &EphemeralRunnerInfo{
		UID:               string(obj.GetUID()),
		Name:              obj.GetName(),
		Namespace:         obj.GetNamespace(),
		CreationTimestamp: obj.GetCreationTimestamp(),
	}

	// Extract labels - runner set name is in labels
	labels := obj.GetLabels()
	if labels != nil {
		if scaleSetName, ok := labels[LabelScaleSetName]; ok {
			info.RunnerSetName = scaleSetName
		}
	}

	// Extract annotations
	annotations := obj.GetAnnotations()
	if annotations != nil {
		if runID, ok := annotations[AnnotationRunID]; ok {
			info.GithubRunID = parseAnnotationInt64(runID)
		}
		if runNumber, ok := annotations[AnnotationRunNumber]; ok {
			info.GithubRunNumber = int(parseAnnotationInt64(runNumber))
		}
		if jobID, ok := annotations[AnnotationJobID]; ok {
			info.GithubJobID = parseAnnotationInt64(jobID)
		}
		if workflow, ok := annotations[AnnotationWorkflow]; ok {
			info.WorkflowName = workflow
		}
		if repo, ok := annotations[AnnotationRepository]; ok {
			info.Repository = repo
		}
		if branch, ok := annotations[AnnotationBranch]; ok {
			info.Branch = branch
		}
		if sha, ok := annotations[AnnotationSHA]; ok {
			info.HeadSHA = sha
		}
	}

	// Fallback: try to get runner set name from spec (for older versions)
	if info.RunnerSetName == "" {
		spec, found, _ := unstructured.NestedMap(obj.Object, "spec")
		if found {
			if runnerSetName, ok, _ := unstructured.NestedString(spec, "runnerSetName"); ok {
				info.RunnerSetName = runnerSetName
			}
		}
	}

	// Extract status fields
	status, found, _ := unstructured.NestedMap(obj.Object, "status")
	if found {
		if phase, ok, _ := unstructured.NestedString(status, "phase"); ok {
			info.Phase = phase
			info.IsCompleted = phase == EphemeralRunnerPhaseSucceeded || phase == EphemeralRunnerPhaseFailed
		}
		// Try to get completion time
		if completionTime, ok, _ := unstructured.NestedString(status, "completionTime"); ok {
			if t, err := time.Parse(time.RFC3339, completionTime); err == nil {
				info.CompletionTime = metav1.Time{Time: t}
			}
		}
	}

	return info
}

// ParseGitHubURL parses a GitHub URL and extracts owner and repo
func ParseGitHubURL(url string) (owner, repo string) {
	url = strings.TrimSuffix(url, "/")
	parts := strings.Split(url, "/")

	if len(parts) < 4 {
		return "", ""
	}

	ghIndex := -1
	for i, part := range parts {
		if strings.Contains(part, "github.com") {
			ghIndex = i
			break
		}
	}

	if ghIndex < 0 || ghIndex+1 >= len(parts) {
		return "", ""
	}

	owner = parts[ghIndex+1]
	if ghIndex+2 < len(parts) {
		repo = parts[ghIndex+2]
	}

	return owner, repo
}

// parseAnnotationInt64 parses an annotation value as int64
func parseAnnotationInt64(value string) int64 {
	var result int64
	if _, err := strings.NewReader(value).Read(nil); err == nil {
		// Use simple parsing
		for _, c := range value {
			if c >= '0' && c <= '9' {
				result = result*10 + int64(c-'0')
			}
		}
	}
	return result
}

