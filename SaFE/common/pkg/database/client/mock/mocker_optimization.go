// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.
//
// Manual gomock extensions for OptimizationTaskInterface.
// These methods are appended here (instead of inside the generated mocker.go)
// so that regenerating the base mock does not need to preserve them. Keep the
// signatures in sync with common/pkg/database/client/interface.go.

package mock_client

import (
	context "context"
	reflect "reflect"

	client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	gomock "github.com/golang/mock/gomock"
)

// ── MockInterface (aggregate) ────────────────────────────────────────────

// UpsertOptimizationTask mocks base method.
func (m *MockInterface) UpsertOptimizationTask(ctx context.Context, task *client.OptimizationTask) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpsertOptimizationTask", ctx, task)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpsertOptimizationTask indicates an expected call of UpsertOptimizationTask.
func (mr *MockInterfaceMockRecorder) UpsertOptimizationTask(ctx, task interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpsertOptimizationTask",
		reflect.TypeOf((*MockInterface)(nil).UpsertOptimizationTask), ctx, task)
}

// GetOptimizationTask mocks base method.
func (m *MockInterface) GetOptimizationTask(ctx context.Context, id string) (*client.OptimizationTask, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetOptimizationTask", ctx, id)
	ret0, _ := ret[0].(*client.OptimizationTask)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetOptimizationTask indicates an expected call of GetOptimizationTask.
func (mr *MockInterfaceMockRecorder) GetOptimizationTask(ctx, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetOptimizationTask",
		reflect.TypeOf((*MockInterface)(nil).GetOptimizationTask), ctx, id)
}

// ListOptimizationTasks mocks base method.
func (m *MockInterface) ListOptimizationTasks(ctx context.Context, filter client.OptimizationTaskFilter) ([]*client.OptimizationTask, int64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListOptimizationTasks", ctx, filter)
	ret0, _ := ret[0].([]*client.OptimizationTask)
	ret1, _ := ret[1].(int64)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// ListOptimizationTasks indicates an expected call of ListOptimizationTasks.
func (mr *MockInterfaceMockRecorder) ListOptimizationTasks(ctx, filter interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListOptimizationTasks",
		reflect.TypeOf((*MockInterface)(nil).ListOptimizationTasks), ctx, filter)
}

// UpdateOptimizationTaskStatus mocks base method.
func (m *MockInterface) UpdateOptimizationTaskStatus(
	ctx context.Context, id string, status client.OptimizationTaskStatus, currentPhase int, message string,
) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateOptimizationTaskStatus", ctx, id, status, currentPhase, message)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateOptimizationTaskStatus indicates an expected call of UpdateOptimizationTaskStatus.
func (mr *MockInterfaceMockRecorder) UpdateOptimizationTaskStatus(ctx, id, status, currentPhase, message interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateOptimizationTaskStatus",
		reflect.TypeOf((*MockInterface)(nil).UpdateOptimizationTaskStatus),
		ctx, id, status, currentPhase, message)
}

// UpdateOptimizationTaskClawSession mocks base method.
func (m *MockInterface) UpdateOptimizationTaskClawSession(ctx context.Context, id, sessionID string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateOptimizationTaskClawSession", ctx, id, sessionID)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateOptimizationTaskClawSession indicates an expected call of UpdateOptimizationTaskClawSession.
func (mr *MockInterfaceMockRecorder) UpdateOptimizationTaskClawSession(ctx, id, sessionID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateOptimizationTaskClawSession",
		reflect.TypeOf((*MockInterface)(nil).UpdateOptimizationTaskClawSession), ctx, id, sessionID)
}

// UpdateOptimizationTaskResult mocks base method.
func (m *MockInterface) UpdateOptimizationTaskResult(ctx context.Context, id, finalMetrics, reportPath string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateOptimizationTaskResult", ctx, id, finalMetrics, reportPath)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateOptimizationTaskResult indicates an expected call of UpdateOptimizationTaskResult.
func (mr *MockInterfaceMockRecorder) UpdateOptimizationTaskResult(ctx, id, finalMetrics, reportPath interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateOptimizationTaskResult",
		reflect.TypeOf((*MockInterface)(nil).UpdateOptimizationTaskResult),
		ctx, id, finalMetrics, reportPath)
}

// DeleteOptimizationTask mocks base method.
func (m *MockInterface) DeleteOptimizationTask(ctx context.Context, id string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteOptimizationTask", ctx, id)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteOptimizationTask indicates an expected call of DeleteOptimizationTask.
func (mr *MockInterfaceMockRecorder) DeleteOptimizationTask(ctx, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteOptimizationTask",
		reflect.TypeOf((*MockInterface)(nil).DeleteOptimizationTask), ctx, id)
}

// CountRunningOptimizationTasks mocks base method.
func (m *MockInterface) CountRunningOptimizationTasks(ctx context.Context, workspace string) (int64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CountRunningOptimizationTasks", ctx, workspace)
	ret0, _ := ret[0].(int64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CountRunningOptimizationTasks indicates an expected call of CountRunningOptimizationTasks.
func (mr *MockInterfaceMockRecorder) CountRunningOptimizationTasks(ctx, workspace interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CountRunningOptimizationTasks",
		reflect.TypeOf((*MockInterface)(nil).CountRunningOptimizationTasks), ctx, workspace)
}

// AppendOptimizationEvent mocks base method.
func (m *MockInterface) AppendOptimizationEvent(ctx context.Context, event *client.OptimizationEvent) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AppendOptimizationEvent", ctx, event)
	ret0, _ := ret[0].(error)
	return ret0
}

// AppendOptimizationEvent indicates an expected call of AppendOptimizationEvent.
func (mr *MockInterfaceMockRecorder) AppendOptimizationEvent(ctx, event interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AppendOptimizationEvent",
		reflect.TypeOf((*MockInterface)(nil).AppendOptimizationEvent), ctx, event)
}

// ListOptimizationEvents mocks base method.
func (m *MockInterface) ListOptimizationEvents(ctx context.Context, taskID string, afterSeq int64, limit int) ([]*client.OptimizationEvent, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListOptimizationEvents", ctx, taskID, afterSeq, limit)
	ret0, _ := ret[0].([]*client.OptimizationEvent)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListOptimizationEvents indicates an expected call of ListOptimizationEvents.
func (mr *MockInterfaceMockRecorder) ListOptimizationEvents(ctx, taskID, afterSeq, limit interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListOptimizationEvents",
		reflect.TypeOf((*MockInterface)(nil).ListOptimizationEvents), ctx, taskID, afterSeq, limit)
}

// LatestOptimizationEventSeq mocks base method.
func (m *MockInterface) LatestOptimizationEventSeq(ctx context.Context, taskID string) (int64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "LatestOptimizationEventSeq", ctx, taskID)
	ret0, _ := ret[0].(int64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// LatestOptimizationEventSeq indicates an expected call of LatestOptimizationEventSeq.
func (mr *MockInterfaceMockRecorder) LatestOptimizationEventSeq(ctx, taskID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "LatestOptimizationEventSeq",
		reflect.TypeOf((*MockInterface)(nil).LatestOptimizationEventSeq), ctx, taskID)
}
