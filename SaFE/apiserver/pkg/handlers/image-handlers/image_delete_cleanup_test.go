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
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

// imageCleanupHandler builds an ImageHandler whose fake k8s client is seeded
// with an admin user + wildcard role (so authorization passes) plus any extra
// objects (e.g. the import Job under test).
func imageCleanupHandler(t *testing.T, mockDB *mock_client.MockInterface, objs ...ctrlclient.Object) (*ImageHandler, ctrlclient.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("add amd scheme: %v", err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add batch scheme: %v", err)
	}
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
	all := append([]ctrlclient.Object{admin, role}, objs...)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(all...).Build()
	h := &ImageHandler{
		Client:           cl,
		dbClient:         mockDB,
		accessController: &authority.AccessController{Client: cl},
	}
	return h, cl
}

// TestDeleteImagePassesCurrentUser verifies S9: the delete records the current
// request user, not the stale DeletedBy value from the fetched row.
func TestDeleteImagePassesCurrentUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(7)).Return(&model.Image{ID: 7, DeletedBy: "olduser"}, nil)
	// Must be called with the current user "u1", not the stale "olduser".
	m.EXPECT().DeleteImage(gomock.Any(), int32(7), "u1").Return(nil)
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(7)).Return(nil, nil)

	h, _ := imageCleanupHandler(t, m)
	c := ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "id", Value: "7"}})
	_, err := h.deleteImage(c)
	assert.NoError(t, err)
}

// TestDeleteImageCleansUpImportJob verifies P3: deleting an imported image also
// deletes its import Kubernetes Job (best-effort). Tag is empty here so the
// Harbor artifact deletion is skipped and the test focuses on Job cleanup.
func TestDeleteImageCleansUpImportJob(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(7)).Return(&model.Image{ID: 7}, nil)
	m.EXPECT().DeleteImage(gomock.Any(), int32(7), "u1").Return(nil)
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(7)).
		Return(&model.ImageImportJob{ID: 9, ImageID: 7, JobName: "imptimg-7-abc"}, nil)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "imptimg-7-abc", Namespace: common.PrimusSafeNamespace},
	}
	h, cl := imageCleanupHandler(t, m, job)

	c := ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "id", Value: "7"}})
	if _, err := h.deleteImage(c); err != nil {
		t.Fatalf("deleteImage returned error: %v", err)
	}

	got := &batchv1.Job{}
	err := cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "imptimg-7-abc", Namespace: common.PrimusSafeNamespace}, got)
	if err == nil {
		t.Fatal("expected import job to be deleted, but it still exists")
	}
}
