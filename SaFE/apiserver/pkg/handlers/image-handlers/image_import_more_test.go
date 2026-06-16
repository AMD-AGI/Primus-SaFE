/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// importJobHandler builds a handler with admin authorization, a fake ctrl client
// (v1+corev1+batchv1) and a fake clientSet, plus the supplied db mock.
func importJobHandler(t *testing.T, m *mock_client.MockInterface) *ImageHandler {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	admin := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "u1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{"admin"}},
	}
	role := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "admin"},
		Rules: []v1.PolicyRule{{
			Resources:    []string{"*"},
			GrantedUsers: []string{"*"},
			Verbs:        []v1.RoleVerb{"*"},
		}},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(admin, role).Build()
	return &ImageHandler{
		Client:           cl,
		dbClient:         m,
		clientSet:        k8sfake.NewSimpleClientset(),
		accessController: &authority.AccessController{Client: cl},
	}
}

// TestNewImportImageJobPlatformSpecific verifies a platform-specific job spec is built.
func TestNewImportImageJobPlatformSpecific(t *testing.T) {
	job, err := newImportImageJob(1, "job-1", "syncer:latest", []string{"ps1"}, &ImportImageEnv{
		SourceImageName: "docker.io/library/alpine:latest",
		DestImageName:   "harbor.io/p/alpine:latest",
		OsArch:          "linux/amd64",
		Os:              "linux",
		Arch:            "amd64",
	}, "u1", "")
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, "job-1", job.Name)
	require.Len(t, job.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, "syncer:latest", job.Spec.Template.Spec.Containers[0].Image)
}

// TestNewImportImageJobAllPlatforms verifies the all-platform branch and ConfigMap volume.
func TestNewImportImageJobAllPlatforms(t *testing.T) {
	job, err := newImportImageJob(2, "job-2", "syncer:latest", nil, &ImportImageEnv{
		SourceImageName: "src:tag",
		DestImageName:   "dst:tag",
		OsArch:          OsArchAll,
	}, "u1", "auth-cm")
	require.NoError(t, err)
	require.NotNil(t, job)
	// ConfigMap-backed auth volume should be present.
	require.NotEmpty(t, job.Spec.Template.Spec.Volumes)
	assert.NotNil(t, job.Spec.Template.Spec.Volumes[0].ConfigMap)
}

// TestDispatchImportImageJob verifies a k8s Job is created (no user secret path).
func TestDispatchImportImageJob(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	h := importJobHandler(t, m)

	c := ginCtx(t, http.MethodPost, "", nil)
	job, err := h.dispatchImportImageJob(c, &model.Image{ID: 1, CreatedBy: "u1"},
		&model.ImageImportJob{SrcTag: "src:tag", DstName: "dst:tag", Os: "linux", Arch: "amd64"}, nil)
	require.NoError(t, err)
	require.NotNil(t, job)

	created := &batchv1.Job{}
	require.NoError(t, h.Client.Get(context.Background(),
		ctrlclient.ObjectKey{Namespace: job.Namespace, Name: job.Name}, created))
}

// TestRetryDispatchImportImageJobNotFound verifies a missing image yields not-found.
func TestRetryDispatchImportImageJobNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(5)).Return(nil, nil)

	h := importJobHandler(t, m)
	c := ginCtx(t, http.MethodPut, "", gin.Params{{Key: "id", Value: "5"}})
	_, err := h.retryDispatchImportImageJob(c)
	assert.Error(t, err)
}

// TestRetryDispatchImportImageJobSuccess verifies the full retry path dispatches a job.
func TestRetryDispatchImportImageJobSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(5)).Return(&model.Image{ID: 5, CreatedBy: "u1"}, nil)
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(5)).
		Return(&model.ImageImportJob{ID: 9, ImageID: 5, SrcTag: "src:tag", DstName: "dst:tag", Os: "linux", Arch: "amd64"}, nil)
	m.EXPECT().UpsertImage(gomock.Any(), gomock.Any()).Return(nil)
	m.EXPECT().UpsertImageImportJob(gomock.Any(), gomock.Any()).Return(nil)

	h := importJobHandler(t, m)
	c := ginCtx(t, http.MethodPut, "", gin.Params{{Key: "id", Value: "5"}})
	_, err := h.retryDispatchImportImageJob(c)
	assert.NoError(t, err)
}

// TestUpsertImageRegistryInfoUpdate verifies the update branch (existing record) is exercised.
func TestUpsertImageRegistryInfoUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetRegistryInfoById(gomock.Any(), int32(3)).Return(&model.RegistryInfo{ID: 3}, nil)
	m.EXPECT().UpsertRegistryInfo(gomock.Any(), gomock.Any()).Return(nil)

	h := &ImageHandler{dbClient: m}
	// Empty username/password avoids crypto-dependent encryption.
	res, err := h.upsertImageRegistryInfo(context.Background(), &CreateRegistryRequest{
		Id: 3, Name: "r1", Url: "harbor.io",
	})
	require.NoError(t, err)
	assert.Equal(t, int32(3), res.ID)
}

// TestUpsertImageRegistryInfoCreate verifies the create branch (no existing record).
func TestUpsertImageRegistryInfoCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().UpsertRegistryInfo(gomock.Any(), gomock.Any()).Return(nil)

	h := &ImageHandler{dbClient: m}
	res, err := h.upsertImageRegistryInfo(context.Background(), &CreateRegistryRequest{
		Name: "r1", Url: "harbor.io",
	})
	require.NoError(t, err)
	assert.NotNil(t, res)
}
