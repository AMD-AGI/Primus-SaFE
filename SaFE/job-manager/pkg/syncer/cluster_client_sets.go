/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commoncluster "github.com/AMD-AIG-AIMA/SAFE/common/pkg/cluster"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

const (
	ResourceAdd      = "add"
	ResourceDel      = "delete"
	ResourceUpdate   = "update"
	ResourceDeleting = "deleting"
)

type ResourceHandler controller.QueueHandler[*resourceMessage]

// ClusterClientSets manages informers and clients for Kubernetes resources in a specific cluster
// It handles resource events and synchronizes them between admin plane and data plane
type ClusterClientSets struct {
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
	// guards addResourceTemplate / delResourceTemplate against concurrent setup.
	templateMu sync.Mutex
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
	groupId    string
	// dispatch count for this message — note that messages can be redelivered due to failover
	dispatchCount int
}

// newClusterClientSets creates and initializes a new ClusterClientSets instance.
func newClusterClientSets(ctx context.Context, cluster *v1.Cluster,
	adminClient client.Client, handler ResourceHandler) (*ClusterClientSets, error) {
	controlPlane := &cluster.Status.ControlPlaneStatus
	if controlPlane == nil {
		return nil, fmt.Errorf("controlPlane is empty")
	}
	endpoint, err := commoncluster.GetEndpoint(ctx, adminClient, cluster)
	if err != nil {
		return nil, err
	}
	clientFactory, err := commoncluster.NewClientFactoryForCluster(ctx, adminClient, cluster,
		commonclient.EnableDynamicInformer)
	if err != nil {
		return nil, err
	}
	klog.Infof("create cluster client sets, cluster: %s, endpoint: %s, selected: %s",
		cluster.Name, endpoint, clientFactory.Endpoint())
	return &ClusterClientSets{
		ctx:               ctx,
		name:              cluster.Name,
		adminClient:       adminClient,
		dataClientFactory: clientFactory,
		handler:           handler,
		resourceInformers: commonutils.NewObjectManager(),
	}, nil
}

func (r *ClusterClientSets) SetName(name string) {
	r.name = name
}

func (r *ClusterClientSets) SetClientFactory(factory *commonclient.ClientFactory) {
	r.dataClientFactory = factory
}

// ClientFactory returns the data plane client factory.
func (r *ClusterClientSets) ClientFactory() *commonclient.ClientFactory {
	return r.dataClientFactory
}

// GetResourceInformer retrieves the resource informer for a given GVK.
func (r *ClusterClientSets) GetResourceInformer(_ context.Context, gvk schema.GroupVersionKind) (informers.GenericInformer, error) {
	informer := r.getResourceInformer(gvk)
	if informer != nil {
		return informer.GenericInformer, nil
	}
	return nil, fmt.Errorf("failed to find informer, gvk: %v", gvk)
}

// getResourceInformer retrieves the internal resource informer for a given GVK.
func (r *ClusterClientSets) getResourceInformer(gvk schema.GroupVersionKind) *resourceInformer {
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

// informerCount returns the number of running resource informers.
func (r *ClusterClientSets) informerCount() int {
	return r.resourceInformers.Len()
}

// needsInformerRetry reports whether any required resource template informer is still missing.
// Missing informers are retried periodically so CRDs installed after startup (e.g. Sandbox) can
// be picked up without restarting job-manager.
func (r *ClusterClientSets) needsInformerRetry(rtList *v1.ResourceTemplateList) bool {
	if rtList == nil || len(rtList.Items) == 0 {
		return false
	}
	for i := range rtList.Items {
		gvk := rtList.Items[i].ToSchemaGVK()
		if !r.resourceInformers.Has(gvk.String()) {
			return true
		}
	}
	return false
}

// addResourceTemplate adds a resource template and creates corresponding informer.
func (r *ClusterClientSets) addResourceTemplate(gvk schema.GroupVersionKind) error {
	r.templateMu.Lock()
	defer r.templateMu.Unlock()
	if r.resourceInformers.Has(gvk.String()) {
		return nil
	}
	// Resolve GVR against the data-plane cluster: the informer targets the data plane,
	// and the CRD may not be installed on the admin (management) cluster.
	mapper, err := r.dataClientFactory.Mapper().RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		if apimeta.IsNoMatchError(err) {
			// CRD is not installed yet; retry on the next maintain cycle in case it is added later.
			klog.V(2).InfoS("resource template pending: CRD not installed on cluster",
				"cluster", r.name, "gvk", gvk)
			return nil
		}
		klog.ErrorS(err, "failed to do mapping", "cluster", r.name, "gvk", gvk)
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
			if !r.dataClientFactory.IsValid() {
				r.dataClientFactory.SetValid(true, "")
			}
			r.handleResource(ctx, nil, obj, ResourceAdd)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if !r.dataClientFactory.IsValid() {
				r.dataClientFactory.SetValid(true, "")
			}
			r.handleResource(ctx, oldObj, newObj, ResourceUpdate)
		},
		DeleteFunc: func(obj interface{}) {
			if !r.dataClientFactory.IsValid() {
				r.dataClientFactory.SetValid(true, "")
			}
			r.handleResource(ctx, obj, obj, ResourceDel)
		},
	})
	if err != nil {
		klog.ErrorS(err, "failed to add event handler for resource informer",
			"cluster", r.name, "gvk", gvk)
		return err
	}
	if err = informer.Informer().SetWatchErrorHandler(commonclient.WatchErrorHandler(r.ctx, r.dataClientFactory)); err != nil {
		klog.ErrorS(err, "failed to set watch error handler", "cluster", r.name, "gvk", gvk)
		return err
	}

	if r.resourceInformers.Add(gvk.String(), informer) == nil {
		informer.start()
		klog.Infof("start resource syncer, cluster: %s, gvr: %s, kind: %s",
			r.name, mapper.Resource.String(), gvk.Kind)
	}
	return nil
}

// toUnstructured extracts an unstructured object from an informer event payload.
// Relist-driven delete events arrive as cache.DeletedFinalStateUnknown tombstones.
func toUnstructured(obj interface{}) (*unstructured.Unstructured, bool) {
	if obj == nil {
		return nil, false
	}
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}
	u, ok := obj.(*unstructured.Unstructured)
	return u, ok
}

// handleResource processes resource events (add, update, delete).
func (r *ClusterClientSets) handleResource(_ context.Context, oldObj, newObj interface{}, action string) {
	newUnstructured, ok := toUnstructured(newObj)
	if !ok {
		if action == ResourceDel {
			klog.InfoS("ignored delete event with unsupported object type",
				"cluster", r.name, "type", fmt.Sprintf("%T", newObj))
		}
		return
	}
	msg := &resourceMessage{
		cluster:       r.name,
		name:          newUnstructured.GetName(),
		namespace:     newUnstructured.GetNamespace(),
		uid:           newUnstructured.GetUID(),
		gvk:           newUnstructured.GroupVersionKind(),
		action:        action,
		dispatchCount: 1,
	}
	if msg.action != ResourceDel && !newUnstructured.GetDeletionTimestamp().IsZero() {
		msg.action = ResourceDeleting
	}

	// Only resources dispatched by this system are currently synchronized; others are ignored
	if msg.workloadId = v1.GetWorkloadId(newUnstructured); msg.workloadId == "" {
		if newUnstructured.GetLabels()[monarchMeshLabel] == "" {
			return
		}
	}
	if strCount := newUnstructured.GetLabels()[v1.WorkloadDispatchCntLabel]; strCount != "" {
		if n, err := strconv.Atoi(strCount); err == nil {
			msg.dispatchCount = n
		}
	}
	msg.groupId = v1.GetGroupId(newUnstructured)

	switch msg.action {
	case ResourceAdd:
		klog.Infof("object: %s/%s is created, uid: %s, kind: %s, generation: %d, workload: %s, dispatch.cnt: %d",
			newUnstructured.GetNamespace(), newUnstructured.GetName(), newUnstructured.GetUID(),
			msg.gvk.Kind, newUnstructured.GetGeneration(), msg.workloadId, msg.dispatchCount)
	case ResourceDel:
		if oldUnstructured, ok := toUnstructured(oldObj); ok {
			klog.Infof("object: %s/%s is deleted, uid: %s, kind: %s, generation: %d, workload: %s, dispatch.cnt: %d",
				oldUnstructured.GetNamespace(), oldUnstructured.GetName(), oldUnstructured.GetUID(),
				msg.gvk.Kind, oldUnstructured.GetGeneration(), msg.workloadId, msg.dispatchCount)
		}
	}
	r.syncGithubAnnotations(newUnstructured)

	r.handler(msg)
}

// delResourceTemplate removes a resource template and its corresponding informer.
func (r *ClusterClientSets) delResourceTemplate(gvk schema.GroupVersionKind) {
	r.templateMu.Lock()
	defer r.templateMu.Unlock()
	if err := r.resourceInformers.Delete(gvk.String()); err != nil {
		klog.ErrorS(err, "failed to delete resource informer", "gvk", gvk)
	}
	klog.Infof("delete resource informer, cluster: %s, gvk: %s", r.name, gvk.String())
}

// Release cleans up all resources associated with the ClusterClientSets.
// it implements the interface of commonutils.Object.
func (r *ClusterClientSets) Release() error {
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

// needsClientFactoryRefresh reports whether the data-plane client factory should be rebuilt.
func (r *ClusterClientSets) needsClientFactoryRefresh(ctx context.Context, cluster *v1.Cluster,
	adminClient client.Client) bool {
	if r.dataClientFactory == nil || !r.dataClientFactory.IsValid() {
		return true
	}
	return commoncluster.ClientFactoryNeedsRefresh(ctx, adminClient, cluster, r.dataClientFactory)
}

// recreateClientFactory rebuilds the data-plane client and clears informers.
func (r *ClusterClientSets) recreateClientFactory(ctx context.Context, cluster *v1.Cluster,
	adminClient client.Client) error {
	if r.dataClientFactory != nil {
		_ = r.dataClientFactory.Release()
	}
	factory, err := commoncluster.NewClientFactoryForCluster(ctx, adminClient, cluster,
		commonclient.EnableDynamicInformer)
	if err != nil {
		return err
	}
	r.dataClientFactory = factory
	r.resourceInformers.Clear()
	klog.Infof("recreated cluster client factory, cluster: %s, endpoint: %s", r.name, factory.Endpoint())
	return nil
}

// GetClusterClientSets retrieves a ClusterClientSets by name from the ObjectManager.
func GetClusterClientSets(managers *commonutils.ObjectManager, name string) (*ClusterClientSets, error) {
	obj, ok := managers.Get(name)
	if !ok {
		return nil, fmt.Errorf("failed to get cluster clientSet, name: %s", name)
	}
	clientSets, ok := obj.(*ClusterClientSets)
	if !ok {
		return nil, fmt.Errorf("failed to get cluster clientSet, name: %s", name)
	}
	return clientSets, nil
}
