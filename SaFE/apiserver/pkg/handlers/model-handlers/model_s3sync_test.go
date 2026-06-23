/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func s3SyncHandler(t *testing.T) (*Handler, ctrlclient.Client) {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	cl := ctrlfake.NewClientBuilder().WithScheme(s).Build()
	return newMockModelHandler(cl), cl
}

// TestCreateModelFromS3SyncValidation covers the request validation branches.
func TestCreateModelFromS3SyncValidation(t *testing.T) {
	h, _ := s3SyncHandler(t)
	ctx := context.Background()

	cases := []struct {
		name string
		req  *CreateModelRequest
	}{
		{"missing s3 source", &CreateModelRequest{DisplayName: "M"}},
		{"missing display name", &CreateModelRequest{S3Source: &S3SourceReq{URI: "s3://b/p"}}},
		{"not s3 scheme", &CreateModelRequest{DisplayName: "M", S3Source: &S3SourceReq{URI: "http://b/p"}}},
		{"empty bucket", &CreateModelRequest{DisplayName: "M", S3Source: &S3SourceReq{URI: "s3:///p"}}},
		{"unsafe uri", &CreateModelRequest{DisplayName: "M", S3Source: &S3SourceReq{URI: "s3://b/$(x)"}}},
		{"ak without sk", &CreateModelRequest{DisplayName: "M", S3Source: &S3SourceReq{URI: "s3://b/p", AccessKeyID: "ak"}}},
		{"creds without endpoint", &CreateModelRequest{DisplayName: "M", S3Source: &S3SourceReq{URI: "s3://b/p", AccessKeyID: "ak", SecretAccessKey: "sk"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.createModelFromS3Sync(ctx, tc.req, "", "")
			assert.Error(t, err)
		})
	}
}

// TestCreateModelFromS3SyncSuccessNoCreds verifies a pending model is created without credentials.
func TestCreateModelFromS3SyncSuccessNoCreds(t *testing.T) {
	h, cl := s3SyncHandler(t)
	req := &CreateModelRequest{
		DisplayName: "S3 Model",
		S3Source:    &S3SourceReq{URI: "s3://my-bucket/prefix"},
	}
	res, err := h.createModelFromS3Sync(context.Background(), req, "uid", "uname")
	require.NoError(t, err)
	resp := res.(*CreateResponse)

	created := &v1.Model{}
	require.NoError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: resp.ID}, created))
	assert.Equal(t, v1.ModelPhasePending, created.Status.Phase)
	assert.Equal(t, v1.TrueStr, created.Labels[v1.ModelS3ImportLabel])
}

// TestCreateModelFromS3SyncSuccessWithCreds verifies the source secret is created with credentials.
func TestCreateModelFromS3SyncSuccessWithCreds(t *testing.T) {
	h, cl := s3SyncHandler(t)
	req := &CreateModelRequest{
		DisplayName: "S3 Model Creds",
		S3Source: &S3SourceReq{
			URI:             "s3://my-bucket/prefix",
			AccessKeyID:     "ak",
			SecretAccessKey: "sk",
			Endpoint:        "https://s3.us-west-2.amazonaws.com",
			Region:          "us-west-2",
		},
	}
	res, err := h.createModelFromS3Sync(context.Background(), req, "uid", "uname")
	require.NoError(t, err)
	resp := res.(*CreateResponse)

	created := &v1.Model{}
	require.NoError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: resp.ID}, created))
	secretName := created.Annotations[v1.ModelS3SourceSecretAnn]
	assert.NotEmpty(t, secretName)
}

// TestCreateModelFromS3SyncDuplicate verifies an existing model with the same source is rejected.
func TestCreateModelFromS3SyncDuplicate(t *testing.T) {
	h, cl := s3SyncHandler(t)
	existing := &v1.Model{}
	existing.Name = "existing"
	existing.Spec.Source.URL = "s3://my-bucket/prefix"
	existing.Spec.Source.AccessMode = v1.AccessModeLocal
	require.NoError(t, cl.Create(context.Background(), existing))

	req := &CreateModelRequest{
		DisplayName: "Dup",
		S3Source:    &S3SourceReq{URI: "s3://my-bucket/prefix"},
	}
	_, err := h.createModelFromS3Sync(context.Background(), req, "", "")
	assert.Error(t, err)
}
