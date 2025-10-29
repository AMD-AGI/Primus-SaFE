package k8sUtil

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetOwnerObject(
	ctx context.Context,
	k8sClient client.Client,
	ownerRef metav1.OwnerReference,
	namespace string,
) (*unstructured.Unstructured, error) {
	return GetObjectByGvk(ctx, ownerRef.APIVersion, ownerRef.Kind, namespace, ownerRef.Name, k8sClient)
}
