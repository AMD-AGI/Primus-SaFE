package grafana

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDatasourceSyncer(t *testing.T) {
	t.Run("creates syncer with namespace and labels", func(t *testing.T) {
		labels := map[string]string{
			"grafana": "main",
			"app":     "grafana",
		}
		syncer := NewDatasourceSyncer("monitoring", labels)

		assert.NotNil(t, syncer)
		assert.Equal(t, "monitoring", syncer.namespace)
		assert.Equal(t, labels, syncer.instanceLabels)
		assert.Nil(t, syncer.dynamicClient)
		assert.False(t, syncer.crdAvailable)
		assert.False(t, syncer.crdChecked)
	})

	t.Run("creates syncer with empty labels", func(t *testing.T) {
		syncer := NewDatasourceSyncer("default", nil)

		assert.NotNil(t, syncer)
		assert.Equal(t, "default", syncer.namespace)
		assert.Nil(t, syncer.instanceLabels)
	})
}

func TestGrafanaDatasourceGVR(t *testing.T) {
	assert.Equal(t, "grafana.integreatly.org", grafanaDatasourceGVR.Group)
	assert.Equal(t, "v1beta1", grafanaDatasourceGVR.Version)
	assert.Equal(t, "grafanadatasources", grafanaDatasourceGVR.Resource)
}

func TestIsNoKindMatchError(t *testing.T) {
	t.Run("returns false for nil error", func(t *testing.T) {
		result := isNoKindMatchError(nil)
		assert.False(t, result)
	})

	t.Run("returns true for no matches for kind error", func(t *testing.T) {
		err := &mockError{msg: "no matches for kind \"GrafanaDatasource\" in version \"grafana.integreatly.org/v1beta1\""}
		result := isNoKindMatchError(err)
		assert.True(t, result)
	})

	t.Run("returns true for server could not find resource error", func(t *testing.T) {
		err := &mockError{msg: "the server could not find the requested resource"}
		result := isNoKindMatchError(err)
		assert.True(t, result)
	})

	t.Run("returns false for other errors", func(t *testing.T) {
		err := &mockError{msg: "some other error"}
		result := isNoKindMatchError(err)
		assert.False(t, result)
	})
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substr   string
		expected bool
	}{
		{
			name:     "contains substring",
			s:        "hello world",
			substr:   "world",
			expected: true,
		},
		{
			name:     "does not contain substring",
			s:        "hello world",
			substr:   "foo",
			expected: false,
		},
		{
			name:     "exact match",
			s:        "hello",
			substr:   "hello",
			expected: true,
		},
		{
			name:     "empty substring",
			s:        "hello",
			substr:   "",
			expected: true,
		},
		{
			name:     "empty string",
			s:        "",
			substr:   "hello",
			expected: false,
		},
		{
			name:     "both empty",
			s:        "",
			substr:   "",
			expected: true,
		},
		{
			name:     "substring at start",
			s:        "hello world",
			substr:   "hello",
			expected: true,
		},
		{
			name:     "substring at end",
			s:        "hello world",
			substr:   "world",
			expected: true,
		},
		{
			name:     "case sensitive",
			s:        "Hello World",
			substr:   "hello",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.s, tt.substr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDatasourceSyncer_CheckCRDAvailability(t *testing.T) {
	t.Run("marks CRD unavailable when dynamic client is nil", func(t *testing.T) {
		syncer := NewDatasourceSyncer("monitoring", nil)

		syncer.checkCRDAvailability(nil)

		assert.False(t, syncer.crdAvailable)
		assert.True(t, syncer.crdChecked)
	})
}

// mockError is a simple mock error for testing
type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}
