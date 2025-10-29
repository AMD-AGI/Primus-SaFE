/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	rookv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

type StorageClusterController struct {
	client.Client
	*storageClusters
	defaultStorageCluster *storageCluster
	queue                 v1.RequestWorkQueue
}

// SetupStorageClusterController initializes and registers the StorageClusterController with the controller manager
func SetupStorageClusterController(mgr manager.Manager) error {
	r := &StorageClusterController{
		Client:                mgr.GetClient(),
		storageClusters:       newStorageClusters(),
		defaultStorageCluster: nil,
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.StorageCluster{}, builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		Watches(&v1.Cluster{}, r.handleClusterEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Storage Cluster Controller successfully")
	return nil
}

// handleClusterEvent creates an event handler that enqueues StorageCluster requests when related Cluster resources change
func (r *StorageClusterController) handleClusterEvent() handler.EventHandler {
	enqueue := func(kc *v1.Cluster, queue v1.RequestWorkQueue) {
		added := map[string]struct{}{}
		for _, s := range kc.Spec.Storages {
			added[s.StorageCluster] = struct{}{}
			queue.Add(reconcile.Request{
				types.NamespacedName{Name: s.StorageCluster},
			})
		}
		for _, s := range kc.Status.StorageStatus {
			if _, ok := added[s.StorageCluster]; !ok {
				queue.Add(reconcile.Request{
					types.NamespacedName{Name: s.StorageCluster},
				})
			}
		}
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, event event.CreateEvent, queue v1.RequestWorkQueue) {
			if r.queue == nil {
				r.queue = queue
			}
			kc, ok := event.Object.(*v1.Cluster)
			if !ok {
				return
			}
			enqueue(kc, queue)
		},
		UpdateFunc: func(ctx context.Context, updateEvent event.UpdateEvent, queue v1.RequestWorkQueue) {
			newKC, ok := updateEvent.ObjectNew.(*v1.Cluster)
			if !ok {
				return
			}
			oldKC, ok := updateEvent.ObjectOld.(*v1.Cluster)
			if !ok {
				return
			}
			if newKC.Generation != oldKC.Generation {
				enqueue(newKC, queue)
			}
		},
		DeleteFunc: func(ctx context.Context, event event.DeleteEvent, queue v1.RequestWorkQueue) {
			kc, ok := event.Object.(*v1.Cluster)
			if !ok {
				return
			}
			enqueue(kc, queue)
		},
		GenericFunc: nil,
	}
}

// Reconcile is the main control loop for StorageCluster resources
func (r *StorageClusterController) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	cluster := new(v1.StorageCluster)
	err := r.Get(ctx, req.NamespacedName, cluster)
	if err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if err = r.addFinalizer(ctx, cluster); err != nil {
		return ctrlruntime.Result{}, err
	}

	scluster, err := r.getStorageCluster(ctx, cluster)
	if err != nil {
		if errors.IsNotFound(err) && !cluster.DeletionTimestamp.IsZero() {
			if err = utils.RemoveFinalizer(ctx, r.Client, cluster); err != nil {
				return ctrlruntime.Result{}, err
			}
			return ctrlruntime.Result{}, nil
		}
		klog.ErrorS(err, "failed to get StorageCluster")
		return ctrlruntime.Result{Requeue: true, RequeueAfter: time.Second * 5}, nil
	}
	if scluster == nil {
		return ctrlruntime.Result{}, fmt.Errorf("storage cluster %s not found", cluster.Name)
	}

	if !cluster.DeletionTimestamp.IsZero() {
		return ctrlruntime.Result{}, r.delete(ctx, cluster, scluster)
	}

	return ctrlruntime.Result{}, r.processCluster(ctx, cluster, scluster)
}

// delete handles the deletion of a StorageCluster resource by cleaning up associated resources and removing the finalizer
func (r *StorageClusterController) delete(ctx context.Context, cluster *v1.StorageCluster, scluster *storageCluster) error {
	clusters := new(v1.ClusterList)
	err := r.List(ctx, clusters)
	if err != nil {
		return fmt.Errorf("failed to list storage cluster: %v", err)
	}
	if err = scluster.delete(ctx, cluster, clusters); err != nil {
		return err
	}
	if err = utils.RemoveFinalizer(ctx, r.Client, cluster); err != nil {
		return err
	}
	return nil
}

// processCluster handles the main processing logic for a StorageCluster resource
func (r *StorageClusterController) processCluster(ctx context.Context, cluster *v1.StorageCluster, scluster *storageCluster) error {
	originalCluster := client.MergeFrom(cluster.DeepCopy())
	status := cluster.Status.DeepCopy()
	cephCluster, err := scluster.getCephCluster(ctx, cluster)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(status, cluster.Status) {
		err = r.Status().Patch(ctx, cluster, originalCluster)
		if err != nil {
			return err
		}
	}
	if cephCluster.Status.Phase != rookv1.ConditionReady {
		return nil
	}
	clusters := new(v1.ClusterList)
	if err = r.List(ctx, clusters); err != nil {
		return err
	}

	for _, cc := range clusters.Items {
		kc := cc.DeepCopy()
		oldStatus := kc.DeepCopy().Status.StorageStatus
		for _, s := range kc.Spec.Storages {
			if s.StorageCluster == cluster.Name {
				stat, err := scluster.getStorage(ctx, cluster, kc, s)
				if err != nil {
					return err
				}
				// klog.Infof("storage cluster %s status %+v", cluster.Name, stat)
				updateStorageStatus(kc, *stat)
			}
		}
		for _, stat := range kc.Status.StorageStatus {
			if stat.StorageCluster == cluster.Name && stat.Ref == nil {
				// klog.Infof("kubernetes cluster storage %s ", stat.Name)
				if _, ok := kc.GetStorage(stat.Name); !ok {
					err = scluster.deleteStorage(ctx, cluster, kc, v1.Storage{
						Name:           stat.Name,
						Type:           stat.Type,
						StorageCluster: stat.StorageCluster,
					})
					if err != nil {
						return err
					}
					kc.DeleteStorageStatus(stat.Name)
				}
			}
		}
		if !reflect.DeepEqual(oldStatus, kc.Status.StorageStatus) {
			err = r.Status().Update(ctx, kc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// updateCephCsiConfig updates the Ceph CSI configuration with cluster monitor information
func (r *StorageClusterController) updateCephCsiConfig(ctx context.Context, cluster *v1.StorageCluster, scluster *storageCluster) error {
	configMap, err := scluster.clientset.CoreV1().ConfigMaps(cephCSIRBDNamespace).Get(ctx, cephCSIRBDName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get ceph csi configmap %s failed %+v", cephCSIRBDName, err)
	}
	var infos []ClusterInfo
	if conf, ok := configMap.Data["config.json"]; ok {
		err = json.Unmarshal([]byte(conf), &infos)
		if err != nil {
			return fmt.Errorf("unmarshal ceph csi ceph-csi-config %s failed %+v", cephCSIRBDName, err)
		}
	}
	index := -1
	for k, info := range infos {
		if info.ClusterID == cluster.Status.CephClusterStatus.ClusterId {
			if !reflect.DeepEqual(infos[k].Monitors, cluster.Status.CephClusterStatus.Monitors) {
				index = k
				break
			} else {
				return nil
			}
		}
	}
	if index == -1 {
		infos = append(infos, ClusterInfo{
			ClusterID:    cluster.Status.CephClusterStatus.ClusterId,
			Monitors:     cluster.Status.CephClusterStatus.Monitors,
			CephFS:       CephFS{},
			RBD:          RBD{},
			NFS:          NFS{},
			ReadAffinity: ReadAffinity{},
		})
	} else {
		infos[index].Monitors = cluster.Status.CephClusterStatus.Monitors
	}
	klog.Infof("update ceph csi config monitors %+v", cluster.Status.CephClusterStatus.Monitors)
	info, err := json.Marshal(infos)
	if err != nil {
		return fmt.Errorf("marshal ceph csi ceph-csi-config %s failed %+v", cephCSIRBDName, err)
	}
	configMap.Data["config.json"] = string(info)
	configMap, err = scluster.clientset.CoreV1().ConfigMaps(configMap.Namespace).Update(ctx, configMap, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update ceph csi ceph-csi-config failed %+v", err)
	}
	return nil
}

// getStorageClusterByName retrieves a storage cluster by name, creating it if not found in cache
func (r *StorageClusterController) getStorageClusterByName(ctx context.Context, name string) (*storageCluster, error) {
	sc, ok := r.storageClusters.get(name)
	if ok {
		return sc, nil
	}
	cluster := new(v1.Cluster)
	err := r.Get(ctx, types.NamespacedName{Name: name}, cluster)
	if err != nil {
		return nil, err
	}
	if r.queue == nil {
		return nil, fmt.Errorf("queue is nil")
	}
	sc, err = newStorageCluster(ctx, r.Client, cluster, r.queue, make(chan struct{}))
	if err != nil {
		return nil, err
	}
	r.storageClusters.add(name, sc)
	return sc, nil
}

// addFinalizer adds the storage finalizer to the StorageCluster resource if not already present
func (r *StorageClusterController) addFinalizer(ctx context.Context, cluster *v1.StorageCluster) error {
	for _, v := range cluster.Finalizers {
		if v == v1.StorageFinalizer {
			return nil
		}
	}
	klog.Infof("addFinalizer %s ResourceVersion %s", cluster.Name, cluster.ResourceVersion)
	cluster.Finalizers = append(cluster.Finalizers, v1.StorageFinalizer)
	return r.Update(ctx, cluster)
}

// getStorageCluster retrieves the appropriate storage cluster based on StorageCluster spec or default configuration
func (r *StorageClusterController) getStorageCluster(ctx context.Context, sc *v1.StorageCluster) (*storageCluster, error) {
	if r.queue == nil {
		return nil, fmt.Errorf("queue is nil")
	}
	if sc.Spec.Cluster != "" {
		return r.getStorageClusterByName(ctx, sc.Spec.Cluster)
	}

	if r.defaultStorageCluster != nil {
		return r.defaultStorageCluster, nil
	}

	list := new(v1.ClusterList)
	err := r.List(ctx, list)
	if err != nil {
		return nil, err
	}

	for _, cluster := range list.Items {
		for key := range cluster.Labels {
			if key == v1.StorageDefaultClusterLabel {
				scluster, ok := r.storageClusters.get(cluster.Name)
				if ok {
					return scluster, nil
				}
				scluster, err := newStorageCluster(ctx, r.Client, &cluster, r.queue, make(chan struct{}))
				if err != nil {
					return nil, fmt.Errorf("error getting default storage cluster: %v", err)
				}
				r.defaultStorageCluster = scluster
				r.storageClusters.add(cluster.Name, scluster)
				return scluster, nil
			}
		}
	}
	return nil, fmt.Errorf("error getting default storage cluster: %v", err)
}

// updateStorageStatus updates the storage status in the Cluster resource
func updateStorageStatus(kc *v1.Cluster, s v1.StorageStatus) {
	for i, stats := range kc.Status.StorageStatus {
		if stats.Name == s.Name {
			kc.Status.StorageStatus[i].SecretKey = s.SecretKey
			return
		}
	}
	kc.Status.StorageStatus = append(kc.Status.StorageStatus, s)
}
