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

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/floatutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/maps"
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
	obj := &v1.Workload{}
	if err := m.decoder.Decode(req, obj); err != nil {
		return handleError(v1.WorkloadKind, err)
	}
	if !obj.GetDeletionTimestamp().IsZero() {
		return admission.Allowed("")
	}

	isChanged := false
	switch req.Operation {
	case admissionv1.Create:
		isChanged = m.mutateCreate(ctx, obj)
	case admissionv1.Update:
		oldObj := &v1.Workload{}
		if m.decoder.DecodeRaw(req.OldObject, oldObj) == nil {
			isChanged = m.mutateUpdate(oldObj, obj)
		}
	}
	if !isChanged {
		return admission.Allowed("")
	}

	marshaledResult, err := json.Marshal(obj)
	if err != nil {
		return handleError(v1.WorkloadKind, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledResult)
}

func (m *WorkloadMutator) mutateCreate(ctx context.Context, workload *v1.Workload) bool {
	workspace, err := getWorkspace(ctx, m.Client, workload.Spec.Workspace)
	if err != nil {
		return false
	}

	m.mutateMeta(ctx, workload, workspace)
	m.mutateGvk(ctx, workload)

	switch workload.Spec.Kind {
	case common.DeploymentKind:
		m.mutateDeployment(workload)
	case common.StatefulSetKind:
		m.mutateStatefulSet(workload)
	case common.PytorchJobKind:
	}

	m.mutateResource(workload, workspace)
	m.mutateHealthCheck(workload)
	m.mutateService(workload)
	m.mutateMaxRetry(workload)
	m.mutateCreateEnv(workload)
	m.mutateCommon(workload)
	return true
}

func (m *WorkloadMutator) mutateUpdate(oldObj, newObj *v1.Workload) bool {
	m.mutateResource(newObj, nil)
	m.mutateUpdateEnv(oldObj, newObj)
	m.mutateCommon(newObj)
	return true
}

func (m *WorkloadMutator) mutateCommon(obj *v1.Workload) bool {
	m.mutatePriority(obj)
	m.mutateImage(obj)
	return true
}

func (m *WorkloadMutator) mutateMeta(ctx context.Context, workload *v1.Workload, workspace *v1.Workspace) {
	if workload.Name != "" {
		workload.Name = stringutil.NormalizeName(workload.Name)
	}
	if !hasOwnerReferences(workload, workspace.Name) {
		if err := controllerutil.SetControllerReference(workspace, workload, m.Client.Scheme()); err != nil {
			klog.ErrorS(err, "fail to SetControllerReference")
		}
	}
	if v1.GetClusterId(workload) == "" {
		metav1.SetMetaDataLabel(&workload.ObjectMeta, v1.ClusterIdLabel, workspace.Spec.Cluster)
	}
	if v1.GetWorkspaceId(workload) == "" {
		metav1.SetMetaDataLabel(&workload.ObjectMeta, v1.WorkspaceIdLabel, workload.Spec.Workspace)
	}
	if workload.Annotations[v1.WorkloadMainContainer] == "" && len(workload.Spec.Resources) > 0 {
		cm, err := commonworkload.GetWorkloadTemplateConfig(ctx, m.Client,
			workload.Spec.Kind, workload.Spec.Resources[0].GPUName)
		if err == nil {
			metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.WorkloadMainContainer, cm.Labels[common.MainContainer])
		}
	}
	if workload.Annotations[v1.EnableHostNetworkAnnotation] == "" {
		metav1.SetMetaDataAnnotation(&workload.ObjectMeta,
			v1.EnableHostNetworkAnnotation, strconv.FormatBool(m.canUseHostNetwork(ctx, workload, workspace)))
	}
	controllerutil.AddFinalizer(workload, v1.WorkloadFinalizer)
}

func (m *WorkloadMutator) mutateGvk(ctx context.Context, workload *v1.Workload) {
	if workload.Spec.Kind == "" {
		workload.Spec.Kind = common.PytorchJobKind
	}
	if workload.Spec.Group == "" || workload.Spec.Version == "" {
		rtl := &v1.ResourceTemplateList{}
		err := m.List(ctx, rtl)
		if err != nil {
			return
		}
		for _, rt := range rtl.Items {
			if rt.Spec.GroupVersionKind.Kind != workload.Spec.Kind {
				continue
			}
			if workload.Spec.Group == "" {
				workload.Spec.Group = rt.Spec.GroupVersionKind.Group
			}
			if workload.Spec.Version == "" {
				workload.Spec.Version = rt.Spec.GroupVersionKind.Version
			}
		}
	}
}

func (m *WorkloadMutator) mutatePriority(workload *v1.Workload) bool {
	isChanged := false
	if workload.Spec.Priority > v1.MaxPriority {
		workload.Spec.Priority = v1.MaxPriority
		isChanged = true
	} else if workload.Spec.Priority < v1.MinPriority {
		workload.Spec.Priority = v1.MinPriority
		isChanged = true
	}
	return isChanged
}

func (m *WorkloadMutator) mutateResource(workload *v1.Workload, workspace *v1.Workspace) bool {
	isChanged := false
	for i := range workload.Spec.Resources {
		if workload.Spec.Resources[i].GPU != "" && workspace != nil {
			workload.Spec.Resources[i].GPUName = v1.GetGpuResourceName(workspace)
		}
		if workload.Spec.Resources[i].ShareMemory == "" && workload.Spec.Resources[i].Memory != "" {
			memQuantity, err := resource.ParseQuantity(workload.Spec.Resources[i].Memory)
			if err == nil && memQuantity.Value() > 0 {
				shareMemQuantity := resource.NewQuantity(memQuantity.Value()/2, memQuantity.Format)
				if shareMemQuantity != nil {
					workload.Spec.Resources[i].ShareMemory = shareMemQuantity.String()
					isChanged = true
				}
			}
		}
		if workload.Spec.Resources[i].EphemeralStorage == "" {
			workload.Spec.Resources[i].EphemeralStorage = DefaultEphemeralStorage
			isChanged = true
		}
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

func (m *WorkloadMutator) canUseHostNetwork(ctx context.Context, adminWorkload *v1.Workload, workspace *v1.Workspace) bool {
	if commonworkload.GetTotalCount(adminWorkload) == 1 {
		return false
	}
	nf := &v1.NodeFlavor{}
	err := m.Get(ctx, client.ObjectKey{Name: workspace.Spec.NodeFlavor}, nf)
	if err != nil {
		return false
	}
	gpuCount := 0
	if nf.HasGpu() {
		gpuCount = int(nf.Spec.Gpu.Quantity.Value())
	}
	for _, res := range adminWorkload.Spec.Resources {
		if res.GPU == "" {
			return false
		}
		n, err := strconv.Atoi(res.GPU)
		if err != nil || n != gpuCount {
			return false
		}
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
	// role is only used for PyTorch Job
	if len(workload.Spec.Resources) > 0 {
		workload.Spec.Resources[0].Role = "main"
	}
}

func (m *WorkloadMutator) mutateStatefulSet(workload *v1.Workload) {
	workload.Spec.IsSupervised = false
	workload.Spec.MaxRetry = 0
	// role is only used for PyTorch Job
	if len(workload.Spec.Resources) > 0 {
		workload.Spec.Resources[0].Role = "main"
	}
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

func (m *WorkloadMutator) mutateCreateEnv(workload *v1.Workload) {
	if len(workload.Spec.Env) == 0 {
		return
	}
	workload.Spec.Env = maps.RemoveValue(workload.Spec.Env, "")
}

func (m *WorkloadMutator) mutateUpdateEnv(oldObj, newObj *v1.Workload) {
	newObj.Spec.Env = maps.RemoveValue(newObj.Spec.Env, "")
	// A null or empty value means the field should be removed.
	for key := range oldObj.Spec.Env {
		if _, ok := newObj.Spec.Env[key]; !ok {
			newObj.Spec.Env[key] = ""
		}
	}
}

type WorkloadValidator struct {
	client.Client
	decoder admission.Decoder
}

func (v *WorkloadValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &v1.Workload{}
	var err error
	switch req.Operation {
	case admissionv1.Create:
		if err = v.decoder.Decode(req, obj); err != nil {
			break
		}
		if !obj.GetDeletionTimestamp().IsZero() {
			break
		}
		err = v.validateCreate(ctx, obj)
	case admissionv1.Update:
		if err = v.decoder.Decode(req, obj); err != nil {
			break
		}
		if !obj.GetDeletionTimestamp().IsZero() {
			break
		}
		oldObj := &v1.Workload{}
		if err = v.decoder.DecodeRaw(req.OldObject, oldObj); err == nil {
			err = v.validateUpdate(ctx, obj, oldObj)
		}
	default:
	}
	if err != nil {
		return handleError(v1.WorkloadKind, err)
	}
	return admission.Allowed("")
}

func (v *WorkloadValidator) validateCreate(ctx context.Context, w *v1.Workload) error {
	if err := v.validateCommon(ctx, w); err != nil {
		return err
	}
	if err := validateDNSName(v1.GetDisplayName(w)); err != nil {
		return err
	}
	if err := v.validateResourceValid(w); err != nil {
		return err
	}
	if err := v.validateScope(ctx, w); err != nil {
		return err
	}
	return nil
}

func (v *WorkloadValidator) validateUpdate(ctx context.Context, newObj, oldObj *v1.Workload) error {
	if err := v.validateImmutableFields(newObj, oldObj); err != nil {
		return err
	}
	if err := v.validateCommon(ctx, newObj); err != nil {
		return err
	}
	if err := v.validateSpecChanged(newObj, oldObj); err != nil {
		return err
	}
	if err := v.validateCronScaleChanged(newObj, oldObj); err != nil {
		return err
	}
	return nil
}

func (v *WorkloadValidator) validateCommon(ctx context.Context, w *v1.Workload) error {
	if err := v.validateRequiredParams(w); err != nil {
		return err
	}
	if err := v.validatePytorchJob(w); err != nil {
		return err
	}
	if err := v.validateApplication(w); err != nil {
		return err
	}
	if err := v.validateResourceEnough(ctx, w); err != nil {
		return err
	}
	if err := v.validateResourceTemplate(ctx, w); err != nil {
		return err
	}
	if err := v.validateDisplayName(w); err != nil {
		return err
	}
	if err := v.validateCronScales(w); err != nil {
		return err
	}
	return nil
}

func (v *WorkloadValidator) validateRequiredParams(w *v1.Workload) error {
	var errs []error
	if v1.GetDisplayName(w) == "" {
		errs = append(errs, fmt.Errorf("the displayName is empty"))
	}
	if v1.GetClusterId(w) == "" {
		errs = append(errs, fmt.Errorf("the cluster is empty"))
	}
	if w.Spec.Workspace == "" {
		errs = append(errs, fmt.Errorf("the workspace is empty"))
	}
	if w.Spec.Image == "" {
		errs = append(errs, fmt.Errorf("the image is empty"))
	}
	if w.Spec.EntryPoint == "" {
		errs = append(errs, fmt.Errorf("the entryPoint is empty"))
	}
	if w.Spec.Group == "" || w.Spec.Version == "" || w.Spec.Kind == "" {
		errs = append(errs, fmt.Errorf("the gvk is empty"))
	}
	if len(w.Spec.Resources) == 0 {
		errs = append(errs, fmt.Errorf("the resources are empty"))
	}
	if len(w.Spec.Resources) > 1 && w.Spec.Kind != common.PytorchJobKind {
		return commonerrors.NewBadRequest("only PytorchJob supports multi resource")
	}
	if err := utilerrors.NewAggregate(errs); err != nil {
		return err
	}
	return nil
}

func (v *WorkloadValidator) validatePytorchJob(w *v1.Workload) error {
	if w.Spec.Kind != common.PytorchJobKind {
		return nil
	}
	if !isContainTemplateName(w.Spec.Resources, common.PytorchMaster) {
		return fmt.Errorf("%s resource must be specified", common.PytorchMaster)
	}
	if len(w.Spec.Resources) > 1 {
		if !isContainTemplateName(w.Spec.Resources, common.PytorchWorker) {
			return fmt.Errorf("%s resource not found", common.PytorchWorker)
		}
	}
	return nil
}

func (v *WorkloadValidator) validateApplication(w *v1.Workload) error {
	if !commonworkload.IsApplication(w) {
		return nil
	}
	if len(w.Spec.Resources) > 1 {
		return commonerrors.NewBadRequest(fmt.Sprintf("the %s workload only supports one resource", w.Spec.Kind))
	}
	if w.Spec.Service != nil {
		if err := validatePort("service", w.Spec.Service.Port); err != nil {
			return err
		}
		if err := validatePort("service/target", w.Spec.Service.TargetPort); err != nil {
			return err
		}
		if w.Spec.Service.NodePort > 0 {
			if err := validatePort("service/node", w.Spec.Service.NodePort); err != nil {
				return err
			}
		}
		if w.Spec.Service.Protocol != corev1.ProtocolTCP && w.Spec.Service.Protocol != corev1.ProtocolUDP {
			return fmt.Errorf("the service protocol only supports %s and %s",
				corev1.ProtocolTCP, corev1.ProtocolUDP)
		}
		if w.Spec.Service.ServiceType != corev1.ServiceTypeClusterIP &&
			w.Spec.Service.ServiceType != corev1.ServiceTypeNodePort {
			return fmt.Errorf("the service type only supports %s and %s",
				corev1.ServiceTypeClusterIP, corev1.ServiceTypeNodePort)
		}
	}
	if w.Spec.Liveness != nil {
		if w.Spec.Liveness.Path == "" {
			return fmt.Errorf("the path for liveness is not found")
		}
		if err := validatePort("liveness", w.Spec.Liveness.Port); err != nil {
			return err
		}
	}
	if w.Spec.Readiness != nil {
		if w.Spec.Readiness.Path == "" {
			return fmt.Errorf("the path for readiness is not found")
		}
		if err := validatePort("readiness", w.Spec.Readiness.Port); err != nil {
			return err
		}
	}
	return nil
}

func isContainTemplateName(resources []v1.WorkloadResource, name string) bool {
	for _, res := range resources {
		if res.Role == name {
			return true
		}
	}
	return false
}

func (v *WorkloadValidator) validateResourceValid(w *v1.Workload) error {
	var errs []error
	if len(w.Spec.Resources) == 0 {
		errs = append(errs, fmt.Errorf("the resources are empty"))
	}
	for _, res := range w.Spec.Resources {
		if res.Replica <= 0 {
			errs = append(errs, fmt.Errorf("the replica is invalid"))
		}
		if res.CPU == "" {
			errs = append(errs, fmt.Errorf("the cpu is empty"))
		}
		if res.Memory == "" {
			errs = append(errs, fmt.Errorf("the memory is empty"))
		}
	}
	if err := utilerrors.NewAggregate(errs); err != nil {
		return err
	}
	return nil
}

func (v *WorkloadValidator) validateResourceEnough(ctx context.Context, w *v1.Workload) error {
	if commonworkload.GetTotalCount(w) == 0 {
		return nil
	}
	// workspace must exist
	workspace, err := getWorkspace(ctx, v.Client, w.Spec.Workspace)
	if err != nil {
		return commonerrors.NewNotFound("workspaces", w.Spec.Workspace)
	}
	if v1.IsWorkloadForcedFailover(w) {
		return nil
	}
	if workspace.IsAbnormal() {
		return commonerrors.NewInternalError(fmt.Sprintf("workspace %s is abnormal", workspace.Name))
	}
	if commonworkload.GetTotalCount(w) > workspace.Spec.Replica {
		return commonerrors.NewQuotaInsufficient(
			fmt.Sprintf("Insufficient resource: request.replica: %d, total.replica: %d",
				commonworkload.GetTotalCount(w), workspace.Spec.Replica))
	}

	if workspace.Spec.NodeFlavor == "" {
		return nil
	}
	nf := &v1.NodeFlavor{}
	if err = v.Get(ctx, client.ObjectKey{Name: workspace.Spec.NodeFlavor}, nf); err != nil {
		return err
	}
	nodeResource := nf.ToResourceList()
	availNodeResource := quantity.GetAvailResource(nodeResource)
	var maxEphemeralStoreQuantity *resource.Quantity
	if !floatutil.FloatEqual(commonconfig.GetMaxEphemeralStorePercent(), 0) {
		maxEphemeralStoreQuantity, _ = quantity.GetMaxEphemeralStoreQuantity(nodeResource)
	}
	maxMemoryQuantity := availNodeResource[corev1.ResourceMemory]
	for _, res := range w.Spec.Resources {

		// 校验share memory输入是否正确
		shareMemQuantity, err := resource.ParseQuantity(res.ShareMemory)
		if err != nil {
			return err
		}
		if shareMemQuantity.Value() <= 0 || shareMemQuantity.Value() > maxMemoryQuantity.Value() {
			return fmt.Errorf("invalid share memory")
		}

		// 校验workload申请的资源是否超过单节点限制
		rl, err := quantity.CvtToResourceList(res.CPU, res.Memory, res.GPU, res.GPUType, res.EphemeralStorage, 1)
		if err != nil {
			return err
		}
		if ok, key := quantity.IsSubResource(rl, availNodeResource); !ok {
			return commonerrors.NewQuotaInsufficient(
				fmt.Sprintf("Insufficient resource: %s, request: %v, available: %v",
					key, rl, availNodeResource))
		}

		// 检查存储是否超过限制，存储会做额外限制
		if !commonworkload.IsVirtualMachine(w) && maxEphemeralStoreQuantity != nil {
			requestStoreQuantity, ok := rl[corev1.ResourceEphemeralStorage]
			if ok && maxEphemeralStoreQuantity.Cmp(requestStoreQuantity) < 0 {
				return commonerrors.NewQuotaInsufficient(
					fmt.Sprintf("Insufficient resource: %s, request: %v, max: %v",
						corev1.ResourceEphemeralStorage, requestStoreQuantity, *maxEphemeralStoreQuantity))
			}
		}
	}
	return nil
}

func (v *WorkloadValidator) validateResourceTemplate(ctx context.Context, w *v1.Workload) error {
	rtl := &v1.ResourceTemplateList{}
	err := v.List(ctx, rtl)
	if err != nil {
		return err
	}
	hasFound := false
	for _, rt := range rtl.Items {
		if rt.Spec.GroupVersionKind.Kind == w.Spec.Kind {
			hasFound = true
			break
		}
	}
	if !hasFound {
		return commonerrors.NewBadRequest(
			fmt.Sprintf("the workload kind(%s) is not supported", w.Spec.Kind))
	}
	return nil
}

func (v *WorkloadValidator) validateDisplayName(w *v1.Workload) error {
	// 名字过长会导致dns解析失败
	l := len(v1.GetDisplayName(w))
	if l > commonutils.MaxDisplayNameLen {
		return fmt.Errorf("the maximum length of the workload name [%s] is %d",
			v1.GetDisplayName(w), commonutils.MaxDisplayNameLen)
	} else if l == 0 {
		return fmt.Errorf("the display name is empty")
	}
	return nil
}

func (v *WorkloadValidator) validateImmutableFields(newObj, oldObj *v1.Workload) error {
	if newObj.Spec.Workspace != oldObj.Spec.Workspace {
		return field.Forbidden(field.NewPath("spec").Key("workspace"), "immutable")
	}
	if newObj.Spec.GroupVersionKind != oldObj.Spec.GroupVersionKind {
		return field.Forbidden(field.NewPath("spec").Key("gvk"), "immutable")
	}
	if len(newObj.Spec.Resources) != 0 && len(oldObj.Spec.Resources) != 0 {
		if len(newObj.Spec.Resources) != len(oldObj.Spec.Resources) {
			return field.Forbidden(
				field.NewPath("spec", "resources").Key("length"), "immutable")
		}
		resourceNameSet := sets.NewSet()
		for _, res := range oldObj.Spec.Resources {
			resourceNameSet.Insert(res.Name)
		}
		for _, res := range newObj.Spec.Resources {
			if !resourceNameSet.Has(res.Name) {
				return field.Forbidden(
					field.NewPath("spec", "resources").Key("name"), "immutable")
			}
		}
	}
	if oldObj.Spec.Service != nil && (newObj.Spec.Service == nil ||
		!reflect.DeepEqual(*oldObj.Spec.Service, *newObj.Spec.Service)) {
		return field.Forbidden(
			field.NewPath("spec", "service"), "immutable")
	}
	return nil
}

// pytorchJob资源类/virtualMachine修改，只能在任务排队时操作
func (v *WorkloadValidator) validateSpecChanged(newObj, oldObj *v1.Workload) error {
	if commonworkload.IsApplication(newObj) || !v1.IsWorkloadScheduled(newObj) {
		return nil
	}
	if oldObj.Spec.EntryPoint != newObj.Spec.EntryPoint {
		return commonerrors.NewForbidden("EntryPoint cannot be changed when the workload has been scheduled")
	}
	if oldObj.Spec.Image != newObj.Spec.Image {
		return commonerrors.NewForbidden("Image cannot be changed when the workload has been scheduled")
	}
	if !commonworkload.IsResourceListEqual(oldObj.Spec.Resources, newObj.Spec.Resources) {
		return commonerrors.NewForbidden("Resources cannot be changed when the workload has been scheduled")
	}
	if !maps.EqualIgnoreOrder(oldObj.Spec.Env, newObj.Spec.Env) {
		return commonerrors.NewForbidden("Env cannot be changed when the workload has been scheduled")
	}
	return nil
}

func (v *WorkloadValidator) validateScope(ctx context.Context, w *v1.Workload) error {
	if v1.GetUserId(w) == common.SystemUserId {
		return nil
	}
	scope := commonworkload.GetScope(w.Spec.Kind, v1.IsAuthoring(w))
	if scope == "" {
		return commonerrors.NewBadRequest("the scope for workload is invalid")
	}
	workspace, err := getWorkspace(ctx, v.Client, w.Spec.Workspace)
	if err != nil {
		return err
	}
	hasFound := false
	for _, s := range workspace.Spec.Scopes {
		if s == scope {
			hasFound = true
			break
		}
	}
	if !hasFound {
		return commonerrors.NewForbidden(fmt.Sprintf("The workspace only supports %v and does not suuport %s",
			workspace.Spec.Scopes, scope))
	}
	return nil
}

func (v *WorkloadValidator) validateCronScales(w *v1.Workload) error {
	if len(w.Spec.CronScales) == 0 {
		return nil
	}
	if (len(w.Spec.CronScales) % 2) != 0 {
		return commonerrors.NewBadRequest("the cronScales must appear in pairs")
	}
	for _, s := range w.Spec.CronScales {
		_, _, err := timeutil.ParseCronStandard(s.Schedule)
		if err != nil {
			return err
		}
		if s.TargetSize < 0 || s.TargetRatio < 0 {
			return commonerrors.NewBadRequest("the targetSize or targetRatio is less than 0")
		}
		if s.TargetSize == 0 && floatutil.FloatEqual(s.TargetRatio, 0) {
			return commonerrors.NewBadRequest("one of targetSize and targetRatio must be configured")
		}
	}
	return nil
}

func (v *WorkloadValidator) validateCronScaleChanged(newObj, oldObj *v1.Workload) error {
	if len(newObj.Spec.CronScales) != 0 && len(oldObj.Spec.CronScales) == 0 {
		return nil
	}
	if len(oldObj.Spec.CronScales) != 0 && len(newObj.Spec.CronScales) == 0 {
		return nil
	}
	if reflect.DeepEqual(oldObj.Spec.CronScales, newObj.Spec.CronScales) {
		return nil
	}
	return commonerrors.NewForbidden("The cron-scale cannot be changed")
}
