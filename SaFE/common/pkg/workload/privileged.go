/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// privilegedTemplateKinds are the workload kinds whose stock templates
// (charts/primus-safe/templates/configmap) run a privileged container whatever the
// workload asks for. AutoscalingRunnerSet, EphemeralRunner, and UnifiedJob carry a
// privileged container. GithubRunner's Docker sidecar is privileged when
// cicd.docker_enabled is set, and that container is part of the chart, so the kind
// takes the same grant. Code in a privileged pod controls the node.
// The chart is the source of truth: a test fails when a template and this set disagree.
var privilegedTemplateKinds = map[string]struct{}{
	common.CICDScaleRunnerSetKind:  {},
	common.CICDEphemeralRunnerKind: {},
	common.CICDGithubRunnerKind:    {},
	common.UnifiedJobKind:          {},
}

// HasPrivilegedTemplate reports whether the workload's kind is one whose stock template
// runs a privileged container whatever the workload asks for.
func HasPrivilegedTemplate(w *v1.Workload) bool {
	_, ok := privilegedTemplateKinds[w.SpecKind()]
	return ok
}

// RunsPrivileged reports whether the workload's pods run a privileged container: the
// workload asks for privileged mode, or its kind's template runs one regardless.
func RunsPrivileged(w *v1.Workload) bool {
	return v1.IsPrivileged(w) || HasPrivilegedTemplate(w)
}
