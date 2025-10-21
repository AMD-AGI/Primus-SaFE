/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

func AddOpsJobWebhook(mgr ctrlruntime.Manager, server *webhook.Server, decoder admission.Decoder) {
	(*server).Register(generateMutatePath(v1.OpsJobKind), &webhook.Admission{Handler: &OpsJobMutator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
	(*server).Register(generateValidatePath(v1.OpsJobKind), &webhook.Admission{Handler: &OpsJobValidator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
}

type OpsJobMutator struct {
	client.Client
	decoder admission.Decoder
}

func (m *OpsJobMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create {
		return admission.Allowed("")
	}

	job := &v1.OpsJob{}
	if err := m.decoder.Decode(req, job); err != nil {
		return handleError(v1.OpsJobKind, err)
	}
	m.mutateOnCreation(ctx, job)
	data, err := json.Marshal(job)
	if err != nil {
		return handleError(v1.OpsJobKind, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, data)
}

func (m *OpsJobMutator) mutateOnCreation(ctx context.Context, job *v1.OpsJob) bool {
	m.mutateJobInputs(ctx, job)
	m.mutateMeta(ctx, job)
	m.mutateJobSpec(ctx, job)
	return true
}

func (m *OpsJobMutator) mutateMeta(ctx context.Context, job *v1.OpsJob) bool {
	job.Name = stringutil.NormalizeName(job.Name)
	v1.SetLabel(job, v1.OpsJobTypeLabel, string(job.Spec.Type))

	if v1.GetClusterId(job) == "" || v1.GetNodeFlavorId(job) == "" {
		if nodeParam := job.GetParameter(v1.ParameterNode); nodeParam != nil {
			if node, err := getNode(ctx, m.Client, nodeParam.Value); err == nil {
				v1.SetLabel(job, v1.ClusterIdLabel, v1.GetClusterId(node))
				v1.SetLabel(job, v1.NodeFlavorIdLabel, node.GetSpecNodeFlavor())
			}
		}
	}
	if clusterId := v1.GetClusterId(job); clusterId != "" {
		if cl, err := getCluster(ctx, m.Client, clusterId); err == nil {
			if !hasOwnerReferences(job, cl.Name) {
				if err = controllerutil.SetControllerReference(cl, job, m.Client.Scheme()); err != nil {
					klog.ErrorS(err, "failed to SetControllerReference")
				}
			}
		}
	}
	controllerutil.AddFinalizer(job, v1.OpsJobFinalizer)
	return true
}

func (m *OpsJobMutator) mutateJobSpec(ctx context.Context, job *v1.OpsJob) {
	if job.Spec.TTLSecondsAfterFinished <= 0 {
		job.Spec.TTLSecondsAfterFinished = commonconfig.GetOpsJobTTLSecond()
	}
	if job.Spec.TimeoutSecond == 0 {
		job.Spec.TimeoutSecond = commonconfig.GetOpsJobTimeoutSecond()
	}
	for i := range job.Spec.Inputs {
		job.Spec.Inputs[i].Name = stringutil.NormalizeName(job.Spec.Inputs[i].Name)
	}
	if job.Spec.Resource != nil {
		if job.Spec.Resource.GPU != "" {
			nf, err := getNodeFlavor(ctx, m.Client, v1.GetNodeFlavorId(job))
			if err == nil && nf.HasGpu() {
				job.Spec.Resource.GPUName = nf.Spec.Gpu.ResourceName
			}
		}
		job.Spec.Resource.Replica = 0
		for _, param := range job.Spec.Inputs {
			if param.Name == v1.ParameterNode {
				job.Spec.Resource.Replica++
			}
		}
	}
}

func (m *OpsJobMutator) mutateJobInputs(ctx context.Context, job *v1.OpsJob) {
	m.generateAddonTemplates(ctx, job)
	m.removeDuplicates(job)
	m.filterUnhealthyNodes(ctx, job)
}

// generateAddonTemplates retrieves the NodeTemplate specified in the job's parameters and
// appends its addon templates to the job's inputs as ParameterAddonTemplate parameters.
func (m *OpsJobMutator) generateAddonTemplates(ctx context.Context, job *v1.OpsJob) {
	param := job.GetParameter(v1.ParameterNodeTemplate)
	if param == nil {
		return
	}
	nt := &v1.NodeTemplate{}
	if err := m.Get(ctx, client.ObjectKey{Name: param.Value}, nt); err != nil {
		return
	}
	for _, addOn := range nt.Spec.AddOnTemplates {
		job.Spec.Inputs = append(job.Spec.Inputs, v1.Parameter{
			Name:  v1.ParameterAddonTemplate,
			Value: addOn,
		})
	}
}

// removeDuplicates removes duplicate parameters from the job's inputs based on parameter name and value.
// It ensures that each parameter with the same name and value combination appears only once in the inputs list.
func (m *OpsJobMutator) removeDuplicates(job *v1.OpsJob) {
	uniqMap := make(map[string]string)
	uniqInputs := make([]v1.Parameter, 0, len(job.Spec.Inputs))
	for i, in := range job.Spec.Inputs {
		val, ok := uniqMap[in.Name]
		if ok && val == in.Value {
			continue
		}
		uniqInputs = append(uniqInputs, job.Spec.Inputs[i])
		uniqMap[in.Name] = in.Value
	}
	job.Spec.Inputs = uniqInputs
}

// For preflight jobs, if tolerate taints are not set, remove unhealthy nodes. Additionally, ignore preflight taints.
func (m *OpsJobMutator) filterUnhealthyNodes(ctx context.Context, job *v1.OpsJob) {
	if job.Spec.Type != v1.OpsJobPreflightType {
		return
	}
	newInputs := make([]v1.Parameter, 0, len(job.Spec.Inputs))
	for i, p := range job.Spec.Inputs {
		if p.Name != v1.ParameterNode {
			newInputs = append(newInputs, job.Spec.Inputs[i])
			continue
		}
		node, err := getNode(ctx, m.Client, p.Value)
		if err != nil || !node.IsMachineReady() || !node.GetDeletionTimestamp().IsZero() {
			continue
		}
		if job.Spec.IsTolerateAll {
			// do nothing
		} else if len(node.Status.Taints) > 1 {
			continue
		} else if len(node.Status.Taints) == 1 {
			monitorId := ""
			switch job.Spec.Type {
			case v1.OpsJobPreflightType:
				monitorId = common.PreflightMonitorId
			}
			if node.Status.Taints[0].Key != commonfaults.GenerateTaintKey(monitorId) {
				continue
			}
		}
		newInputs = append(newInputs, job.Spec.Inputs[i])
	}
	if len(job.Spec.Inputs) == len(newInputs) {
		return
	}
	job.Spec.Inputs = newInputs
}

type OpsJobValidator struct {
	client.Client
	decoder admission.Decoder
}

func (v *OpsJobValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	job := &v1.OpsJob{}
	var err error
	switch req.Operation {
	case admissionv1.Create:
		if err = v.decoder.Decode(req, job); err != nil {
			break
		}
		err = v.validateOnCreation(ctx, job)
	case admissionv1.Update:
		if err = v.decoder.Decode(req, job); err != nil {
			break
		}
		if !job.GetDeletionTimestamp().IsZero() {
			break
		}
		oldJob := &v1.OpsJob{}
		if err = v.decoder.DecodeRaw(req.OldObject, oldJob); err == nil {
			err = v.validateOnUpdate(ctx, job, oldJob)
		}
	default:
	}
	if err != nil {
		return handleError(v1.OpsJobKind, err)
	}
	return admission.Allowed("")
}

func (v *OpsJobValidator) validateOnCreation(ctx context.Context, job *v1.OpsJob) error {
	if err := v.validateRequiredParams(ctx, job); err != nil {
		return err
	}
	if err := v.validateNodes(ctx, job); err != nil {
		return err
	}
	var err error
	switch job.Spec.Type {
	case v1.OpsJobAddonType:
		err = v.validateAddon(ctx, job)
	case v1.OpsJobPreflightType:
		err = v.validatePreflight(ctx, job)
	case v1.OpsJobDumpLogType:
		err = v.validateDumplog(ctx, job)
	}
	if err != nil {
		return err
	}
	return nil
}

func (v *OpsJobValidator) validateOnUpdate(ctx context.Context, newJob, oldJob *v1.OpsJob) error {
	if err := v.validateRequiredParams(ctx, newJob); err != nil {
		return err
	}
	if err := v.validateImmutableFields(newJob, oldJob); err != nil {
		return err
	}
	return nil
}

func (v *OpsJobValidator) validateRequiredParams(ctx context.Context, job *v1.OpsJob) error {
	var errs []error
	if v1.GetDisplayName(job) == "" {
		errs = append(errs, fmt.Errorf("the displayName is empty"))
	}
	if err := validateDisplayName(v1.GetDisplayName(job)); err != nil {
		errs = append(errs, err)
	}
	if job.Spec.Type == "" {
		errs = append(errs, fmt.Errorf("the type of ops job is empty"))
	}
	if _, err := getCluster(ctx, v.Client, v1.GetClusterId(job)); err != nil {
		errs = append(errs, err)
	}
	if len(job.Spec.Inputs) == 0 {
		errs = append(errs, fmt.Errorf("the inputs of ops job are empty"))
	}
	if job.Spec.Type == v1.OpsJobAddonType || job.Spec.Type == v1.OpsJobPreflightType {
		if job.GetParameter(v1.ParameterNode) == nil {
			errs = append(errs, fmt.Errorf("opsjob nodes are either empty or unhealthy"))
		}
	}
	if err := utilerrors.NewAggregate(errs); err != nil {
		return err
	}
	return nil
}

// validateNodeDuplicated checks if there are any other running jobs of the same type that are using the same node inputs.
// It prevents multiple jobs from running simultaneously on the same nodes to avoid conflicts and resource contention.
func (v *OpsJobValidator) validateNodeDuplicated(ctx context.Context, job *v1.OpsJob) error {
	currentJobs, err := v.listRelatedRunningJobs(ctx, v1.GetClusterId(job), []string{string(job.Spec.Type)})
	if err != nil {
		return err
	}
	for _, currentJob := range currentJobs {
		if job.Name == currentJob.Name {
			continue
		}
		if v.hasDuplicateInput(job.Spec.Inputs, currentJob.Spec.Inputs, v1.ParameterNode) {
			return commonerrors.NewResourceProcessing(
				fmt.Sprintf("another ops job (%s) is running, job.type: %s", currentJob.Name, currentJob.Spec.Type))
		}
	}
	return nil
}

// validatePreflight validates the preflight job by checking for duplicate node inputs,
// ensuring required fields are not empty, and verifying resource requirements do not exceed thresholds.
func (v *OpsJobValidator) validatePreflight(ctx context.Context, job *v1.OpsJob) error {
	err := v.validateNodeDuplicated(ctx, job)
	if err != nil {
		return err
	}
	if job.Spec.Resource == nil {
		return fmt.Errorf("the resource of job is empty")
	}
	if job.Spec.Image == nil || *job.Spec.Image == "" {
		return fmt.Errorf("the image of job is empty")
	}
	if job.Spec.EntryPoint == nil || *job.Spec.EntryPoint == "" {
		return fmt.Errorf("the entryPoint of job is empty")
	}
	nf, err := getNodeFlavor(ctx, v.Client, v1.GetNodeFlavorId(job))
	if err != nil {
		return err
	}
	return validateResourceEnough(nf, job.Spec.Resource)
}

// validateDumplog validates the dumplog job by checking if there are any other running dumplog jobs
// with the same workload input parameters. It prevents multiple dumplog jobs from running simultaneously
// on the same workload to avoid conflicts and resource contention.
func (v *OpsJobValidator) validateDumplog(ctx context.Context, job *v1.OpsJob) error {
	currentJobs, err := v.listRelatedRunningJobs(ctx, v1.GetClusterId(job), []string{string(v1.OpsJobDumpLogType)})
	if err != nil {
		return err
	}
	for _, currentJob := range currentJobs {
		if job.Name == currentJob.Name {
			continue
		}
		if v.hasDuplicateInput(job.Spec.Inputs, currentJob.Spec.Inputs, v1.ParameterWorkload) {
			return commonerrors.NewResourceProcessing(
				fmt.Sprintf("another ops job (%s) is running, job.type: %s", currentJob.Name, currentJob.Spec.Type))
		}
	}
	return nil
}

// validateAddon validates addon jobs by checking for duplicate node usage and ensuring addon template parameters are present.
func (v *OpsJobValidator) validateAddon(ctx context.Context, job *v1.OpsJob) error {
	err := v.validateNodeDuplicated(ctx, job)
	if err != nil {
		return err
	}
	hasFound := false
	for _, p := range job.Spec.Inputs {
		if p.Name != v1.ParameterAddonTemplate {
			continue
		}
		addonTemplate := &v1.AddonTemplate{}
		err = v.Get(ctx, client.ObjectKey{Name: p.Value}, addonTemplate)
		if err != nil {
			return err
		}
		if addonTemplate.Spec.Type == v1.AddonTemplateHelm {
			return commonerrors.NewBadRequest("The addon job does not support Helm installation.")
		}
		hasFound = true
	}
	if !hasFound {
		return commonerrors.NewBadRequest(
			fmt.Sprintf("either %s or %s must be specified in the job.",
				v1.ParameterAddonTemplate, v1.ParameterNodeTemplate))
	}
	return nil
}

func (v *OpsJobValidator) validateImmutableFields(newJob, oldJob *v1.OpsJob) error {
	if v1.GetClusterId(newJob) != v1.GetClusterId(oldJob) {
		return field.Forbidden(field.NewPath("spec").Key("cluster"), "immutable")
	}
	if newJob.Spec.Type != oldJob.Spec.Type {
		return field.Forbidden(field.NewPath("spec").Key("type"), "immutable")
	}
	if !reflect.DeepEqual(newJob.Spec.Inputs, oldJob.Spec.Inputs) {
		return field.Forbidden(field.NewPath("spec").Key("inputs"), "immutable")
	}
	return nil
}

func (v *OpsJobValidator) hasDuplicateInput(params1, params2 []v1.Parameter, paramName string) bool {
	params1Map := make(map[string]string)
	for _, p := range params1 {
		if paramName == p.Name {
			params1Map[p.Name] = p.Value
		}
	}
	for _, p := range params2 {
		if p.Name != paramName {
			continue
		}
		value2, ok := params1Map[p.Name]
		if ok && value2 == p.Value {
			return true
		}
	}
	return false
}

// Find the running opsjob of the same type.
func (v *OpsJobValidator) listRelatedRunningJobs(ctx context.Context, cluster string, jobTypes []string) ([]v1.OpsJob, error) {
	var labelSelector = labels.NewSelector()
	req1, _ := labels.NewRequirement(v1.ClusterIdLabel, selection.Equals, []string{cluster})
	labelSelector = labelSelector.Add(*req1)
	req2, _ := labels.NewRequirement(v1.OpsJobTypeLabel, selection.In, jobTypes)
	labelSelector = labelSelector.Add(*req2)

	jobList := &v1.OpsJobList{}
	if err := v.List(ctx, jobList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	result := make([]v1.OpsJob, 0, len(jobList.Items))
	for i := range jobList.Items {
		if jobList.Items[i].IsEnd() {
			continue
		}
		result = append(result, jobList.Items[i])
	}
	return result, nil
}

// Check whether the nodes involved in the ops job belong to the same cluster and the same node flavor.
// Additionally, both cluster and node flavor must not be empty.
func (v *OpsJobValidator) validateNodes(ctx context.Context, job *v1.OpsJob) error {
	nodeParams := job.GetParameters(v1.ParameterNode)
	clusterId := ""
	nodeFlavor := ""
	for _, param := range nodeParams {
		adminNode, err := getNode(ctx, v.Client, param.Value)
		if err != nil {
			return err
		}
		if clusterId == "" {
			if clusterId = v1.GetClusterId(adminNode); clusterId == "" {
				return fmt.Errorf("The node(%s) is not managed by the cluster.", param.Value)
			}
		} else if clusterId != v1.GetClusterId(adminNode) {
			return fmt.Errorf("The nodes to be operated must belong to the same cluster.")
		}

		if nodeFlavor == "" {
			if nodeFlavor = adminNode.GetSpecNodeFlavor(); nodeFlavor == "" {
				return fmt.Errorf("The node(%s) does not have node flavor.", param.Value)
			}
		} else if nodeFlavor != adminNode.GetSpecNodeFlavor() {
			return fmt.Errorf("The nodes to be operated must have the same node flavor.")
		}
	}
	return nil
}
