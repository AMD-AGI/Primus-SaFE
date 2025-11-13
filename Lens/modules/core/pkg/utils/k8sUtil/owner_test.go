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

// mockClient 是一个用于测试的 mock controller-runtime client
type mockClientOwner struct {
	client.Client
	getFunc       func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error
	restMapperVal meta.RESTMapper
}

func (m *mockClientOwner) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	if m.getFunc != nil {
		return m.getFunc(ctx, key, obj, opts...)
	}
	return nil
}

func (m *mockClientOwner) RESTMapper() meta.RESTMapper {
	return m.restMapperVal
}

func (m *mockClientOwner) Scheme() *runtime.Scheme {
	return runtime.NewScheme()
}

// mockRESTMapper 是一个简单的 mock RESTMapper
type mockRESTMapperOwner struct {
	meta.RESTMapper
	mappingFunc func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error)
}

func (m *mockRESTMapperOwner) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	if m.mappingFunc != nil {
		return m.mappingFunc(gk, versions...)
	}
	return nil, nil
}

func TestGetOwnerObject(t *testing.T) {
	t.Run("获取Owner对象成功", func(t *testing.T) {
		ctx := context.Background()
		namespace := "default"

		ownerRef := metav1.OwnerReference{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "test-deployment",
			UID:        "12345",
		}

		mockClient := &mockClientOwner{
			restMapperVal: &mockRESTMapperOwner{
				mappingFunc: func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
					return &meta.RESTMapping{
						Scope: meta.RESTScopeNamespace,
					}, nil
				},
			},
			getFunc: func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
				u := obj.(*unstructured.Unstructured)
				u.SetAPIVersion(ownerRef.APIVersion)
				u.SetKind(ownerRef.Kind)
				u.SetName(ownerRef.Name)
				u.SetNamespace(namespace)
				u.SetUID(ownerRef.UID)
				return nil
			},
		}

		obj, err := GetOwnerObject(ctx, mockClient, ownerRef, namespace)

		assert.NoError(t, err)
		assert.NotNil(t, obj)
		assert.Equal(t, ownerRef.Name, obj.GetName())
		assert.Equal(t, ownerRef.Kind, obj.GetKind())
		assert.Equal(t, ownerRef.APIVersion, obj.GetAPIVersion())
		assert.Equal(t, namespace, obj.GetNamespace())
	})

	t.Run("Owner是集群级别资源", func(t *testing.T) {
		ctx := context.Background()
		namespace := "default"

		ownerRef := metav1.OwnerReference{
			APIVersion: "v1",
			Kind:       "Node",
			Name:       "test-node",
			UID:        "67890",
		}

		mockClient := &mockClient{
			restMapperVal: &mockRESTMapper{
				mappingFunc: func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
					return &meta.RESTMapping{
						Scope: meta.RESTScopeRoot,
					}, nil
				},
			},
			getFunc: func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
				// 验证集群资源不应该有 namespace
				assert.Empty(t, key.Namespace)
				
				u := obj.(*unstructured.Unstructured)
				u.SetAPIVersion(ownerRef.APIVersion)
				u.SetKind(ownerRef.Kind)
				u.SetName(ownerRef.Name)
				u.SetUID(ownerRef.UID)
				return nil
			},
		}

		obj, err := GetOwnerObject(ctx, mockClient, ownerRef, namespace)

		assert.NoError(t, err)
		assert.NotNil(t, obj)
		assert.Equal(t, ownerRef.Name, obj.GetName())
		assert.Empty(t, obj.GetNamespace())
	})

	t.Run("获取不存在的Owner", func(t *testing.T) {
		ctx := context.Background()
		namespace := "default"

		ownerRef := metav1.OwnerReference{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "missing-deployment",
			UID:        "missing",
		}

		mockClient := &mockClientOwner{
			restMapperVal: &mockRESTMapperOwner{
				mappingFunc: func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
					return &meta.RESTMapping{
						Scope: meta.RESTScopeNamespace,
					}, nil
				},
			},
			getFunc: func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
				return assert.AnError
			},
		}

		_, err := GetOwnerObject(ctx, mockClient, ownerRef, namespace)

		assert.Error(t, err)
	})

	t.Run("多级Owner链", func(t *testing.T) {
		ctx := context.Background()
		namespace := "default"

		// Pod -> ReplicaSet -> Deployment 的 Owner 链
		deploymentOwnerRef := metav1.OwnerReference{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "test-deployment",
			UID:        "deployment-uid",
		}

		replicaSetOwnerRef := metav1.OwnerReference{
			APIVersion: "apps/v1",
			Kind:       "ReplicaSet",
			Name:       "test-replicaset",
			UID:        "replicaset-uid",
		}

		mockClient := &mockClientOwner{
			restMapperVal: &mockRESTMapperOwner{
				mappingFunc: func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
					return &meta.RESTMapping{
						Scope: meta.RESTScopeNamespace,
					}, nil
				},
			},
			getFunc: func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
				u := obj.(*unstructured.Unstructured)
				
				// 根据请求的名称返回不同的对象
				if key.Name == replicaSetOwnerRef.Name {
					u.SetAPIVersion(replicaSetOwnerRef.APIVersion)
					u.SetKind(replicaSetOwnerRef.Kind)
					u.SetName(replicaSetOwnerRef.Name)
					u.SetNamespace(namespace)
					u.SetUID(replicaSetOwnerRef.UID)
					
					// ReplicaSet 有一个 Deployment owner
					u.SetOwnerReferences([]metav1.OwnerReference{deploymentOwnerRef})
				} else if key.Name == deploymentOwnerRef.Name {
					u.SetAPIVersion(deploymentOwnerRef.APIVersion)
					u.SetKind(deploymentOwnerRef.Kind)
					u.SetName(deploymentOwnerRef.Name)
					u.SetNamespace(namespace)
					u.SetUID(deploymentOwnerRef.UID)
				}
				
				return nil
			},
		}

		// 首先获取 ReplicaSet
		replicaSet, err := GetOwnerObject(ctx, mockClient, replicaSetOwnerRef, namespace)
		assert.NoError(t, err)
		assert.NotNil(t, replicaSet)
		assert.Equal(t, replicaSetOwnerRef.Name, replicaSet.GetName())

		// 然后获取 ReplicaSet 的 owner (Deployment)
		owners := replicaSet.GetOwnerReferences()
		assert.Len(t, owners, 1)

		deployment, err := GetOwnerObject(ctx, mockClient, owners[0], namespace)
		assert.NoError(t, err)
		assert.NotNil(t, deployment)
		assert.Equal(t, deploymentOwnerRef.Name, deployment.GetName())
		assert.Equal(t, "Deployment", deployment.GetKind())
	})

	t.Run("Controller Owner Reference", func(t *testing.T) {
		ctx := context.Background()
		namespace := "default"

		trueVal := true
		ownerRef := metav1.OwnerReference{
			APIVersion: "apps/v1",
			Kind:       "ReplicaSet",
			Name:       "test-replicaset",
			UID:        "rs-uid",
			Controller: &trueVal,
		}

		mockClient := &mockClientOwner{
			restMapperVal: &mockRESTMapperOwner{
				mappingFunc: func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
					return &meta.RESTMapping{
						Scope: meta.RESTScopeNamespace,
					}, nil
				},
			},
			getFunc: func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
				u := obj.(*unstructured.Unstructured)
				u.SetAPIVersion(ownerRef.APIVersion)
				u.SetKind(ownerRef.Kind)
				u.SetName(ownerRef.Name)
				u.SetNamespace(namespace)
				u.SetUID(ownerRef.UID)
				return nil
			},
		}

		obj, err := GetOwnerObject(ctx, mockClient, ownerRef, namespace)

		assert.NoError(t, err)
		assert.NotNil(t, obj)
		// 验证这是控制器 owner
		assert.NotNil(t, ownerRef.Controller)
		assert.True(t, *ownerRef.Controller)
	})

	t.Run("空Owner Reference", func(t *testing.T) {
		ctx := context.Background()
		namespace := "default"

		ownerRef := metav1.OwnerReference{}

		mockClient := &mockClientOwner{
			restMapperVal: &mockRESTMapperOwner{
				mappingFunc: func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
					return nil, assert.AnError
				},
			},
		}

		// 空的 OwnerReference 应该导致错误，因为无法解析 API version 和 kind
		_, err := GetOwnerObject(ctx, mockClient, ownerRef, namespace)

		// 应该因为无效的 API version 或 kind 而失败
		assert.Error(t, err)
	})
}

func TestOwnerReferenceHelpers(t *testing.T) {
	t.Run("检查是否有Controller Owner", func(t *testing.T) {
		trueVal := true
		falseVal := false

		ownerRefs := []metav1.OwnerReference{
			{
				Name:       "owner1",
				Controller: &falseVal,
			},
			{
				Name:       "owner2",
				Controller: &trueVal,
			},
			{
				Name: "owner3",
			},
		}

		var controllerOwner *metav1.OwnerReference
		for i := range ownerRefs {
			if ownerRefs[i].Controller != nil && *ownerRefs[i].Controller {
				controllerOwner = &ownerRefs[i]
				break
			}
		}

		assert.NotNil(t, controllerOwner)
		assert.Equal(t, "owner2", controllerOwner.Name)
	})

	t.Run("验证OwnerReference字段", func(t *testing.T) {
		trueVal := true
		falseVal := false

		ownerRef := metav1.OwnerReference{
			APIVersion:         "apps/v1",
			Kind:               "Deployment",
			Name:               "test-deployment",
			UID:                "test-uid",
			Controller:         &trueVal,
			BlockOwnerDeletion: &falseVal,
		}

		assert.Equal(t, "apps/v1", ownerRef.APIVersion)
		assert.Equal(t, "Deployment", ownerRef.Kind)
		assert.Equal(t, "test-deployment", ownerRef.Name)
		assert.Equal(t, types.UID("test-uid"), ownerRef.UID)
		assert.True(t, *ownerRef.Controller)
		assert.False(t, *ownerRef.BlockOwnerDeletion)
	})
}

