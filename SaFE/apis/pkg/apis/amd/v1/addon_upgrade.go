/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import "k8s.io/apimachinery/pkg/util/version"

// AutoUpgradeVersion returns the semantic version of a template that takes part in automatic Addon upgrades: it has
// a chart URL and a semantic version, is not being deleted, and is not labelled out of automatic upgrades.
func AutoUpgradeVersion(template *AddonTemplate) (*version.Version, bool) {
	if template.Spec.URL == "" || !template.DeletionTimestamp.IsZero() ||
		template.Labels[AddonAutoUpgradeLabel] == "false" {
		return nil, false
	}
	v, err := version.ParseSemantic(template.Spec.Version)
	if err != nil {
		return nil, false
	}
	return v, true
}

// IsSameChart reports whether two templates install the same chart: the same URL and type.
func IsSameChart(a, b *AddonTemplate) bool {
	return a.Spec.URL == b.Spec.URL && a.Spec.Type == b.Spec.Type
}

// IsUpgradeTarget reports whether a template of version v is an automatic upgrade target for an Addon on version
// current: its chart values are synced, v is above current, and v is a pre-release only when current is one.
func IsUpgradeTarget(template *AddonTemplate, v, current *version.Version) bool {
	return template.Status.HelmStatus.Values != "" && current.LessThan(v) &&
		(v.PreRelease() == "" || current.PreRelease() != "")
}
