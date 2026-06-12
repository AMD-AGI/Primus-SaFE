/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"encoding/json"
	"testing"

	"gotest.tools/assert"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// newScheme builds a runtime scheme with the project and client-go types.
func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	assert.NilError(t, clientscheme.AddToScheme(s))
	assert.NilError(t, v1.AddToScheme(s))
	return s
}

// newDecoder builds an admission decoder for tests.
func newDecoder(t *testing.T) admission.Decoder {
	t.Helper()
	return admission.NewDecoder(newScheme(t))
}

// newRequest builds an admission request for the given operation and objects.
func newRequest(t *testing.T, op admissionv1.Operation, obj, oldObj interface{}) admission.Request {
	t.Helper()
	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: op,
		},
	}
	if obj != nil {
		raw, err := json.Marshal(obj)
		assert.NilError(t, err)
		req.Object = runtime.RawExtension{Raw: raw}
	}
	if oldObj != nil {
		raw, err := json.Marshal(oldObj)
		assert.NilError(t, err)
		req.OldObject = runtime.RawExtension{Raw: raw}
	}
	return req
}
