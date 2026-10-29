/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

const (
	// sandboxLayersAnnotation carries the Agent Sandbox template layers as a JSON list.
	sandboxLayersAnnotation = "agent-sandbox.io/layers"
	maxSandboxLayers        = 4
	sandboxLayerVolume      = "as-layer-"
	envdInjectorName        = "envd-injector"
	// signingKeyRequiredEnv makes the envd-injector fail when no signing key file is mounted.
	signingKeyRequiredEnv = "SIGNING_KEY_REQUIRED"
)

// sandboxLayerReservedPaths are owned by the sandbox runtime and may not hold a layer.
var sandboxLayerReservedPaths = []string{
	"/app", "/bin", "/dev", "/etc", "/lib", "/lib32", "/lib64", "/proc", "/run",
	"/sbin", "/shared", "/sys", "/tmp", "/usr", "/var/run",
}

var (
	sandboxLayerImage = regexp.MustCompile(`^[a-z0-9][a-z0-9._/:-]*@sha256:[0-9a-f]{64}$`)
	sandboxLayerPath  = regexp.MustCompile(`^/[A-Za-z0-9._/-]*$`)
)

// sandboxLayer is one image directory copied into the sandbox before it starts.
type sandboxLayer struct {
	Image    string `json:"image"`
	From     string `json:"from"`
	Mount    string `json:"mount"`
	ReadOnly *bool  `json:"readOnly,omitempty"`
}

// parseSandboxLayers reads and validates the layers annotation of a Workload.
func parseSandboxLayers(workload *v1.Workload) ([]sandboxLayer, error) {
	raw := v1.GetAnnotation(workload, sandboxLayersAnnotation)
	if raw == "" {
		return nil, nil
	}
	var layers []sandboxLayer
	if err := json.Unmarshal([]byte(raw), &layers); err != nil {
		return nil, fmt.Errorf("%s is not a JSON list: %v", sandboxLayersAnnotation, err)
	}
	if len(layers) > maxSandboxLayers {
		return nil, fmt.Errorf("%s: at most %d layers, got %d", sandboxLayersAnnotation, maxSandboxLayers, len(layers))
	}
	var mounts []string
	for i, l := range layers {
		if !sandboxLayerImage.MatchString(l.Image) {
			return nil, fmt.Errorf("layers[%d].image must be pinned by digest", i)
		}
		if err := checkSandboxLayerPath(l.From); err != nil {
			return nil, fmt.Errorf("layers[%d].from: %v", i, err)
		}
		if err := checkSandboxLayerPath(l.Mount); err != nil {
			return nil, fmt.Errorf("layers[%d].mount: %v", i, err)
		}
		for _, r := range append(append([]string{}, sandboxLayerReservedPaths...), mounts...) {
			if pathsOverlap(l.Mount, r) {
				return nil, fmt.Errorf("layers[%d].mount %s overlaps %s", i, l.Mount, r)
			}
		}
		mounts = append(mounts, l.Mount)
	}
	return layers, nil
}

// checkSandboxLayerPath accepts a clean absolute path other than "/" with no shell metacharacters.
func checkSandboxLayerPath(p string) error {
	if p == "" || p == "/" || !sandboxLayerPath.MatchString(p) || path.Clean(p) != p {
		return fmt.Errorf("%q must be a clean absolute path other than / using letters, digits, '.', '_', '-', '/'", p)
	}
	return nil
}

// pathsOverlap reports whether a equals b or one contains the other.
func pathsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// ensureSandboxLayers adds one emptyDir, one copy init container after envd-injector,
// and one main-container mount per layer. A bad annotation fails the Workload.
func ensureSandboxLayers(obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) error {
	layers, err := parseSandboxLayers(workload)
	if err != nil {
		return commonerrors.NewBadRequest(err.Error())
	}
	if len(layers) == 0 {
		return nil
	}
	containers, containersPath, err := getContainers(workload, obj, *resourceSpec)
	if err != nil {
		return err
	}
	for _, mount := range containerMountPaths(containers) {
		for i, l := range layers {
			if pathsOverlap(l.Mount, mount) {
				return commonerrors.NewBadRequest(fmt.Sprintf("layers[%d].mount %s overlaps %s", i, l.Mount, mount))
			}
		}
	}
	initPath := podSpecPath(workload, resourceSpec, "initContainers")
	inits, _, _ := jobutils.NestedSlice(obj.Object, initPath)
	var securityContext interface{}
	for _, item := range inits {
		if init, ok := item.(map[string]interface{}); ok && init["name"] == envdInjectorName {
			securityContext = init["securityContext"]
		}
	}
	volumesPath := podSpecPath(workload, resourceSpec, "volumes")
	volumes, _, _ := jobutils.NestedSlice(obj.Object, volumesPath)
	for i, l := range layers {
		name := fmt.Sprintf("%s%d", sandboxLayerVolume, i)
		readOnly := l.ReadOnly == nil || *l.ReadOnly
		if !hasNamedItem(volumes, name) {
			volumes = append(volumes, map[string]interface{}{"name": name, "emptyDir": map[string]interface{}{}})
		}
		if !hasNamedItem(inits, name) {
			init := map[string]interface{}{
				"name":            name,
				"image":           l.Image,
				"imagePullPolicy": "IfNotPresent",
				"command":         []interface{}{"sh", "-c", "set -e; cp -a '" + l.From + "/.' '" + l.Mount + "/'"},
				"volumeMounts":    []interface{}{map[string]interface{}{"name": name, "mountPath": l.Mount}},
			}
			if securityContext != nil {
				init["securityContext"] = securityContext
			}
			inits = append(inits, init)
		}
		for j := range containers {
			container, ok := containers[j].(map[string]interface{})
			if !ok {
				continue
			}
			mounts, _ := container["volumeMounts"].([]interface{})
			if !hasNamedItem(mounts, name) {
				mounts = append(mounts, map[string]interface{}{"name": name, "mountPath": l.Mount, "readOnly": readOnly})
			}
			container["volumeMounts"] = mounts
			containers[j] = container
		}
	}
	if err = jobutils.SetNestedField(obj.Object, volumes, volumesPath); err != nil {
		return err
	}
	if err = jobutils.SetNestedField(obj.Object, inits, initPath); err != nil {
		return err
	}
	return jobutils.SetNestedField(obj.Object, containers, containersPath)
}

// containerMountPaths lists the mount paths already present on the containers.
func containerMountPaths(containers []interface{}) []string {
	var out []string
	for _, item := range containers {
		container, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		mounts, _ := container["volumeMounts"].([]interface{})
		for _, m := range mounts {
			if raw, ok := m.(map[string]interface{}); ok {
				if p, _ := raw["mountPath"].(string); p != "" {
					out = append(out, path.Clean(p))
				}
			}
		}
	}
	return out
}

// requireSandboxSigningKey makes envd-injector fail when the creator expects a
// signing key and no key file is mounted.
func requireSandboxSigningKey(obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) error {
	if workload.Spec.Env[signingKeyEnv] == "" {
		return nil
	}
	initPath := podSpecPath(workload, resourceSpec, "initContainers")
	inits, found, err := jobutils.NestedSlice(obj.Object, initPath)
	if err != nil || !found {
		return err
	}
	for i := range inits {
		init, ok := inits[i].(map[string]interface{})
		if !ok || init["name"] != envdInjectorName {
			continue
		}
		setEnvValue(init, signingKeyRequiredEnv, "1")
		init["terminationMessagePolicy"] = "FallbackToLogsOnError"
		inits[i] = init
	}
	return jobutils.SetNestedField(obj.Object, inits, initPath)
}
