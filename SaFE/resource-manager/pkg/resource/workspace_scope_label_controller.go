/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"encoding/json"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// WorkspaceScopeLabelReconciler keeps WorkspaceSandboxScopeLabel in step with Spec.Scopes.
// The workspace webhook sets the label on every write it sees, but a workspace written before
// the label existed carries none until something writes it again, and a few update paths skip
// the webhook's mutation. On start every workspace arrives as a create event, so existing
// workspaces are labelled without anyone touching them.
type WorkspaceScopeLabelReconciler struct {
	client.Client
}

func SetupWorkspaceScopeLabelController(mgr manager.Manager) error {
	r := &WorkspaceScopeLabelReconciler{Client: mgr.GetClient()}
	return ctrlruntime.NewControllerManagedBy(mgr).
		Named("workspace-scope-label").
		For(&v1.Workspace{}, builder.WithPredicates(scopeLabelOutOfDatePredicate{})).
		Complete(r)
}

// scopeLabelOutOfDatePredicate passes a Workspace whose label differs from what its scopes say.
type scopeLabelOutOfDatePredicate struct {
	predicate.Funcs
}

func scopeLabelOutOfDate(obj client.Object) bool {
	ws, ok := obj.(*v1.Workspace)
	return ok && ws.GetDeletionTimestamp().IsZero() &&
		v1.GetLabel(ws, v1.WorkspaceSandboxScopeLabel) != ws.SandboxScopeLabelValue()
}

func (scopeLabelOutOfDatePredicate) Create(e event.CreateEvent) bool {
	return scopeLabelOutOfDate(e.Object)
}

func (scopeLabelOutOfDatePredicate) Update(e event.UpdateEvent) bool {
	return scopeLabelOutOfDate(e.ObjectNew)
}

func (scopeLabelOutOfDatePredicate) Delete(event.DeleteEvent) bool { return false }

func (scopeLabelOutOfDatePredicate) Generic(event.GenericEvent) bool { return false }

// Reconcile patches only the one label, so it cannot overwrite a concurrent change to anything
// else; the webhook recomputes the same value from the same scopes.
func (r *WorkspaceScopeLabelReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	ws := &v1.Workspace{}
	if err := r.Get(ctx, req.NamespacedName, ws); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if !scopeLabelOutOfDate(ws) {
		return ctrlruntime.Result{}, nil
	}
	var value interface{}
	if want := ws.SandboxScopeLabelValue(); want != "" {
		value = want
	}
	patch, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{v1.WorkspaceSandboxScopeLabel: value},
		},
	})
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	err = r.Patch(ctx, ws, client.RawPatch(types.MergePatchType, patch))
	if apierrors.IsNotFound(err) {
		return ctrlruntime.Result{}, nil
	}
	return ctrlruntime.Result{}, err
}
