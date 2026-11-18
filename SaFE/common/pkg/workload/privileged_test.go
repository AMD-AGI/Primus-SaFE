/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

var (
	// templateKindLabel is the label a workload template carries to name the kind it renders.
	templateKindLabel = regexp.MustCompile(`(?m)^\s*primus-safe\.workload\.kind:\s*"?([A-Za-z]+)"?\s*$`)
	// privilegedSetting is a container that is privileged, or whose privileged setting is left to
	// a value the chart fills in, which the test must treat as privileged.
	privilegedSetting = regexp.MustCompile(`(?m)^\s*privileged:\s*(true|"true"|\{\{)`)
)

// The kinds whose stock templates run a privileged container are read from the chart, not
// copied: every workload template under charts/primus-safe/templates/configmap that has a
// privileged container must be in privilegedTemplateKinds, and every kind in it must have one, so
// a template that becomes privileged cannot be created without the grant unnoticed.
func TestPrivilegedTemplateKindsMatchTheChart(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// here is .../SaFE/common/pkg/workload/privileged_test.go
	dir := filepath.Join(filepath.Dir(here), "..", "..", "..", "charts", "primus-safe", "templates", "configmap")
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	require.NoError(t, err)

	var templates int
	var fromChart []string
	for _, file := range files {
		raw, err := os.ReadFile(file)
		require.NoError(t, err)
		kind := templateKindLabel.FindSubmatch(raw)
		if kind == nil {
			continue
		}
		templates++
		if privilegedSetting.Match(raw) {
			fromChart = append(fromChart, string(kind[1]))
		}
	}
	// The chart holds a dozen workload templates; finding few means the files were not read.
	require.GreaterOrEqual(t, templates, 10, "workload templates found under %s", dir)

	var inCode []string
	for kind := range privilegedTemplateKinds {
		inCode = append(inCode, kind)
	}
	sort.Strings(fromChart)
	sort.Strings(inCode)
	require.Equal(t, fromChart, inCode)
}

func TestRunsPrivileged(t *testing.T) {
	workload := func(kind string, annotations map[string]string) *v1.Workload {
		return &v1.Workload{
			ObjectMeta: metav1.ObjectMeta{Annotations: annotations},
			Spec:       v1.WorkloadSpec{GroupVersionKind: v1.GroupVersionKind{Version: "v1", Kind: kind}},
		}
	}
	privileged := map[string]string{v1.WorkloadPrivilegedAnnotation: v1.TrueStr}
	for _, kind := range []string{common.CICDScaleRunnerSetKind, common.CICDEphemeralRunnerKind,
		common.CICDGithubRunnerKind, common.UnifiedJobKind} {
		require.True(t, HasPrivilegedTemplate(workload(kind, nil)), kind)
		require.True(t, RunsPrivileged(workload(kind, nil)), kind)
	}
	for _, kind := range []string{common.PytorchJobKind, common.AuthoringKind, common.DeploymentKind,
		common.SandboxKind, common.RayJobKind, ""} {
		require.False(t, HasPrivilegedTemplate(workload(kind, nil)), kind)
		require.False(t, RunsPrivileged(workload(kind, nil)), kind)
		require.True(t, RunsPrivileged(workload(kind, privileged)), kind)
	}
	require.False(t, RunsPrivileged(workload(common.PytorchJobKind,
		map[string]string{v1.WorkloadPrivilegedAnnotation: "false"})))
}
