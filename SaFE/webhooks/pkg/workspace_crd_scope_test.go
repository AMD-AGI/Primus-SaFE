/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"os"
	"testing"

	"gotest.tools/assert"
	apiextensionsinternal "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/listtype"
	apiservervalidation "k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/yaml"
)

// workspaceCRDValidate builds, from the CRD shipped in the chart, the two checks the Kubernetes
// API server applies to a Workspace object: the OpenAPI schema and the list-type (set) rules.
func workspaceCRDValidate(t *testing.T) func(map[string]interface{}) field.ErrorList {
	t.Helper()
	raw, err := os.ReadFile("../../charts/primus-safe/crds/amd.com_workspaces.yaml")
	assert.NilError(t, err)
	crd := &apiextensionsv1.CustomResourceDefinition{}
	assert.NilError(t, yaml.Unmarshal(raw, crd))
	assert.Equal(t, len(crd.Spec.Versions), 1)

	scheme := runtime.NewScheme()
	assert.NilError(t, apiextensionsinternal.AddToScheme(scheme))
	assert.NilError(t, apiextensionsv1.AddToScheme(scheme))
	internal := &apiextensionsinternal.CustomResourceValidation{}
	assert.NilError(t, scheme.Convert(crd.Spec.Versions[0].Schema, internal, nil))
	validator, _, err := apiservervalidation.NewSchemaValidator(internal.OpenAPIV3Schema)
	assert.NilError(t, err)
	structural, err := structuralschema.NewStructural(internal.OpenAPIV3Schema)
	assert.NilError(t, err)
	return func(obj map[string]interface{}) field.ErrorList {
		errs := apiservervalidation.ValidateCustomResource(nil, obj, validator)
		return append(errs, listtype.ValidateListSetsAndMaps(nil, structural, obj)...)
	}
}

func workspaceWithVolumeScopes(scopes ...interface{}) map[string]interface{} {
	volume := map[string]interface{}{"id": int64(1), "type": "pfs", "mountPath": "/mnt/ceph"}
	if scopes != nil {
		volume["scopes"] = scopes
	}
	return map[string]interface{}{
		"apiVersion": "amd.com/v1",
		"kind":       "Workspace",
		"metadata":   map[string]interface{}{"name": "ws1"},
		"spec": map[string]interface{}{
			"cluster": "c1",
			"volumes": []interface{}{volume},
		},
	}
}

// TestWorkspaceCRDVolumeScopes verifies the chart's CRD accepts a volume without scopes and with
// known scopes, and refuses an unknown or repeated scope.
func TestWorkspaceCRDVolumeScopes(t *testing.T) {
	validate := workspaceCRDValidate(t)

	assert.Equal(t, len(validate(workspaceWithVolumeScopes())), 0)
	assert.Equal(t, len(validate(workspaceWithVolumeScopes(
		"Authoring", "CICD", "Train", "Infer", "Ray"))), 0)

	errs := validate(workspaceWithVolumeScopes("Train", "Sandboxes"))
	assert.Assert(t, len(errs) > 0)
	assert.ErrorContains(t, errs.ToAggregate(), "spec.volumes[0].scopes[1]")

	errs = validate(workspaceWithVolumeScopes("Train", "Train"))
	assert.Assert(t, len(errs) > 0)
	assert.ErrorContains(t, errs.ToAggregate(), "Duplicate value")
}
