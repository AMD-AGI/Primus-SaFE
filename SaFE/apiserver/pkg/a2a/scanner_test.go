/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package a2a

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func setScannerConfig(t *testing.T, namespaces []string, labelSelector string) {
	t.Helper()
	viper.Reset()
	viper.Set("a2a.scanner.namespaces", namespaces)
	viper.Set("a2a.scanner.label_selector", labelSelector)
	t.Cleanup(viper.Reset)
}

func newTestHTTPClient() *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			statusCode := http.StatusOK
			body := `{}`
			switch req.URL.Path {
			case "/a2a/.well-known/agent.json":
				body = `{"skills":[{"id":"demo-skill"}]}`
			case "/a2a/health":
				body = `{"status":"ok"}`
			default:
				statusCode = http.StatusNotFound
			}
			return &http.Response{
				StatusCode: statusCode,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString(body)),
			}, nil
		}),
	}
}

func newA2AService(name, namespace string, enabled bool) *corev1.Service {
	labels := map[string]string{}
	if enabled {
		labels["a2a.primus.io/enabled"] = "true"
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Port: 8089}},
		},
	}
}

func TestScannerScanAcrossAllNamespacesWhenNamespacesEmpty(t *testing.T) {
	setScannerConfig(t, []string{}, "a2a.primus.io/enabled=true")

	svc1 := newA2AService("agent-one", "ns-one", true)
	svc2 := newA2AService("agent-two", "ns-two", true)
	ignored := newA2AService("agent-ignored", "ns-three", false)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	mockDB := mock_client.NewMockInterface(ctrl)
	scanner := NewScanner(
		ctrlfake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(svc1, svc2, ignored).Build(),
		mockDB,
	)
	scanner.httpClient = newTestHTTPClient()

	synced := map[string]*dbclient.A2AServiceRegistry{}
	mockDB.EXPECT().
		UpsertA2AService(gomock.Any(), gomock.Any()).
		Times(2).
		DoAndReturn(func(_ context.Context, reg *dbclient.A2AServiceRegistry) error {
			synced[reg.ServiceName] = reg
			return nil
		})

	scanner.scan(context.Background())

	require.Len(t, synced, 2)
	assert.NotContains(t, synced, "agent-ignored")

	assert.Equal(t, "ns-one", synced["agent-one"].K8sNamespace.String)
	assert.Equal(t, "agent-one", synced["agent-one"].K8sService.String)
	assert.Equal(t, "http://agent-one.ns-one.svc.cluster.local:8089", synced["agent-one"].Endpoint)
	assert.Equal(t, "healthy", synced["agent-one"].A2AHealth)
	assert.Equal(t, `[{"id":"demo-skill"}]`, synced["agent-one"].A2ASkills.String)

	assert.Equal(t, "ns-two", synced["agent-two"].K8sNamespace.String)
	assert.Equal(t, "agent-two", synced["agent-two"].K8sService.String)
	assert.Equal(t, "http://agent-two.ns-two.svc.cluster.local:8089", synced["agent-two"].Endpoint)
	assert.Equal(t, "healthy", synced["agent-two"].A2AHealth)
}

func TestScannerScanUsesConfiguredNamespacesOnly(t *testing.T) {
	setScannerConfig(t, []string{"primus-lens"}, "a2a.primus.io/enabled=true")

	svc1 := newA2AService("agent-in-scope", "primus-lens", true)
	svc2 := newA2AService("agent-out-of-scope", "other-ns", true)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	mockDB := mock_client.NewMockInterface(ctrl)
	scanner := NewScanner(
		ctrlfake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(svc1, svc2).Build(),
		mockDB,
	)
	scanner.httpClient = newTestHTTPClient()

	mockDB.EXPECT().
		UpsertA2AService(gomock.Any(), gomock.Any()).
		Times(1).
		DoAndReturn(func(_ context.Context, reg *dbclient.A2AServiceRegistry) error {
			assert.Equal(t, "agent-in-scope", reg.ServiceName)
			assert.Equal(t, "primus-lens", reg.K8sNamespace.String)
			return nil
		})

	scanner.scan(context.Background())
}

func TestParseLabelSelector(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:     "single label",
			input:    "a2a.primus.io/enabled=true",
			expected: map[string]string{"a2a.primus.io/enabled": "true"},
		},
		{
			name:     "multiple labels",
			input:    "a2a.primus.io/enabled=true, app=test",
			expected: map[string]string{"a2a.primus.io/enabled": "true", "app": "test"},
		},
		{
			name:     "empty",
			input:    "",
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLabelSelector(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d labels, got %d", len(tt.expected), len(result))
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("expected %s=%s, got %s=%s", k, v, k, result[k])
				}
			}
		})
	}
}

func TestSplitTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		sep      string
		expected []string
	}{
		{"normal", "a,b,c", ",", []string{"a", "b", "c"}},
		{"with spaces", " a , b , c ", ",", []string{"a", "b", "c"}},
		{"empty", "", ",", []string{}},
		{"single", "abc", ",", []string{"abc"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitTrim(tt.input, tt.sep)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d parts, got %d: %v", len(tt.expected), len(result), result)
			}
			for i, v := range tt.expected {
				if i < len(result) && result[i] != v {
					t.Errorf("expected %s at index %d, got %s", v, i, result[i])
				}
			}
		})
	}
}
