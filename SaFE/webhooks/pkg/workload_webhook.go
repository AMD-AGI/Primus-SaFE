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
	"strconv"
	"strings"
	"time"

	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/floatutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

const (
	DefaultEphemeralStorage    = "50Gi"
	DefaultInitialDelaySeconds = 600
	DefaultPeriodSeconds       = 3
	DefaultFailureThreshold    = 3
	DefaultMaxUnavailable      = "25%"
	DefaultMaxMaxSurge         = "25%"
	DefaultMaxFailover         = 50

	MaxCICDScaleSetNameLen = 39

	ResourcesEnv  = "RESOURCES"
	ImageEnv      = "IMAGE"
	EntrypointEnv = "ENTRYPOINT"
)

// AddWorkloadWebhook registers the workload validation and mutation webhooks.
func AddWorkloadWebhook(mgr ctrlruntime.Manager, server *webhook.Server, decoder admission.Decoder) {
	(*server).Register(generateMutatePath(v1.WorkloadKind), &webhook.Admission{Handler: &WorkloadMutator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
	(*server).Register(generateValidatePath(v1.WorkloadKind), &webhook.Admission{Handler: &WorkloadValidator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
}

// WorkloadMutator handles mutation logic for Workload resources on create and update.
type WorkloadMutator struct {
	client.Client
	decoder admission.Decoder
}

// Handle processes workload admission requests and applies mutations on create and update.
func (m *WorkloadMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation == admissionv1.Delete {
		return admission.Allowed("")
	}
	workload := &v1.Workload{}
	if err := m.decoder.Decode(req, workload); err != nil {
		return handleError(v1.WorkloadKind, err)
	}
	if !workload.GetDeletionTimestamp().IsZero() {
		return admission.Allowed("")
	}

	isChanged := false
	switch req.Operation {
	case admissionv1.Create:
		isChanged = m.mutateOnCreation(ctx, workload)
	case admissionv1.Update:
		oldObj := &v1.Workload{}
		if m.decoder.DecodeRaw(req.OldObject, oldObj) == nil {
			isChanged = m.mutateOnUpdate(ctx, oldObj, workload)
		}
	}
	if !isChanged {
		return admission.Allowed("")
	}

	data, err := json.Marshal(workload)
	if err != nil {
		return handleError(v1.WorkloadKind, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, data)
}

// mutateOnCreation applies default values and normalizations during creation.
func (m *WorkloadMutator) mutateOnCreation(ctx context.Context, workload *v1.Workload) bool {
	workspace, _ := getWorkspace(ctx, m.Client, workload.Spec.Workspace)
	m.mutateGvk(workload)
	m.mutateMeta(ctx, workload, workspace)
	m.mutateTTLSeconds(workload)
	m.mutateCommon(ctx, nil, workload, workspace)
	return true
}

// mutateOnUpdate applies mutations during updates.
func (m *WorkloadMutator) mutateOnUpdate(ctx context.Context, oldWorkload, newWorkload *v1.Workload) bool {
	workspace, _ := getWorkspace(ctx, m.Client, newWorkload.Spec.Workspace)
	m.mutateCommon(ctx, oldWorkload, newWorkload, workspace)
	return true
}

// mutateCommon normalizes resources, hostpath, priority, image, entry point, host network and so on
func (m *WorkloadMutator) mutateCommon(ctx context.Context, oldWorkload, newWorkload *v1.Workload, workspace *v1.Workspace) bool {
	switch newWorkload.SpecKind() {
	case common.DeploymentKind, common.StatefulSetKind:
		m.mutateDeployment(newWorkload)
	case common.AuthoringKind:
		m.mutateAuthoring(newWorkload)
	case common.CICDScaleRunnerSetKind:
		m.mutateCICDScaleSet(newWorkload)
	}
	m.mutateResource(newWorkload, workspace)
	m.mutateHostpath(newWorkload, workspace)
	m.mutatePriority(newWorkload)
	m.mutateImage(newWorkload)
	m.mutateEntryPoint(newWorkload)
	m.mutateEnv(oldWorkload, newWorkload)
	m.mutateMaxRetry(newWorkload)
	m.mutateHostNetwork(ctx, newWorkload)
	m.mutateCustomerLabels(newWorkload)
	m.mutateCronJobs(newWorkload)
	m.mutateHealthCheck(newWorkload)
	m.mutateService(newWorkload)
	m.mutateSecrets(ctx, newWorkload, workspace)
	return true
}

// mutateMeta sets normalized name, ownership, labels, main container and finalizer.
func (m *WorkloadMutator) mutateMeta(ctx context.Context, workload *v1.Workload, workspace *v1.Workspace) {
	if workload.Name != "" {
		workload.Name = stringutil.NormalizeName(workload.Name)
	}

	m.mutateOwnerReference(ctx, workload, workspace)
	if workspace != nil {
		v1.SetLabel(workload, v1.ClusterIdLabel, workspace.Spec.Cluster)
		v1.SetLabel(workload, v1.NodeFlavorIdLabel, workspace.Spec.NodeFlavor)
		if workspace.Spec.EnablePreempt {
			v1.SetAnnotation(workload, v1.WorkloadEnablePreemptAnnotation, v1.TrueStr)
		}
	}

	if val := workload.GetEnv(common.ScaleRunnerID); val != "" {
		v1.SetLabel(workload, v1.CICDScaleRunnerIdLabel, val)
	}
	v1.SetLabel(workload, v1.WorkspaceIdLabel, workload.Spec.Workspace)
	v1.SetLabel(workload, v1.WorkloadKindLabel, workload.Spec.Kind)
	v1.SetLabel(workload, v1.WorkloadIdLabel, workload.Name)
	if v1.GetUserName(workload) == "" {
		v1.SetAnnotation(workload, v1.UserNameAnnotation, v1.GetUserId(workload))
	}
	v1.SetLabel(workload, v1.UserNameMd5Label, stringutil.MD5(v1.GetUserName(workload)))
	if v1.GetMainContainer(workload) == "" {
		cm, err := commonworkload.GetWorkloadTemplate(ctx, m.Client, workload)
		if err == nil {
			v1.SetAnnotation(workload, v1.MainContainerAnnotation, v1.GetMainContainer(cm))
		}
	}
	controllerutil.AddFinalizer(workload, v1.WorkloadFinalizer)
}

func (m *WorkloadMutator) mutateOwnerReference(ctx context.Context, workload *v1.Workload, workspace *v1.Workspace) {
	var err error
	switch workload.SpecKind() {
	case common.CICDEphemeralRunnerKind:
		scaleRunnerSetId := workload.GetEnv(common.ScaleRunnerSetID)
		if scaleRunnerSetId == "" {
			break
		}
		scaleRunnerSetWorkload := &v1.Workload{}
		if err = m.Get(ctx, client.ObjectKey{Name: scaleRunnerSetId}, scaleRunnerSetWorkload); err == nil {
			if !commonutils.HasOwnerReferences(workload, scaleRunnerSetId) {
				err = controllerutil.SetControllerReference(scaleRunnerSetWorkload, workload, m.Client.Scheme())
			}
			v1.SetAnnotation(workload, v1.CICDScaleSetIdAnnotation, scaleRunnerSetWorkload.Status.RunnerScaleSetId)
		}
	case common.UnifiedJobKind:
		scaleRunnerId := workload.GetEnv(common.ScaleRunnerID)
		if scaleRunnerId == "" {
			break
		}
		labelSelector := labels.SelectorFromSet(map[string]string{
			v1.WorkloadKindLabel: common.CICDEphemeralRunnerKind, v1.CICDScaleRunnerIdLabel: scaleRunnerId})
		scaleRunnerWorkloads := &v1.WorkloadList{}
		if err = m.List(ctx, scaleRunnerWorkloads, &client.ListOptions{LabelSelector: labelSelector}); err == nil {
			if len(scaleRunnerWorkloads.Items) > 0 && !commonutils.HasOwnerReferences(workload, scaleRunnerWorkloads.Items[0].Name) {
				err = controllerutil.SetControllerReference(&scaleRunnerWorkloads.Items[0], workload, m.Client.Scheme())
			}
		}
	default:
		if workspace != nil && !commonutils.HasOwnerReferences(workload, workspace.Name) {
			err = controllerutil.SetControllerReference(workspace, workload, m.Client.Scheme())
		}
	}
	if err != nil {
		klog.ErrorS(err, "failed to SetControllerReference")
	}
}

// mutateGvk defaults kind/version and clears group.
func (m *WorkloadMutator) mutateGvk(workload *v1.Workload) {
	if workload.Spec.Kind == "" {
		workload.Spec.Kind = common.PytorchJobKind
	}
	if workload.Spec.Version == "" {
		workload.Spec.Version = common.DefaultVersion
	}
	// the group is not currently in use
	workload.Spec.Group = ""
}

// mutatePriority clamps priority within allowed bounds.
func (m *WorkloadMutator) mutatePriority(workload *v1.Workload) bool {
	isChanged := false
	if workload.Spec.Priority > common.HighPriorityInt {
		workload.Spec.Priority = common.HighPriorityInt
		isChanged = true
	} else if workload.Spec.Priority < common.LowPriorityInt {
		workload.Spec.Priority = common.LowPriorityInt
		isChanged = true
	}
	return isChanged
}

// mutateResource sets GPU name, shared memory and default ephemeral storage.
func (m *WorkloadMutator) mutateResource(workload *v1.Workload, workspace *v1.Workspace) bool {
	isChanged := false
	if workload.Spec.Resource.GPU == "0" {
		workload.Spec.Resource.GPU = ""
		isChanged = true
	} else if workload.Spec.Resource.GPU != "" && workspace != nil {
		workload.Spec.Resource.GPUName = v1.GetGpuResourceName(workspace)
		isChanged = true
	}

	if workload.Spec.Resource.SharedMemory == "" && workload.Spec.Resource.Memory != "" {
		memQuantity, err := resource.ParseQuantity(workload.Spec.Resource.Memory)
		if err == nil && memQuantity.Value() > 0 {
			shareMemQuantity := resource.NewQuantity(memQuantity.Value()/2, memQuantity.Format)
			if shareMemQuantity != nil {
				workload.Spec.Resource.SharedMemory = shareMemQuantity.String()
				isChanged = true
			}
		}
	}
	if workload.Spec.Resource.EphemeralStorage == "" {
		workload.Spec.Resource.EphemeralStorage = DefaultEphemeralStorage
		isChanged = true
	}
	return isChanged
}

// mutateHostpath removes hostpath duplicated by the workspace; workloads inherit workspace hostpath.
func (m *WorkloadMutator) mutateHostpath(workload *v1.Workload, workspace *v1.Workspace) {
	if len(workload.Spec.Hostpath) == 0 {
		return
	}
	hostpathSet := sets.NewSet()
	if workspace != nil {
		for _, vol := range workspace.Spec.Volumes {
			if vol.Type == v1.HOSTPATH {
				hostpathSet.Insert(vol.HostPath)
			}
		}
	}
	hostpath := make([]string, 0, len(workload.Spec.Hostpath))
	for _, path := range workload.Spec.Hostpath {
		if !hostpathSet.Has(path) {
			hostpath = append(hostpath, path)
			hostpathSet.Insert(path)
		}
	}
	workload.Spec.Hostpath = hostpath
}

// mutateHealthCheck fills default probe timings for liveness/readiness.
func (m *WorkloadMutator) mutateHealthCheck(workload *v1.Workload) {
	if !commonworkload.IsApplication(workload) {
		workload.Spec.Readiness = nil
		workload.Spec.Liveness = nil
		return
	}
	if workload.Spec.Readiness != nil {
		mutateHealthCheck(workload.Spec.Readiness)
	}
	if workload.Spec.Liveness != nil {
		mutateHealthCheck(workload.Spec.Liveness)
	}
}

// mutateHealthCheck sets default initial delay, period and failures.
func mutateHealthCheck(field *v1.HealthCheck) {
	if field.InitialDelaySeconds == 0 {
		field.InitialDelaySeconds = DefaultInitialDelaySeconds
	}
	if field.PeriodSeconds == 0 {
		field.PeriodSeconds = DefaultPeriodSeconds
	}
	if field.FailureThreshold == 0 {
		field.FailureThreshold = DefaultFailureThreshold
	}
}

// mutateService uppercases protocol and defaults to TCP.
func (m *WorkloadMutator) mutateService(workload *v1.Workload) {
	if workload.Spec.Service == nil {
		return
	}
	if workload.Spec.Service.Protocol != "" {
		workload.Spec.Service.Protocol = corev1.Protocol(strings.ToUpper(string(workload.Spec.Service.Protocol)))
	} else {
		workload.Spec.Service.Protocol = corev1.ProtocolTCP
	}
	if workload.Spec.Service.Port == 0 {
		workload.Spec.Service.Port = workload.Spec.Service.TargetPort
	}
	if workload.Spec.Service.Extends == nil {
		workload.Spec.Service.Extends = make(map[string]string)
	}
	if _, ok := workload.Spec.Service.Extends["maxUnavailable"]; !ok {
		workload.Spec.Service.Extends["maxUnavailable"] = DefaultMaxUnavailable
	}
	if _, ok := workload.Spec.Service.Extends["maxSurge"]; !ok {
		workload.Spec.Service.Extends["maxSurge"] = DefaultMaxMaxSurge
	}
}

// isHostNetworkEnabled Check whether to enable hostNetwork. It should only be set to true.
func (m *WorkloadMutator) isHostNetworkEnabled(workload *v1.Workload, nf *v1.NodeFlavor) bool {
	if workload.Spec.Resource.Replica <= 1 {
		return false
	}
	gpuCount := 0
	if nf.HasGpu() {
		gpuCount = int(nf.Spec.Gpu.Quantity.Value())
	}
	if workload.Spec.Resource.GPU == "" {
		return false
	}
	n, err := strconv.Atoi(workload.Spec.Resource.GPU)
	if err != nil || n != gpuCount {
		return false
	}
	return true
}

// mutateDeployment resets supervision and rollout defaults for Deployments.
func (m *WorkloadMutator) mutateDeployment(workload *v1.Workload) {
	workload.Spec.IsSupervised = false
	workload.Spec.MaxRetry = 0
	workload.Spec.Dependencies = nil
}

// mutateAuthoring sets one-replica, entrypoint for Authoring.
func (m *WorkloadMutator) mutateAuthoring(workload *v1.Workload) {
	workload.Spec.IsSupervised = false
	workload.Spec.MaxRetry = 0
	workload.Spec.Resource.Replica = 1
	workload.Spec.Timeout = nil
	workload.Spec.EntryPoint = stringutil.Base64Encode("sleep infinity")
	workload.Spec.Dependencies = nil
}

// mutateCICDScaleSet sets one-replica, disable Supervised for cicd.
func (m *WorkloadMutator) mutateCICDScaleSet(workload *v1.Workload) {
	workload.Spec.IsSupervised = false
	workload.Spec.MaxRetry = 0
	workload.Spec.Resource.Replica = 1
	workload.Spec.Timeout = nil
	workload.Spec.Dependencies = nil
}

// mutateImage trims image name and entry point.
func (m *WorkloadMutator) mutateImage(workload *v1.Workload) {
	workload.Spec.Image = strings.TrimSpace(workload.Spec.Image)
}

// mutateMaxRetry bounds MaxRetry to [0, DefaultMaxFailover].
func (m *WorkloadMutator) mutateMaxRetry(workload *v1.Workload) {
	if workload.Spec.MaxRetry > DefaultMaxFailover {
		workload.Spec.MaxRetry = DefaultMaxFailover
	}
	if workload.Spec.MaxRetry < 0 {
		workload.Spec.MaxRetry = 0
	}
}

// mutateEnv removes empty values and preserves deletions from the old spec.
func (m *WorkloadMutator) mutateEnv(oldWorkload, newWorkload *v1.Workload) {
	newEnv := make(map[string]string)
	for key, val := range newWorkload.Spec.Env {
		newEnv[strings.TrimSpace(key)] = val
	}
	newWorkload.Spec.Env = newEnv

	if oldWorkload != nil {
		var envToBeRemoved []string
		for key := range oldWorkload.Spec.Env {
			if _, ok := newEnv[key]; !ok {
				envToBeRemoved = append(envToBeRemoved, key)
			}
		}
		if len(envToBeRemoved) > 0 {
			v1.SetAnnotation(newWorkload, v1.EnvToBeRemovedAnnotation, string(jsonutils.MarshalSilently(envToBeRemoved)))
		}
	}
}

// mutateTTLSeconds sets a default TTL if not provided.
func (m *WorkloadMutator) mutateTTLSeconds(workload *v1.Workload) {
	if workload.Spec.TTLSecondsAfterFinished == nil {
		workload.Spec.TTLSecondsAfterFinished = ptr.To(commonconfig.GetWorkloadTTLSecond())
	}
}

// mutateEntryPoint base64-encodes entry point for the required jobs.
func (m *WorkloadMutator) mutateEntryPoint(workload *v1.Workload) {
	workload.Spec.EntryPoint = strings.TrimSpace(workload.Spec.EntryPoint)
	if commonworkload.IsAuthoring(workload) || commonworkload.IsOpsJob(workload) {
		return
	}
	if !stringutil.IsBase64(workload.Spec.EntryPoint) {
		workload.Spec.EntryPoint = stringutil.Base64Encode(workload.Spec.EntryPoint)
	}
}

// mutateHostNetwork enables hostNetwork when replica equals per-node GPU count.
// Also sets RDMA resources if enabled and flavor defines RDMA capacity.
func (m *WorkloadMutator) mutateHostNetwork(ctx context.Context, workload *v1.Workload) {
	flavorId := v1.GetNodeFlavorId(workload)
	if flavorId == "" {
		return
	}
	nf, _ := getNodeFlavor(ctx, m.Client, flavorId)
	if nf == nil {
		return
	}
	isEnableHostNetWork := m.isHostNetworkEnabled(workload, nf)
	v1.SetAnnotation(workload, v1.EnableHostNetworkAnnotation, strconv.FormatBool(isEnableHostNetWork))

	rdmaName := commonconfig.GetRdmaName()
	if isEnableHostNetWork && rdmaName != "" {
		rdmaQuantity, ok := nf.Spec.ExtendResources[corev1.ResourceName(rdmaName)]
		if ok {
			workload.Spec.Resource.RdmaResource = rdmaQuantity.String()
		} else {
			workload.Spec.Resource.RdmaResource = "1"
		}
	} else {
		workload.Spec.Resource.RdmaResource = ""
	}
}

// mutateCustomerLabels applies mutations to the resource.
func (m *WorkloadMutator) mutateCustomerLabels(workload *v1.Workload) {
	if len(workload.Spec.CustomerLabels) == 0 {
		return
	}
	var toRemoveKeys []string
	for key, val := range workload.Spec.CustomerLabels {
		if key == "" || val == "" {
			toRemoveKeys = append(toRemoveKeys, key)
		}
	}
	for _, key := range toRemoveKeys {
		delete(workload.Spec.CustomerLabels, key)
	}
}

// mutateCronJobs applies mutations to the resource.
func (m *WorkloadMutator) mutateCronJobs(workload *v1.Workload) {
	for i := range workload.Spec.CronJobs {
		if workload.Spec.CronJobs[i].Action == "" {
			workload.Spec.CronJobs[i].Action = v1.CronStart
		}
	}
}

// mutateSecrets handles workload Secrets configuration by:
// 1. Filtering out invalid/duplicate secrets from the workload spec
// 2. Inheriting ImageSecrets from workspace when available
// 3. Adding default cluster image secret when no workspace exists but global config is present
func (m *WorkloadMutator) mutateSecrets(ctx context.Context, workload *v1.Workload, workspace *v1.Workspace) {
	secretsSet := sets.NewSet()
	newSecrets := make([]v1.SecretEntity, 0, len(workload.Spec.Secrets))
	for i, s := range workload.Spec.Secrets {
		if secretsSet.Has(s.Id) {
			continue
		}
		secret := &corev1.Secret{}
		if m.Get(ctx, types.NamespacedName{Name: s.Id, Namespace: common.PrimusSafeNamespace}, secret) != nil {
			continue
		}
		secretsSet.Insert(s.Id)
		newSecrets = append(newSecrets, workload.Spec.Secrets[i])
	}
	if workspace != nil {
		for _, s := range workspace.Spec.ImageSecrets {
			if secretsSet.Has(s.Name) {
				continue
			}
			secretsSet.Insert(s.Name)
			newSecrets = append(newSecrets, v1.SecretEntity{Id: s.Name, Type: v1.SecretImage})
		}
	} else if commonconfig.GetImageSecret() != "" {
		clusterSecretId := commonutils.GenerateClusterSecret(v1.GetClusterId(workload), commonconfig.GetImageSecret())
		if !secretsSet.Has(clusterSecretId) {
			newSecrets = append(newSecrets, v1.SecretEntity{Id: clusterSecretId, Type: v1.SecretImage})
		}
	}
	workload.Spec.Secrets = newSecrets
}

// WorkloadValidator validates Workload resources on create and update operations.
type WorkloadValidator struct {
	client.Client
	decoder admission.Decoder
}

// Handle validates workload resources on create, update, and delete operations.
func (v *WorkloadValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	workload := &v1.Workload{}
	var err error
	switch req.Operation {
	case admissionv1.Create:
		if err = v.decoder.Decode(req, workload); err != nil {
			break
		}
		if !workload.GetDeletionTimestamp().IsZero() {
			break
		}
		err = v.validateOnCreation(ctx, workload)
	case admissionv1.Update:
		if err = v.decoder.Decode(req, workload); err != nil {
			break
		}
		if !workload.GetDeletionTimestamp().IsZero() {
			break
		}
		oldWorkload := &v1.Workload{}
		if err = v.decoder.DecodeRaw(req.OldObject, oldWorkload); err == nil {
			err = v.validateOnUpdate(ctx, workload, oldWorkload)
		}
	default:
	}
	if err != nil {
		return handleError(v1.WorkloadKind, err)
	}
	return admission.Allowed("")
}

// validateOnCreation validates workload spec, resources, scope and cron jobs on creation.
func (v *WorkloadValidator) validateOnCreation(ctx context.Context, workload *v1.Workload) error {
	if err := v.validateCommon(ctx, workload); err != nil {
		return err
	}
	if err := validateDNSName(v1.GetDisplayName(workload)); err != nil {
		return err
	}
	if err := v.validateScope(ctx, workload); err != nil {
		return err
	}
	if err := v.validateCronJobs(workload); err != nil {
		return err
	}
	return nil
}

// validateOnUpdate validates immutable fields, spec changes and cron jobs on update.
func (v *WorkloadValidator) validateOnUpdate(ctx context.Context, newWorkload, oldWorkload *v1.Workload) error {
	if err := v.validateImmutableFields(newWorkload, oldWorkload); err != nil {
		return err
	}
	if err := v.validateCommon(ctx, newWorkload); err != nil {
		return err
	}
	if !reflect.DeepEqual(oldWorkload.Spec.CronJobs, newWorkload.Spec.CronJobs) {
		if err := v.validateCronJobs(newWorkload); err != nil {
			return err
		}
	}
	return nil
}

// validateCommon validates required params, workspace, service, health check, resources, template and display name.
func (v *WorkloadValidator) validateCommon(ctx context.Context, workload *v1.Workload) error {
	if err := v.validateRequiredParams(workload); err != nil {
		return err
	}
	if err := v.validateWorkspace(ctx, workload); err != nil {
		return err
	}
	if err := v.validateService(workload); err != nil {
		return err
	}
	if err := v.validateHealthCheck(workload); err != nil {
		return err
	}
	if err := v.validateResourceEnough(ctx, workload); err != nil {
		return err
	}
	if err := v.validateTemplate(ctx, workload); err != nil {
		return err
	}
	if err := validateLabels(workload.Spec.CustomerLabels); err != nil {
		return err
	}
	return nil
}

// validateRequiredParams ensures display name, cluster, workspace, image, entry point, GVK and resources are set.
func (v *WorkloadValidator) validateRequiredParams(workload *v1.Workload) error {
	var errs []error
	if v1.GetDisplayName(workload) == "" {
		errs = append(errs, fmt.Errorf("the displayName is empty"))
	}
	if err := validateDisplayName(v1.GetDisplayName(workload)); err != nil {
		errs = append(errs, err)
	}
	if v1.GetClusterId(workload) == "" {
		errs = append(errs, fmt.Errorf("the cluster is empty"))
	}
	if workload.Spec.Workspace == "" {
		errs = append(errs, fmt.Errorf("the workspace is empty"))
	}
	if workload.Spec.GroupVersionKind.Kind == "" || workload.Spec.GroupVersionKind.Version == "" {
		errs = append(errs, fmt.Errorf("the gvk is empty"))
	}
	if commonworkload.IsCICDScalingRunnerSet(workload) {
		if err := v.validateCICDScalingRunnerSet(workload); err != nil {
			errs = append(errs, err)
		}
	} else {
		if workload.Spec.EntryPoint == "" {
			errs = append(errs, fmt.Errorf("the entryPoint is empty"))
		}
		if workload.Spec.Image == "" {
			errs = append(errs, fmt.Errorf("the image is empty"))
		}
	}
	if err := v.validateResource(&workload.Spec.Resource); err != nil {
		errs = append(errs, err)
	}
	if err := utilerrors.NewAggregate(errs); err != nil {
		return err
	}
	return nil
}

func (v *WorkloadValidator) validateCICDScalingRunnerSet(workload *v1.Workload) error {
	if len(v1.GetDisplayName(workload)) > MaxCICDScaleSetNameLen {
		return fmt.Errorf("the displayName is too long, maximum length is %d characters", MaxCICDScaleSetNameLen)
	}
	keys := []string{ResourcesEnv, EntrypointEnv, ImageEnv, common.GithubConfigUrl}
	for _, key := range keys {
		if val, ok := workload.Spec.Env[key]; !ok || val == "" {
			return fmt.Errorf("the %s of workload environment variables is empty", key)
		}
	}
	workloadResource := &v1.WorkloadResource{}
	err := json.Unmarshal([]byte(workload.Spec.Env[ResourcesEnv]), workloadResource)
	if err != nil {
		return err
	}
	if err = v.validateResource(workloadResource); err != nil {
		return err
	}
	return nil
}

func (v *WorkloadValidator) validateResource(resource *v1.WorkloadResource) error {
	var errs []error
	if resource.Replica <= 0 {
		errs = append(errs, fmt.Errorf("the replica is empty"))
	}
	if resource.CPU == "" {
		errs = append(errs, fmt.Errorf("the cpu is empty"))
	}
	if resource.Memory == "" {
		errs = append(errs, fmt.Errorf("the memory is empty"))
	}
	if resource.EphemeralStorage == "" {
		errs = append(errs, fmt.Errorf("the ephemeralStorage is empty"))
	}
	if err := utilerrors.NewAggregate(errs); err != nil {
		return err
	}
	return nil
}

// validateService validates service ports, protocol and type configuration.
func (v *WorkloadValidator) validateService(workload *v1.Workload) error {
	if workload.Spec.Service == nil {
		return nil
	}
	if err := validatePort("service", workload.Spec.Service.Port); err != nil {
		return err
	}
	if err := validatePort("service/target", workload.Spec.Service.TargetPort); err != nil {
		return err
	}
	if workload.Spec.Service.NodePort > 0 {
		if err := validatePort("service/node", workload.Spec.Service.NodePort); err != nil {
			return err
		}
	}
	if workload.Spec.Service.Protocol != corev1.ProtocolTCP && workload.Spec.Service.Protocol != corev1.ProtocolUDP {
		return fmt.Errorf("the service protocol only supports %s and %s",
			corev1.ProtocolTCP, corev1.ProtocolUDP)
	}
	if workload.Spec.Service.ServiceType != corev1.ServiceTypeClusterIP &&
		workload.Spec.Service.ServiceType != corev1.ServiceTypeNodePort {
		return fmt.Errorf("the service type only supports %s and %s",
			corev1.ServiceTypeClusterIP, corev1.ServiceTypeNodePort)
	}
	if workload.Spec.Service.ServiceType == corev1.ServiceTypeNodePort {
		if workload.Spec.Service.NodePort <= 0 {
			return fmt.Errorf("the nodePort is empty")
		}
	}
	return nil
}

// validateHealthCheck validates liveness and readiness probe configuration.
func (v *WorkloadValidator) validateHealthCheck(workload *v1.Workload) error {
	if workload.Spec.Liveness != nil {
		if workload.Spec.Liveness.Path == "" {
			return fmt.Errorf("the path for liveness is not found")
		}
		if err := validatePort("liveness", workload.Spec.Liveness.Port); err != nil {
			return err
		}
	}
	if workload.Spec.Readiness != nil {
		if workload.Spec.Readiness.Path == "" {
			return fmt.Errorf("the path for readiness is not found")
		}
		if err := validatePort("readiness", workload.Spec.Readiness.Port); err != nil {
			return err
		}
	}
	return nil
}

// validateWorkspace ensures the workspace exists.
func (v *WorkloadValidator) validateWorkspace(ctx context.Context, workload *v1.Workload) error {
	workspace, _ := getWorkspace(ctx, v.Client, workload.Spec.Workspace)
	if workspace == nil {
		if v1.GetOpsJobId(workload) == "" {
			return commonerrors.NewNotFound(v1.WorkspaceKind, workload.Spec.Workspace)
		}
		return nil
	}
	if workload.Spec.Resource.Replica > workspace.Spec.Replica {
		return commonerrors.NewQuotaInsufficient(
			fmt.Sprintf("Insufficient resource: request.replica: %d, total.replica: %d",
				workload.Spec.Resource.Replica, workspace.Spec.Replica))
	}
	return nil
}

// validateResourceEnough checks if the workload resources do not exceed node flavor limits.
func (v *WorkloadValidator) validateResourceEnough(ctx context.Context, workload *v1.Workload) error {
	if workload.Spec.Resource.Replica <= 0 {
		return nil
	}
	nf, err := getNodeFlavor(ctx, v.Client, v1.GetNodeFlavorId(workload))
	if nf == nil {
		return err
	}
	return validateResourceEnough(nf, &workload.Spec.Resource)
}

// validateResourceEnough checks if requested resources exceed per-node limits and configured thresholds.
func validateResourceEnough(nf *v1.NodeFlavor, res *v1.WorkloadResource) error {
	nodeResources := nf.ToResourceList(commonconfig.GetRdmaName())
	availNodeResources := quantity.GetAvailableResource(nodeResources)

	// Validate if the request resource requests exceed the per-node resource limits
	podResources, err := commonworkload.GetPodResources(res)
	if err != nil {
		klog.ErrorS(err, "failed to get pod resource", "input", *res)
		return err
	}
	if ok, key := quantity.IsSubResource(podResources, availNodeResources); !ok {
		return commonerrors.NewQuotaInsufficient(
			fmt.Sprintf("Insufficient resource: %s, request: %v, available: %v",
				key, podResources, availNodeResources))
	}

	// Validate if the share memory requests exceed the memory
	if res.SharedMemory != "" {
		shareMemQuantity, err := resource.ParseQuantity(res.SharedMemory)
		if err != nil {
			return err
		}
		maxMemoryQuantity := availNodeResources[corev1.ResourceMemory]
		if shareMemQuantity.Value() <= 0 || shareMemQuantity.Value() > maxMemoryQuantity.Value() {
			return fmt.Errorf("invalid share memory")
		}
	}

	// Validate if ephemeral storage requests exceed the limit
	if !floatutil.FloatEqual(commonconfig.GetMaxEphemeralStorePercent(), 0) {
		maxEphemeralStoreQuantity, _ := quantity.GetMaxEphemeralStoreQuantity(nodeResources)
		requestQuantity, ok := podResources[corev1.ResourceEphemeralStorage]
		if ok && maxEphemeralStoreQuantity.Cmp(requestQuantity) < 0 {
			return commonerrors.NewQuotaInsufficient(
				fmt.Sprintf("Insufficient resource: %s, request: %v, max: %v",
					corev1.ResourceEphemeralStorage, requestQuantity, *maxEphemeralStoreQuantity))
		}
	}
	return nil
}

// validateTemplate ensures the resource template and task template for the workload kind exist.
func (v *WorkloadValidator) validateTemplate(ctx context.Context, workload *v1.Workload) error {
	if _, err := commonworkload.GetResourceTemplate(ctx, v.Client, workload); err != nil {
		return err
	}
	_, err := commonworkload.GetWorkloadTemplate(ctx, v.Client, workload)
	if err != nil {
		return err
	}
	return nil
}

// validateImmutableFields ensures cluster, workspace, display name and GVK cannot be modified.
func (v *WorkloadValidator) validateImmutableFields(newWorkload, oldWorkload *v1.Workload) error {
	if newWorkload.Spec.Workspace != oldWorkload.Spec.Workspace {
		return field.Forbidden(field.NewPath("spec").Key("workspace"), "immutable")
	}
	if newWorkload.Spec.GroupVersionKind != oldWorkload.Spec.GroupVersionKind {
		return field.Forbidden(field.NewPath("spec").Key("gvk"), "immutable")
	}
	if commonworkload.IsCICDScalingRunnerSet(newWorkload) {
		val1, _ := oldWorkload.Spec.Env[common.UnifiedJobEnable]
		val2, _ := newWorkload.Spec.Env[common.UnifiedJobEnable]
		if val1 != val2 {
			return field.Forbidden(field.NewPath("spec").Key("env").Key(common.UnifiedJobEnable), "immutable")
		}
	}
	return nil
}

// validateScope ensures the workspace supports the workload scope type.
func (v *WorkloadValidator) validateScope(ctx context.Context, workload *v1.Workload) error {
	if commonworkload.IsOpsJob(workload) {
		return nil
	}
	scope := commonworkload.GetScope(workload)
	if scope == "" {
		return commonerrors.NewBadRequest(fmt.Sprintf("unknown workload kind, %s", workload.SpecKind()))
	}
	workspace, err := getWorkspace(ctx, v.Client, workload.Spec.Workspace)
	if err != nil {
		return err
	}
	if workspace == nil || len(workspace.Spec.Scopes) == 0 {
		return nil
	}
	if !workspace.HasScope(scope) {
		return commonerrors.NewForbidden(
			fmt.Sprintf("The workspace only supports %v and does not suuport %s", workspace.Spec.Scopes, scope))
	}
	return nil
}

// validateCronJobs validates cron schedule syntax and unique identifiers.
func (v *WorkloadValidator) validateCronJobs(workload *v1.Workload) error {
	parseCronJob := func(job v1.CronJob) error {
		if job.Schedule == "" {
			return commonerrors.NewBadRequest("CronJob schedule is empty")
		}
		_, scheduleTime, err := timeutil.CvtTime3339ToCronStandard(job.Schedule)
		if err != nil {
			return err
		}
		if job.Action == v1.CronStart {
			if !workload.HasScheduled() {
				now := time.Now().UTC()
				oneYearLaterMinusOneMin := now.AddDate(1, 0, 0).Add(-time.Minute).UTC()
				if !scheduleTime.After(now) || scheduleTime.After(oneYearLaterMinusOneMin) {
					return commonerrors.NewBadRequest(fmt.Sprintf("Invalid schedulerTime(%s) of request, "+
						"it must be within one year in the future, currentTime: %s", job.Schedule, now.String()))
				}
			}
		}
		return nil
	}
	for _, cj := range workload.Spec.CronJobs {
		if err := parseCronJob(cj); err != nil {
			return err
		}
	}
	return nil
}
