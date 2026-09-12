/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	"regexp"
	"strconv"
	"strings"
)

var kubeVersionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)

var kubeSprayK8sVersions = map[string]string{
	"primussafe/kubespray:20200530": "1.32.5",
	"primussafe/kubespray:v2.29.1":  "1.33.7",
	"primussafe/kubespray:v2.30.0":  "1.34.3",
	"primussafe/kubespray:v2.31.0":  "1.35.4",
}

// KubeSprayK8sVersions returns the supported KubeSpray image and Kubernetes version pairs.
func KubeSprayK8sVersions() map[string]string {
	result := make(map[string]string, len(kubeSprayK8sVersions))
	for image, version := range kubeSprayK8sVersions {
		result[image] = version
	}
	return result
}

// KubeVersionForKubeSprayImage returns the Kubernetes version supported by an image.
func KubeVersionForKubeSprayImage(image string) (string, bool) {
	version, ok := kubeSprayK8sVersions[image]
	return version, ok
}

// ParseKubeVersion parses a strict semantic Kubernetes version.
func ParseKubeVersion(version string) (major, minor, patch int, ok bool) {
	if !kubeVersionPattern.MatchString(version) {
		return 0, 0, 0, false
	}
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	major, errMajor := strconv.Atoi(parts[0])
	minor, errMinor := strconv.Atoi(parts[1])
	patch, errPatch := strconv.Atoi(parts[2])
	if errMajor != nil || errMinor != nil || errPatch != nil {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}

// IsAllowedKubeVersionUpgrade permits patch upgrades and one minor version step.
func IsAllowedKubeVersionUpgrade(from, to string) bool {
	fromMajor, fromMinor, fromPatch, fromOK := ParseKubeVersion(from)
	toMajor, toMinor, toPatch, toOK := ParseKubeVersion(to)
	if !fromOK || !toOK || fromMajor != toMajor {
		return false
	}
	if toMinor == fromMinor {
		return toPatch >= fromPatch
	}
	return toMinor == fromMinor+1
}
