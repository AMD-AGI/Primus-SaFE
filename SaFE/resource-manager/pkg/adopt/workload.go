/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package adopt imports the Workloads of the SaFE running on an adopted cluster into the admin plane.
package adopt

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

// lastAppliedAnnotation is written by kubectl apply and is not copied to the admin plane.
const lastAppliedAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

// grantAnnotations record authorizations given to the owner of a Workload. They are not kept
// when the admin plane owner differs from the data plane owner.
var grantAnnotations = []string{
	v1.WorkloadHostPathAuthzAnnotation,
	v1.WorkloadPrivilegedVolumesAuthzAnnotation,
	v1.WorkloadRuntimeExtendedByAnnotation,
}

// ErrNotImportable marks a Workload that cannot be imported until its admin plane state changes,
// such as a missing workspace or a workspace of another cluster.
var ErrNotImportable = errors.New("workload is not importable")

// ListUnfinishedWorkloads returns the Workloads of the data plane SaFE that have not ended.
// It returns none when the data plane has no Workload CRD.
func ListUnfinishedWorkloads(ctx context.Context, dc dynamic.Interface) ([]*v1.Workload, error) {
	if dc == nil {
		return nil, nil
	}
	list, err := dc.Resource(v1.SchemeGroupVersion.WithResource("workloads")).List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil, nil
		}
		return nil, err
	}
	result := make([]*v1.Workload, 0, len(list.Items))
	for i := range list.Items {
		workload := &v1.Workload{}
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(list.Items[i].Object, workload); err != nil {
			return nil, fmt.Errorf("decode data plane workload %s: %w", list.Items[i].GetName(), err)
		}
		if workload.IsEnd() {
			continue
		}
		result = append(result, workload)
	}
	return result, nil
}

// MatchedGlobalUser returns the admin plane User whose id and name both equal the owner of
// the data plane Workload, or nil when there is none.
func MatchedGlobalUser(ctx context.Context, c client.Client, local *v1.Workload) (*v1.User, error) {
	id := v1.GetUserId(local)
	if id == "" {
		return nil, nil
	}
	user := &v1.User{}
	if err := c.Get(ctx, types.NamespacedName{Name: id}, user); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	if v1.GetUserName(user) != v1.GetUserName(local) {
		return nil, nil
	}
	return user, nil
}

// Import actions decided by PlanImports.
const (
	// ActionExists means a Workload of the same name is already on the admin plane.
	ActionExists = "exists"
	// ActionUnscheduled means the data plane SaFE has not scheduled the Workload yet.
	ActionUnscheduled = "unscheduled"
	// ActionMatched means the owner matches an admin plane user by id and name.
	ActionMatched = "matched"
	// ActionMapped means the owner is taken from the user map.
	ActionMapped = "mapped"
	// ActionUnmapped means no admin plane owner was found.
	ActionUnmapped = "unmapped"
)

// Plan is the import decision for one data plane Workload. User is set for matched and mapped workloads.
type Plan struct {
	Workload *v1.Workload
	User     *v1.User
	Action   string
}

// PlanImports decides how each data plane Workload is imported. An owner that does not match an
// admin plane user is looked up in userMap, keyed by the data plane user name, whose value is an
// admin plane user id. A user id in userMap that does not exist is an error. Workloads the data
// plane SaFE has neither scheduled nor dispatched are left to it unless includeUnscheduled is set.
func PlanImports(ctx context.Context, c client.Client, locals []*v1.Workload,
	userMap map[string]string, includeUnscheduled bool) ([]Plan, error) {
	plans := make([]Plan, 0, len(locals))
	for _, local := range locals {
		plan := Plan{Workload: local, Action: ActionUnmapped}
		if err := c.Get(ctx, types.NamespacedName{Name: local.Name}, &v1.Workload{}); err == nil {
			plan.Action = ActionExists
			plans = append(plans, plan)
			continue
		} else if !apierrors.IsNotFound(err) {
			return nil, err
		}
		if !includeUnscheduled && !v1.IsWorkloadScheduled(local) && !v1.IsWorkloadDispatched(local) {
			plan.Action = ActionUnscheduled
			plans = append(plans, plan)
			continue
		}
		user, err := MatchedGlobalUser(ctx, c, local)
		if err != nil {
			return nil, err
		}
		if user != nil {
			plan.User, plan.Action = user, ActionMatched
		} else if id := userMap[v1.GetUserName(local)]; id != "" {
			user = &v1.User{}
			if err = c.Get(ctx, types.NamespacedName{Name: id}, user); err != nil {
				return nil, fmt.Errorf("mapped user %s of %s: %w", id, v1.GetUserName(local), err)
			}
			plan.User, plan.Action = user, ActionMapped
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

// AdminWorkload returns the admin plane copy of a data plane Workload owned by user.
// Spec, labels and annotations are copied as they are, the owner is replaced by user, and
// the copy is marked as adopted with the UID of the data plane Workload. Grants of the data plane
// owner are dropped when user is another owner.
func AdminWorkload(local *v1.Workload, user *v1.User) *v1.Workload {
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:        local.Name,
			Labels:      copyMap(local.Labels),
			Annotations: copyMap(local.Annotations),
		},
		Spec: *local.Spec.DeepCopy(),
	}
	delete(workload.Annotations, lastAppliedAnnotation)
	if user.Name != v1.GetUserId(local) {
		for _, key := range grantAnnotations {
			delete(workload.Annotations, key)
		}
	}
	userName := v1.GetUserName(user)
	if userName == "" {
		userName = user.Name
	}
	v1.SetLabel(workload, v1.UserIdLabel, user.Name)
	v1.SetAnnotation(workload, v1.UserNameAnnotation, userName)
	v1.SetLabel(workload, v1.UserNameMd5Label, stringutil.MD5(userName))
	v1.SetAnnotation(workload, v1.WorkloadAdoptAnnotation, v1.TrueStr)
	v1.SetAnnotation(workload, v1.WorkloadAdoptUidAnnotation, string(local.UID))
	return workload
}

// ImportWorkload creates the admin plane copy of a data plane Workload of the given cluster and
// copies its status. It returns false when a Workload of the same name exists, and only completes
// the status of an imported Workload that has none. The Workload must run in a workspace of the
// cluster. Errors that persist until the admin plane state changes wrap ErrNotImportable.
func ImportWorkload(ctx context.Context, c client.Client, clusterName string, local *v1.Workload, user *v1.User) (bool, error) {
	if user == nil {
		return false, fmt.Errorf("workload %s has no admin plane owner: %w", local.Name, ErrNotImportable)
	}
	existing := &v1.Workload{}
	if err := c.Get(ctx, types.NamespacedName{Name: local.Name}, existing); err == nil {
		if v1.IsAdoptedWorkload(existing) {
			return false, copyStatus(ctx, c, existing.Name, &local.Status)
		}
		return false, nil
	} else if !apierrors.IsNotFound(err) {
		return false, err
	}
	workspace := &v1.Workspace{}
	if err := c.Get(ctx, types.NamespacedName{Name: local.Spec.Workspace}, workspace); err != nil {
		if apierrors.IsNotFound(err) {
			err = fmt.Errorf("%w: %w", ErrNotImportable, err)
		}
		return false, fmt.Errorf("workspace %s of workload %s: %w", local.Spec.Workspace, local.Name, err)
	}
	if workspace.Spec.Cluster != clusterName {
		return false, fmt.Errorf("workspace %s of workload %s belongs to cluster %s, not %s: %w",
			workspace.Name, local.Name, workspace.Spec.Cluster, clusterName, ErrNotImportable)
	}
	workload := AdminWorkload(local, user)
	if err := c.Create(ctx, workload); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return false, nil
		}
		if apierrors.IsInvalid(err) || apierrors.IsForbidden(err) || apierrors.IsBadRequest(err) {
			err = fmt.Errorf("%w: %w", ErrNotImportable, err)
		}
		return false, fmt.Errorf("create workload %s: %w", local.Name, err)
	}
	return true, copyStatus(ctx, c, workload.Name, &local.Status)
}

// copyStatus writes the data plane status to a newly created admin Workload. A status already
// set by the admin plane controllers is kept.
func copyStatus(ctx context.Context, c client.Client, name string, status *v1.WorkloadStatus) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		workload := &v1.Workload{}
		if err := c.Get(ctx, types.NamespacedName{Name: name}, workload); err != nil {
			return err
		}
		if workload.Status.Phase != "" {
			return nil
		}
		workload.Status = *status.DeepCopy()
		return c.Status().Update(ctx, workload)
	})
}

func copyMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
