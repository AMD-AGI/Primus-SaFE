/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
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
	m.mutateMeta(ctx, job)
	m.mutateJobSpec(job)
	m.mutateJobInputs(ctx, job)
	return true
}

func (m *OpsJobMutator) mutateMeta(ctx context.Context, job *v1.OpsJob) bool {
	jobName := v1.OpsJobKind + "-" + string(job.Spec.Type)
	job.Name = commonutils.GenerateName(strings.ToLower(jobName))

	v1.SetLabel(job, v1.OpsJobTypeLabel, string(job.Spec.Type))
	if v1.GetAnnotation(job, v1.OpsJobBatchCountAnnotation) == "" && commonconfig.GetOpsJobBatchCount() > 0 {
		v1.SetAnnotation(job, v1.OpsJobBatchCountAnnotation, strconv.Itoa(commonconfig.GetOpsJobBatchCount()))
	}

	if job.Spec.Cluster != "" {
		v1.SetLabel(job, v1.ClusterIdLabel, job.Spec.Cluster)
		cl := &v1.Cluster{}
		if err := m.Get(ctx, client.ObjectKey{Name: job.Spec.Cluster}, cl); err == nil {
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

func (m *OpsJobMutator) mutateJobSpec(job *v1.OpsJob) {
	if job.Spec.TTLSecondsAfterFinished <= 0 {
		job.Spec.TTLSecondsAfterFinished = commonconfig.GetOpsJobTTLSecond()
	}
	if job.Spec.TimeoutSecond == 0 {
		job.Spec.TimeoutSecond = commonconfig.GetOpsJobTimeoutSecond()
	}
	for i := range job.Spec.Inputs {
		job.Spec.Inputs[i].Name = stringutil.NormalizeName(job.Spec.Inputs[i].Name)
	}
}

func (m *OpsJobMutator) mutateJobInputs(ctx context.Context, job *v1.OpsJob) {
	param := job.GetParameter(v1.ParameterNodeTemplate)
	if param == nil {
		return
	}
	nt := &v1.NodeTemplate{}
	if err := m.Get(ctx, client.ObjectKey{Name: param.Value}, nt); err != nil {
		return
	}
	currentAddOns := sets.NewSet()
	for _, p := range job.Spec.Inputs {
		if p.Name == v1.ParameterAddonTemplate {
			currentAddOns.Insert(p.Value)
		}
	}
	for _, addOn := range nt.Spec.AddOnTemplates {
		if currentAddOns.Has(addOn) {
			continue
		}
		job.Spec.Inputs = append(job.Spec.Inputs, v1.Parameter{
			Name:  v1.ParameterAddonTemplate,
			Value: addOn,
		})
	}
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
	var err error
	switch job.Spec.Type {
	case v1.OpsJobAddonType:
		if err = v.validateAddonTemplate(ctx, job); err != nil {
			break
		}
		err = v.validateNodeDuplicated(ctx, job)
	case v1.OpsJobDumplogType:
		err = v.validateWorkloadDuplicated(ctx, job)
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
	if job.Spec.Type == "" {
		errs = append(errs, fmt.Errorf("the type of ops job is empty"))
	}
	if job.Spec.Cluster == "" {
		errs = append(errs, fmt.Errorf("the cluster of ops job is empty"))
	}
	cl := &v1.Cluster{}
	if err := v.Get(ctx, client.ObjectKey{Name: job.Spec.Cluster}, cl); err != nil {
		errs = append(errs, err)
	}
	if len(job.Spec.Inputs) == 0 {
		errs = append(errs, fmt.Errorf("the inputs of ops job are empty"))
	}
	if err := utilerrors.NewAggregate(errs); err != nil {
		return err
	}
	return nil
}

func (v *OpsJobValidator) validateNodeDuplicated(ctx context.Context, job *v1.OpsJob) error {
	currentJobs, err := v.listRelatedRunningJobs(ctx, job)
	if err != nil {
		return err
	}
	for _, currentJob := range currentJobs {
		// If the node parameter is empty, it indicates all nodes.
		if job.GetParameter(v1.ParameterNode) == nil || currentJob.GetParameter(v1.ParameterNode) == nil ||
			v.hasDuplicateInput(job.Spec.Inputs, currentJob.Spec.Inputs, v1.ParameterNode) {
			return commonerrors.NewResourceProcessing(
				fmt.Sprintf("another ops job (%s) is running, job.type: %s", currentJob.Name, currentJob.Spec.Type))
		}
	}
	return nil
}

func (v *OpsJobValidator) validateWorkloadDuplicated(ctx context.Context, job *v1.OpsJob) error {
	currentJobs, err := v.listRelatedRunningJobs(ctx, job)
	if err != nil {
		return err
	}
	for _, currentJob := range currentJobs {
		if v.hasDuplicateInput(job.Spec.Inputs, currentJob.Spec.Inputs, v1.ParameterWorkload) {
			return commonerrors.NewResourceProcessing(
				fmt.Sprintf("another ops job (%s) is running, job.type: %s", currentJob.Name, currentJob.Spec.Type))
		}
	}
	return nil
}

func (v *OpsJobValidator) validateAddonTemplate(ctx context.Context, job *v1.OpsJob) error {
	hasFound := false
	for _, p := range job.Spec.Inputs {
		if p.Name != v1.ParameterAddonTemplate {
			continue
		}
		addonTemplate := &v1.AddonTemplate{}
		err := v.Get(ctx, client.ObjectKey{Name: p.Value}, addonTemplate)
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
	if newJob.Spec.Cluster != oldJob.Spec.Cluster {
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

func (v *OpsJobValidator) listRelatedRunningJobs(ctx context.Context, job *v1.OpsJob) ([]v1.OpsJob, error) {
	labelSelector := labels.SelectorFromSet(map[string]string{
		v1.ClusterIdLabel: job.Spec.Cluster, v1.OpsJobTypeLabel: string(job.Spec.Type)})
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
