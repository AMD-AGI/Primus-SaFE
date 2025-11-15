package pods

import (
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/prompb"
	"github.com/stretchr/testify/assert"
)

func TestGetName(t *testing.T) {
	tests := []struct {
		name     string
		labels   []prompb.Label
		expected string
	}{
		{
			name: "find __name__ label",
			labels: []prompb.Label{
				{Name: "__name__", Value: "gpu_utilization"},
				{Name: "pod", Value: "test-pod"},
			},
			expected: "gpu_utilization",
		},
		{
			name: "__name__ label not first",
			labels: []prompb.Label{
				{Name: "pod", Value: "test-pod"},
				{Name: "__name__", Value: "cpu_usage"},
				{Name: "node", Value: "node-1"},
			},
			expected: "cpu_usage",
		},
		{
			name: "no __name__ label",
			labels: []prompb.Label{
				{Name: "pod", Value: "test-pod"},
				{Name: "node", Value: "node-1"},
			},
			expected: "",
		},
		{
			name:     "empty labels",
			labels:   []prompb.Label{},
			expected: "",
		},
		{
			name:     "nil labels",
			labels:   nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getName(tt.labels)
			assert.Equal(t, tt.expected, result, "getName should return correct metric name")
		})
	}
}

func TestGetPodNameValues(t *testing.T) {
	tests := []struct {
		name        string
		labelValues map[string]string
		expected    string
	}{
		{
			name: "pod label exists",
			labelValues: map[string]string{
				"pod":  "test-pod-1",
				"node": "node-1",
			},
			expected: "test-pod-1",
		},
		{
			name: "pod_name label exists",
			labelValues: map[string]string{
				"pod_name": "test-pod-2",
				"node":     "node-1",
			},
			expected: "test-pod-2",
		},
		{
			name: "exported_pod label exists",
			labelValues: map[string]string{
				"exported_pod": "test-pod-3",
				"node":         "node-1",
			},
			expected: "test-pod-3",
		},
		{
			name: "pod label takes priority over pod_name",
			labelValues: map[string]string{
				"pod":      "test-pod-4",
				"pod_name": "other-pod",
				"node":     "node-1",
			},
			expected: "test-pod-4",
		},
		{
			name: "pod_name takes priority over exported_pod",
			labelValues: map[string]string{
				"pod_name":     "test-pod-5",
				"exported_pod": "other-pod",
				"node":         "node-1",
			},
			expected: "test-pod-5",
		},
		{
			name: "no pod labels",
			labelValues: map[string]string{
				"node":      "node-1",
				"container": "main",
			},
			expected: "",
		},
		{
			name:        "empty label values",
			labelValues: map[string]string{},
			expected:    "",
		},
		{
			name:        "nil label values",
			labelValues: nil,
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPodNameValue(tt.labelValues)
			assert.Equal(t, tt.expected, result, "getPodName should return correct pod name")
		})
	}
}

func TestGetLabelByDeviceType(t *testing.T) {
	tests := []struct {
		name       string
		deviceType string
		expected   string
	}{
		{
			name:       "GPU device type",
			deviceType: constant.DeviceTypeGPU,
			expected:   "gpu_id",
		},
		{
			name:       "IB device type",
			deviceType: constant.DeviceTypeIB,
			expected:   "device",
		},
		{
			name:       "RDMA device type",
			deviceType: constant.DeviceTypeRDMA,
			expected:   "device",
		},
		{
			name:       "ASIC device type",
			deviceType: "ASIC",
			expected:   "asic",
		},
		{
			name:       "unknown device type",
			deviceType: "unknown_type",
			expected:   "unknown",
		},
		{
			name:       "empty device type",
			deviceType: "",
			expected:   "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getLabelByDeviceType(tt.deviceType)
			assert.Equal(t, tt.expected, result, "getLabelByDeviceType should return correct label")
		})
	}
}

func TestGetDeviceKey(t *testing.T) {
	tests := []struct {
		name     string
		device   *dbModel.NodeContainerDevices
		expected string
	}{
		{
			name: "GPU device",
			device: &dbModel.NodeContainerDevices{
				DeviceType: constant.DeviceTypeGPU,
				DeviceNo:   3,
				DeviceName: "GPU-3",
			},
			expected: "3",
		},
		{
			name: "IB device",
			device: &dbModel.NodeContainerDevices{
				DeviceType: constant.DeviceTypeIB,
				DeviceNo:   0,
				DeviceName: "mlx5_0",
			},
			expected: "mlx5_0",
		},
		{
			name: "RDMA device",
			device: &dbModel.NodeContainerDevices{
				DeviceType: constant.DeviceTypeRDMA,
				DeviceNo:   1,
				DeviceName: "rdma0",
			},
			expected: "rdma0",
		},
		{
			name: "ASIC device",
			device: &dbModel.NodeContainerDevices{
				DeviceType: "ASIC",
				DeviceNo:   2,
				DeviceName: "asic-dev-2",
			},
			expected: "asic-dev-2",
		},
		{
			name: "unknown device type",
			device: &dbModel.NodeContainerDevices{
				DeviceType: "unknown",
				DeviceNo:   5,
				DeviceName: "unknown-dev",
			},
			expected: "unknown",
		},
		{
			name: "GPU device with zero index",
			device: &dbModel.NodeContainerDevices{
				DeviceType: constant.DeviceTypeGPU,
				DeviceNo:   0,
				DeviceName: "GPU-0",
			},
			expected: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetDeviceKey(tt.device)
			assert.Equal(t, tt.expected, result, "GetDeviceKey should return correct key")
		})
	}
}

func TestGetWorkloadsByPodName(t *testing.T) {
	// Setup test cache
	testCache := map[string][][]string{
		"pod-1": {
			{"workload-1", "uid-1"},
			{"workload-2", "uid-2"},
		},
		"pod-2": {
			{"workload-3", "uid-3"},
		},
	}

	// Save original cache and restore after test
	originalCache := podWorkloadCache
	podWorkloadCache = testCache
	defer func() {
		podWorkloadCache = originalCache
	}()

	tests := []struct {
		name     string
		podName  string
		expected [][]string
	}{
		{
			name:    "pod with multiple workloads",
			podName: "pod-1",
			expected: [][]string{
				{"workload-1", "uid-1"},
				{"workload-2", "uid-2"},
			},
		},
		{
			name:    "pod with single workload",
			podName: "pod-2",
			expected: [][]string{
				{"workload-3", "uid-3"},
			},
		},
		{
			name:     "pod not in cache",
			podName:  "pod-non-existent",
			expected: nil,
		},
		{
			name:     "empty pod name",
			podName:  "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetWorkloadsByPodName(tt.podName)
			assert.Equal(t, tt.expected, result, "GetWorkloadsByPodName should return correct workloads")
		})
	}
}

func TestGetWorkloadsByPodUid(t *testing.T) {
	// Setup test cache
	testCache := map[string][][]string{
		"uid-1": {
			{"workload-1", "w-uid-1"},
		},
		"uid-2": {
			{"workload-2", "w-uid-2"},
			{"workload-3", "w-uid-3"},
		},
	}

	// Save original cache and restore after test
	originalCache := podUidWorkloadCache
	podUidWorkloadCache = testCache
	defer func() {
		podUidWorkloadCache = originalCache
	}()

	tests := []struct {
		name     string
		podUid   string
		expected [][]string
	}{
		{
			name:   "pod uid with single workload",
			podUid: "uid-1",
			expected: [][]string{
				{"workload-1", "w-uid-1"},
			},
		},
		{
			name:   "pod uid with multiple workloads",
			podUid: "uid-2",
			expected: [][]string{
				{"workload-2", "w-uid-2"},
				{"workload-3", "w-uid-3"},
			},
		},
		{
			name:     "pod uid not in cache",
			podUid:   "uid-non-existent",
			expected: nil,
		},
		{
			name:     "empty pod uid",
			podUid:   "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetWorkloadsByPodUid(tt.podUid)
			assert.Equal(t, tt.expected, result, "GetWorkloadsByPodUid should return correct workloads")
		})
	}
}

func TestGetNodeDevicePodCache(t *testing.T) {
	// Setup test cache
	testCache := map[string]map[string]map[string][]string{
		"node-1": {
			"gpu_id": {
				"0": {"pod-1", "uid-1"},
				"1": {"pod-2", "uid-2"},
			},
		},
	}

	// Save original cache and restore after test
	originalCache := nodeDevicePodCache
	nodeDevicePodCache = testCache
	defer func() {
		nodeDevicePodCache = originalCache
	}()

	result := GetNodeDevicePodCache()
	assert.Equal(t, testCache, result, "GetNodeDevicePodCache should return the cache")
	assert.NotNil(t, result, "Cache should not be nil")
}

func TestGetPodWorkloadCache(t *testing.T) {
	// Setup test cache
	testCache := map[string][][]string{
		"pod-1": {
			{"workload-1", "uid-1"},
		},
	}

	// Save original cache and restore after test
	originalCache := podWorkloadCache
	podWorkloadCache = testCache
	defer func() {
		podWorkloadCache = originalCache
	}()

	result := GetPodWorkloadCache()
	assert.Equal(t, testCache, result, "GetPodWorkloadCache should return the cache")
	assert.NotNil(t, result, "Cache should not be nil")
}

func TestGetPodUidWorkloadCache(t *testing.T) {
	// Setup test cache
	testCache := map[string][][]string{
		"uid-1": {
			{"workload-1", "w-uid-1"},
		},
	}

	// Save original cache and restore after test
	originalCache := podUidWorkloadCache
	podUidWorkloadCache = testCache
	defer func() {
		podUidWorkloadCache = originalCache
	}()

	result := GetPodUidWorkloadCache()
	assert.Equal(t, testCache, result, "GetPodUidWorkloadCache should return the cache")
	assert.NotNil(t, result, "Cache should not be nil")
}

func TestGetPodLabelValueWithDeviceCache(t *testing.T) {
	// Setup test cache
	testDeviceCache := map[string]map[string]map[string][]string{
		"node-1": {
			"gpu_id": {
				"0": {"test-pod-1", "test-uid-1"},
				"1": {"test-pod-2", "test-uid-2"},
			},
			"device": {
				"mlx5_0": {"test-pod-3", "test-uid-3"},
			},
		},
	}

	// Save original cache and restore after test
	originalCache := nodeDevicePodCache
	nodeDevicePodCache = testDeviceCache
	defer func() {
		nodeDevicePodCache = originalCache
	}()

	tests := []struct {
		name            string
		labels          []prompb.Label
		expectedPodName string
		expectedPodUid  string
	}{
		{
			name: "find pod by GPU device",
			labels: []prompb.Label{
				{Name: constant.PrimusLensNodeLabelName, Value: "node-1"},
				{Name: "gpu_id", Value: "0"},
			},
			expectedPodName: "test-pod-1",
			expectedPodUid:  "test-uid-1",
		},
		{
			name: "find pod by different GPU",
			labels: []prompb.Label{
				{Name: constant.PrimusLensNodeLabelName, Value: "node-1"},
				{Name: "gpu_id", Value: "1"},
			},
			expectedPodName: "test-pod-2",
			expectedPodUid:  "test-uid-2",
		},
		{
			name: "find pod by RDMA device",
			labels: []prompb.Label{
				{Name: constant.PrimusLensNodeLabelName, Value: "node-1"},
				{Name: "device", Value: "mlx5_0"},
			},
			expectedPodName: "test-pod-3",
			expectedPodUid:  "test-uid-3",
		},
		{
			name: "node not in cache",
			labels: []prompb.Label{
				{Name: constant.PrimusLensNodeLabelName, Value: "node-99"},
				{Name: "gpu_id", Value: "0"},
			},
			expectedPodName: "",
			expectedPodUid:  "",
		},
		{
			name: "no node label",
			labels: []prompb.Label{
				{Name: "gpu_id", Value: "0"},
			},
			expectedPodName: "",
			expectedPodUid:  "",
		},
		{
			name: "device not in cache",
			labels: []prompb.Label{
				{Name: constant.PrimusLensNodeLabelName, Value: "node-1"},
				{Name: "gpu_id", Value: "99"},
			},
			expectedPodName: "",
			expectedPodUid:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podName, podUid := GetPodLabelValue(tt.labels)
			assert.Equal(t, tt.expectedPodName, podName, "Pod name should match")
			assert.Equal(t, tt.expectedPodUid, podUid, "Pod UID should match")
		})
	}
}

func TestGetPodLabelValueWithKubeStateMetrics(t *testing.T) {
	// Empty device cache for this test
	originalCache := nodeDevicePodCache
	nodeDevicePodCache = map[string]map[string]map[string][]string{}
	defer func() {
		nodeDevicePodCache = originalCache
	}()

	tests := []struct {
		name            string
		labels          []prompb.Label
		expectedPodName string
		expectedPodUid  string
	}{
		{
			name: "no pod information without node label",
			labels: []prompb.Label{
				{Name: "pod", Value: "kube-pod-1"},
				{Name: "uid", Value: "kube-uid-1"},
			},
			expectedPodName: "",
			expectedPodUid:  "",
		},
		{
			name: "no pod information without node",
			labels: []prompb.Label{
				{Name: "pod", Value: "kubelet-pod-1"},
			},
			expectedPodName: "",
			expectedPodUid:  "",
		},
		{
			name: "no pod information",
			labels: []prompb.Label{
				{Name: "node", Value: "node-1"},
			},
			expectedPodName: "",
			expectedPodUid:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podName, podUid := GetPodLabelValue(tt.labels)
			assert.Equal(t, tt.expectedPodName, podName, "Pod name should match")
			assert.Equal(t, tt.expectedPodUid, podUid, "Pod UID should match")
		})
	}
}
