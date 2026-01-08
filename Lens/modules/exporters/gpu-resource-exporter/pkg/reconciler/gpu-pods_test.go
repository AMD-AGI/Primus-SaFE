// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package reconciler

import (
	"context"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetConditionFromSnapshot(t *testing.T) {
	tests := []struct {
		name     string
		snapshot *model.PodSnapshot
		expected []corev1.PodCondition
	}{
		{
			name:     "nil snapshot",
			snapshot: nil,
			expected: nil,
		},
		{
			name: "snapshot with empty Status",
			snapshot: &model.PodSnapshot{
				PodUID:    "test-uid",
				PodName:   "test-pod",
				Namespace: "default",
				Status:    model.ExtType{},
			},
			expected: nil,
		},
		{
			name: "snapshot with single Condition",
			snapshot: &model.PodSnapshot{
				PodUID:    "test-uid",
				PodName:   "test-pod",
				Namespace: "default",
				Status: model.ExtType{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   string(corev1.PodReady),
							"status": string(corev1.ConditionTrue),
							"reason": "ContainersReady",
						},
					},
				},
			},
			expected: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
					Reason: "ContainersReady",
				},
			},
		},
		{
			name: "snapshot with multiple Conditions",
			snapshot: &model.PodSnapshot{
				PodUID:    "test-uid",
				PodName:   "test-pod",
				Namespace: "default",
				Status: model.ExtType{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   string(corev1.PodScheduled),
							"status": string(corev1.ConditionTrue),
							"reason": "PodScheduled",
						},
						map[string]interface{}{
							"type":   string(corev1.PodReady),
							"status": string(corev1.ConditionTrue),
							"reason": "ContainersReady",
						},
						map[string]interface{}{
							"type":   string(corev1.ContainersReady),
							"status": string(corev1.ConditionTrue),
						},
					},
				},
			},
			expected: []corev1.PodCondition{
				{
					Type:   corev1.PodScheduled,
					Status: corev1.ConditionTrue,
					Reason: "PodScheduled",
				},
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
					Reason: "ContainersReady",
				},
				{
					Type:   corev1.ContainersReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
		{
			name: "Conditions is nil",
			snapshot: &model.PodSnapshot{
				PodUID:    "test-uid",
				PodName:   "test-pod",
				Namespace: "default",
				Status: model.ExtType{
					"conditions": nil,
				},
			},
			expected: nil,
		},
		{
			name: "Conditions is empty array",
			snapshot: &model.PodSnapshot{
				PodUID:    "test-uid",
				PodName:   "test-pod",
				Namespace: "default",
				Status: model.ExtType{
					"conditions": []interface{}{},
				},
			},
			expected: []corev1.PodCondition{},
		},
		{
			name: "Condition with False status",
			snapshot: &model.PodSnapshot{
				PodUID:    "test-uid",
				PodName:   "test-pod",
				Namespace: "default",
				Status: model.ExtType{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":    string(corev1.PodReady),
							"status":  string(corev1.ConditionFalse),
							"reason":  "ContainersNotReady",
							"message": "containers with unready status: [app]",
						},
					},
				},
			},
			expected: []corev1.PodCondition{
				{
					Type:    corev1.PodReady,
					Status:  corev1.ConditionFalse,
					Reason:  "ContainersNotReady",
					Message: "containers with unready status: [app]",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getConditionFromSnapshot(tt.snapshot)
			
			if tt.expected == nil {
				assert.Nil(t, result, "Expected nil result")
			} else {
				assert.Equal(t, len(tt.expected), len(result), "Conditions count mismatch")
				
				for i, expectedCond := range tt.expected {
					if i < len(result) {
						assert.Equal(t, expectedCond.Type, result[i].Type, "Condition type mismatch at index %d", i)
						assert.Equal(t, expectedCond.Status, result[i].Status, "Condition status mismatch at index %d", i)
						assert.Equal(t, expectedCond.Reason, result[i].Reason, "Condition reason mismatch at index %d", i)
					}
				}
			}
		})
	}
}

func TestCompareSnapshotAndGetNewEvent(t *testing.T) {
	g := &GpuPodsReconciler{}
	ctx := context.Background()

	tests := []struct {
		name            string
		pod             *corev1.Pod
		formerSnapshot  *model.PodSnapshot
		currentSnapshot *model.PodSnapshot
		expectedCount   int
		validate        func(t *testing.T, events []*model.GpuPodsEvent)
	}{
		{
			name: "no former snapshot - all True conditions are new events",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "pod-uid-1",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{RestartCount: 0},
					},
				},
			},
			formerSnapshot: nil,
			currentSnapshot: &model.PodSnapshot{
				Status: model.ExtType{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   string(corev1.PodScheduled),
							"status": string(corev1.ConditionTrue),
						},
						map[string]interface{}{
							"type":   string(corev1.PodReady),
							"status": string(corev1.ConditionTrue),
						},
					},
				},
			},
			expectedCount: 2,
			validate: func(t *testing.T, events []*model.GpuPodsEvent) {
				assert.Len(t, events, 2)
				assert.Equal(t, "pod-uid-1", events[0].PodUUID)
				assert.Equal(t, string(corev1.PodRunning), events[0].PodPhase)
				assert.Equal(t, int32(0), events[0].RestartCount)
			},
		},
		{
			name: "add one condition",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "pod-uid-2",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{RestartCount: 1},
					},
				},
			},
			formerSnapshot: &model.PodSnapshot{
				Status: model.ExtType{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   string(corev1.PodScheduled),
							"status": string(corev1.ConditionTrue),
						},
					},
				},
			},
			currentSnapshot: &model.PodSnapshot{
				Status: model.ExtType{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   string(corev1.PodScheduled),
							"status": string(corev1.ConditionTrue),
						},
						map[string]interface{}{
							"type":   string(corev1.PodReady),
							"status": string(corev1.ConditionTrue),
						},
					},
				},
			},
			expectedCount: 1,
			validate: func(t *testing.T, events []*model.GpuPodsEvent) {
				assert.Len(t, events, 1)
				assert.Equal(t, string(corev1.PodReady), events[0].EventType)
				assert.Equal(t, int32(1), events[0].RestartCount)
			},
		},
		{
			name: "no new conditions - all conditions already exist",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "pod-uid-3",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			formerSnapshot: &model.PodSnapshot{
				Status: model.ExtType{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   string(corev1.PodReady),
							"status": string(corev1.ConditionTrue),
							"reason": "ContainersReady",
						},
					},
				},
			},
			currentSnapshot: &model.PodSnapshot{
				Status: model.ExtType{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   string(corev1.PodReady),
							"status": string(corev1.ConditionTrue),
							"reason": "ContainersReady",
						},
					},
				},
			},
			expectedCount: 0,
			validate: func(t *testing.T, events []*model.GpuPodsEvent) {
				assert.Empty(t, events)
			},
		},
		{
			name: "ignore False status conditions",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "pod-uid-4",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					ContainerStatuses: []corev1.ContainerStatus{
						{RestartCount: 0},
					},
				},
			},
			formerSnapshot: nil,
			currentSnapshot: &model.PodSnapshot{
				Status: model.ExtType{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   string(corev1.PodScheduled),
							"status": string(corev1.ConditionTrue),
						},
						map[string]interface{}{
							"type":   string(corev1.PodReady),
							"status": string(corev1.ConditionFalse),
							"reason": "ContainersNotReady",
						},
					},
				},
			},
			expectedCount: 1, // only Scheduled (True) will be recorded
			validate: func(t *testing.T, events []*model.GpuPodsEvent) {
				assert.Len(t, events, 1)
				assert.Equal(t, string(corev1.PodScheduled), events[0].EventType)
			},
		},
		{
			name: "multiple new conditions",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "pod-uid-5",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{RestartCount: 2},
					},
				},
			},
			formerSnapshot: &model.PodSnapshot{
				Status: model.ExtType{
					"conditions": []interface{}{},
				},
			},
			currentSnapshot: &model.PodSnapshot{
				Status: model.ExtType{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   string(corev1.PodScheduled),
							"status": string(corev1.ConditionTrue),
						},
						map[string]interface{}{
							"type":   string(corev1.ContainersReady),
							"status": string(corev1.ConditionTrue),
						},
						map[string]interface{}{
							"type":   string(corev1.PodReady),
							"status": string(corev1.ConditionTrue),
						},
					},
				},
			},
			expectedCount: 3,
			validate: func(t *testing.T, events []*model.GpuPodsEvent) {
				assert.Len(t, events, 3)
				eventTypes := []string{}
				for _, e := range events {
					eventTypes = append(eventTypes, e.EventType)
					assert.Equal(t, int32(2), e.RestartCount)
				}
				assert.Contains(t, eventTypes, string(corev1.PodScheduled))
				assert.Contains(t, eventTypes, string(corev1.ContainersReady))
				assert.Contains(t, eventTypes, string(corev1.PodReady))
			},
		},
		{
			name: "Pod without ContainerStatus",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "pod-uid-6",
				},
				Status: corev1.PodStatus{
					Phase:             corev1.PodPending,
					ContainerStatuses: []corev1.ContainerStatus{}, // empty array
				},
			},
			formerSnapshot: nil,
			currentSnapshot: &model.PodSnapshot{
				Status: model.ExtType{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   string(corev1.PodScheduled),
							"status": string(corev1.ConditionTrue),
						},
					},
				},
			},
			expectedCount: 1,
			validate: func(t *testing.T, events []*model.GpuPodsEvent) {
				assert.Len(t, events, 1)
				assert.Equal(t, int32(0), events[0].RestartCount, "RestartCount should be 0 when no container status")
			},
		},
		{
			name: "condition status changes from False to True",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "pod-uid-7",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{RestartCount: 3},
					},
				},
			},
			formerSnapshot: &model.PodSnapshot{
				Status: model.ExtType{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   string(corev1.PodReady),
							"status": string(corev1.ConditionFalse),
							"reason": "ContainersNotReady",
						},
					},
				},
			},
			currentSnapshot: &model.PodSnapshot{
				Status: model.ExtType{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   string(corev1.PodReady),
							"status": string(corev1.ConditionTrue),
							"reason": "ContainersReady",
						},
					},
				},
			},
			expectedCount: 1, // False -> True should be considered as new event
			validate: func(t *testing.T, events []*model.GpuPodsEvent) {
				assert.Len(t, events, 1)
				assert.Equal(t, string(corev1.PodReady), events[0].EventType)
				assert.Equal(t, int32(3), events[0].RestartCount)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := g.compareSnapshotAndGetNewEvent(ctx, tt.pod, tt.formerSnapshot, tt.currentSnapshot)
			
			assert.NoError(t, err, "Should not return error")
			assert.Equal(t, tt.expectedCount, len(events), "Event count mismatch")
			
			if tt.validate != nil {
				tt.validate(t, events)
			}
		})
	}
}

func TestCompareSnapshotAndGetNewEvent_EdgeCases(t *testing.T) {
	g := &GpuPodsReconciler{}
	ctx := context.Background()

	t.Run("both snapshots are nil", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				UID: "test-uid",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}
		
		events, err := g.compareSnapshotAndGetNewEvent(ctx, pod, nil, nil)
		assert.NoError(t, err)
		assert.Empty(t, events)
	})

	t.Run("current snapshot is empty", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				UID: "test-uid",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}
		
		formerSnapshot := &model.PodSnapshot{
			Status: model.ExtType{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   string(corev1.PodReady),
						"status": string(corev1.ConditionTrue),
					},
				},
			},
		}
		
		events, err := g.compareSnapshotAndGetNewEvent(ctx, pod, formerSnapshot, &model.PodSnapshot{})
		assert.NoError(t, err)
		assert.Empty(t, events)
	})
}

