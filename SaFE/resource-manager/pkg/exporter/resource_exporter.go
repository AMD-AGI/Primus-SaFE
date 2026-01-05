/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
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
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// ResourceFilter defines a function type for filtering resource updates.
// Returns true if the update should be filtered out (ignored), false otherwise.
type ResourceFilter func(objectOld, objectNew *unstructured.Unstructured) bool

// ResourceHandler defines a function type for handling resource objects.
// Processes the resource object and returns any error encountered.
type ResourceHandler func(ctx context.Context, object *unstructured.Unstructured) error

// ResourceExporter exports Kubernetes resources by watching for changes and processing them.
type ResourceExporter struct {
	// Kubernetes client for API operations
	client.Client
	// work queue
	*commonctrl.Controller[types.NamespacedName]
	// GroupVersionKind of the resource being exported
	gvk schema.GroupVersionKind
	// Handler function for processing resource objects
	handler ResourceHandler
}

// addExporter creates and registers a new ResourceExporter with the controller manager.
// It sets up watches for the specified resource type and configures filtering and handling.
func addExporter(ctx context.Context, mgr manager.Manager, gvk schema.GroupVersionKind,
	resourceHandler ResourceHandler, resourceFilter ResourceFilter) error {
	if gvk.Kind == "" {
		return fmt.Errorf("gvk.Kind is required")
	}
	if resourceHandler == nil {
		return fmt.Errorf("resourceHandler is required")
	}
	exporter := &ResourceExporter{
		Client:  mgr.GetClient(),
		gvk:     gvk,
		handler: resourceHandler,
	}
	exporter.Controller = commonctrl.NewController[types.NamespacedName](exporter, 1)
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

// Reconcile adds the resource to the processing queue when a change is detected.
func (r *ResourceExporter) Reconcile(_ context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	r.Controller.Add(req.NamespacedName)
	return ctrlruntime.Result{}, nil
}

// start initializes and starts the worker goroutines for processing resources.
func (r *ResourceExporter) start(ctx context.Context) error {
	for i := 0; i < r.MaxConcurrent; i++ {
		r.Run(ctx)
	}
	return nil
}

// getObject retrieves an unstructured object by its namespaced name.
func (r *ResourceExporter) getObject(ctx context.Context, objKey types.NamespacedName) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(r.gvk)
	if err := r.Get(ctx, objKey, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// Do processes a resource object by calling the handler and managing finalizers.
func (r *ResourceExporter) Do(ctx context.Context, msg types.NamespacedName) (ctrlruntime.Result, error) {
	obj, err := r.getObject(ctx, types.NamespacedName{Name: msg.Name, Namespace: msg.Namespace})
	if err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}

	if obj.GetDeletionTimestamp().IsZero() {
		if err = r.addFinalizer(ctx, obj); err != nil {
			return ctrlruntime.Result{}, err
		}
	}

	if r.handler != nil {
		if err = r.handler(ctx, obj); err != nil {
			klog.ErrorS(err, "failed to handle resource")
			return ctrlruntime.Result{}, err
		}
	}

	if !obj.GetDeletionTimestamp().IsZero() {
		if err = r.removeFinalizer(ctx, obj); err != nil {
			return ctrlruntime.Result{}, err
		}
	}
	return ctrlruntime.Result{}, nil
}

// addFinalizer adds the exporter finalizer to the object if it doesn't already exist.
// It uses a patch operation to update the object's metadata.
func (r *ResourceExporter) addFinalizer(ctx context.Context, object *unstructured.Unstructured) error {
	if !ctrlutil.AddFinalizer(object, v1.ExporterFinalizer) {
		return nil
	}
	return commonutils.PatchObjectFinalizer(ctx, r.Client, object)
}

// removeFinalizer removes the exporter finalizer from the object if it exists.
// It uses a patch operation to update the object's metadata.
func (r *ResourceExporter) removeFinalizer(ctx context.Context, object *unstructured.Unstructured) error {
	if !ctrlutil.RemoveFinalizer(object, v1.ExporterFinalizer) {
		return nil
	}
	return commonutils.PatchObjectFinalizer(ctx, r.Client, object)
}
