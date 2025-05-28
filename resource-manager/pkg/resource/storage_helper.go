/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	rookv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rook "github.com/rook/rook/pkg/client/clientset/versioned"
	rookexter "github.com/rook/rook/pkg/client/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
)

const (
	cephCSIRBDNamespace = "ceph-csi-rbd"
	cephCSIRBDName      = "ceph-csi-config"
	cephImage           = "quay.io/ceph/ceph:v18.2.2"
)

type ClusterInfo struct {
	ClusterID    string       `json:"clusterID"`
	Monitors     []string     `json:"monitors"`
	CephFS       CephFS       `json:"cephFS"`
	RBD          RBD          `json:"rbd"`
	NFS          NFS          `json:"nfs"`
	ReadAffinity ReadAffinity `json:"readAffinity"`
}

type CephFS struct {
	NetNamespaceFilePath string `json:"netNamespaceFilePath"`
	SubvolumeGroup       string `json:"subvolumeGroup"`
	KernelMountOptions   string `json:"kernelMountOptions"`
	FuseMountOptions     string `json:"fuseMountOptions"`
}
type RBD struct {
	NetNamespaceFilePath string `json:"netNamespaceFilePath"`
	RadosNamespace       string `json:"radosNamespace"`
	MirrorDaemonCount    int    `json:"mirrorDaemonCount"`
}

type NFS struct {
	NetNamespaceFilePath string `json:"netNamespaceFilePath"`
}

type ReadAffinity struct {
	Enabled             bool     `json:"enabled"`
	CrushLocationLabels []string `json:"crushLocationLabels"`
}

type storageClusters struct {
	clusters map[string]*storageCluster
	sync.Mutex
}

func newStorageClusters() *storageClusters {
	return &storageClusters{
		clusters: make(map[string]*storageCluster),
	}
}

type storageCluster struct {
	name          string
	clientset     kubernetes.Interface
	rookClientset *rook.Clientset
	cephInformer  rookexter.SharedInformerFactory
	queue         v1.RequestWorkQueue
}

var mgrResources = corev1.ResourceRequirements{
	Limits: corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("8"),
		corev1.ResourceMemory: resource.MustParse("16Gi"),
	},
	Requests: corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("8"),
		corev1.ResourceMemory: resource.MustParse("16Gi"),
	},
}

var hddResources = corev1.ResourceRequirements{
	Limits: corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("2"),
		corev1.ResourceMemory: resource.MustParse("4Gi"),
	},
	Requests: corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("2"),
		corev1.ResourceMemory: resource.MustParse("4Gi"),
	},
}

var ssdResources = corev1.ResourceRequirements{
	Limits: corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4"),
		corev1.ResourceMemory: resource.MustParse("16Gi"),
	},
	Requests: corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("2"),
		corev1.ResourceMemory: resource.MustParse("16Gi"),
	},
}

var nvmeResources = corev1.ResourceRequirements{
	Limits: corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4"),
		corev1.ResourceMemory: resource.MustParse("16Gi"),
	},
	Requests: corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4"),
		corev1.ResourceMemory: resource.MustParse("16Gi"),
	},
}

type GetQueue func() v1.RequestWorkQueue

func newStorageCluster(ctx context.Context, cluster *v1.Cluster, queue v1.RequestWorkQueue, stopCh chan struct{}) (*storageCluster, error) {
	if queue == nil {
		return nil, fmt.Errorf("queue is nil")
	}
	stat := cluster.Status.ControlPlaneStatus
	_, config, err := k8sclient.NewClientSet(fmt.Sprintf("https://%s.%s.svc", cluster.Name, common.PrimusSafeNamespace),
		stat.CertData, stat.KeyData, stat.CAData, true)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	rClient, err := rook.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	_, err = rClient.CephV1().CephClusters("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	informer := rookexter.NewSharedInformerFactory(rClient, time.Hour)
	queueByCephCluster := func(obj interface{}) {
		cc, ok := obj.(*rookv1.CephCluster)
		if !ok {
			return
		}
		// klog.Infof("queueByCephCluster ResourceVersion %s CephCluster storage cluster  %s ", cc.ResourceVersion, cc.Name)
		queue.Add(ctrlruntime.Request{types.NamespacedName{Name: cc.Name}})
	}
	_, err = informer.Ceph().V1().CephClusters().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			queueByCephCluster(obj)
		},
		UpdateFunc: func(_, obj interface{}) {
			queueByCephCluster(obj)
		},
		DeleteFunc: func(obj interface{}) {
			queueByCephCluster(obj)
		},
	})
	if err != nil {
		return nil, err
	}
	queueByCephObjectStore := func(obj interface{}) {
		obs, ok := obj.(*rookv1.CephObjectStore)
		if !ok {
			return
		}
		// klog.Infof("queueByCephObjectStore ResourceVersion %s", obs.ResourceVersion)
		if name, ok := obs.Labels[v1.StorageClusterNameLabel]; ok {
			// klog.Infof("CephObjectStore stroage cluster %s", name)
			queue.Add(ctrlruntime.Request{types.NamespacedName{Name: name}})
		}
	}
	_, err = informer.Ceph().V1().CephObjectStores().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			queueByCephObjectStore(obj)
		},
		UpdateFunc: func(_, obj interface{}) {
			queueByCephObjectStore(obj)
		},
		DeleteFunc: func(obj interface{}) {
			queueByCephObjectStore(obj)
		},
	})
	if err != nil {
		return nil, err
	}

	queueByCephObjectStoreUser := func(obj interface{}) {
		user, ok := obj.(*rookv1.CephObjectStoreUser)
		if !ok {
			return
		}
		klog.Infof("queueByCephObjectStoreUser ResourceVersion %s", user.ResourceVersion)
		if name, ok := user.Labels[v1.StorageClusterNameLabel]; ok {
			klog.Infof("queueByCephObjectStoreUser storage cluster %s", name)
			queue.Add(ctrlruntime.Request{types.NamespacedName{Name: name}})
		}
	}
	_, err = informer.Ceph().V1().CephObjectStoreUsers().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			queueByCephObjectStoreUser(obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			queueByCephObjectStoreUser(newObj)
		},
		DeleteFunc: func(obj interface{}) {
			queueByCephObjectStoreUser(obj)
		},
	})
	if err != nil {
		return nil, err
	}

	queueByCephBlockPool := func(obj interface{}) {
		pool, ok := obj.(*rookv1.CephBlockPool)
		if !ok {
			return
		}
		klog.Infof("queueByCephBlockPool CephBlockPool storage cluster %s ResourceVersion %s", pool.Name, pool.ResourceVersion)
		if name, ok := pool.Labels[v1.StorageClusterNameLabel]; ok {
			klog.Infof("queueByCephBlockPool storage cluster %s", name)
			queue.Add(ctrlruntime.Request{types.NamespacedName{Name: name}})
		}
	}
	_, err = informer.Ceph().V1().CephBlockPools().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			queueByCephBlockPool(obj)
		},
		UpdateFunc: func(_, newObj interface{}) {
			queueByCephBlockPool(newObj)
		},
		DeleteFunc: func(obj interface{}) {
			queueByCephBlockPool(obj)
		},
	})
	if err != nil {
		return nil, err
	}
	informer.Start(stopCh)
	sc := &storageCluster{
		clientset:     client,
		rookClientset: rClient,
		cephInformer:  informer,
		queue:         queue,
	}
	return sc, nil
}

func (s *storageClusters) get(name string) (*storageCluster, bool) {
	s.Lock()
	defer s.Unlock()
	sc, ok := s.clusters[name]
	return sc, ok
}

func (s *storageClusters) add(name string, sc *storageCluster) {
	s.Lock()
	defer s.Unlock()
	s.clusters[name] = sc
}

func (s *storageClusters) delete(name string) {
	s.Lock()
	defer s.Unlock()
	delete(s.clusters, name)
}

func (s *storageCluster) getCephCluster(ctx context.Context, cluster *v1.StorageCluster) (*rookv1.CephCluster, error) {
	cephCluster, err := s.rookClientset.CephV1().CephClusters(cluster.Name).Get(ctx, cluster.Name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get ceph cluster %s cluster: %v", cluster.Name, err)
		}
		ns, err := s.clientset.CoreV1().Namespaces().Get(ctx, cluster.Name, metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, err
			}
			ns = &corev1.Namespace{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name: cluster.Name,
				},
				Spec:   corev1.NamespaceSpec{},
				Status: corev1.NamespaceStatus{},
			}
			ns, err = s.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			if err != nil {
				if !errors.IsAlreadyExists(err) {
					return nil, fmt.Errorf("failed to create ceph cluster %s namespace %v", cluster.Name, err)
				}
			}
		}
		err = permissions(ctx, s.clientset, cluster.Name)
		if err != nil {
			return nil, err
		}

		monCount := 3
		if monCount > cluster.Spec.Count {
			monCount = 1
		} else if cluster.Spec.Count > 4 {
			monCount = 5
		}
		mgrCount := 3
		if mgrCount > cluster.Spec.Count {
			mgrCount = 1
		}

		cephNodes, err := s.getCephNodes(ctx, cluster.Spec.Count, cluster.Name, cluster.Spec.Flavor)
		if err != nil {
			return nil, err
		}
		klog.Infof("cephNodes %+v", cephNodes)
		image := cephImage
		if cluster.Spec.Image != nil {
			image = *cluster.Spec.Image
		}
		maxLogSize := resource.MustParse("600M")
		cephCluster = &rookv1.CephCluster{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Name,
				Labels: map[string]string{
					v1.StorageClusterNameLabel: cluster.Name,
				},
			},
			Spec: rookv1.ClusterSpec{
				CephVersion: rookv1.CephVersionSpec{
					Image:            image,
					AllowUnsupported: false,
					ImagePullPolicy:  "",
				},
				Storage: rookv1.StorageScopeSpec{
					Nodes:                        cephNodes,
					UseAllNodes:                  false,
					OnlyApplyOSDPlacement:        false,
					Config:                       nil,
					Selection:                    rookv1.Selection{},
					StorageClassDeviceSets:       nil,
					Store:                        rookv1.OSDStore{},
					FlappingRestartIntervalHours: 0,
				},
				Annotations: nil,
				Labels:      nil,
				Placement: rookv1.PlacementSpec{
					rookv1.KeyAll: rookv1.Placement{
						NodeAffinity: nodeAffinity(cluster.Name),
					},
				},
				Network: rookv1.NetworkSpec{
					Connections: &rookv1.ConnectionsSpec{
						Encryption: &rookv1.EncryptionSpec{
							Enabled: false,
						},
						Compression: &rookv1.CompressionSpec{
							Enabled: false,
						},
						RequireMsgr2: false,
					},
					Provider:    "",
					HostNetwork: true,
				},
				Resources: map[string]corev1.ResourceRequirements{
					"mgr":            mgrResources,
					"mon":            mgrResources,
					"osd":            hddResources,
					"osd-hdd":        hddResources,
					"osd-ssd":        ssdResources,
					"osd-nvme":       nvmeResources,
					"prepareosd":     corev1.ResourceRequirements{},
					"mgr-sidecar":    corev1.ResourceRequirements{},
					"crashcollector": corev1.ResourceRequirements{},
					"logcollector":   corev1.ResourceRequirements{},
					"cleanup":        corev1.ResourceRequirements{},
					"exporter":       corev1.ResourceRequirements{},
				},
				PriorityClassNames: rookv1.PriorityClassNamesSpec{
					rookv1.KeyMon: "system-node-critical",
					rookv1.KeyOSD: "system-node-critical",
					rookv1.KeyMgr: "system-cluster-critical",
				},
				DataDirHostPath:   "/var/lib/rook",
				SkipUpgradeChecks: false,
				ContinueUpgradeAfterChecksEvenIfNotHealthy: false,
				WaitTimeoutForHealthyOSDInMinutes:          0,
				DisruptionManagement:                       rookv1.DisruptionManagementSpec{},
				Mon: rookv1.MonSpec{
					Count:                monCount,
					AllowMultiplePerNode: false,
					FailureDomainLabel:   "",
					Zones:                nil,
					StretchCluster:       nil,
					VolumeClaimTemplate:  nil,
				},
				CrashCollector: rookv1.CrashCollectorSpec{
					Disable:      false,
					DaysToRetain: 0,
				},
				Dashboard:  rookv1.DashboardSpec{},
				Monitoring: rookv1.MonitoringSpec{},
				External:   rookv1.ExternalSpec{},
				Mgr: rookv1.MgrSpec{
					Count:                mgrCount,
					AllowMultiplePerNode: false,
					Modules:              nil,
				},
				RemoveOSDsIfOutAndSafeToRemove: false,
				CleanupPolicy:                  rookv1.CleanupPolicySpec{},
				HealthCheck: rookv1.CephClusterHealthCheckSpec{
					DaemonHealth: rookv1.DaemonHealthSpec{
						Status: rookv1.HealthCheckSpec{
							Disabled: false,
							Interval: &metav1.Duration{
								Duration: time.Minute,
							},
							Timeout: "45s",
						},
						Monitor: rookv1.HealthCheckSpec{
							Disabled: false,
							Interval: &metav1.Duration{
								Duration: time.Minute,
							},
							Timeout: "45s",
						},
						ObjectStorageDaemon: rookv1.HealthCheckSpec{
							Disabled: false,
							Interval: &metav1.Duration{
								Duration: time.Minute,
							},
							Timeout: "45s",
						},
					},
					LivenessProbe: map[rookv1.KeyType]*rookv1.ProbeSpec{
						rookv1.KeyMon: &rookv1.ProbeSpec{
							Disabled: false,
							Probe:    nil,
						},
						rookv1.KeyMgr: &rookv1.ProbeSpec{
							Disabled: false,
							Probe:    nil,
						},
						rookv1.KeyOSD: &rookv1.ProbeSpec{
							Disabled: false,
							Probe:    nil,
						},
					},
					StartupProbe: map[rookv1.KeyType]*rookv1.ProbeSpec{
						rookv1.KeyMon: &rookv1.ProbeSpec{
							Disabled: false,
							Probe:    nil,
						},
						rookv1.KeyMgr: &rookv1.ProbeSpec{
							Disabled: false,
							Probe:    nil,
						},
						rookv1.KeyOSD: &rookv1.ProbeSpec{
							Disabled: false,
							Probe:    nil,
						},
					},
				},
				Security: rookv1.SecuritySpec{},
				LogCollector: rookv1.LogCollectorSpec{
					Enabled:     true,
					Periodicity: "daily",
					MaxLogSize:  &maxLogSize,
				},
				CSI: rookv1.CSIDriverSpec{
					ReadAffinity: rookv1.ReadAffinitySpec{
						Enabled:             false,
						CrushLocationLabels: nil,
					},
					CephFS: rookv1.CSICephFSSpec{
						KernelMountOptions: "",
						FuseMountOptions:   "",
					},
				},
				CephConfig: map[string]map[string]string{},
			},
			Status: rookv1.ClusterStatus{},
		}
		cephCluster, err = s.rookClientset.CephV1().CephClusters(cephCluster.Namespace).Create(ctx, cephCluster, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("create cephcluster %s failed  %+v", cluster.Name, err)
		}
	}
	if count := cluster.Spec.Count - len(cephCluster.Spec.Storage.Nodes); count > 0 {
		cephNodes, err := s.getCephNodes(ctx, count, cluster.Name, cluster.Spec.Flavor)
		if err != nil {
			return nil, fmt.Errorf("get ceph cluster %s nodes failed: %v", cluster.Name, err)
		}
		if len(cephNodes) > 0 {
			cephCluster.Spec.Storage.Nodes = append(cephCluster.Spec.Storage.Nodes, cephNodes...)
			cephCluster, err = s.rookClientset.CephV1().CephClusters(cephCluster.Namespace).Update(ctx, cephCluster, metav1.UpdateOptions{})
			if err != nil {
				return nil, fmt.Errorf("update ceph cluster %s failed: %v", cluster.Name, err)
			}
			klog.Infof("ceph cluster %s update storage nodes %+v", cluster.Name, cephNodes)
		}
	}
	cluster.Status.Phase = v1.Phase(cephCluster.Status.Phase)
	if cluster.Status.CephClusterStatus == nil {
		cluster.Status.CephClusterStatus = &v1.CephClusterStatus{}
	}
	if cephCluster.Status.CephStatus != nil {
		cluster.Status.CephClusterStatus.Health = cephCluster.Status.CephStatus.Health
		cluster.Status.CephClusterStatus.PreviousHealth = cephCluster.Status.CephStatus.PreviousHealth
		cluster.Status.CephClusterStatus.LastChanged = cephCluster.Status.CephStatus.LastChanged
		cluster.Status.CephClusterStatus.LastChecked = cephCluster.Status.CephStatus.LastChecked
		cluster.Status.CephClusterStatus.Capacity.AvailableBytes = cephCluster.Status.CephStatus.Capacity.AvailableBytes
		cluster.Status.CephClusterStatus.Capacity.UsedBytes = cephCluster.Status.CephStatus.Capacity.UsedBytes
		cluster.Status.CephClusterStatus.Capacity.TotalBytes = cephCluster.Status.CephStatus.Capacity.TotalBytes
		cluster.Status.CephClusterStatus.Capacity.LastUpdated = cephCluster.Status.CephStatus.Capacity.LastUpdated
		cluster.Status.CephClusterStatus.ClusterId = cephCluster.Status.CephStatus.FSID
		count := 0
		for _, v := range cephCluster.Status.CephStorage.OSD.StoreType {
			count += v
		}
		cluster.Status.CephClusterStatus.OSD = count
	}

	if cluster.Status.Phase != v1.Ready {
		return cephCluster, nil
	}
	endpoints, err := s.getEndPoints(ctx, cephCluster.Namespace)
	if err != nil {
		return nil, err
	}
	cluster.Status.CephClusterStatus.Monitors = endpoints

	crypto := crypto.NewCrypto()
	sk := ""
	if cluster.Status.CephClusterStatus.SecretKey != "" {
		sk, err = crypto.Decrypt(cluster.Status.CephClusterStatus.SecretKey)
		if err != nil {
			return nil, fmt.Errorf("storage cluster %s decrypt secret key %s failed", cluster.Name, cluster.Status.CephClusterStatus.SecretKey)
		}
	}
	secret, err := s.clientset.CoreV1().Secrets(cephCluster.Namespace).Get(ctx, "rook-ceph-mon", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get secret %s failed %+v", "rook-ceph-mon", err)
	}
	if string(secret.Data["ceph-secret"]) != sk {
		cluster.Status.CephClusterStatus.SecretKey, err = crypto.Encrypt(secret.Data["ceph-secret"])
		if err != nil {
			return nil, fmt.Errorf("storage cluster %s ecrypt secret failed", cluster.Name)
		}
		cluster.Status.CephClusterStatus.AccessKey = "admin"
	}
	return cephCluster, nil
}

func (s *storageCluster) getCephNodes(ctx context.Context, count int, cluster, flavor string) ([]rookv1.Node, error) {
	nodes, err := s.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	cephNodes := []rookv1.Node{}
	for _, node := range nodes.Items {
		if _, ok := node.Labels[v1.StorageClusterNameLabel]; ok {
			cephNodes = append(cephNodes, rookv1.Node{
				Name:      node.Name,
				Resources: corev1.ResourceRequirements{},
				Config:    nil,
				Selection: rookv1.Selection{
					UseAllDevices:        pointer.Bool(true),
					DeviceFilter:         "",
					DevicePathFilter:     "",
					Devices:              nil,
					VolumeClaimTemplates: nil,
				},
			})
		}
	}

	for _, node := range nodes.Items {
		if _, ok := node.Labels[v1.StorageClusterNameLabel]; ok {
			continue
		}
		if node.Labels[v1.NodeFlavorIdLabel] == flavor {
			ph, err := json.Marshal(map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						v1.StorageClusterNameLabel: cluster,
					},
				},
			})
			if err != nil {
				return nil, err
			}
			klog.Infof("pathch labels %s", string(ph))
			_, err = s.clientset.CoreV1().Nodes().Patch(ctx, node.Name, types.MergePatchType, ph, metav1.PatchOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to patch storage cluster node %s labels  %v", node.Name, err)
			}
			cephNodes = append(cephNodes, rookv1.Node{
				Name:      node.Name,
				Resources: corev1.ResourceRequirements{},
				Config:    nil,
				Selection: rookv1.Selection{
					UseAllDevices:        pointer.Bool(true),
					DeviceFilter:         "",
					DevicePathFilter:     "",
					Devices:              nil,
					VolumeClaimTemplates: nil,
				},
			})
			count--
			if count == 0 {
				return cephNodes, nil
			}
		}
	}
	return cephNodes, nil
}

func (s *storageCluster) delete(ctx context.Context, cluster *v1.StorageCluster, clusters *v1.ClusterList) error {
	for _, kc := range clusters.Items {
		for _, storage := range kc.Spec.Storages {
			if storage.StorageCluster == cluster.Name {
				return fmt.Errorf("storage cluster %s is used %s", cluster.Name, kc.Name)
			}
		}
	}
	for _, c := range clusters.Items {
		for _, storage := range c.Spec.Storages {
			if storage.StorageCluster == cluster.Name {
				err := s.deleteStorage(ctx, cluster, &c, storage)
				if err != nil {
					return err
				}
			}
		}
	}
	err := s.rookClientset.CephV1().CephClusters(cluster.Name).Delete(ctx, cluster.Name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	nodes, err := s.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, node := range nodes.Items {
		if node.Labels[v1.StorageClusterNameLabel] == cluster.Name {
			ph, err := json.Marshal(map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						v1.StorageClusterNameLabel: nil,
					},
				},
			})
			if err != nil {
				return err
			}
			_, err = s.clientset.CoreV1().Nodes().Patch(ctx, node.Name, types.MergePatchType, ph, metav1.PatchOptions{})
			if err != nil {
				return fmt.Errorf("failed to patch storage cluster node %s labels  %v", node.Name, err)
			}
		}
	}
	err = deletePermissions(ctx, s.clientset, cluster.Name)
	if err != nil {
		return err
	}
	return nil
}

func (s *storageCluster) getStorage(ctx context.Context, cluster *v1.StorageCluster, kc *v1.Cluster, storage v1.Storage) (*v1.StorageStatus, error) {
	name := fmt.Sprintf("%s-%s", kc.Name, storage.Name)
	if cluster.Status.Phase != v1.Ready {
		return &v1.StorageStatus{Storage: storage}, nil
	}
	switch storage.Type {
	case v1.OBS:
		return s.getObjectStore(ctx, cluster, name, storage)
	case v1.RBD:
		return s.getRBD(ctx, cluster, name, storage)
	case v1.FS:
		return s.getFileSystem(ctx, cluster, name, storage)
	}
	return &v1.StorageStatus{Storage: storage}, nil
}

func (s *storageCluster) getObjectStore(ctx context.Context, cluster *v1.StorageCluster, obsName string, storage v1.Storage) (*v1.StorageStatus, error) {
	username := storage.Name
	namespace := cluster.Name
	obs, err := s.rookClientset.CephV1().CephObjectStores(namespace).Get(ctx, obsName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}
		list, err := s.cephInformer.Ceph().V1().CephObjectStores().Lister().CephObjectStores(namespace).List(labels.Everything())
		if err != nil {
			return nil, err
		}
		port := int32(80 + len(list))
		obs = &rookv1.CephObjectStore{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      obsName,
				Namespace: namespace,
				Labels: map[string]string{
					v1.StorageClusterNameLabel: cluster.Name,
				},
			},
			Spec: rookv1.ObjectStoreSpec{
				MetadataPool: rookv1.PoolSpec{
					FailureDomain: "host",
					Replicated: rookv1.ReplicatedSpec{
						Size:                   3,
						RequireSafeReplicaSize: true,
					},
					Parameters: map[string]string{
						"compression_mode": "none",
					},
				},
				DataPool: rookv1.PoolSpec{
					FailureDomain: "host",
					Parameters: map[string]string{
						"compression_mode": "none",
					},
					EnableRBDStats: false,
					Mirroring:      rookv1.MirroringSpec{},
					StatusCheck:    rookv1.MirrorHealthCheckSpec{},
					Quotas:         rookv1.QuotaSpec{},
				},
				PreservePoolsOnDelete: true,
				Gateway: rookv1.GatewaySpec{
					Port:              port,
					Instances:         2,
					SSLCertificateRef: "",
					Placement: rookv1.Placement{
						NodeAffinity: nodeAffinity(namespace),
					},
					DisableMultisiteSyncTraffic: false,
					Annotations:                 nil,
					Labels:                      nil,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("4"),
							corev1.ResourceMemory: resource.MustParse("8Gi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("4"),
							corev1.ResourceMemory: resource.MustParse("8Gi"),
						},
					},
					PriorityClassName:    "",
					ExternalRgwEndpoints: nil,
					Service:              nil,
					HostNetwork:          nil,
					DashboardEnabled:     nil,
				},
				Zone: rookv1.ZoneSpec{},
				HealthCheck: rookv1.ObjectHealthCheckSpec{
					ReadinessProbe: &rookv1.ProbeSpec{
						Disabled: false,
					},
					StartupProbe: &rookv1.ProbeSpec{
						Disabled: false,
					},
				},
				Security:               nil,
				AllowUsersInNamespaces: nil,
			},
			Status: nil,
		}
		if storage.ErasureCoded != nil {
			obs.Spec.DataPool.ErasureCoded = rookv1.ErasureCodedSpec{
				CodingChunks: storage.ErasureCoded.CodingChunks,
				DataChunks:   storage.ErasureCoded.DataChunks,
				Algorithm:    storage.ErasureCoded.Algorithm,
			}
		} else if storage.Replicated != nil {
			obs.Spec.DataPool.Replicated = rookv1.ReplicatedSpec{
				Size: storage.Replicated.Size,
				// TargetSizeRatio:          float64(storage.Replicated.TargetSizeRatio) / 100,
				RequireSafeReplicaSize:   storage.Replicated.RequireSafeReplicaSize,
				ReplicasPerFailureDomain: storage.Replicated.ReplicasPerFailureDomain,
				SubFailureDomain:         storage.Replicated.SubFailureDomain,
				HybridStorage:            nil,
			}
			if storage.Replicated.HybridStorage != nil {
				obs.Spec.DataPool.Replicated.HybridStorage = &rookv1.HybridStorageSpec{
					PrimaryDeviceClass:   storage.Replicated.HybridStorage.PrimaryDeviceClass,
					SecondaryDeviceClass: storage.Replicated.HybridStorage.SecondaryDeviceClass,
				}
			}
		} else {
			obs.Spec.DataPool.Replicated = rookv1.ReplicatedSpec{
				Size: 3,
			}
		}
		obs, err = s.rookClientset.CephV1().CephObjectStores(namespace).Create(ctx, obs, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}
	}

	user, err := s.rookClientset.CephV1().CephObjectStoreUsers(namespace).Get(ctx, username, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}
		user = &rookv1.CephObjectStoreUser{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      username,
				Namespace: namespace,
				Labels: map[string]string{
					v1.StorageClusterNameLabel: cluster.Name,
				},
			},
			Spec: rookv1.ObjectStoreUserSpec{
				Store:            obsName,
				DisplayName:      "",
				Capabilities:     nil,
				Quotas:           nil,
				ClusterNamespace: "",
			},
			Status: nil,
		}
		user, err = s.rookClientset.CephV1().CephObjectStoreUsers(namespace).Create(ctx, user, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}
	}
	status := &v1.StorageStatus{
		Storage:   storage,
		ClusterId: cluster.Status.CephClusterStatus.ClusterId,
		Monitors:  nil,
		Pool:      obsName,
	}
	if status.Storage.StorageClass == "" {
		status.Storage.StorageClass = storage.Name
	}
	if user.Status == nil {
		return status, nil
	}
	status.Phase = v1.Phase(user.Status.Phase)
	if secretName, ok := user.Status.Info["secretName"]; ok {
		secret, err := s.clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		status.AccessKey = string(secret.Data["AccessKey"])
		status.SecretKey = string(secret.Data["SecretKey"])

		for _, host := range obs.Status.Endpoints.Insecure {
			u, err := url.Parse(host)
			if err != nil {
				klog.Errorf("parse host %+v failed", err)
				continue
			}
			index := strings.Index(u.Hostname(), ".")
			if index == -1 {
				continue
			}
			ep, err := s.clientset.CoreV1().Endpoints(namespace).Get(ctx, u.Hostname()[:index], metav1.GetOptions{})
			if err != nil {
				klog.Errorf("get endpoint %s %+v failed", u.Hostname(), err)
				continue
			}
			subnets := make([]corev1.EndpointSubset, 0, len(ep.Subsets))
			for _, sub := range ep.Subsets {
				newSub := sub.DeepCopy()
				for i := range newSub.Addresses {
					newSub.Addresses[i].NodeName = nil
					newSub.Addresses[i].TargetRef = nil
				}
				subnets = append(subnets, *newSub)
			}
			status.Subsets = subnets
		}
	}

	return status, nil
}

func (s *storageCluster) getRBD(ctx context.Context, cluster *v1.StorageCluster, name string, storage v1.Storage) (*v1.StorageStatus, error) {
	namespace := cluster.Name
	crypto := crypto.NewCrypto()
	sk := ""
	var err error
	if cluster.Status.CephClusterStatus.SecretKey != "" {
		sk, err = crypto.Decrypt(cluster.Status.CephClusterStatus.SecretKey)
		if err != nil {
			return nil, fmt.Errorf("storage cluster %s decrypt secret key %s failed", cluster.Name, cluster.Status.CephClusterStatus.SecretKey)
		}
	}
	status := &v1.StorageStatus{
		Storage:   storage,
		ClusterId: cluster.Status.CephClusterStatus.ClusterId,
		Monitors:  cluster.Status.CephClusterStatus.Monitors,
		Pool:      name,
		Phase:     v1.Creating,
		AccessKey: cluster.Status.CephClusterStatus.AccessKey,
		SecretKey: sk,
	}

	pool, err := s.rookClientset.CephV1().CephBlockPools(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("get ceph block pool %s failed %+v", name, err)
		}
		pool = &rookv1.CephBlockPool{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					v1.StorageClusterNameLabel: cluster.Name,
				},
			},
			Spec: rookv1.NamedBlockPoolSpec{
				PoolSpec: rookv1.PoolSpec{
					Parameters: map[string]string{},
				},
			},
			Status: nil,
		}
		if storage.ErasureCoded != nil {
			metaName := fmt.Sprintf("%s-metadata", name)
			metaPool, err := s.rookClientset.CephV1().CephBlockPools(namespace).Get(ctx, metaName, metav1.GetOptions{})
			if err != nil {
				if !errors.IsNotFound(err) {
					return nil, fmt.Errorf("failed to get ceph block pool %s error: %v", name, err)

				}
				metaPool = &rookv1.CephBlockPool{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      metaName,
						Namespace: namespace,
					},
					Spec: rookv1.NamedBlockPoolSpec{
						PoolSpec: rookv1.PoolSpec{
							Replicated: rookv1.ReplicatedSpec{
								Size: 3,
							},
							Parameters: map[string]string{},
						},
					},
					Status: nil,
				}
				metaPool, err = s.rookClientset.CephV1().CephBlockPools(namespace).Create(ctx, metaPool, metav1.CreateOptions{})
				if err != nil {
					return nil, fmt.Errorf("failed to create ceph block metadata pool %s error: %v", metaName, err)
				}
			}
			pool.Spec.ErasureCoded = rookv1.ErasureCodedSpec{
				CodingChunks: storage.ErasureCoded.CodingChunks,
				DataChunks:   storage.ErasureCoded.DataChunks,
				Algorithm:    storage.ErasureCoded.Algorithm,
			}
		} else if storage.Replicated != nil {
			pool.Spec.Replicated = rookv1.ReplicatedSpec{
				Size: storage.Replicated.Size,
				// TargetSizeRatio:          float64(storage.Replicated.TargetSizeRatio) / 100,
				RequireSafeReplicaSize:   storage.Replicated.RequireSafeReplicaSize,
				ReplicasPerFailureDomain: storage.Replicated.ReplicasPerFailureDomain,
				SubFailureDomain:         storage.Replicated.SubFailureDomain,
				HybridStorage:            nil,
			}
			if storage.Replicated.HybridStorage != nil {
				pool.Spec.Replicated.HybridStorage = &rookv1.HybridStorageSpec{
					PrimaryDeviceClass:   storage.Replicated.HybridStorage.PrimaryDeviceClass,
					SecondaryDeviceClass: storage.Replicated.HybridStorage.SecondaryDeviceClass,
				}
			}
		} else {
			pool.Spec.Replicated = rookv1.ReplicatedSpec{
				Size: 3,
			}
		}
		pool, err = s.rookClientset.CephV1().CephBlockPools(namespace).Create(ctx, pool, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to create ceph block pool %s error  %+v", name, err)
		}
	}

	if pool.Status == nil {
		return status, nil
	}
	status.Phase = v1.Phase(pool.Status.Phase)

	return status, nil
}

func (s *storageCluster) getFileSystem(ctx context.Context, cluster *v1.StorageCluster, name string, storage v1.Storage) (*v1.StorageStatus, error) {
	namespace := cluster.Name
	crypto := crypto.NewCrypto()
	sk := ""
	var err error
	if cluster.Status.CephClusterStatus.SecretKey != "" {
		sk, err = crypto.Decrypt(cluster.Status.CephClusterStatus.SecretKey)
		if err != nil {
			return nil, fmt.Errorf("storage cluster %s decrypt secret key %s failed", cluster.Name, cluster.Status.CephClusterStatus.SecretKey)
		}
	}
	status := &v1.StorageStatus{
		Storage:   storage,
		ClusterId: cluster.Status.CephClusterStatus.ClusterId,
		Monitors:  cluster.Status.CephClusterStatus.Monitors,
		Pool:      name,
		Phase:     v1.Creating,
		AccessKey: cluster.Status.CephClusterStatus.AccessKey,
		SecretKey: sk,
	}
	fs, err := s.rookClientset.CephV1().CephFilesystems(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return status, err
		}
		fs = &rookv1.CephFilesystem{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: rookv1.FilesystemSpec{
				MetadataPool: rookv1.NamedPoolSpec{
					Name: name + "meta",
					PoolSpec: rookv1.PoolSpec{
						FailureDomain:  "",
						CrushRoot:      "",
						DeviceClass:    "",
						Replicated:     rookv1.ReplicatedSpec{},
						ErasureCoded:   rookv1.ErasureCodedSpec{},
						Parameters:     nil,
						EnableRBDStats: false,
						Mirroring:      rookv1.MirroringSpec{},
						StatusCheck:    rookv1.MirrorHealthCheckSpec{},
						Quotas:         rookv1.QuotaSpec{},
					},
				},
				DataPools:                  []rookv1.NamedPoolSpec{},
				PreservePoolsOnDelete:      false,
				PreserveFilesystemOnDelete: false,
				MetadataServer: rookv1.MetadataServerSpec{
					ActiveCount:   1,
					ActiveStandby: true,
					Placement: rookv1.Placement{
						NodeAffinity: nodeAffinity(namespace),
					},
					Annotations:       nil,
					Labels:            nil,
					Resources:         corev1.ResourceRequirements{},
					PriorityClassName: "",
					LivenessProbe:     nil,
					StartupProbe:      nil,
				},
				Mirroring:   nil,
				StatusCheck: rookv1.MirrorHealthCheckSpec{},
			},
			Status: nil,
		}
		if storage.ErasureCoded != nil {
			replica := replicated(storage)
			fs.Spec.MetadataPool.Replicated = replica
			fs.Spec.DataPools = append(fs.Spec.DataPools, rookv1.NamedPoolSpec{
				PoolSpec: rookv1.PoolSpec{
					Replicated: replica,
				},
			})
			fs.Spec.DataPools = append(fs.Spec.DataPools, rookv1.NamedPoolSpec{
				Name: "erasurecoded",
				PoolSpec: rookv1.PoolSpec{
					ErasureCoded: rookv1.ErasureCodedSpec{
						CodingChunks: storage.ErasureCoded.CodingChunks,
						DataChunks:   storage.ErasureCoded.DataChunks,
						Algorithm:    storage.ErasureCoded.Algorithm,
					},
					Parameters: map[string]string{
						"compression_mode": "none",
					},
				},
			})
		} else {
			replica := replicated(storage)
			fs.Spec.MetadataPool.Replicated = replica
			fs.Spec.DataPools = append(fs.Spec.DataPools, rookv1.NamedPoolSpec{
				Name: "replicated",
				PoolSpec: rookv1.PoolSpec{
					Replicated: replica,
				},
			})
		}
		fs, err = s.rookClientset.CephV1().CephFilesystems(namespace).Create(ctx, fs, metav1.CreateOptions{})
		if err != nil {
			return status, err
		}
	}
	if fs.Status == nil {
		return status, nil
	}
	status.Phase = v1.Phase(fs.Status.Phase)

	svg, err := s.rookClientset.CephV1().CephFilesystemSubVolumeGroups(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return status, err
		}
		svg = &rookv1.CephFilesystemSubVolumeGroup{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "ceph.rook.io/v1",
						Kind:               "CephFilesystem",
						Name:               fs.Name,
						UID:                fs.UID,
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					},
				},
			},
			Spec: rookv1.CephFilesystemSubVolumeGroupSpec{
				Name:           "primus-safe",
				FilesystemName: name,
			},
			Status: nil,
		}
		svg, err = s.rookClientset.CephV1().CephFilesystemSubVolumeGroups(namespace).Create(ctx, svg, metav1.CreateOptions{})
		if err != nil {
			return status, err
		}
	}
	return status, nil
}

func (s *storageCluster) deleteStorage(ctx context.Context, cluster *v1.StorageCluster, kc *v1.Cluster, storage v1.Storage) error {
	name := fmt.Sprintf("%s-%s", kc.Name, storage.Name)
	klog.Infof("storageCluster %s deleteStorage name %s", cluster.Name, name)
	switch storage.Type {
	case v1.OBS:
		return s.deleteObjectStore(ctx, cluster, name, storage.Name)
	case v1.RBD:
		return s.deleteRBD(ctx, cluster, name)
	case v1.FS:
		return s.deleteFileSystem(ctx, cluster, name)
	}
	return nil
}

func (s *storageCluster) deleteObjectStore(ctx context.Context, cluster *v1.StorageCluster, obsName, userName string) error {
	klog.Infof("storageCluster deleteObjectStore obsName %s userName %s", obsName, userName)
	namespace := cluster.Name
	err := s.rookClientset.CephV1().CephObjectStoreUsers(namespace).Delete(ctx, userName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete  object store user %s error  %v", userName, err)
	}
	err = s.rookClientset.CephV1().CephObjectStores(namespace).Delete(ctx, obsName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete object store %s error  %v", obsName, err)
	}
	return nil
}

func (s *storageCluster) deleteRBD(ctx context.Context, cluster *v1.StorageCluster, name string) error {
	err := s.rookClientset.CephV1().CephBlockPools(cluster.Name).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ceph block pool %s error  %v", cluster.Name, err)
	}
	return nil
}

func (s *storageCluster) deleteFileSystem(ctx context.Context, cluster *v1.StorageCluster, name string) error {
	err := s.rookClientset.CephV1().CephFilesystems(cluster.Name).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ceph block pool %s error  %v", cluster.Name, err)
	}
	return nil
}

func (s *storageCluster) getEndPoints(ctx context.Context, namespace string) ([]string, error) {
	cm, err := s.clientset.CoreV1().ConfigMaps(namespace).Get(ctx, "rook-ceph-mon-endpoints", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get config map %s errpr  %v", "rook-ceph-mon-endpoints", err)
	}
	endpoints := make([]string, 0)
	ep := strings.Split(cm.Data["data"], ",")
	for _, e := range ep {
		ee := strings.Split(e, "=")
		if len(ee) == 2 {
			endpoints = append(endpoints, ee[1])
		}
	}
	return endpoints, nil
}

func permissions(ctx context.Context, client kubernetes.Interface, namespace string) error {
	for _, sa := range serviceAccounts {
		sa.Namespace = namespace
		_, err := client.CoreV1().ServiceAccounts(namespace).Create(ctx, sa, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			return err
		}
	}

	for _, clusterRole := range clusterRoles {
		_, err := client.RbacV1().ClusterRoles().Create(ctx, clusterRole, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			return err
		}
	}
	for _, crb := range clusterRoleBindings {
		for i := range crb.Subjects {
			crb.Subjects[i].Namespace = namespace
		}
		crb.Name = namespace
		_, err := client.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			return err
		}
	}
	for _, role := range roles {
		role.Namespace = namespace
		_, err := client.RbacV1().Roles(namespace).Create(ctx, role, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			return err
		}
	}

	for _, rb := range roleBindings {
		rb.Namespace = namespace
		for i := range rb.Subjects {
			rb.Subjects[i].Namespace = namespace
		}
		_, err := client.RbacV1().RoleBindings(namespace).Create(ctx, rb, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			return err
		}
	}

	return nil
}

func deletePermissions(ctx context.Context, client kubernetes.Interface, namespace string) error {
	for _, sa := range serviceAccounts {
		sa.Namespace = namespace
		err := client.CoreV1().ServiceAccounts(namespace).Delete(ctx, sa.Name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	for _, role := range roles {
		role.Namespace = namespace
		err := client.RbacV1().Roles(namespace).Delete(ctx, role.Name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	for _, rb := range roleBindings {
		err := client.RbacV1().RoleBindings(namespace).Delete(ctx, rb.Name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

var serviceAccounts = []*corev1.ServiceAccount{
	{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph-rgw",
		},
	},
	{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph-purge-osd",
		},
	},
	{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph-osd",
		},
	},
	{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph-mgr",
		},
	},
	{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph-cmd-reporter",
		},
	},
	{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph-default",
			Labels: map[string]string{
				"operator":        "rook",
				"storage-backend": "ceph",
			},
		},
	},
}

var roleBindings = []*rbacv1.RoleBinding{
	{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph-mgr-system",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "rook-ceph-mgr-system",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "ServiceAccount",
				Name: "rook-ceph-mgr",
			},
		},
	},
	{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph-purge-osd",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     "rook-ceph-purge-osd",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "ServiceAccount",
				Name: "rook-ceph-purge-osd",
			},
		},
	},
	{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph-osd-external",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     "rook-ceph-osd",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "ServiceAccount",
				Name: "rook-ceph-osd",
			},
		},
	},
	{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph-cluster-mgmt",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "rook-ceph-cluster-mgmt",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "ServiceAccount",
				Name: "rook-ceph-system",
			},
		},
	},
	{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph-cmd-reporter",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     "rook-ceph-cmd-reporter",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "ServiceAccount",
				Name: "rook-ceph-cmd-reporter",
			},
		},
	},
	{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph-psp",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "psp:rook",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "ServiceAccount",
				Name: "rook-ceph-cmd-reporter",
			},
			{
				Kind: "ServiceAccount",
				Name: "rook-ceph-mgr",
			},
			{
				Kind: "ServiceAccount",
				Name: "rook-ceph-osd",
			},
			{
				Kind: "ServiceAccount",
				Name: "rook-ceph-purge-osd",
			},
			{
				Kind: "ServiceAccount",
				Name: "rook-ceph-rgw",
			},
			{
				Kind: "ServiceAccount",
				Name: "default",
			},
		},
	},
}

var roles = []*rbacv1.Role{
	{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph-purge-osd",
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs: []string{
					"get",
				},
				APIGroups: []string{
					"",
				},
				Resources: []string{
					"configmaps",
				},
			},
			{
				Verbs: []string{
					"get",
					"delete",
				},
				APIGroups: []string{
					"apps",
				},
				Resources: []string{
					"deployments",
				},
			},
			{
				Verbs: []string{
					"get",
					"list",
					"delete",
				},
				APIGroups: []string{
					"batch",
				},
				Resources: []string{
					"jobs",
				},
			},
			{
				Verbs: []string{
					"get",
					"list",
					"update",
					"delete",
				},
				APIGroups: []string{
					"",
				},
				Resources: []string{
					"persistentvolumeclaims",
				},
			},
		},
	},
	{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph-cmd-reporter",
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs: []string{
					"get",
					"list",
					"watch",
					"create",
					"update",
					"delete",
				},
				APIGroups: []string{
					"",
				},
				Resources: []string{
					"pods",
					"configmaps",
				},
			},
		},
	},
	{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph-osd",
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs: []string{
					"get",
					"update",
				},
				APIGroups: []string{
					"",
				},
				Resources: []string{
					"secrets",
				},
			},
			{
				Verbs: []string{
					"get",
					"list",
					"watch",
					"create",
					"update",
					"delete",
				},
				APIGroups: []string{
					"",
				},
				Resources: []string{
					"configmaps",
				},
			},
			{
				Verbs: []string{
					"get",
					"list",
					"create",
					"update",
					"delete",
				},
				APIGroups: []string{
					"ceph.rook.io",
				},
				Resources: []string{
					"cephclusters",
					"cephclusters/finalizers",
				},
			},
		},
	},
}

var clusterRoleBindings = []*rbacv1.ClusterRoleBinding{
	{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph-osd",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "rook-ceph-osd",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "ServiceAccount",
				Name: "rook-ceph-osd",
			},
		},
	},
}

var clusterRoles = []*rbacv1.ClusterRole{
	{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph-osd",
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs: []string{
					"get",
					"list",
				},
				APIGroups: []string{
					"",
				},
				Resources: []string{
					"nodes",
				},
			},
		},
	},
}

func nodeAffinity(value string) *corev1.NodeAffinity {
	return &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      v1.StorageClusterNameLabel,
							Operator: "In",
							Values:   []string{value},
						},
					},
					MatchFields: nil,
				},
			}},
		PreferredDuringSchedulingIgnoredDuringExecution: nil,
	}
}

func replicated(storage v1.Storage) rookv1.ReplicatedSpec {
	if storage.Replicated == nil {
		return rookv1.ReplicatedSpec{
			Size: 3,
		}
	}
	spec := rookv1.ReplicatedSpec{
		Size: storage.Replicated.Size,
		// TargetSizeRatio:          float64(storage.Replicated.TargetSizeRatio) / 100,
		RequireSafeReplicaSize:   storage.Replicated.RequireSafeReplicaSize,
		ReplicasPerFailureDomain: storage.Replicated.ReplicasPerFailureDomain,
		SubFailureDomain:         storage.Replicated.SubFailureDomain,
		HybridStorage:            nil,
	}
	if storage.Replicated.HybridStorage != nil {
		spec.HybridStorage = &rookv1.HybridStorageSpec{
			PrimaryDeviceClass:   storage.Replicated.HybridStorage.PrimaryDeviceClass,
			SecondaryDeviceClass: storage.Replicated.HybridStorage.SecondaryDeviceClass,
		}
	}
	return spec
}
