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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/floatutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/maps"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const (
	DefaultEphemeralStorage    = "50Gi"
	DefaultInitialDelaySeconds = 600
	DefaultPeriodSeconds       = 3
	DefaultFailureThreshold    = 3
	DefaultMaxUnavailable      = "25%"
	DefaultMaxMaxSurge         = "25%"
	DefaultMaxFailover         = 50
	DefaultWorkloadTTL         = 60
)

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

type WorkloadMutator struct {
	client.Client
	decoder admission.Decoder
}

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

func (m *WorkloadMutator) mutateOnCreation(ctx context.Context, workload *v1.Workload) bool {
	workspace, _ := getWorkspace(ctx, m.Client, workload.Spec.Workspace)
	m.mutateGvk(workload)
	m.mutateMeta(ctx, workload, workspace)

	switch workload.SpecKind() {
	case common.DeploymentKind:
		m.mutateDeployment(workload)
	case common.StatefulSetKind:
		m.mutateStatefulSet(workload)
	case common.AuthoringKind:
		m.mutateAuthoring(workload)
	}

	m.mutateResource(workload, workspace)
	m.mutateHealthCheck(workload)
	m.mutateService(workload)
	m.mutateMaxRetry(workload)
	m.mutateEnv(nil, workload)
	m.mutateTTLSeconds(workload)
	m.mutateCommon(ctx, workload)
	return true
}

func (m *WorkloadMutator) mutateOnUpdate(ctx context.Context, oldWorkload, newWorkload *v1.Workload) bool {
	m.mutateResource(newWorkload, nil)
	m.mutateEnv(oldWorkload, newWorkload)
	m.mutateCommon(ctx, newWorkload)
	return true
}

func (m *WorkloadMutator) mutateCommon(ctx context.Context, workload *v1.Workload) bool {
	m.mutatePriority(workload)
	m.mutateImage(workload)
	m.mutateEntryPoint(workload)
	m.mutateHostNetwork(ctx, workload)
	return true
}

func (m *WorkloadMutator) mutateMeta(ctx context.Context, workload *v1.Workload, workspace *v1.Workspace) {
	if workload.Name != "" {
		workload.Name = stringutil.NormalizeName(workload.Name)
	}
	if workspace != nil {
		if !hasOwnerReferences(workload, workspace.Name) {
			if err := controllerutil.SetControllerReference(workspace, workload, m.Client.Scheme()); err != nil {
				klog.ErrorS(err, "failed to SetControllerReference")
			}
		}
		v1.SetLabel(workload, v1.ClusterIdLabel, workspace.Spec.Cluster)
		v1.SetLabel(workload, v1.NodeFlavorIdLabel, workspace.Spec.NodeFlavor)
		if workspace.Spec.EnablePreempt {
			v1.SetAnnotation(workload, v1.WorkloadEnablePreemptAnnotation, "true")
		}
	}

	v1.SetLabel(workload, v1.WorkspaceIdLabel, workload.Spec.Workspace)
	v1.SetLabel(workload, v1.WorkloadKindLabel, workload.Spec.Kind)
	v1.SetLabel(workload, v1.WorkloadIdLabel, workload.Name)
	if v1.GetUserName(workload) == "" {
		v1.SetAnnotation(workload, v1.UserNameAnnotation, v1.GetUserId(workload))
	}
	if v1.GetUserName(workload) != "" {
		v1.SetLabel(workload, v1.UserNameMd5Label, stringutil.MD5(v1.GetUserName(workload)))
	}
	if v1.GetMainContainer(workload) == "" {
		cm, err := commonworkload.GetWorkloadTemplate(ctx, m.Client, workload)
		if err == nil {
			v1.SetAnnotation(workload, v1.MainContainerAnnotation, v1.GetMainContainer(cm))
		}
	}
	controllerutil.AddFinalizer(workload, v1.WorkloadFinalizer)
}

func (m *WorkloadMutator) mutateGvk(workload *v1.Workload) {
	if workload.Spec.Kind == "" {
		workload.Spec.Kind = common.PytorchJobKind
	}
	if workload.Spec.Version == "" {
		workload.Spec.Version = v1.SchemeGroupVersion.Version
	}
	// the group is not currently in use
	workload.Spec.Group = ""
}

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

func (m *WorkloadMutator) mutateResource(workload *v1.Workload, workspace *v1.Workspace) bool {
	isChanged := false
	if workload.Spec.Resource.GPU != "" && workspace != nil {
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

func (m *WorkloadMutator) mutateHealthCheck(workload *v1.Workload) {
	if workload.Spec.Readiness != nil {
		if workload.Spec.Readiness.InitialDelaySeconds == 0 {
			workload.Spec.Readiness.InitialDelaySeconds = DefaultInitialDelaySeconds
		}
		if workload.Spec.Readiness.PeriodSeconds == 0 {
			workload.Spec.Readiness.PeriodSeconds = DefaultPeriodSeconds
		}
		if workload.Spec.Readiness.FailureThreshold == 0 {
			workload.Spec.Readiness.FailureThreshold = DefaultFailureThreshold
		}
	}
	if workload.Spec.Liveness != nil {
		if workload.Spec.Liveness.InitialDelaySeconds == 0 {
			workload.Spec.Liveness.InitialDelaySeconds = DefaultInitialDelaySeconds
		}
		if workload.Spec.Liveness.PeriodSeconds == 0 {
			workload.Spec.Liveness.PeriodSeconds = DefaultPeriodSeconds
		}
		if workload.Spec.Liveness.FailureThreshold == 0 {
			workload.Spec.Liveness.FailureThreshold = DefaultFailureThreshold
		}
	}
}

func (m *WorkloadMutator) mutateService(workload *v1.Workload) {
	if workload.Spec.Service == nil {
		return
	}
	if workload.Spec.Service.Protocol != "" {
		workload.Spec.Service.Protocol = corev1.Protocol(strings.ToUpper(string(workload.Spec.Service.Protocol)))
	}
}

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

func (m *WorkloadMutator) mutateDeployment(workload *v1.Workload) {
	workload.Spec.IsSupervised = false
	workload.Spec.MaxRetry = 0
	if workload.Spec.Service == nil {
		return
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

func (m *WorkloadMutator) mutateStatefulSet(workload *v1.Workload) {
	workload.Spec.IsSupervised = false
	workload.Spec.MaxRetry = 0
}

func (m *WorkloadMutator) mutateAuthoring(workload *v1.Workload) {
	workload.Spec.IsSupervised = false
	workload.Spec.MaxRetry = 0
	workload.Spec.Resource.Replica = 1
	workload.Spec.Timeout = nil
	workload.Spec.EntryPoint = stringutil.Base64Encode("sleep infinity")
}

func (m *WorkloadMutator) mutateImage(workload *v1.Workload) {
	workload.Spec.Image = strings.TrimSpace(workload.Spec.Image)
	workload.Spec.EntryPoint = strings.TrimSpace(workload.Spec.EntryPoint)
}

func (m *WorkloadMutator) mutateMaxRetry(workload *v1.Workload) {
	if workload.Spec.MaxRetry > DefaultMaxFailover {
		workload.Spec.MaxRetry = DefaultMaxFailover
	}
	if workload.Spec.MaxRetry < 0 {
		workload.Spec.MaxRetry = 0
	}
}

func (m *WorkloadMutator) mutateEnv(oldWorkload, newWorkload *v1.Workload) {
	newWorkload.Spec.Env = maps.RemoveValue(newWorkload.Spec.Env, "")
	// A null or empty value means the field should be removed.
	if oldWorkload != nil {
		for key := range oldWorkload.Spec.Env {
			if _, ok := newWorkload.Spec.Env[key]; !ok {
				newWorkload.Spec.Env[key] = ""
			}
		}
	}
}

func (m *WorkloadMutator) mutateTTLSeconds(workload *v1.Workload) {
	if commonworkload.IsAuthoring(workload) {
		return
	}
	if workload.Spec.TTLSecondsAfterFinished == nil {
		workload.Spec.TTLSecondsAfterFinished = ptr.To(commonconfig.GetWorkloadTTLSecond())
	}
}

func (m *WorkloadMutator) mutateEntryPoint(workload *v1.Workload) {
	if commonworkload.IsAuthoring(workload) || commonworkload.IsOpsJob(workload) {
		return
	}
	if !stringutil.IsBase64(workload.Spec.EntryPoint) {
		workload.Spec.EntryPoint = stringutil.Base64Encode(workload.Spec.EntryPoint)
	}
}

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

type WorkloadValidator struct {
	client.Client
	decoder admission.Decoder
}

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

func (v *WorkloadValidator) validateOnCreation(ctx context.Context, workload *v1.Workload) error {
	if err := v.validateCommon(ctx, workload); err != nil {
		return err
	}
	if err := validateDNSName(v1.GetDisplayName(workload)); err != nil {
		return err
	}
	if err := v.validateResourceValid(workload); err != nil {
		return err
	}
	if err := v.validateScope(ctx, workload); err != nil {
		return err
	}
	return nil
}

func (v *WorkloadValidator) validateOnUpdate(ctx context.Context, newWorkload, oldWorkload *v1.Workload) error {
	if err := v.validateImmutableFields(newWorkload, oldWorkload); err != nil {
		return err
	}
	if err := v.validateCommon(ctx, newWorkload); err != nil {
		return err
	}
	if err := v.validateSpecChanged(newWorkload, oldWorkload); err != nil {
		return err
	}
	return nil
}

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
	if err := v.validateDisplayName(workload); err != nil {
		return err
	}
	return nil
}

func (v *WorkloadValidator) validateRequiredParams(workload *v1.Workload) error {
	var errs []error
	if v1.GetDisplayName(workload) == "" {
		errs = append(errs, fmt.Errorf("the displayName is empty"))
	}
	if v1.GetClusterId(workload) == "" {
		errs = append(errs, fmt.Errorf("the cluster is empty"))
	}
	if workload.Spec.Workspace == "" {
		errs = append(errs, fmt.Errorf("the workspace is empty"))
	}
	if workload.Spec.EntryPoint == "" {
		errs = append(errs, fmt.Errorf("the entryPoint is empty"))
	}
	if workload.Spec.Image == "" {
		errs = append(errs, fmt.Errorf("the image is empty"))
	}
	if workload.Spec.GroupVersionKind.Empty() {
		errs = append(errs, fmt.Errorf("the gvk is empty"))
	}
	if workload.Spec.Resource.Replica <= 0 {
		errs = append(errs, fmt.Errorf("the replica is empty"))
	}
	if workload.Spec.Resource.CPU == "" {
		errs = append(errs, fmt.Errorf("the cpu is empty"))
	}
	if workload.Spec.Resource.Memory == "" {
		errs = append(errs, fmt.Errorf("the memory is empty"))
	}
	if workload.Spec.Resource.EphemeralStorage == "" {
		errs = append(errs, fmt.Errorf("the ephemeralStorage is empty"))
	}
	if err := utilerrors.NewAggregate(errs); err != nil {
		return err
	}
	return nil
}

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
	return nil
}

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

func (v *WorkloadValidator) validateResourceValid(workload *v1.Workload) error {
	var errs []error
	if workload.Spec.Resource.Replica <= 0 {
		errs = append(errs, fmt.Errorf("the replica is zero"))
	}
	if workload.Spec.Resource.CPU == "" {
		errs = append(errs, fmt.Errorf("the cpu is empty"))
	}
	if workload.Spec.Resource.Memory == "" {
		errs = append(errs, fmt.Errorf("the memory is empty"))
	}
	if workload.Spec.Resource.EphemeralStorage == "" {
		errs = append(errs, fmt.Errorf("the ephemeralStorage is empty"))
	}
	if err := utilerrors.NewAggregate(errs); err != nil {
		return err
	}
	return nil
}

func (v *WorkloadValidator) validateWorkspace(ctx context.Context, workload *v1.Workload) error {
	workspace, _ := getWorkspace(ctx, v.Client, workload.Spec.Workspace)
	if workspace == nil {
		if v1.GetOpsJobId(workload) == "" {
			return commonerrors.NewNotFound(v1.WorkspaceKind, workload.Spec.Workspace)
		}
		return nil
	}
	if workspace.IsAbnormal() && !workload.Spec.IsTolerateAll {
		return commonerrors.NewQuotaInsufficient(fmt.Sprintf("workspace %s is abnormal", workspace.Name))
	}
	if workload.Spec.Resource.Replica > workspace.Spec.Replica {
		return commonerrors.NewQuotaInsufficient(
			fmt.Sprintf("Insufficient resource: request.replica: %d, total.replica: %d",
				workload.Spec.Resource.Replica, workspace.Spec.Replica))
	}
	return nil
}

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

func (v *WorkloadValidator) validateTemplate(ctx context.Context, workload *v1.Workload) error {
	if _, err := getResourceTemplate(ctx, v.Client, workload.Spec.GroupVersionKind); err != nil {
		return err
	}
	_, err := commonworkload.GetWorkloadTemplate(ctx, v.Client, workload)
	if err != nil {
		return err
	}
	return nil
}

func (v *WorkloadValidator) validateDisplayName(workload *v1.Workload) error {
	l := len(v1.GetDisplayName(workload))
	if l > commonutils.MaxDisplayNameLen {
		return fmt.Errorf("the maximum length of the workload name [%s] is %d",
			v1.GetDisplayName(workload), commonutils.MaxDisplayNameLen)
	} else if l == 0 {
		return fmt.Errorf("the display name is empty")
	}
	return nil
}

func (v *WorkloadValidator) validateImmutableFields(newWorkload, oldWorkload *v1.Workload) error {
	if newWorkload.Spec.Workspace != oldWorkload.Spec.Workspace {
		return field.Forbidden(field.NewPath("spec").Key("workspace"), "immutable")
	}
	if newWorkload.Spec.GroupVersionKind != oldWorkload.Spec.GroupVersionKind {
		return field.Forbidden(field.NewPath("spec").Key("gvk"), "immutable")
	}
	if oldWorkload.Spec.Service != nil && (newWorkload.Spec.Service == nil ||
		!reflect.DeepEqual(*oldWorkload.Spec.Service, *newWorkload.Spec.Service)) {
		return field.Forbidden(
			field.NewPath("spec", "service"), "immutable")
	}
	return nil
}

// Changes to the PyTorchJob are only allowed when the job is queued.
func (v *WorkloadValidator) validateSpecChanged(newWorkload, oldWorkload *v1.Workload) error {
	if commonworkload.IsApplication(newWorkload) || !v1.IsWorkloadScheduled(newWorkload) {
		return nil
	}
	if oldWorkload.Spec.EntryPoint != newWorkload.Spec.EntryPoint {
		return commonerrors.NewForbidden("EntryPoint cannot be changed once the workload has been scheduled")
	}
	if oldWorkload.Spec.Image != newWorkload.Spec.Image {
		return commonerrors.NewForbidden("Image cannot be changed once the workload has been scheduled")
	}
	if !commonworkload.IsResourceEqual(oldWorkload, newWorkload) {
		return commonerrors.NewForbidden("Resources cannot be changed once the workload has been scheduled")
	}
	if !maps.EqualIgnoreOrder(oldWorkload.Spec.Env, newWorkload.Spec.Env) {
		return commonerrors.NewForbidden("Env cannot be changed once the workload has been scheduled")
	}
	return nil
}

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
	hasFound := false
	for _, s := range workspace.Spec.Scopes {
		if s == scope {
			hasFound = true
			break
		}
	}
	if !hasFound {
		return commonerrors.NewForbidden(
			fmt.Sprintf("The workspace only supports %v and does not suuport %s", workspace.Spec.Scopes, scope))
	}
	return nil
}
