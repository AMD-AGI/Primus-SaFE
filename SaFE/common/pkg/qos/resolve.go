/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package qos

import (
	"context"
	"fmt"
	"strings"

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

// Ceiling loads the user and the global and cluster defaults, then resolves the workspace scope.
// A missing user or Qos is an empty contribution, not an error.
func Ceiling(ctx context.Context, c client.Reader, userID, clusterID, scope string) (Limits, error) {
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
	globalQos, err := Load(ctx, c, v1.QosDefaultName)
	if err != nil {
		return Limits{}, err
	}
	return Resolve(user, clusterQos, globalQos, clusterID, scope), nil
}

// Resolve picks each field independently. A set value stops the search.
// Order: this user on the cluster, the cluster default, this user on every cluster, the global default.
// scope is a workspace scope such as Train or Infer.
func Resolve(user *v1.User, clusterQos, globalQos *v1.Qos, clusterID, scope string) Limits {
	var userCluster, userGlobal v1.UserQosScope
	if user != nil && user.Spec.Qos != nil {
		userGlobal = v1.UserQosScope{MaxResources: user.Spec.Qos.MaxResources, MaxRuntime: user.Spec.Qos.MaxRuntime}
		if scope, ok := user.Spec.Qos.Clusters[clusterID]; ok {
			userCluster = scope
		}
	}
	return Limits{
		CPU:            firstSet(capCPU(userCluster.MaxResources), capCPU(specCap(clusterQos)), capCPU(userGlobal.MaxResources), capCPU(specCap(globalQos))),
		Memory:         firstSet(capMemory(userCluster.MaxResources), capMemory(specCap(clusterQos)), capMemory(userGlobal.MaxResources), capMemory(specCap(globalQos))),
		GPU:            firstSet(capGPU(userCluster.MaxResources), capGPU(specCap(clusterQos)), capGPU(userGlobal.MaxResources), capGPU(specCap(globalQos))),
		RuntimeSeconds: firstRuntime(scope, userCluster.MaxRuntime, specRuntime(clusterQos), userGlobal.MaxRuntime, specRuntime(globalQos)),
	}
}

// QuotaReason reports the dimensions where used plus request passes the ceiling.
// An empty result means the request fits. A request that is already larger than the ceiling
// is the same condition with used at zero, and it stays pending.
func QuotaReason(userID, clusterID string, used, request corev1.ResourceList, limits Limits) (string, error) {
	var parts []string
	checks := []struct {
		name  string
		limit string
		used  resource.Quantity
		ask   resource.Quantity
	}{
		{"cpu", limits.CPU, used[corev1.ResourceCPU], request[corev1.ResourceCPU]},
		{"memory", limits.Memory, used[corev1.ResourceMemory], request[corev1.ResourceMemory]},
		{"gpu", limits.GPU, gpuQuantity(used), gpuQuantity(request)},
	}
	for _, check := range checks {
		if strings.TrimSpace(check.limit) == "" {
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
