package k8sUtil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestGvkToGvr 测试 GvkToGvr 函数
// 注意：GvkToGvr 函数依赖于实际的 Kubernetes 客户端和 Discovery API，
// 在单元测试中很难完全模拟，因此这里只做基本的测试
func TestGvkToGvr(t *testing.T) {
	t.Run("无效的apiVersion格式", func(t *testing.T) {
		apiVersion := "invalid//version"
		kind := "Pod"

		// 使用 nil 客户端测试错误情况
		_, err := GvkToGvr(apiVersion, kind, nil)

		assert.Error(t, err)
	})

	t.Run("有效的apiVersion解析", func(t *testing.T) {
		// 测试 GroupVersion 解析逻辑
		gv, err := schema.ParseGroupVersion("apps/v1")
		assert.NoError(t, err)
		assert.Equal(t, "apps", gv.Group)
		assert.Equal(t, "v1", gv.Version)

		// 测试核心 API
		gv, err = schema.ParseGroupVersion("v1")
		assert.NoError(t, err)
		assert.Equal(t, "", gv.Group)
		assert.Equal(t, "v1", gv.Version)
	})
}

// mockClient 是一个用于测试的 mock controller-runtime client
type mockClient struct {
	client.Client
	getFunc       func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error
	restMapperVal meta.RESTMapper
}

func (m *mockClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	if m.getFunc != nil {
		return m.getFunc(ctx, key, obj, opts...)
	}
	return nil
}

func (m *mockClient) RESTMapper() meta.RESTMapper {
	return m.restMapperVal
}

func (m *mockClient) Scheme() *runtime.Scheme {
	return runtime.NewScheme()
}

// mockRESTMapper 是一个简单的 mock RESTMapper
type mockRESTMapper struct {
	meta.RESTMapper
	mappingFunc func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error)
}

func (m *mockRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	if m.mappingFunc != nil {
		return m.mappingFunc(gk, versions...)
	}
	return nil, nil
}

func TestGetObjectByGvk(t *testing.T) {
	t.Run("获取命名空间资源", func(t *testing.T) {
		ctx := context.Background()
		apiVersion := "v1"
		kind := "Pod"
		namespace := "default"
		name := "test-pod"

		mockMapper := &mockRESTMapper{
			mappingFunc: func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
				return &meta.RESTMapping{
					Resource: schema.GroupVersionResource{
						Group:    "",
						Version:  "v1",
						Resource: "pods",
					},
					GroupVersionKind: schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Pod",
					},
					Scope: meta.RESTScopeNamespace,
				}, nil
			},
		}

		mock := &mockClient{
			restMapperVal: mockMapper,
			getFunc: func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
				// 设置返回的对象
				u := obj.(*unstructured.Unstructured)
				u.SetName(name)
				u.SetNamespace(namespace)
				u.SetAPIVersion(apiVersion)
				u.SetKind(kind)
				return nil
			},
		}

		obj, err := GetObjectByGvk(ctx, apiVersion, kind, namespace, name, mock)

		assert.NoError(t, err)
		assert.NotNil(t, obj)
		assert.Equal(t, name, obj.GetName())
		assert.Equal(t, namespace, obj.GetNamespace())
		assert.Equal(t, kind, obj.GetKind())
	})

	t.Run("获取集群级别资源", func(t *testing.T) {
		ctx := context.Background()
		apiVersion := "v1"
		kind := "Node"
		name := "test-node"

		mockMapper := &mockRESTMapper{
			mappingFunc: func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
				return &meta.RESTMapping{
					Resource: schema.GroupVersionResource{
						Group:    "",
						Version:  "v1",
						Resource: "nodes",
					},
					GroupVersionKind: schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Node",
					},
					Scope: meta.RESTScopeRoot,
				}, nil
			},
		}

		mock := &mockClient{
			restMapperVal: mockMapper,
			getFunc: func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
				// 集群资源不应该有 namespace
				assert.Empty(t, key.Namespace)
				u := obj.(*unstructured.Unstructured)
				u.SetName(name)
				u.SetAPIVersion(apiVersion)
				u.SetKind(kind)
				return nil
			},
		}

		obj, err := GetObjectByGvk(ctx, apiVersion, kind, "default", name, mock)

		assert.NoError(t, err)
		assert.NotNil(t, obj)
		assert.Equal(t, name, obj.GetName())
		assert.Empty(t, obj.GetNamespace()) // 集群资源没有 namespace
	})

	t.Run("REST mapping 失败", func(t *testing.T) {
		ctx := context.Background()
		apiVersion := "v1"
		kind := "UnknownKind"
		namespace := "default"
		name := "test"

		mockMapper := &mockRESTMapper{
			mappingFunc: func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
				return nil, assert.AnError
			},
		}

		mock := &mockClient{
			restMapperVal: mockMapper,
		}

		_, err := GetObjectByGvk(ctx, apiVersion, kind, namespace, name, mock)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot get REST mapping")
	})

	t.Run("获取对象失败", func(t *testing.T) {
		ctx := context.Background()
		apiVersion := "v1"
		kind := "Pod"
		namespace := "default"
		name := "missing-pod"

		mockMapper := &mockRESTMapper{
			mappingFunc: func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
				return &meta.RESTMapping{
					Resource: schema.GroupVersionResource{
						Group:    "",
						Version:  "v1",
						Resource: "pods",
					},
					GroupVersionKind: schema.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Pod",
					},
					Scope: meta.RESTScopeNamespace,
				}, nil
			},
		}

		mock := &mockClient{
			restMapperVal: mockMapper,
			getFunc: func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
				return assert.AnError
			},
		}

		_, err := GetObjectByGvk(ctx, apiVersion, kind, namespace, name, mock)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get object")
	})
}

func TestSchemaConversions(t *testing.T) {
	t.Run("GroupVersion解析", func(t *testing.T) {
		tests := []struct {
			name        string
			apiVersion  string
			expectGroup string
			expectVer   string
			expectErr   bool
		}{
			{
				name:        "核心API",
				apiVersion:  "v1",
				expectGroup: "",
				expectVer:   "v1",
				expectErr:   false,
			},
			{
				name:        "带组的API",
				apiVersion:  "apps/v1",
				expectGroup: "apps",
				expectVer:   "v1",
				expectErr:   false,
			},
			{
				name:        "自定义资源",
				apiVersion:  "example.com/v1alpha1",
				expectGroup: "example.com",
				expectVer:   "v1alpha1",
				expectErr:   false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				gv, err := schema.ParseGroupVersion(tt.apiVersion)
				if tt.expectErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.expectGroup, gv.Group)
					assert.Equal(t, tt.expectVer, gv.Version)
				}
			})
		}
	})

	t.Run("FromAPIVersionAndKind", func(t *testing.T) {
		apiVersion := "apps/v1"
		kind := "Deployment"

		gvk := schema.FromAPIVersionAndKind(apiVersion, kind)

		assert.Equal(t, "apps", gvk.Group)
		assert.Equal(t, "v1", gvk.Version)
		assert.Equal(t, "Deployment", gvk.Kind)
	})
}

func TestAPIResourceDiscovery(t *testing.T) {
	t.Run("API资源列表结构", func(t *testing.T) {
		resources := []*metav1.APIResourceList{
			{
				GroupVersion: "v1",
				APIResources: []metav1.APIResource{
					{Name: "pods", Kind: "Pod", Namespaced: true},
					{Name: "services", Kind: "Service", Namespaced: true},
					{Name: "nodes", Kind: "Node", Namespaced: false},
				},
			},
			{
				GroupVersion: "apps/v1",
				APIResources: []metav1.APIResource{
					{Name: "deployments", Kind: "Deployment", Namespaced: true},
					{Name: "statefulsets", Kind: "StatefulSet", Namespaced: true},
				},
			},
		}

		assert.Len(t, resources, 2)
		assert.Equal(t, "v1", resources[0].GroupVersion)
		assert.Len(t, resources[0].APIResources, 3)
		assert.Equal(t, "apps/v1", resources[1].GroupVersion)
		assert.Len(t, resources[1].APIResources, 2)
	})
}

