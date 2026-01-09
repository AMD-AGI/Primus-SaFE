// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package grafana

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGroupVersion(t *testing.T) {
	assert.Equal(t, "grafana.integreatly.org", GroupVersion.Group)
	assert.Equal(t, "v1beta1", GroupVersion.Version)
}

func TestGrafanaDatasource_DeepCopy(t *testing.T) {
	t.Run("deep copies nil returns nil", func(t *testing.T) {
		var ds *GrafanaDatasource
		result := ds.DeepCopy()
		assert.Nil(t, result)
	})

	t.Run("deep copies basic datasource", func(t *testing.T) {
		ds := &GrafanaDatasource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ds",
				Namespace: "monitoring",
			},
			Spec: GrafanaDatasourceSpec{
				AllowCrossNamespaceImport: true,
				ResyncPeriod:              "5m",
			},
		}

		result := ds.DeepCopy()

		assert.NotNil(t, result)
		assert.Equal(t, ds.Name, result.Name)
		assert.Equal(t, ds.Namespace, result.Namespace)
		assert.Equal(t, ds.Spec.AllowCrossNamespaceImport, result.Spec.AllowCrossNamespaceImport)
		assert.Equal(t, ds.Spec.ResyncPeriod, result.Spec.ResyncPeriod)

		// Verify it's a deep copy (modifying original doesn't affect copy)
		ds.Name = "modified"
		assert.NotEqual(t, ds.Name, result.Name)
	})

	t.Run("deep copies datasource with internal config", func(t *testing.T) {
		ds := &GrafanaDatasource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "prometheus-ds",
				Namespace: "monitoring",
			},
			Spec: GrafanaDatasourceSpec{
				Datasource: &GrafanaDatasourceInternal{
					Name:      "Prometheus",
					Type:      "prometheus",
					Access:    "proxy",
					URL:       "http://prometheus:9090",
					IsDefault: true,
					JSONData: map[string]interface{}{
						"timeInterval": "15s",
						"httpMethod":   "POST",
					},
					SecureJSONData: map[string]string{
						"httpHeaderValue1": "Bearer token123",
					},
				},
				InstanceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"grafana": "main",
					},
				},
			},
		}

		result := ds.DeepCopy()

		assert.NotNil(t, result)
		assert.NotNil(t, result.Spec.Datasource)
		assert.Equal(t, "Prometheus", result.Spec.Datasource.Name)
		assert.Equal(t, "prometheus", result.Spec.Datasource.Type)
		assert.Equal(t, "proxy", result.Spec.Datasource.Access)
		assert.Equal(t, "http://prometheus:9090", result.Spec.Datasource.URL)
		assert.True(t, result.Spec.Datasource.IsDefault)
		assert.Equal(t, "15s", result.Spec.Datasource.JSONData["timeInterval"])
		assert.Equal(t, "Bearer token123", result.Spec.Datasource.SecureJSONData["httpHeaderValue1"])
		assert.NotNil(t, result.Spec.InstanceSelector)
		assert.Equal(t, "main", result.Spec.InstanceSelector.MatchLabels["grafana"])

		// Verify deep copy (modifying original's maps doesn't affect copy)
		ds.Spec.Datasource.JSONData["timeInterval"] = "30s"
		assert.Equal(t, "15s", result.Spec.Datasource.JSONData["timeInterval"])
	})
}

func TestGrafanaDatasourceInternal_DeepCopy(t *testing.T) {
	t.Run("deep copies nil returns nil", func(t *testing.T) {
		var internal *GrafanaDatasourceInternal
		result := internal.DeepCopy()
		assert.Nil(t, result)
	})

	t.Run("deep copies with all fields", func(t *testing.T) {
		internal := &GrafanaDatasourceInternal{
			Name:      "TestDS",
			Type:      "postgres",
			Access:    "direct",
			URL:       "postgres://db:5432",
			User:      "admin",
			IsDefault: false,
			JSONData: map[string]interface{}{
				"database":  "primus_lens",
				"sslmode":   "require",
				"maxConns":  10,
				"enableLog": true,
			},
			SecureJSONData: map[string]string{
				"password": "secret123",
			},
		}

		result := internal.DeepCopy()

		assert.NotNil(t, result)
		assert.Equal(t, "TestDS", result.Name)
		assert.Equal(t, "postgres", result.Type)
		assert.Equal(t, "direct", result.Access)
		assert.Equal(t, "postgres://db:5432", result.URL)
		assert.Equal(t, "admin", result.User)
		assert.False(t, result.IsDefault)
		assert.Equal(t, "primus_lens", result.JSONData["database"])
		assert.Equal(t, 10, result.JSONData["maxConns"])
		assert.Equal(t, true, result.JSONData["enableLog"])
		assert.Equal(t, "secret123", result.SecureJSONData["password"])
	})

	t.Run("handles nil maps", func(t *testing.T) {
		internal := &GrafanaDatasourceInternal{
			Name: "SimpleDS",
			Type: "prometheus",
		}

		result := internal.DeepCopy()

		assert.NotNil(t, result)
		assert.Nil(t, result.JSONData)
		assert.Nil(t, result.SecureJSONData)
	})
}

func TestGrafanaDatasourceList_DeepCopyList(t *testing.T) {
	t.Run("deep copies nil returns nil", func(t *testing.T) {
		var list *GrafanaDatasourceList
		result := list.DeepCopyList()
		assert.Nil(t, result)
	})

	t.Run("deep copies list with items", func(t *testing.T) {
		list := &GrafanaDatasourceList{
			Items: []GrafanaDatasource{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ds1"},
					Spec: GrafanaDatasourceSpec{
						Datasource: &GrafanaDatasourceInternal{
							Name: "DS1",
							Type: "prometheus",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ds2"},
					Spec: GrafanaDatasourceSpec{
						Datasource: &GrafanaDatasourceInternal{
							Name: "DS2",
							Type: "postgres",
						},
					},
				},
			},
		}

		result := list.DeepCopyList()

		assert.NotNil(t, result)
		assert.Len(t, result.Items, 2)
		assert.Equal(t, "ds1", result.Items[0].Name)
		assert.Equal(t, "ds2", result.Items[1].Name)
		assert.Equal(t, "DS1", result.Items[0].Spec.Datasource.Name)
		assert.Equal(t, "DS2", result.Items[1].Spec.Datasource.Name)

		// Verify deep copy
		list.Items[0].Name = "modified"
		assert.NotEqual(t, list.Items[0].Name, result.Items[0].Name)
	})

	t.Run("deep copies empty list", func(t *testing.T) {
		list := &GrafanaDatasourceList{
			Items: []GrafanaDatasource{},
		}

		result := list.DeepCopyList()

		assert.NotNil(t, result)
		assert.Empty(t, result.Items)
	})
}

func TestGrafanaDatasourceSpec_DeepCopyInto(t *testing.T) {
	t.Run("copies spec with all fields", func(t *testing.T) {
		spec := GrafanaDatasourceSpec{
			AllowCrossNamespaceImport: true,
			ResyncPeriod:              "10m",
			Datasource: &GrafanaDatasourceInternal{
				Name: "TestDS",
				Type: "prometheus",
			},
			InstanceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "grafana",
				},
			},
		}

		out := GrafanaDatasourceSpec{}
		spec.DeepCopyInto(&out)

		assert.True(t, out.AllowCrossNamespaceImport)
		assert.Equal(t, "10m", out.ResyncPeriod)
		assert.NotNil(t, out.Datasource)
		assert.Equal(t, "TestDS", out.Datasource.Name)
		assert.NotNil(t, out.InstanceSelector)
		assert.Equal(t, "grafana", out.InstanceSelector.MatchLabels["app"])
	})
}

func TestGrafanaDatasource_DeepCopyObject(t *testing.T) {
	ds := &GrafanaDatasource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ds",
			Namespace: "monitoring",
		},
	}

	result := ds.DeepCopyObject()

	assert.NotNil(t, result)
	copied, ok := result.(*GrafanaDatasource)
	assert.True(t, ok)
	assert.Equal(t, "test-ds", copied.Name)
}

func TestGrafanaDatasourceList_DeepCopyObject(t *testing.T) {
	list := &GrafanaDatasourceList{
		Items: []GrafanaDatasource{
			{ObjectMeta: metav1.ObjectMeta{Name: "ds1"}},
		},
	}

	result := list.DeepCopyObject()

	assert.NotNil(t, result)
	copied, ok := result.(*GrafanaDatasourceList)
	assert.True(t, ok)
	assert.Len(t, copied.Items, 1)
}

