/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

// sandboxCapabilities is everything a Sandbox container keeps after dropping ALL.
// Sandboxes run as root so that a session can install packages and run services:
// apt and dpkg chown and chmod files (CHOWN, DAC_OVERRIDE, FOWNER, FSETID), su and
// service start-up switch to a service user (SETUID, SETGID), root signals that
// user's processes (KILL), servers bind low ports (NET_BIND_SERVICE) and PAM writes
// audit records (AUDIT_WRITE). Dropped from the runtime default: NET_RAW, MKNOD,
// SYS_CHROOT, SETPCAP and SETFCAP. The cluster's sandbox-pod-guardrails
// ValidatingAdmissionPolicy allows exactly this list.
var sandboxCapabilities = []interface{}{
	"AUDIT_WRITE", "CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID", "KILL",
	"NET_BIND_SERVICE", "SETGID", "SETUID",
}

// applySandboxPodSecurity gives every Sandbox Pod the security posture the
// sandbox-pod-guardrails admission policy requires, whoever asked for the Sandbox:
// no ServiceAccount token, no host namespaces, RuntimeDefault seccomp, and on every
// container and init container no privilege, no privilege escalation and only
// sandboxCapabilities. It is the last Pod spec write on both paths that write one:
// last in initializeObject on create, so the privileged annotation, the external
// path, the envd injector and template layers cannot widen it, and last in
// applyWorkloadSpecToObject (through enforceSandboxPodSecurity) on sync, so
// updateHostNetwork cannot turn hostNetwork back on. Other workload kinds,
// including an Agent Sandbox PyTorchJob, are untouched.
func applySandboxPodSecurity(obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) error {
	if !commonworkload.IsSandBox(workload) {
		return nil
	}
	for field, value := range map[string]interface{}{
		"automountServiceAccountToken": false,
		"hostNetwork":                  false,
		"hostPID":                      false,
		"hostIPC":                      false,
	} {
		if err := jobutils.SetNestedField(obj.Object, value, podSpecPath(workload, resourceSpec, field)); err != nil {
			return err
		}
	}
	// Only the profile is set; the Pod's other securityContext fields are kept.
	if err := jobutils.SetNestedField(obj.Object, map[string]interface{}{"type": "RuntimeDefault"},
		podSpecPath(workload, resourceSpec, "securityContext", "seccompProfile")); err != nil {
		return err
	}
	for _, field := range []string{"containers", "initContainers"} {
		if err := hardenSandboxContainers(obj, workload, resourceSpec, field); err != nil {
			return err
		}
	}
	return nil
}

// hardenSandboxContainers sets the container-level part of applySandboxPodSecurity
// on one container list. A missing list is a no-op.
func hardenSandboxContainers(obj *unstructured.Unstructured, workload *v1.Workload,
	resourceSpec *v1.ResourceSpec, field string) error {
	path := podSpecPath(workload, resourceSpec, field)
	containers, found, err := jobutils.NestedSlice(obj.Object, path)
	if err != nil {
		return err
	}
	if !found || len(containers) == 0 {
		return nil
	}
	for i := range containers {
		container, ok := containers[i].(map[string]interface{})
		if !ok {
			continue
		}
		sc, _ := container["securityContext"].(map[string]interface{})
		if sc == nil {
			sc = map[string]interface{}{}
		}
		sc["privileged"] = false
		sc["allowPrivilegeEscalation"] = false
		sc["capabilities"] = map[string]interface{}{
			"drop": []interface{}{"ALL"},
			"add":  append([]interface{}{}, sandboxCapabilities...),
		}
		// A container-level profile would override the Pod's RuntimeDefault.
		delete(sc, "seccompProfile")
		container["securityContext"] = sc
		containers[i] = container
	}
	return jobutils.SetNestedField(obj.Object, containers, path)
}

// enforceSandboxPodSecurity applies applySandboxPodSecurity to every resource spec
// of a Sandbox object that is being synced. A resource spec whose pod is missing is
// refused, as in initializeObject, instead of being left without the posture.
func enforceSandboxPodSecurity(obj *unstructured.Unstructured, workload *v1.Workload, rt *v1.ResourceTemplate) error {
	if !commonworkload.IsSandBox(workload) || rt == nil {
		return nil
	}
	for i := range rt.Spec.ResourceSpecs {
		if i >= len(workload.Spec.Resources) {
			break
		}
		spec := &rt.Spec.ResourceSpecs[i]
		if len(spec.PrePaths) > 0 {
			if _, found, err := jobutils.NestedField(obj.Object, spec.PrePaths); err != nil || !found {
				return sandboxMissingPodError(spec, err)
			}
		}
		if err := applySandboxPodSecurity(obj, workload, spec); err != nil {
			return err
		}
	}
	return nil
}

// sandboxMissingPodError reports a Sandbox object with nothing at a resource spec's
// PrePaths, so that the dispatch fails instead of producing a Pod without the posture.
func sandboxMissingPodError(spec *v1.ResourceSpec, err error) error {
	if err != nil {
		return fmt.Errorf("sandbox template has no valid object at %v: %v", spec.PrePaths, err)
	}
	return fmt.Errorf("sandbox template has no object at %v", spec.PrePaths)
}
