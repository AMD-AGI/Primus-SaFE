/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exporter

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonctrl "github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
)

type ResourceFilter func(objectOld, objectNew *unstructured.Unstructured) bool
type ResourceHandler func(ctx context.Context, object *unstructured.Unstructured) error

type ResourceExporter struct {
	ctx context.Context
	client.Client
	*commonctrl.Controller[*unstructured.Unstructured]
	gvk     schema.GroupVersionKind
	handler ResourceHandler
}

func addExporter(ctx context.Context, mgr manager.Manager, gvk schema.GroupVersionKind,
	resourceHandler ResourceHandler, resourceFilter ResourceFilter) error {
	if gvk.Kind == "" || resourceHandler == nil {
		return fmt.Errorf("bad request")
	}
	exporter := &ResourceExporter{
		ctx:     ctx,
		Client:  mgr.GetClient(),
		gvk:     gvk,
		handler: resourceHandler,
	}
	exporter.Controller = commonctrl.NewController[*unstructured.Unstructured](exporter, 1)
	if err := exporter.start(ctx); err != nil {
		return err
	}

	filter := func(oldObj, newObj *unstructured.Unstructured) bool {
		if oldObj != nil && oldObj.GroupVersionKind() != gvk {
			return true
		}
		if newObj != nil && newObj.GroupVersionKind() != gvk {
			return true
		}
		if resourceFilter != nil && resourceFilter(oldObj, newObj) {
			return true
		}
		return false
	}
	typedPredicateFuncs := predicate.TypedFuncs[*unstructured.Unstructured]{
		CreateFunc: func(e event.TypedCreateEvent[*unstructured.Unstructured]) bool {
			return !filter(nil, e.Object)
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*unstructured.Unstructured]) bool {
			return !filter(e.ObjectOld, e.ObjectNew)
		},
		GenericFunc: func(e event.TypedGenericEvent[*unstructured.Unstructured]) bool {
			return false
		},
	}

	ctrl, err := controller.New(fmt.Sprintf("%s-exporter", gvk.Kind), mgr, controller.Options{Reconciler: exporter})
	if err != nil {
		return err
	}
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	err = ctrl.Watch(source.Kind(mgr.GetCache(), obj,
		&handler.TypedEnqueueRequestForObject[*unstructured.Unstructured]{}, typedPredicateFuncs))
	if err != nil {
		return err
	}

	klog.Infof("Add Resource %v Exporter successfully", gvk)
	return nil
}

func (r *ResourceExporter) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	obj, err := r.getObject(ctx, req.NamespacedName)
	if err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	r.Controller.Add(obj)
	return ctrlruntime.Result{}, nil
}

func (r *ResourceExporter) start(ctx context.Context) error {
	for i := 0; i < r.MaxConcurrent; i++ {
		r.Run(ctx)
	}
	return nil
}

func (r *ResourceExporter) getObject(ctx context.Context, objKey types.NamespacedName) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(r.gvk)
	if err := r.Get(ctx, objKey, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (r *ResourceExporter) Do(ctx context.Context, obj *unstructured.Unstructured) (commonctrl.Result, error) {
	var err error
	obj, err = r.getObject(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()})
	if err != nil {
		return commonctrl.Result{}, client.IgnoreNotFound(err)
	}

	if obj.GetDeletionTimestamp().IsZero() && !ctrlutil.ContainsFinalizer(obj, v1.ExporterFinalizer) {
		patch := client.MergeFrom(obj.DeepCopy())
		ctrlutil.AddFinalizer(obj, v1.ExporterFinalizer)
		if err = r.Patch(ctx, obj, patch); err != nil {
			return commonctrl.Result{}, err
		}
	}

	if r.handler != nil {
		if err = r.handler(r.ctx, obj); err != nil {
			klog.ErrorS(err, "failed to handle resource")
			return commonctrl.Result{}, err
		}
	}

	if !obj.GetDeletionTimestamp().IsZero() && ctrlutil.ContainsFinalizer(obj, v1.ExporterFinalizer) {
		patch := client.MergeFrom(obj.DeepCopy())
		ctrlutil.RemoveFinalizer(obj, v1.ExporterFinalizer)
		if err = r.Patch(ctx, obj, patch); err != nil {
			return commonctrl.Result{}, err
		}
	}
	return commonctrl.Result{}, nil
}
