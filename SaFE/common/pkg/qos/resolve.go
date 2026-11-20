/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package qos

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// UserQuotaReasonPrefix marks a workload that is waiting on its user's cluster ceiling.
// It is distinct from the workspace capacity reason so the scheduler does not describe it
// as blocked by earlier workloads.
const UserQuotaReasonPrefix = "In queue - user quota exceeded"

// UserQuotaCheckFailedPrefix marks a workload whose quota could not be evaluated, for example
// because a stored ceiling is not a valid quantity. Only that workload waits.
const UserQuotaCheckFailedPrefix = "In queue - user quota check failed"

// IsUserQuotaReason reports whether a pending reason comes from the per-user ceiling.
func IsUserQuotaReason(reason string) bool {
	return strings.HasPrefix(reason, UserQuotaReasonPrefix) || strings.HasPrefix(reason, UserQuotaCheckFailedPrefix)
}

// AdmissionBefore reports whether a is admitted ahead of b.
// An earlier creation time goes first. The same time keeps the higher priority.
// The same priority keeps the name that sorts first.
func AdmissionBefore(a, b *v1.Workload) bool {
	if a == nil || b == nil {
		return false
	}
	if a.CreationTimestamp.Time.Before(b.CreationTimestamp.Time) {
		return true
	}
	if b.CreationTimestamp.Time.Before(a.CreationTimestamp.Time) {
		return false
	}
	if a.Spec.Priority != b.Spec.Priority {
		return a.Spec.Priority > b.Spec.Priority
	}
	return a.Name < b.Name
}

// Limits is the ceiling that applies to one user on one cluster for one workspace scope.
// An empty resource field and a zero runtime mean that dimension is not limited.
type Limits struct {
	CPU            string
	Memory         string
	GPU            string
	RuntimeSeconds int
}

// Load returns the named Qos, or nil when it does not exist.
func Load(ctx context.Context, c client.Reader, name string) (*v1.Qos, error) {
	if name == "" {
		return nil, nil
	}
	qos := &v1.Qos{}
	if err := c.Get(ctx, client.ObjectKey{Name: name}, qos); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return qos, nil
}

// Ceiling loads the user and the cluster Qos, then resolves the workspace scope.
// A missing user or Qos is an empty contribution, not an error.
// The workspace maxRuntime is not considered; CeilingInWorkspace adds it.
func Ceiling(ctx context.Context, c client.Reader, userID, clusterID, scope string) (Limits, error) {
	return CeilingInWorkspace(ctx, c, userID, clusterID, scope, nil)
}

// CeilingInWorkspace is Ceiling with the workspace lowUtilization maxRuntime of the scope
// between the user and the cluster runtime. A nil workspace contributes nothing.
func CeilingInWorkspace(ctx context.Context, c client.Reader,
	userID, clusterID, scope string, workspace *v1.Workspace) (Limits, error) {
	var user *v1.User
	if userID != "" {
		user = &v1.User{}
		if err := c.Get(ctx, client.ObjectKey{Name: userID}, user); err != nil {
			if !apierrors.IsNotFound(err) {
				return Limits{}, err
			}
			user = nil
		}
	}
	clusterQos, err := Load(ctx, c, clusterID)
	if err != nil {
		return Limits{}, err
	}
	return Resolve(user, workspace, clusterQos, clusterID, scope), nil
}

// Resolve picks each field independently. A set value stops the search.
// Order: this user on the cluster, then the cluster ceiling. The runtime also looks at the
// workspace maxRuntime of the scope between the two.
// scope is a workspace scope such as Train or Infer.
func Resolve(user *v1.User, workspace *v1.Workspace, clusterQos *v1.Qos, clusterID, scope string) Limits {
	var userCluster v1.UserQosScope
	if user != nil && user.Spec.Qos != nil {
		if item, ok := user.Spec.Qos.Clusters[clusterID]; ok {
			userCluster = item
		}
	}
	return Limits{
		CPU:            firstSet(capCPU(userCluster.MaxResources), capCPU(specCap(clusterQos))),
		Memory:         firstSet(capMemory(userCluster.MaxResources), capMemory(specCap(clusterQos))),
		GPU:            firstSet(capGPU(userCluster.MaxResources), capGPU(specCap(clusterQos))),
		RuntimeSeconds: firstRuntime(scope, userCluster.MaxRuntime, workspaceRuntime(workspace), specRuntime(clusterQos)),
	}
}

// QuotaReason reports the dimensions where used plus request passes the ceiling.
// An empty result means the request fits.
func QuotaReason(userID, clusterID string, used, request corev1.ResourceList, limits Limits) (string, error) {
	return Exceeded(userID, clusterID, used, request, limits, true, true, true)
}

// ResourceGrew reports which of CPU, memory and GPU is larger in next than in prev.
func ResourceGrew(prev, next corev1.ResourceList) (cpu, memory, gpu bool) {
	prevCPU, nextCPU := quantityOf(prev, corev1.ResourceCPU), quantityOf(next, corev1.ResourceCPU)
	prevMemory, nextMemory := quantityOf(prev, corev1.ResourceMemory), quantityOf(next, corev1.ResourceMemory)
	prevGPU, nextGPU := gpuQuantity(prev), gpuQuantity(next)
	return nextCPU.Cmp(prevCPU) > 0, nextMemory.Cmp(prevMemory) > 0, nextGPU.Cmp(prevGPU) > 0
}

// Exceeded reports the dimensions where used plus request passes the ceiling.
// cpu, memory and gpu select which dimensions are checked.
func Exceeded(userID, clusterID string, used, request corev1.ResourceList, limits Limits, cpu, memory, gpu bool) (string, error) {
	var parts []string
	checks := []struct {
		name  string
		on    bool
		limit string
		used  resource.Quantity
		ask   resource.Quantity
	}{
		{"cpu", cpu, limits.CPU, used[corev1.ResourceCPU], request[corev1.ResourceCPU]},
		{"memory", memory, limits.Memory, used[corev1.ResourceMemory], request[corev1.ResourceMemory]},
		{"gpu", gpu, limits.GPU, gpuQuantity(used), gpuQuantity(request)},
	}
	for _, check := range checks {
		if !check.on || strings.TrimSpace(check.limit) == "" {
			continue
		}
		ceiling, err := resource.ParseQuantity(check.limit)
		if err != nil {
			return "", fmt.Errorf("invalid %s qos limit %q: %w", check.name, check.limit, err)
		}
		total := check.used.DeepCopy()
		total.Add(check.ask)
		if total.Cmp(ceiling) > 0 {
			parts = append(parts, fmt.Sprintf("%s used %s + request %s > limit %s",
				check.name, check.used.String(), check.ask.String(), ceiling.String()))
		}
	}
	if len(parts) == 0 {
		return "", nil
	}
	return fmt.Sprintf("%s: %s (user %s, cluster %s)",
		UserQuotaReasonPrefix, strings.Join(parts, "; "), userID, clusterID), nil
}

// specCap returns the resource ceiling of a Qos, or nil when the object is absent.
func specCap(qos *v1.Qos) *v1.ResourceCap {
	if qos == nil {
		return nil
	}
	return qos.Spec.MaxResources
}

// specRuntime returns the timeout map of a Qos, or nil when the object is absent.
func specRuntime(qos *v1.Qos) map[string]int {
	if qos == nil {
		return nil
	}
	return qos.Spec.MaxRuntime
}

// workspaceRuntime returns the maxRuntime of each workspace scope in seconds.
// A scope whose maxRuntime is empty or not a positive duration is left out.
func workspaceRuntime(workspace *v1.Workspace) map[string]int {
	if workspace == nil {
		return nil
	}
	var runtimes map[string]int
	for scope, policy := range workspace.Spec.LowUtilization {
		d, err := time.ParseDuration(policy.MaxRuntime)
		if err != nil || d < time.Second {
			continue
		}
		if runtimes == nil {
			runtimes = make(map[string]int)
		}
		runtimes[string(scope)] = int(d / time.Second)
	}
	return runtimes
}

// capCPU returns the CPU ceiling, or empty when it is unset.
func capCPU(cap *v1.ResourceCap) string {
	if cap == nil {
		return ""
	}
	return strings.TrimSpace(cap.CPU)
}

// capMemory returns the memory ceiling, or empty when it is unset.
func capMemory(cap *v1.ResourceCap) string {
	if cap == nil {
		return ""
	}
	return strings.TrimSpace(cap.Memory)
}

// capGPU returns the GPU ceiling, or empty when it is unset.
func capGPU(cap *v1.ResourceCap) string {
	if cap == nil {
		return ""
	}
	return strings.TrimSpace(cap.GPU)
}

// firstSet returns the first quantity that is set. Empty and "0" fall through.
func firstSet(values ...string) string {
	for _, value := range values {
		if value != "" && value != "0" {
			return value
		}
	}
	return ""
}

// firstRuntime returns the first positive ceiling for a workspace scope.
func firstRuntime(scope string, runtimes ...map[string]int) int {
	for _, item := range runtimes {
		if seconds, ok := item[scope]; ok && seconds > 0 {
			return seconds
		}
	}
	return 0
}

// quantityOf returns one resource, or zero when it is absent.
func quantityOf(list corev1.ResourceList, name corev1.ResourceName) resource.Quantity {
	if list == nil {
		return resource.Quantity{}
	}
	return list[name]
}

// gpuQuantity sums NVIDIA and AMD GPU cards in a resource list.
func gpuQuantity(list corev1.ResourceList) resource.Quantity {
	total := resource.MustParse("0")
	for _, name := range []corev1.ResourceName{common.NvidiaGpu, common.AmdGpu} {
		if value, ok := list[name]; ok {
			total.Add(value)
		}
	}
	return total
}
