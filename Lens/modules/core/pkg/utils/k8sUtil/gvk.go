package k8sUtil

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GvkToGvr(apiVersion string, kind string, k8sClientsets *kubernetes.Clientset) (schema.GroupVersionResource, error) {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}
	gvk := schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    kind,
	}
	dc := k8sClientsets.DiscoveryClient
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}
	return mapping.Resource, nil

}

func GetObjectByGvk(
	ctx context.Context,
	apiVersion,
	kind,
	namespace,
	name string,
	k8sClient client.Client,
) (*unstructured.Unstructured, error) {
	gvk := schema.FromAPIVersionAndKind(apiVersion, kind)

	mapping, err := k8sClient.RESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, fmt.Errorf("cannot get REST mapping for %s: %w", gvk.String(), err)
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	key := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	if mapping.Scope.Name() == meta.RESTScopeNameRoot {
		key.Namespace = ""
	}
	if err := k8sClient.Get(ctx, key, obj); err != nil {
		return nil, fmt.Errorf("failed to get object %s/%s for kind %s: %w",
			key.Namespace, key.Name, gvk.Kind, err)
	}
	return obj, nil
}
