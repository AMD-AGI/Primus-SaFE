/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

func AddFaultWebhook(mgr ctrlruntime.Manager, server *webhook.Server, decoder admission.Decoder) {
	(*server).Register(generateMutatePath(v1.FaultKind), &webhook.Admission{Handler: &FaultMutator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
	(*server).Register(generateValidatePath(v1.FaultKind), &webhook.Admission{Handler: &FaultValidator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
}

type FaultMutator struct {
	client.Client
	decoder admission.Decoder
}

func (m *FaultMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create {
		return admission.Allowed("")
	}

	obj := &v1.Fault{}
	if err := m.decoder.Decode(req, obj); err != nil {
		return handleError(v1.FaultKind, err)
	}
	if !obj.GetDeletionTimestamp().IsZero() {
		return admission.Allowed("")
	}
	m.mutate(ctx, obj)
	marshaledResult, err := json.Marshal(obj)
	if err != nil {
		return handleError(v1.FaultKind, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledResult)
}

func (m *FaultMutator) mutate(ctx context.Context, f *v1.Fault) {
	if f.Name != "" {
		f.Name = stringutil.NormalizeName(f.Name)
	}
	metav1.SetMetaDataLabel(&f.ObjectMeta, v1.ClusterIdLabel, f.Spec.Node.ClusterName)
	controllerutil.AddFinalizer(f, v1.FaultFinalizer)

	if f.Spec.Node != nil {
		adminNodeName := f.Spec.Node.AdminName
		node := &v1.Node{}
		err := m.Get(ctx, client.ObjectKey{Name: adminNodeName}, node)
		if err != nil {
			return
		}
		metav1.SetMetaDataLabel(&node.ObjectMeta, v1.NodeIdLabel, adminNodeName)
		if !hasOwnerReferences(f, adminNodeName) {
			if err = controllerutil.SetControllerReference(node, f, m.Client.Scheme()); err != nil {
				klog.ErrorS(err, "failed to SetControllerReference")
			}
		}
	}
}

type FaultValidator struct {
	client.Client
	decoder admission.Decoder
}

func (v *FaultValidator) Handle(_ context.Context, req admission.Request) admission.Response {
	obj := &v1.Fault{}
	var err error
	switch req.Operation {
	case admissionv1.Create:
		if err = v.decoder.Decode(req, obj); err != nil {
			break
		}
		err = v.validateCreate(obj)
	case admissionv1.Update:
		if err = v.decoder.Decode(req, obj); err != nil {
			break
		}
		if !obj.GetDeletionTimestamp().IsZero() {
			break
		}
		oldObj := &v1.Fault{}
		if err = v.decoder.DecodeRaw(req.OldObject, oldObj); err == nil {
			err = v.validateUpdate(obj, oldObj)
		}
	default:
	}
	if err != nil {
		return handleError("fault", err)
	}
	return admission.Allowed("")
}

func (v *FaultValidator) validateCreate(obj *v1.Fault) error {
	if err := v.validateFaultSpec(obj); err != nil {
		return err
	}
	if err := validateDisplayName(v1.GetDisplayName(obj)); err != nil {
		return err
	}
	return nil
}

func (v *FaultValidator) validateUpdate(newObj, oldObj *v1.Fault) error {
	if err := v.validateImmutableFields(newObj, oldObj); err != nil {
		return err
	}
	if err := v.validateFaultSpec(newObj); err != nil {
		return err
	}
	return nil
}

func (v *FaultValidator) validateFaultSpec(obj *v1.Fault) error {
	if obj.Spec.Id == "" {
		return fmt.Errorf("the id of spec is empty")
	}
	if obj.Spec.Node != nil {
		if obj.Spec.Node.ClusterName == "" || v1.GetClusterId(obj) == "" {
			return fmt.Errorf("the cluster of spec is empty")
		}
		if obj.Spec.Node.AdminName == "" {
			return fmt.Errorf("the admin node of spec is empty")
		}
		if obj.Spec.Node.K8sName == "" {
			return fmt.Errorf("the k8s node of spec is empty")
		}
	}
	return nil
}

func (v *FaultValidator) validateImmutableFields(newObj, oldObj *v1.Fault) error {
	if v1.GetClusterId(newObj) != v1.GetClusterId(oldObj) {
		return field.Forbidden(field.NewPath("metadata", "labels").Key(v1.ClusterIdLabel), "immutable")
	}
	if newObj.Spec.Node.ClusterName != oldObj.Spec.Node.ClusterName {
		return field.Forbidden(field.NewPath("spec", "node").Key("cluster"), "immutable")
	}
	if newObj.Spec.Id != oldObj.Spec.Id {
		return field.Forbidden(field.NewPath("spec").Key("id"), "immutable")
	}
	if newObj.Spec.Action != oldObj.Spec.Action {
		return field.Forbidden(field.NewPath("spec").Key("action"), "immutable")
	}
	if newObj.Spec.Node != nil && oldObj.Spec.Node != nil && newObj.Spec.Node.K8sName != oldObj.Spec.Node.K8sName {
		return field.Forbidden(field.NewPath("spec", "node").Key("name"), "immutable")
	}
	return nil
}
