// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

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

// mockClient is a mock controller-runtime client for testing
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

// mockRESTMapper is a simple mock RESTMapper
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
	t.Run("get owner object successfully", func(t *testing.T) {
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

	t.Run("owner is cluster-scoped resource", func(t *testing.T) {
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
				// verify cluster resource should not have namespace
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

	t.Run("get non-existent owner", func(t *testing.T) {
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

	t.Run("multi-level owner chain", func(t *testing.T) {
		ctx := context.Background()
		namespace := "default"

		// owner chain: Pod -> ReplicaSet -> Deployment
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
				
				// return different objects based on requested name
				if key.Name == replicaSetOwnerRef.Name {
					u.SetAPIVersion(replicaSetOwnerRef.APIVersion)
					u.SetKind(replicaSetOwnerRef.Kind)
					u.SetName(replicaSetOwnerRef.Name)
					u.SetNamespace(namespace)
					u.SetUID(replicaSetOwnerRef.UID)
					
					// ReplicaSet has a Deployment owner
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

		// first get ReplicaSet
		replicaSet, err := GetOwnerObject(ctx, mockClient, replicaSetOwnerRef, namespace)
		assert.NoError(t, err)
		assert.NotNil(t, replicaSet)
		assert.Equal(t, replicaSetOwnerRef.Name, replicaSet.GetName())

		// then get ReplicaSet's owner (Deployment)
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
		// verify this is controller owner
		assert.NotNil(t, ownerRef.Controller)
		assert.True(t, *ownerRef.Controller)
	})

	t.Run("empty owner reference", func(t *testing.T) {
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

		// empty OwnerReference should cause error because API version and kind cannot be parsed
		_, err := GetOwnerObject(ctx, mockClient, ownerRef, namespace)

		// should fail due to invalid API version or kind
		assert.Error(t, err)
	})
}

func TestOwnerReferenceHelpers(t *testing.T) {
	t.Run("check if has controller owner", func(t *testing.T) {
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

	t.Run("verify OwnerReference fields", func(t *testing.T) {
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

