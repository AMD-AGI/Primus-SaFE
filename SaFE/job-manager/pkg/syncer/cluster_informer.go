/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
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

// ClusterInformer manages informers for Kubernetes resources in a specific cluster
// It handles resource events and synchronizes them between admin plane and data plane
type ClusterInformer struct {
	ctx context.Context
	// cluster name
	name string
	// The k8s client used in admin plane
	adminClient client.Client
	// sets of k8s clients used in the data plane
	dataClientFactory *commonclient.ClientFactory
	// used to process Kubernetes resource events
	handler ResourceHandler
	// Informer cache for cluster resources e.g. Pod, Job, and Event.
	// Key is the GVK, value is the informer instance.
	// it is controlled by resource template
	resourceInformers *commonutils.ObjectManager
}

// resourceInformer wraps a GenericInformer with context management for lifecycle control
type resourceInformer struct {
	informers.GenericInformer
	context  context.Context
	cancel   context.CancelFunc
	isExited bool
}

// resourceMessage represents a resource event message containing details about the event
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

// newClusterInformer creates and initializes a new ClusterInformer instance.
func newClusterInformer(ctx context.Context, cluster *v1.Cluster,
	adminClient client.Client, handler ResourceHandler) (*ClusterInformer, error) {
	controlPlane := &cluster.Status.ControlPlaneStatus
	if controlPlane == nil {
		return nil, fmt.Errorf("controlPlane is empty")
	}
	endpoint, err := commoncluster.GetEndpoint(ctx, adminClient, cluster)
	if err != nil {
		return nil, err
	}
	clientFactory, err := commonclient.NewClientFactory(ctx, cluster.Name, endpoint,
		controlPlane.CertData, controlPlane.KeyData, controlPlane.CAData, commonclient.EnableDynamicInformer)
	if err != nil {
		return nil, err
	}
	klog.Infof("create cluster informer, cluster: %s, endpoint: %s", cluster.Name, endpoint)
	return &ClusterInformer{
		ctx:               ctx,
		name:              cluster.Name,
		adminClient:       adminClient,
		dataClientFactory: clientFactory,
		handler:           handler,
		resourceInformers: commonutils.NewObjectManager(),
	}, nil
}

// ClientFactory returns the data plane client factory.
func (r *ClusterInformer) ClientFactory() *commonclient.ClientFactory {
	return r.dataClientFactory
}

// GetResourceInformer retrieves the resource informer for a given GVK.
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

// getResourceInformer retrieves the internal resource informer for a given GVK.
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

// addResourceTemplate adds a resource template and creates corresponding informer.
func (r *ClusterInformer) addResourceTemplate(gvk schema.GroupVersionKind) error {
	if r.resourceInformers.Has(gvk.String()) {
		return nil
	}
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

// handleResource processes resource events (add, update, delete).
func (r *ClusterInformer) handleResource(_ context.Context, oldObj, newObj interface{}, action string) {
	newUnstructured, ok := newObj.(*unstructured.Unstructured)
	if !ok {
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
		if isRelevantPodEvent(newUnstructured) {
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
		klog.Infof("create object: %s/%s, uid: %s, workload:%s, kind: %s, generation: %d, dispatch.cnt: %d",
			newUnstructured.GetNamespace(), newUnstructured.GetName(), newUnstructured.GetUID(),
			msg.workloadId, msg.gvk.Kind, newUnstructured.GetGeneration(), msg.dispatchCount)
	case ResourceDel:
		if oldUnstructured, ok := oldObj.(*unstructured.Unstructured); ok {
			klog.Infof("delete object: %s/%s, uid: %s, workload:%s, kind: %s, generation: %d, dispatch.cnt: %d",
				oldUnstructured.GetNamespace(), oldUnstructured.GetName(), oldUnstructured.GetUID(),
				msg.workloadId, msg.gvk.Kind, oldUnstructured.GetGeneration(), msg.dispatchCount)
		}
	}
	r.handler(msg)
}

// delResourceTemplate removes a resource template and its corresponding informer.
func (r *ClusterInformer) delResourceTemplate(gvk schema.GroupVersionKind) {
	if err := r.resourceInformers.Delete(gvk.String()); err != nil {
		klog.ErrorS(err, "failed to delete resource informer", "gvk", gvk)
	}
	klog.Infof("delete resource informer, cluster: %s, gvk: %s", r.name, gvk.String())
}

// Release cleans up all resources associated with the ClusterInformer.
// it implements the interface of commonutils.Object.
func (r *ClusterInformer) Release() error {
	r.resourceInformers.Clear()
	return nil
}

// start begins running the informer in a separate goroutine.
func (r *resourceInformer) start() {
	go r.Informer().Run(r.context.Done())
}

// Release cleans up resources associated with the resourceInformer.
func (r *resourceInformer) Release() error {
	if r.isExited {
		return nil
	}
	r.cancel()
	r.isExited = true
	return nil
}

// GetClusterInformer retrieves a ClusterInformer by name from the ObjectManager.
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
