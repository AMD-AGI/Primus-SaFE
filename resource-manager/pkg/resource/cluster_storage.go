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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

func (r *ClusterReconciler) guaranteeStorage(ctx context.Context, cluster *v1.Cluster) (ctrlruntime.Result, error) {
	if !cluster.IsReady() {
		return ctrlruntime.Result{}, nil
	}
	status := cluster.Status.DeepCopy()
	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, cluster.Name)
	if err != nil {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}
	client := k8sClients.ClientSet()
	for i, storage := range cluster.Status.StorageStatus {
		switch storage.Type {
		case v1.OBS:
			if e := r.guaranteeOBS(ctx, client, cluster, i); e != nil {
				err = e
			}
		case v1.RBD:
			if e := r.guaranteeRBD(ctx, client, cluster, i); e != nil {
				err = e
			}
		case v1.FS:
			if e := r.guaranteeFileSystem(ctx, client, cluster, i); e != nil {
				err = e
			}
		}
	}
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if reflect.DeepEqual(status.StorageStatus, cluster.Status.StorageStatus) {
		return ctrlruntime.Result{}, nil
	}
	err = r.Status().Update(ctx, cluster)
	if err != nil {
		return ctrlruntime.Result{}, fmt.Errorf("update cluster status failed %+v", err)
	}
	return ctrlruntime.Result{}, nil
}

func (r *ClusterReconciler) guaranteeOBS(ctx context.Context, client kubernetes.Interface, cluster *v1.Cluster, index int) error {
	storage := cluster.Status.StorageStatus[index]
	name := storage.Name
	if storage.Secret != "" {
		name = storage.Secret
	}
	namespace := v1.DefaultNamespace
	if storage.Namespace != "" {
		namespace = storage.Namespace
	}
	if _, ok := cluster.GetStorage(storage.Name); !ok {
		err := client.CoreV1().Endpoints(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete storage endpoint %s failed %+v", name, err)
		}
		err = client.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete storage service %s failed %+v", name, err)
		}
		err = client.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete storage secret %s failed %+v", name, err)
		}
		cluster.Status.StorageStatus[index].Ref = nil
		return nil
	}
	if storage.Phase != v1.Ready {
		return nil
	}
	klog.Info("create obs service & endpoints")
	if err := r.guaranteeNamespace(ctx, client, namespace); err != nil {
		return err
	}
	ep, err := client.CoreV1().Endpoints(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		ep = &corev1.Endpoints{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Subsets: storage.Subsets,
		}
		ep, err = client.CoreV1().Endpoints(namespace).Create(ctx, ep, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create obs %s endpoint failed %+v", name, err)
		}
	} else if !reflect.DeepEqual(ep.Subsets, storage.Subsets) {
		ep.Subsets = storage.Subsets
		_, err = client.CoreV1().Endpoints(namespace).Update(ctx, ep, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update obs %s endpoint failed %+v", name, err)
		}
	}

	service, err := client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get obs %s service failed %+v", name, err)
		}
		service = &corev1.Service{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeClusterIP,
			},
			Status: corev1.ServiceStatus{},
		}
		for i, sub := range storage.Subsets {
			for k, port := range sub.Ports {
				service.Spec.Ports = append(service.Spec.Ports, corev1.ServicePort{
					Name:     port.Name,
					Protocol: port.Protocol,
					Port:     int32(80 + i + k),
					TargetPort: intstr.IntOrString{
						IntVal: port.Port,
					},
				})
			}
		}
		service, err = client.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		cluster.Status.StorageStatus[index].Ref = &corev1.ObjectReference{
			Kind:       service.Kind,
			Namespace:  service.Namespace,
			Name:       service.Name,
			UID:        service.UID,
			APIVersion: service.APIVersion,
		}
	}
	secret, err := client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get obs %s secret failed %+v", name, err)
		}
		secret = &corev1.Secret{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Immutable: nil,
			Data: map[string][]byte{
				"AccessKey": []byte(storage.AccessKey),
				"SecretKey": []byte(storage.SecretKey),
				"Endpoint":  []byte(fmt.Sprintf("http://%s.%s.svc", name, namespace)),
			},
			StringData: nil,
			Type:       "",
		}
		klog.Infof("%+v", secret)
		secret, err = client.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create obs %s secret failed %+v", name, err)
		}
	}
	return nil
}

func (r *ClusterReconciler) guaranteeNamespace(ctx context.Context, client kubernetes.Interface, namespace string) error {
	_, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		ns := &corev1.Namespace{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
			Spec:   corev1.NamespaceSpec{},
			Status: corev1.NamespaceStatus{},
		}
		_, err = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ClusterReconciler) updateCephCsiConfig(ctx context.Context, client kubernetes.Interface, storage v1.StorageStatus, name, namespace string) error {
	configMap, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "updateCephCsiConfig error")
		return fmt.Errorf("get ceph csi configmap %s failed %+v", name, err)
	}
	var infos []v1.ClusterInfo
	if conf, ok := configMap.Data["config.json"]; ok {
		err = json.Unmarshal([]byte(conf), &infos)
		if err != nil {
			return fmt.Errorf("unmarshal ceph csi ceph-csi-config %s failed %+v", name, err)
		}
	}
	index := -1
	for k, info := range infos {
		if info.ClusterID == storage.ClusterId {
			if !reflect.DeepEqual(infos[k].Monitors, storage.Monitors) {
				index = k
				break
			} else {
				return nil
			}
		}
	}
	if index == -1 {
		infos = append(infos, v1.ClusterInfo{
			ClusterID:    storage.ClusterId,
			Monitors:     storage.Monitors,
			CephFS:       v1.CephFSSpecific{},
			RBD:          v1.RBDSpecific{},
			NFS:          v1.NFSSpecific{},
			ReadAffinity: v1.ReadAffinity{},
		})
	} else {
		infos[index].Monitors = storage.Monitors
	}
	klog.Infof("update ceph csi config monitors %+v", storage.Monitors)
	info, err := json.Marshal(infos)
	if err != nil {
		return fmt.Errorf("marshal ceph csi ceph-csi-config %s failed %+v", name, err)
	}
	configMap.Data["config.json"] = string(info)
	configMap, err = client.CoreV1().ConfigMaps(configMap.Namespace).Update(ctx, configMap, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update ceph csi ceph-csi-config failed %+v", err)
	}
	return nil
}

func (r *ClusterReconciler) guaranteeRBD(ctx context.Context, client kubernetes.Interface, cluster *v1.Cluster, index int) error {
	storage := cluster.Status.StorageStatus[index]
	if storage.Phase != v1.Ready {
		return nil
	}
	name := storage.Name
	if storage.StorageClass != "" {
		name = storage.StorageClass
	}
	namespace := v1.DefaultNamespace
	if storage.Namespace != "" {
		namespace = storage.Namespace
	}

	if _, ok := cluster.GetStorage(storage.Name); !ok {
		err := client.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete storage secret %s failed %+v", name, err)
		}
		err = client.StorageV1().StorageClasses().Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete storage class %s failed %+v", name, err)
		}
		cluster.Status.StorageStatus[index].Ref = nil
		return nil
	}
	if err := r.updateCephCsiConfig(ctx, client, storage, v1.CephCSIConfigName, v1.CephRBDCSINamespace); err != nil {
		return err
	}
	if err := r.guaranteeNamespace(ctx, client, namespace); err != nil {
		return err
	}
	secret, err := client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get storage secret %s failed %+v", name, err)
		}
		secret = &corev1.Secret{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Immutable: nil,
			Data: map[string][]byte{
				"userID":  []byte(storage.AccessKey),
				"userKey": []byte(storage.SecretKey),
			},
			StringData: nil,
			Type:       "kubernetes.io/rbd",
		}
		secret, err = client.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create storage secret %s failed %+v", storage.Name, err)
		}
	}
	sc, err := client.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get storage class %s failed %+v", name, err)
		}
		sc = &storagev1.StorageClass{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Annotations: make(map[string]string),
			},
			Provisioner: "rbd.csi.ceph.com",
			Parameters: map[string]string{
				"clusterID":     storage.ClusterId,
				"pool":          storage.Pool,
				"imageFeatures": "layering",
				"csi.storage.k8s.io/provisioner-secret-name":            name,
				"csi.storage.k8s.io/provisioner-secret-namespace":       namespace,
				"csi.storage.k8s.io/controller-expand-secret-name":      name,
				"csi.storage.k8s.io/controller-expand-secret-namespace": namespace,
				"csi.storage.k8s.io/node-stage-secret-name":             name,
				"csi.storage.k8s.io/node-stage-secret-namespace":        namespace,
				"csi.storage.k8s.io/fstype":                             "xfs",
			},
			ReclaimPolicy:        nil,
			MountOptions:         nil,
			AllowVolumeExpansion: nil,
			VolumeBindingMode:    nil,
			AllowedTopologies:    nil,
		}
		if storage.ErasureCoded != nil {
			sc.Parameters["pool"] = fmt.Sprintf("%s-metadata", storage.Pool)
			sc.Parameters["dataPool"] = storage.Pool
		}
		list, err := client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("list storage classes %s failed %+v", name, err)
		}
		if len(list.Items) == 0 {
			sc.Annotations["storageclass.kubernetes.io/is-default-class"] = v1.TrueStr
		}
		sc, err = client.StorageV1().StorageClasses().Create(ctx, sc, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create storage class %s failed %+v", storage.Name, err)
		}
		cluster.Status.StorageStatus[index].Ref = &corev1.ObjectReference{
			Kind:       sc.Kind,
			Namespace:  sc.Namespace,
			Name:       sc.Name,
			UID:        sc.UID,
			APIVersion: sc.APIVersion,
		}
		klog.Infof("name %s storage %d index status ref %+v", name, index, cluster.Status.StorageStatus[index].Ref)
	}
	return nil
}

func (r *ClusterReconciler) guaranteeFileSystem(ctx context.Context, client kubernetes.Interface, cluster *v1.Cluster, index int) error {
	storage := cluster.Status.StorageStatus[index]
	if storage.Phase != v1.Ready {
		return nil
	}
	name := storage.Name
	if storage.StorageClass != "" {
		name = storage.StorageClass
	}
	namespace := v1.DefaultNamespace
	if storage.Namespace != "" {
		namespace = storage.Namespace
	}
	if _, ok := cluster.GetStorage(storage.Name); !ok {
		err := client.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete storage secret %s failed %+v", name, err)
		}
		err = client.StorageV1().StorageClasses().Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete storage class %s failed %+v", name, err)
		}
		cluster.Status.StorageStatus[index].Ref = nil
		return nil
	}
	if err := r.updateCephCsiConfig(ctx, client, storage, v1.CephCSIConfigName, v1.CephFSCSINamespace); err != nil {
		return err
	}

	if err := r.guaranteeNamespace(ctx, client, namespace); err != nil {
		return err
	}

	sc, err := client.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get storage class %s failed %+v", name, err)
		}
		sc = &storagev1.StorageClass{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Annotations: make(map[string]string),
			},
			Provisioner: "cephfs.csi.ceph.com",
			Parameters: map[string]string{
				"clusterID": storage.ClusterId,
				"fsName":    storage.Pool,
				"csi.storage.k8s.io/provisioner-secret-name":            name,
				"csi.storage.k8s.io/provisioner-secret-namespace":       namespace,
				"csi.storage.k8s.io/controller-expand-secret-name":      name,
				"csi.storage.k8s.io/controller-expand-secret-namespace": namespace,
				"csi.storage.k8s.io/node-stage-secret-name":             name,
				"csi.storage.k8s.io/node-stage-secret-namespace":        namespace,
			},
			ReclaimPolicy:        nil,
			MountOptions:         nil,
			AllowVolumeExpansion: nil,
			VolumeBindingMode:    nil,
			AllowedTopologies:    nil,
		}
		if storage.ErasureCoded != nil {
			sc.Parameters["pool"] = fmt.Sprintf("%s-metadata", storage.Pool)
			sc.Parameters["dataPool"] = storage.Pool
		}
		list, err := client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("list storage classes %s failed %+v", name, err)
		}
		if len(list.Items) == 0 {
			sc.Annotations["storageclass.kubernetes.io/is-default-class"] = v1.TrueStr
		}
		sc, err = client.StorageV1().StorageClasses().Create(ctx, sc, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create storage class %s failed %+v", storage.Name, err)
		}
		cluster.Status.StorageStatus[index].Ref = &corev1.ObjectReference{
			Kind:       sc.Kind,
			Namespace:  sc.Namespace,
			Name:       sc.Name,
			UID:        sc.UID,
			APIVersion: sc.APIVersion,
		}
		klog.Infof("name %s storage %d index status ref %+v", name, index, cluster.Status.StorageStatus[index].Ref)
	}
	secret, err := client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get storage secret %s failed %+v", name, err)
		}
		secret = &corev1.Secret{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "storage.k8s.io/v1",
						Kind:               "StorageClass",
						Name:               sc.Name,
						UID:                sc.UID,
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					},
				},
			},
			Immutable: nil,
			Data: map[string][]byte{
				"adminID":  []byte(storage.AccessKey),
				"adminKey": []byte(storage.SecretKey),
				"userID":   []byte(storage.AccessKey),
				"userKey":  []byte(storage.SecretKey),
			},
			StringData: nil,
			Type:       "kubernetes.io/cephfs",
		}
		secret, err = client.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create storage secret %s failed %+v", storage.Name, err)
		}
	}
	return nil
}

func (r *ClusterReconciler) guaranteeDefaultAddon(ctx context.Context, cluster *v1.Cluster) (ctrlruntime.Result, error) {
	addons := config.GetAddons(cluster.Spec.ControlPlane.KubeVersion)
	for _, addon := range addons {
		template := new(v1.AddonTemplate)
		err := r.Get(ctx, types.NamespacedName{Name: addon}, template)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			klog.Errorf("get addon template %s failed %+v", addon, err)
		}
		component := getComponentName(addon)
		name := fmt.Sprintf("%s-%s", cluster.Name, component)
		addon := new(v1.Addon)
		err = r.Get(ctx, types.NamespacedName{Name: name}, addon)
		if err != nil {
			if apierrors.IsNotFound(err) {
				namespace := template.Spec.HelmDefaultNamespace
				if namespace == "" {
					namespace = "default"
				}
				addon = &v1.Addon{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name: name,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         cluster.APIVersion,
								Kind:               cluster.Kind,
								Name:               cluster.Name,
								UID:                cluster.UID,
								Controller:         pointer.Bool(true),
								BlockOwnerDeletion: pointer.Bool(true),
							},
						},
					},
					Spec: v1.AddonSpec{
						Cluster: &corev1.ObjectReference{
							Kind:            cluster.Kind,
							Namespace:       cluster.Namespace,
							Name:            cluster.Name,
							UID:             cluster.UID,
							APIVersion:      cluster.APIVersion,
							ResourceVersion: cluster.ResourceVersion,
						},
						AddonSource: v1.AddonSource{
							HelmRepository: &v1.HelmRepository{
								ReleaseName:     component,
								PlainHTTP:       false,
								ChartVersion:    "",
								Namespace:       namespace,
								Values:          "",
								PreviousVersion: nil,
								Template: &corev1.ObjectReference{
									Kind:            template.Kind,
									Namespace:       template.Namespace,
									Name:            template.Name,
									UID:             template.UID,
									APIVersion:      template.APIVersion,
									ResourceVersion: template.ResourceVersion,
								},
							},
						},
					},
					Status: v1.AddonStatus{},
				}
				err = r.Create(ctx, addon)
				if err != nil {
					return reconcile.Result{}, fmt.Errorf("create addon %s failed %+v", name, err)
				}
				continue
			}
			klog.Errorf("get addon template %s failed %+v", addon, err)
		}
	}
	return reconcile.Result{}, nil
}

func getComponentName(name string) string {
	index := strings.Index(name, ".")
	if index > 0 {
		return name[:index]
	}
	return name
}
