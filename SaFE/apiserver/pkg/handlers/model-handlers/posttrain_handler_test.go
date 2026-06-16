/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// fakePosttrainStore embeds the generated mock (to satisfy dbclient.Interface)
// and adds the posttrainStore methods so the handler type assertion succeeds.
type fakePosttrainStore struct {
	*mock_client.MockInterface
	views     []*dbclient.PosttrainRunView
	total     int
	getView   *dbclient.PosttrainRunView
	getErr    error
	deleteErr error
	upsertErr error
}

func (f *fakePosttrainStore) UpsertPosttrainRun(ctx context.Context, run *dbclient.PosttrainRun) error {
	return f.upsertErr
}

func (f *fakePosttrainStore) ListPosttrainRunViews(ctx context.Context, filter *dbclient.PosttrainRunFilter) ([]*dbclient.PosttrainRunView, int, error) {
	return f.views, f.total, nil
}

func (f *fakePosttrainStore) GetPosttrainRunView(ctx context.Context, runID string) (*dbclient.PosttrainRunView, error) {
	return f.getView, f.getErr
}

func (f *fakePosttrainStore) SetPosttrainRunDeleted(ctx context.Context, runID string) error {
	return f.deleteErr
}

func newPosttrainHandler(t *testing.T, store *fakePosttrainStore) *Handler {
	t.Helper()
	store.MockInterface = mock_client.NewMockInterface(gomock.NewController(t))
	return &Handler{
		dbClient:  store,
		k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build(),
	}
}

// TestListPosttrainRunsSuccess verifies runs are listed via the store.
func TestListPosttrainRunsSuccess(t *testing.T) {
	store := &fakePosttrainStore{
		views: []*dbclient.PosttrainRunView{
			{RunID: "r1", WorkloadID: "w1", DisplayName: "run1", TrainType: "sft", Strategy: "full"},
		},
		total: 1,
	}
	h := newPosttrainHandler(t, store)

	res, err := h.listPosttrainRuns(sessCtx(t, http.MethodGet, "", "", nil))
	require.NoError(t, err)
	resp, ok := res.(*ListPosttrainRunResponse)
	require.True(t, ok)
	assert.Equal(t, 1, resp.Total)
	assert.Len(t, resp.Items, 1)
}

// TestListPosttrainRunsNoStore verifies an error when the db client is not a posttrain store.
func TestListPosttrainRunsNoStore(t *testing.T) {
	h := &Handler{
		dbClient:  mock_client.NewMockInterface(gomock.NewController(t)),
		k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build(),
	}
	_, err := h.listPosttrainRuns(sessCtx(t, http.MethodGet, "", "", nil))
	assert.Error(t, err)
}

// TestGetPosttrainRunSuccess verifies a single run is fetched and rendered.
func TestGetPosttrainRunSuccess(t *testing.T) {
	store := &fakePosttrainStore{
		getView: &dbclient.PosttrainRunView{RunID: "r1", WorkloadID: "", DisplayName: "run1", TrainType: "sft", Strategy: "full"},
	}
	h := newPosttrainHandler(t, store)

	res, err := h.getPosttrainRun(sessCtx(t, http.MethodGet, "", "", gin.Params{{Key: "id", Value: "r1"}}))
	require.NoError(t, err)
	resp, ok := res.(*PosttrainRunDetailResponse)
	require.True(t, ok)
	assert.Equal(t, "r1", resp.RunID)
}

// TestGetPosttrainRunMissingID verifies an empty id is rejected.
func TestGetPosttrainRunMissingID(t *testing.T) {
	store := &fakePosttrainStore{}
	h := newPosttrainHandler(t, store)
	_, err := h.getPosttrainRun(sessCtx(t, http.MethodGet, "", "", nil))
	assert.Error(t, err)
}

// TestGetPosttrainRunNotFound verifies a store error is propagated.
func TestGetPosttrainRunNotFound(t *testing.T) {
	store := &fakePosttrainStore{getErr: errors.New("not found")}
	h := newPosttrainHandler(t, store)
	_, err := h.getPosttrainRun(sessCtx(t, http.MethodGet, "", "", gin.Params{{Key: "id", Value: "missing"}}))
	assert.Error(t, err)
}

// TestDeletePosttrainRunSuccess verifies a run is marked deleted.
func TestDeletePosttrainRunSuccess(t *testing.T) {
	store := &fakePosttrainStore{}
	h := newPosttrainHandler(t, store)

	res, err := h.deletePosttrainRun(sessCtx(t, http.MethodDelete, "", "", gin.Params{{Key: "id", Value: "r1"}}))
	require.NoError(t, err)
	assert.NotNil(t, res)
}

// TestDeletePosttrainRunMissingID verifies an empty id is rejected.
func TestDeletePosttrainRunMissingID(t *testing.T) {
	store := &fakePosttrainStore{}
	h := newPosttrainHandler(t, store)
	_, err := h.deletePosttrainRun(sessCtx(t, http.MethodDelete, "", "", nil))
	assert.Error(t, err)
}

// TestDeletePosttrainRunError verifies a store delete error is propagated.
func TestDeletePosttrainRunError(t *testing.T) {
	store := &fakePosttrainStore{deleteErr: errors.New("db error")}
	h := newPosttrainHandler(t, store)
	_, err := h.deletePosttrainRun(sessCtx(t, http.MethodDelete, "", "", gin.Params{{Key: "id", Value: "r1"}}))
	assert.Error(t, err)
}

// TestPosttrainPublicWrappers exercises the thin gin wrapper methods end-to-end.
func TestPosttrainPublicWrappers(t *testing.T) {
	store := &fakePosttrainStore{
		views:   []*dbclient.PosttrainRunView{{RunID: "r1", DisplayName: "run1", TrainType: "sft", Strategy: "full"}},
		total:   1,
		getView: &dbclient.PosttrainRunView{RunID: "r1", DisplayName: "run1", TrainType: "sft", Strategy: "full"},
	}
	h := newPosttrainHandler(t, store)

	h.ListPosttrainRuns(sessCtx(t, http.MethodGet, "", "", nil))
	h.GetPosttrainRun(sessCtx(t, http.MethodGet, "", "", gin.Params{{Key: "id", Value: "r1"}}))
	h.DeletePosttrainRun(sessCtx(t, http.MethodDelete, "", "", gin.Params{{Key: "id", Value: "r1"}}))
	h.GetPosttrainRunMetrics(sessCtx(t, http.MethodGet, "", "", gin.Params{{Key: "id", Value: "r1"}}))
}
