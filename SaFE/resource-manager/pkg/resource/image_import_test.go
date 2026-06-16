/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	dbmodel "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func newImageImportReconciler(t *testing.T, db *mockclient.MockInterface, objs ...*batchv1.Job) *ImageImportJobReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	_ = batchv1.AddToScheme(scheme)
	builder := ctrlfake.NewClientBuilder().WithScheme(scheme)
	for _, o := range objs {
		builder = builder.WithObjects(o)
	}
	return &ImageImportJobReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: builder.Build()},
		dbClient:              db,
	}
}

func TestImageImportReconcileNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	r := newImageImportReconciler(t, db)
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestImageImportReconcileNoImportRecord(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().GetImageImportJobByJobName(gomock.Any(), "j1").Return(nil, nil)

	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := newImageImportReconciler(t, db, job)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestImageImportReconcileSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	importJob := &dbmodel.ImageImportJob{JobName: "j1", DstName: "img:1"}
	image := &dbmodel.Image{Tag: "img:1"}
	db.EXPECT().GetImageImportJobByJobName(gomock.Any(), "j1").Return(importJob, nil)
	db.EXPECT().GetImageByTag(gomock.Any(), "img:1").Return(image, nil)
	db.EXPECT().UpsertImage(gomock.Any(), gomock.Any()).Return(nil)
	db.EXPECT().UpsertImageImportJob(gomock.Any(), gomock.Any()).Return(nil)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Status:     batchv1.JobStatus{Succeeded: 1},
	}
	r := newImageImportReconciler(t, db, job)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}
