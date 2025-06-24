/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commoncluster "github.com/AMD-AIG-AIMA/SAFE/common/pkg/cluster"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

const (
	ResourceAdd    = "add"
	ResourceDel    = "delete"
	ResourceUpdate = "update"
)

type ResourceHandler controller.QueueHandler[*resourceMessage]

type ClusterInformer struct {
	ctx context.Context
	// cluster name
	name string
	// the k8s client used in admin plane
	adminClient client.Client
	// set of k8s clients used in the data plane
	dataClientFactory *commonclient.ClientFactory
	// used to process Kubernetes resource events
	handler ResourceHandler
	// Informer cache for cluster resources such as Pod, Job, and Event.
	// Key is the GVK, value is the informer instance.
	// it is controlled by resource template
	resourceInformers *commonutils.ObjectManager
}

type resourceInformer struct {
	informers.GenericInformer
	context context.Context
	cancel  context.CancelFunc
}

type resourceMessage struct {
	cluster    string
	name       string
	namespace  string
	uid        apitypes.UID
	gvk        schema.GroupVersionKind
	action     string
	workloadId string
	// dispatch count for this message — note that messages can be redelivered due to failover
	dispatchCount int
}

func newClusterInformer(ctx context.Context, name string, controlPlane *v1.ControlPlaneStatus,
	adminClient client.Client, handler ResourceHandler) (*ClusterInformer, error) {
	if controlPlane == nil {
		return nil, fmt.Errorf("controlPlane is empty")
	}
	endpoint, err := commoncluster.GetEndpoint(ctx, adminClient, name, controlPlane.Endpoints)
	if err != nil {
		return nil, err
	}
	clientFactory, err := commonclient.NewClientFactory(ctx, name, endpoint,
		controlPlane.CertData, controlPlane.KeyData, controlPlane.CAData, commonclient.EnableDynamicInformer)
	if err != nil {
		return nil, err
	}
	return &ClusterInformer{
		ctx:               ctx,
		name:              name,
		adminClient:       adminClient,
		dataClientFactory: clientFactory,
		handler:           handler,
		resourceInformers: commonutils.NewObjectManager(),
	}, nil
}

func (r *ClusterInformer) ClientFactory() *commonclient.ClientFactory {
	return r.dataClientFactory
}

// Get the resource informer, and if an error occurs, retrieve the detailed error reason.
func (r *ClusterInformer) GetResourceInformer(ctx context.Context, gvk schema.GroupVersionKind) (informers.GenericInformer, error) {
	informer := r.getResourceInformer(gvk)
	if informer != nil {
		return informer.GenericInformer, nil
	}
	if _, err := jobutils.GetResourceTemplate(ctx, r.adminClient, gvk); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("failed to find informer, gvk: %v", gvk)
}

func (r *ClusterInformer) getResourceInformer(gvk schema.GroupVersionKind) *resourceInformer {
	obj, ok := r.resourceInformers.Get(gvk.String())
	if !ok {
		return nil
	}
	informer, ok := obj.(*resourceInformer)
	if !ok {
		return nil
	}
	return informer
}

func (r *ClusterInformer) addResourceTemplate(rt *v1.ResourceTemplate) error {
	gvk := rt.ToSchemaGVK()
	mapper, err := r.adminClient.RESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		klog.ErrorS(err, "failed to do mapping", "gvk", gvk)
		return err
	}
	ctx, cancel := context.WithCancel(r.ctx)

	informer := &resourceInformer{
		GenericInformer: r.dataClientFactory.DynamicSharedInformerFactory().ForResource(mapper.Resource),
		context:         ctx,
		cancel:          cancel,
	}
	_, err = informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			r.handleResource(ctx, nil, obj, ResourceAdd)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			r.handleResource(ctx, oldObj, newObj, ResourceUpdate)
		},
		DeleteFunc: func(obj interface{}) {
			r.handleResource(ctx, obj, obj, ResourceDel)
		},
	})
	if err != nil {
		klog.ErrorS(err, "failed to add event handler for resource informer",
			"cluster", r.name, "gvk", gvk)
		return err
	}

	if r.resourceInformers.Add(gvk.String(), informer) == nil {
		informer.start()
		klog.Infof("start resource syncer, cluster: %s, gvr: %s, kind: %s",
			r.name, mapper.Resource.String(), gvk.Kind)
	}
	return nil
}

func (r *ClusterInformer) handleResource(ctx context.Context, oldObj, newObj interface{}, action string) {
	newUnstructured, ok := newObj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	if !r.checkNamespace(ctx, newUnstructured.GetNamespace()) {
		return
	}
	msg := &resourceMessage{
		cluster:       r.name,
		name:          newUnstructured.GetName(),
		namespace:     newUnstructured.GetNamespace(),
		uid:           newUnstructured.GetUID(),
		gvk:           newUnstructured.GroupVersionKind(),
		action:        action,
		dispatchCount: 0,
	}
	if newUnstructured.GetKind() == common.EventKind {
		if isCaredPodEvent(newUnstructured) {
			r.handler(msg)
		}
		return
	}

	// Only resources dispatched by this system are currently synchronized; others are ignored
	if msg.workloadId = v1.GetWorkloadId(newUnstructured); msg.workloadId == "" {
		return
	}
	strCount := newUnstructured.GetLabels()[v1.WorkloadDispatchCntLabel]
	if n, err := strconv.Atoi(strCount); err == nil {
		msg.dispatchCount = n
	}

	switch action {
	case ResourceAdd:
		klog.Infof("create object: %s/%s, workload:%s, kind: %s, generation: %d",
			newUnstructured.GetNamespace(), newUnstructured.GetName(),
			msg.workloadId, msg.gvk.Kind, newUnstructured.GetGeneration())
	case ResourceDel:
		if oldUnstructured, ok := oldObj.(*unstructured.Unstructured); ok {
			klog.Infof("delete object: %s/%s, workload:%s, kind: %s, generation: %d",
				oldUnstructured.GetNamespace(), oldUnstructured.GetName(),
				msg.workloadId, msg.gvk.Kind, oldUnstructured.GetGeneration())
		}
	}
	r.handler(msg)
}

// Ignore the resource if its namespace does not belong to the current workspace
func (r *ClusterInformer) checkNamespace(ctx context.Context, namespace string) bool {
	workspace := &v1.Workspace{}
	if err := r.adminClient.Get(ctx, client.ObjectKey{Name: namespace}, workspace); err != nil {
		return false
	}
	return true
}

func (r *ClusterInformer) delResourceTemplate(rt *v1.ResourceTemplate) {
	gvk := rt.ToSchemaGVK()
	if err := r.resourceInformers.Delete(gvk.String()); err != nil {
		klog.ErrorS(err, "failed to delete resource informer", "gvk", gvk)
	}
	klog.Infof("delete resource informer, cluster: %s, gvk :%s", r.name, gvk)
}

func (r *ClusterInformer) Release() error {
	r.resourceInformers.Clear()
	return nil
}

func (r *resourceInformer) start() {
	go r.Informer().Run(r.context.Done())
}

func (r *resourceInformer) Release() error {
	r.cancel()
	return nil
}

func GetClusterInformer(clusterInformers *commonutils.ObjectManager, name string) (*ClusterInformer, error) {
	obj, ok := clusterInformers.Get(name)
	if !ok {
		return nil, fmt.Errorf("failed to get cluster informer, name: %s", name)
	}
	informer, ok := obj.(*ClusterInformer)
	if !ok {
		return nil, fmt.Errorf("failed to get cluster informer, name: %s", name)
	}
	return informer, nil
}
