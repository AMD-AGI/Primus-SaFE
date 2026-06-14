/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// ctrlScheme builds a scheme with the project API types.
func ctrlScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

// cdJob builds a CD-type OpsJob with the given deployment request id and phase.
func cdJob(name, reqId string, phase v1.OpsJobPhase) *v1.OpsJob {
	j := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.OpsJobSpec{
			Type:   v1.OpsJobCDType,
			Inputs: []v1.Parameter{{Name: v1.ParameterDeploymentRequestId, Value: reqId}},
		},
	}
	j.Status.Phase = phase
	return j
}

// reconcileReq builds a reconcile request for the named object.
func reconcileReq(name string) ctrlruntime.Request {
	return ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: name}}
}

func newCDReconciler(t *testing.T, mockDB dbclient.Interface, objs ...client.Object) *CDOpsJobReconciler {
	cl := fake.NewClientBuilder().WithScheme(ctrlScheme(t)).WithObjects(objs...).Build()
	return &CDOpsJobReconciler{Client: cl, dbClient: mockDB}
}

func TestCDReconcileJobNotFound(t *testing.T) {
	r := newCDReconciler(t, nil)
	res, err := r.Reconcile(context.Background(), reconcileReq("missing"))
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestCDReconcileNonCDType(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}, Spec: v1.OpsJobSpec{Type: v1.OpsJobDownloadType}}
	r := newCDReconciler(t, nil, job)
	res, err := r.Reconcile(context.Background(), reconcileReq("j1"))
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestCDReconcileMissingRequestId(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}, Spec: v1.OpsJobSpec{Type: v1.OpsJobCDType}}
	r := newCDReconciler(t, nil, job)
	_, err := r.Reconcile(context.Background(), reconcileReq("j1"))
	assert.NoError(t, err)
}

func TestCDReconcileInvalidRequestId(t *testing.T) {
	r := newCDReconciler(t, nil, cdJob("j1", "not-a-number", v1.OpsJobRunning))
	_, err := r.Reconcile(context.Background(), reconcileReq("j1"))
	assert.NoError(t, err)
}

func TestCDReconcileGetDeploymentRequestError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().GetDeploymentRequest(gomock.Any(), int64(5)).Return(nil, assertErr())

	r := newCDReconciler(t, mockDB, cdJob("j1", "5", v1.OpsJobRunning))
	_, err := r.Reconcile(context.Background(), reconcileReq("j1"))
	assert.NoError(t, err)
}

func TestCDReconcileRunningUpdatesStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().GetDeploymentRequest(gomock.Any(), int64(1)).
		Return(&dbclient.DeploymentRequest{Id: 1, Status: ""}, nil)
	mockDB.EXPECT().UpdateDeploymentRequest(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *dbclient.DeploymentRequest) error {
			assert.Equal(t, StatusDeploying, req.Status)
			return nil
		})

	r := newCDReconciler(t, mockDB, cdJob("j1", "1", v1.OpsJobRunning))
	_, err := r.Reconcile(context.Background(), reconcileReq("j1"))
	assert.NoError(t, err)
}

func TestCDReconcileStatusUnchanged(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().GetDeploymentRequest(gomock.Any(), int64(1)).
		Return(&dbclient.DeploymentRequest{Id: 1, Status: StatusDeploying}, nil)

	r := newCDReconciler(t, mockDB, cdJob("j1", "1", v1.OpsJobRunning))
	_, err := r.Reconcile(context.Background(), reconcileReq("j1"))
	assert.NoError(t, err)
}

func TestCDReconcileFailedUpdatesStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().GetDeploymentRequest(gomock.Any(), int64(1)).
		Return(&dbclient.DeploymentRequest{Id: 1, Status: StatusDeploying}, nil)
	mockDB.EXPECT().UpdateDeploymentRequest(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *dbclient.DeploymentRequest) error {
			assert.Equal(t, StatusFailed, req.Status)
			return nil
		})

	r := newCDReconciler(t, mockDB, cdJob("j1", "1", v1.OpsJobFailed))
	_, err := r.Reconcile(context.Background(), reconcileReq("j1"))
	assert.NoError(t, err)
}

func TestCDReconcileSucceededCreatesSnapshot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().GetDeploymentRequest(gomock.Any(), int64(1)).
		Return(&dbclient.DeploymentRequest{Id: 1, Status: StatusDeploying, DeployType: DeployTypeLens, EnvConfig: "{}"}, nil)
	mockDB.EXPECT().CreateEnvironmentSnapshot(gomock.Any(), gomock.Any()).Return(int64(1), nil)
	mockDB.EXPECT().UpdateDeploymentRequest(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *dbclient.DeploymentRequest) error {
			assert.Equal(t, StatusDeployed, req.Status)
			return nil
		})

	r := newCDReconciler(t, mockDB, cdJob("j1", "1", v1.OpsJobSucceeded))
	_, err := r.Reconcile(context.Background(), reconcileReq("j1"))
	assert.NoError(t, err)
}

func TestCDReconcileUpdateErrorRequeues(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().GetDeploymentRequest(gomock.Any(), int64(1)).
		Return(&dbclient.DeploymentRequest{Id: 1, Status: ""}, nil)
	mockDB.EXPECT().UpdateDeploymentRequest(gomock.Any(), gomock.Any()).Return(assertErr())

	r := newCDReconciler(t, mockDB, cdJob("j1", "1", v1.OpsJobRunning))
	res, err := r.Reconcile(context.Background(), reconcileReq("j1"))
	assert.NoError(t, err)
	assert.Equal(t, time.Second*5, res.RequeueAfter)
}

func TestCDReconcileUnknownPhase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().GetDeploymentRequest(gomock.Any(), int64(1)).
		Return(&dbclient.DeploymentRequest{Id: 1, Status: StatusDeploying}, nil)

	r := newCDReconciler(t, mockDB, cdJob("j1", "1", v1.OpsJobPhase("weird")))
	_, err := r.Reconcile(context.Background(), reconcileReq("j1"))
	assert.NoError(t, err)
}

// assertErr returns a simple error for mock expectations.
func assertErr() error { return &simpleErr{} }

type simpleErr struct{}

func (*simpleErr) Error() string { return "boom" }
