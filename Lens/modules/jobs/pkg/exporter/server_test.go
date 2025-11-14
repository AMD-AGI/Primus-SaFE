package exporter

import (
	"encoding/json"
	"testing"

	pb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/pb/exporter"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestGetFromEvent(t *testing.T) {
	server := &EventServer{}

	tests := []struct {
		name      string
		event     *pb.ContainerEvent
		target    interface{}
		wantErr   bool
		errMsg    string
		validate  func(t *testing.T, target interface{})
	}{
		{
			name: "valid event with string data",
			event: &pb.ContainerEvent{
				ContainerId: "test-container-123",
				Data: func() *structpb.Struct {
					data := map[string]interface{}{
						"name":   "test-container",
						"status": "running",
					}
					s, _ := structpb.NewStruct(data)
					return s
				}(),
			},
			target:  &map[string]interface{}{},
			wantErr: false,
			validate: func(t *testing.T, target interface{}) {
				result := target.(*map[string]interface{})
				assert.Equal(t, "test-container", (*result)["name"])
				assert.Equal(t, "running", (*result)["status"])
			},
		},
		{
			name: "valid event with nested data",
			event: &pb.ContainerEvent{
				ContainerId: "test-container-456",
				Data: func() *structpb.Struct {
					data := map[string]interface{}{
						"container": map[string]interface{}{
							"id":   "456",
							"name": "nested-container",
						},
						"count": float64(10),
					}
					s, _ := structpb.NewStruct(data)
					return s
				}(),
			},
			target:  &map[string]interface{}{},
			wantErr: false,
			validate: func(t *testing.T, target interface{}) {
				result := target.(*map[string]interface{})
				assert.Contains(t, *result, "container")
				assert.Equal(t, float64(10), (*result)["count"])
			},
		},
		{
			name:    "nil event",
			event:   nil,
			target:  &map[string]interface{}{},
			wantErr: true,
			errMsg:  "event is nil",
		},
		{
			name: "empty container id",
			event: &pb.ContainerEvent{
				ContainerId: "",
				Data: func() *structpb.Struct {
					s, _ := structpb.NewStruct(map[string]interface{}{})
					return s
				}(),
			},
			target:  &map[string]interface{}{},
			wantErr: true,
			errMsg:  "container_id is empty",
		},
		{
			name: "nil data",
			event: &pb.ContainerEvent{
				ContainerId: "test-container-789",
				Data:        nil,
			},
			target:  &map[string]interface{}{},
			wantErr: true,
			errMsg:  "event data is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := server.getFromEvent(tt.event, tt.target)

			if tt.wantErr {
				assert.Error(t, err, "Should return error")
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg, "Error message should contain expected text")
				}
			} else {
				assert.NoError(t, err, "Should not return error")
				if tt.validate != nil {
					tt.validate(t, tt.target)
				}
			}
		})
	}
}

func TestGetFromEventWithDifferentTargetTypes(t *testing.T) {
	server := &EventServer{}

	type customStruct struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Count  int    `json:"count"`
	}

	tests := []struct {
		name     string
		event    *pb.ContainerEvent
		target   interface{}
		expected interface{}
	}{
		{
			name: "unmarshal to struct",
			event: &pb.ContainerEvent{
				ContainerId: "test-123",
				Data: func() *structpb.Struct {
					data := map[string]interface{}{
						"name":   "test",
						"status": "active",
						"count":  float64(5),
					}
					s, _ := structpb.NewStruct(data)
					return s
				}(),
			},
			target: &customStruct{},
			expected: &customStruct{
				Name:   "test",
				Status: "active",
				Count:  5,
			},
		},
		{
			name: "unmarshal to map",
			event: &pb.ContainerEvent{
				ContainerId: "test-456",
				Data: func() *structpb.Struct {
					data := map[string]interface{}{
						"key1": "value1",
						"key2": float64(42),
					}
					s, _ := structpb.NewStruct(data)
					return s
				}(),
			},
			target: &map[string]interface{}{},
			expected: &map[string]interface{}{
				"key1": "value1",
				"key2": float64(42),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := server.getFromEvent(tt.event, tt.target)
			assert.NoError(t, err, "Should not return error")

			// Compare JSON representations for easier comparison
			actualJSON, _ := json.Marshal(tt.target)
			expectedJSON, _ := json.Marshal(tt.expected)
			assert.JSONEq(t, string(expectedJSON), string(actualJSON), "Target should match expected")
		})
	}
}

func TestGetFromEventWithComplexData(t *testing.T) {
	server := &EventServer{}

	event := &pb.ContainerEvent{
		ContainerId: "complex-container",
		Data: func() *structpb.Struct {
			data := map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":      "pod-1",
					"namespace": "default",
					"labels": map[string]interface{}{
						"app":     "web",
						"version": "v1",
					},
				},
				"devices": []interface{}{
					map[string]interface{}{
						"type": "gpu",
						"id":   float64(0),
					},
					map[string]interface{}{
						"type": "gpu",
						"id":   float64(1),
					},
				},
			}
			s, _ := structpb.NewStruct(data)
			return s
		}(),
	}

	target := &map[string]interface{}{}
	err := server.getFromEvent(event, target)

	assert.NoError(t, err, "Should handle complex nested data")
	assert.Contains(t, *target, "metadata", "Should contain metadata")
	assert.Contains(t, *target, "devices", "Should contain devices")

	metadata := (*target)["metadata"].(map[string]interface{})
	assert.Equal(t, "pod-1", metadata["name"])
	assert.Equal(t, "default", metadata["namespace"])

	devices := (*target)["devices"].([]interface{})
	assert.Len(t, devices, 2, "Should have 2 devices")
}

func TestGetFromEventDataTypes(t *testing.T) {
	server := &EventServer{}

	type dataStruct struct {
		StringField  string  `json:"string_field"`
		IntField     int     `json:"int_field"`
		FloatField   float64 `json:"float_field"`
		BoolField    bool    `json:"bool_field"`
		NullableStr  *string `json:"nullable_str,omitempty"`
	}

	event := &pb.ContainerEvent{
		ContainerId: "type-test",
		Data: func() *structpb.Struct {
			data := map[string]interface{}{
				"string_field": "test string",
				"int_field":    float64(42),
				"float_field":  3.14,
				"bool_field":   true,
			}
			s, _ := structpb.NewStruct(data)
			return s
		}(),
	}

	target := &dataStruct{}
	err := server.getFromEvent(event, target)

	assert.NoError(t, err, "Should handle different data types")
	assert.Equal(t, "test string", target.StringField)
	assert.Equal(t, 42, target.IntField)
	assert.Equal(t, 3.14, target.FloatField)
	assert.True(t, target.BoolField)
}

func TestGetFromEventEmptyData(t *testing.T) {
	server := &EventServer{}

	event := &pb.ContainerEvent{
		ContainerId: "empty-data",
		Data: func() *structpb.Struct {
			s, _ := structpb.NewStruct(map[string]interface{}{})
			return s
		}(),
	}

	target := &map[string]interface{}{}
	err := server.getFromEvent(event, target)

	assert.NoError(t, err, "Should handle empty data")
	assert.Empty(t, *target, "Target should be empty")
}

func TestGetFromEventSpecialCharacters(t *testing.T) {
	server := &EventServer{}

	event := &pb.ContainerEvent{
		ContainerId: "special-chars-test",
		Data: func() *structpb.Struct {
			data := map[string]interface{}{
				"name":        "test/container:latest",
				"namespace":   "kube-system",
				"annotation":  "key=value,key2=value2",
				"unicode":     "测试",
			}
			s, _ := structpb.NewStruct(data)
			return s
		}(),
	}

	target := &map[string]interface{}{}
	err := server.getFromEvent(event, target)

	assert.NoError(t, err, "Should handle special characters")
	assert.Equal(t, "test/container:latest", (*target)["name"])
	assert.Equal(t, "kube-system", (*target)["namespace"])
	assert.Equal(t, "测试", (*target)["unicode"])
}

