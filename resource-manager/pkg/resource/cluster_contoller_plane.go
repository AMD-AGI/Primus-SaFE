/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/secure"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
)

func (r *ClusterReconciler) guaranteeClusterControlPlane(ctx context.Context, cluster *v1.Cluster) error {
	klog.Infof("cluster %s, phase %s", cluster.Name, cluster.Status.ControlPlaneStatus.Phase)
	if len(cluster.Spec.ControlPlane.Nodes) == 0 {
		return nil
	}
	if err := r.patchKubeControlPlanNodes(ctx, cluster); err != nil {
		return err
	}
	if err := r.fetchProvisionedClusterKubeConfig(ctx, cluster); err != nil {
		return err
	}

	if guaranteeControllerPlane(cluster) {
		hostsContent, err := r.generateHosts(ctx, cluster, nil)
		if err != nil {
			if !cluster.DeletionTimestamp.IsZero() {
				klog.Infof("delete %s finalizer", v1.ClusterFinalizer)
				var ok bool
				cluster.Finalizers, ok = slice.RemoveString(cluster.Finalizers, v1.ClusterFinalizer)
				if !ok {
					return nil
				}
				return r.Update(ctx, cluster)
			}
			return err
		}
		if cluster.Status.ControlPlaneStatus.Phase != v1.ReadyPhase && hostsContent == nil && cluster.DeletionTimestamp.IsZero() {
			klog.Infof("cluster %s Kube control plane nodes not ready, plase wait", cluster.Name)
			return nil
		}
		if !cluster.DeletionTimestamp.IsZero() {
			return r.reset(ctx, cluster, hostsContent)
		}
		if err := r.addFinalizer(ctx, cluster); err != nil {
			return err
		}

		if cluster.Spec.ControlPlane.SSHSecret == nil {
			err := r.generateSSHSecret(ctx, cluster)
			if err != nil {
				return err
			}
		}
		pod, err := r.guaranteeCreateWorkerPodCreated(ctx, cluster, hostsContent)
		if err != nil {
			return err
		}
		if pod != nil {
			c := client.MergeFrom(cluster.DeepCopy())
			cluster.Status.ControlPlaneStatus.Phase = v1.CreatingPhase
			if pod.Status.Phase == corev1.PodSucceeded {
				cluster.Status.ControlPlaneStatus.Phase = v1.CreatedPhase
			} else if pod.Status.Phase == corev1.PodFailed {
				cluster.Status.ControlPlaneStatus.Phase = v1.CreationFailed
			}
			if err := r.Status().Patch(ctx, cluster, c); err != nil {
				return err
			}
		} else {
			return nil
		}
		return nil
	}

	if err := r.podClear(ctx, cluster); err != nil {
		return err
	}
	return nil
}

func (r *ClusterReconciler) reset(ctx context.Context, cluster *v1.Cluster, hostsContent *HostTemplateContent) error {
	if cluster.Status.ControlPlaneStatus.Phase == v1.DeletedPhase {
		if err := r.patchKubeControlPlanNodes(ctx, cluster); err != nil {
			return err
		}
		return nil
	}

	c := client.MergeFrom(cluster.DeepCopy())
	if cluster.Status.ControlPlaneStatus.Phase == v1.CreationFailed || cluster.Status.ControlPlaneStatus.Phase == v1.PendingPhase {
		cluster.Status.ControlPlaneStatus.Phase = v1.DeletedPhase
	} else {
		pod, err := r.guaranteeResetWorkPodCreated(ctx, cluster, hostsContent)
		if err != nil {
			return err
		}
		_, err = r.guaranteeHostsConfigMapCreated(ctx, cluster.Name, metav1.OwnerReference{
			APIVersion:         "v1",
			Kind:               "Pod",
			Name:               pod.Name,
			UID:                pod.UID,
			Controller:         pointer.Bool(true),
			BlockOwnerDeletion: pointer.Bool(true),
		}, hostsContent)
		if err != nil {
			return err
		}
		if pod.Status.Phase == corev1.PodSucceeded {
			cluster.Status.ControlPlaneStatus.Phase = v1.DeletedPhase
		} else if pod.Status.Phase == corev1.PodFailed {
			cluster.Status.ControlPlaneStatus.Phase = v1.DeleteFailedPhase
		} else {
			cluster.Status.ControlPlaneStatus.Phase = v1.DeletingPhase
		}
	}
	return r.Status().Patch(ctx, cluster, c)
}

func (r *ClusterReconciler) getUsername(ctx context.Context, cluster *v1.Cluster) (string, error) {
	node := &v1.Node{}
	err := r.Get(ctx, types.NamespacedName{Name: cluster.Spec.ControlPlane.Nodes[0]}, node)
	if err != nil {
		return "", err
	}
	return r.ClusterBaseReconciler.getUsername(ctx, node, cluster)
}

func (r *ClusterReconciler) guaranteeCreateWorkerPodCreated(ctx context.Context, cluster *v1.Cluster, hostsContent *HostTemplateContent) (*corev1.Pod, error) {
	pod := new(corev1.Pod)
	err := r.Get(ctx, types.NamespacedName{Namespace: common.PrimusSafeNamespace, Name: fmt.Sprintf("%s-%s", cluster.Name, v1.ClusterCreateAction)}, pod)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	if err == nil {
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == cluster.Kind && owner.UID == cluster.UID {
				_, err := r.guaranteeHostsConfigMapCreated(ctx, cluster.Name, metav1.OwnerReference{
					APIVersion:         "v1",
					Kind:               "Pod",
					Name:               pod.Name,
					UID:                pod.UID,
					Controller:         pointer.Bool(true),
					BlockOwnerDeletion: pointer.Bool(true),
				}, hostsContent)
				if err != nil {
					return nil, err
				}
				return pod, nil
			}
		}
		return nil, r.Delete(ctx, pod)
	}
	username, err := r.getUsername(ctx, cluster)
	if err != nil {
		return nil, err
	}
	cmd := getKubeSprayCreateCMD(username, getKubeSprayEnv(cluster))
	pod = generateWorkerPod(v1.ClusterCreateAction, cluster, username, cmd, getKubesprayImage(cluster), cluster.Name, hostsContent)
	for _, m := range hostsContent.Controllers {
		pod.OwnerReferences = append(pod.OwnerReferences, metav1.OwnerReference{
			APIVersion: m.APIVersion,
			Kind:       m.Kind,
			Name:       m.Name,
			UID:        m.UID,
		})
	}
	if err := r.Create(ctx, pod); err != nil {
		return nil, err
	}

	_, err = r.guaranteeHostsConfigMapCreated(ctx, cluster.Name, metav1.OwnerReference{
		APIVersion:         "v1",
		Kind:               "Pod",
		Name:               pod.Name,
		UID:                pod.UID,
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
	}, hostsContent)
	if err != nil {
		return nil, err
	}
	return pod, nil
}

func (r *ClusterReconciler) guaranteeResetWorkPodCreated(ctx context.Context, cluster *v1.Cluster, hostsContent *HostTemplateContent) (*corev1.Pod, error) {
	labelSelector := client.MatchingLabels{v1.ClusterManageActionLabel: string(v1.ClusterResetAction), v1.ClusterManageClusterLabel: cluster.Name}
	list := new(corev1.PodList)
	err := r.List(ctx, list, client.InNamespace(common.PrimusSafeNamespace), labelSelector)
	if err != nil {
		return nil, err
	}
	if len(list.Items) > 0 {
		return &list.Items[0], nil
	}
	username, err := r.getUsername(ctx, cluster)
	if err != nil {
		return nil, err
	}
	cmd := getKubeSprayResetCMD(username, getKubeSprayEnv(cluster))
	pod := generateWorkerPod(v1.ClusterResetAction, cluster, username, cmd, getKubesprayImage(cluster), cluster.Name, hostsContent)
	if err := r.Create(ctx, pod); err != nil {
		return nil, err
	}
	return pod, nil
}

func (r *ClusterReconciler) getControllerPlaneNodes(ctx context.Context, cluster *v1.Cluster) ([]*v1.Node, error) {
	nodes := make([]*v1.Node, 0, len(cluster.Spec.ControlPlane.Nodes))
	for _, v := range cluster.Spec.ControlPlane.Nodes {
		node := new(v1.Node)
		if err := r.Get(ctx, types.NamespacedName{Name: v}, node); err != nil {
			return nil, err
		}
		nodes = append(nodes, node.DeepCopy())
	}
	return nodes, nil
}

func (r *ClusterReconciler) fetchProvisionedClusterKubeConfig(ctx context.Context, cluster *v1.Cluster) error {
	if cluster.Status.ControlPlaneStatus.Phase != v1.CreatedPhase && cluster.Status.ControlPlaneStatus.Phase != "" {
		return nil
	}
	if len(cluster.Status.ControlPlaneStatus.CAData) != 0 && len(cluster.Status.ControlPlaneStatus.CertData) != 0 && len(cluster.Status.ControlPlaneStatus.KeyData) != 0 {
		return nil
	}
	nodes, err := r.getControllerPlaneNodes(ctx, cluster)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return nil
	}
	node := nodes[0]
	sshConfig, err := utils.GetSSHConfig(ctx, r.Client, node)
	if err != nil {
		return err
	}
	port := node.Spec.Port
	if port == nil {
		p := int32(22)
		port = &p
	}
	conn, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", node.Spec.PrivateIP, *port), sshConfig)
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	session, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("new sesssion failed %s", err)
	}
	var b bytes.Buffer
	session.Stdout = &b

	if err = session.Run(fmt.Sprintf("sudo cat %s", ProvisionedKubeConfigPath)); err != nil {
		klog.Infof("cluster get %s config failed  %v", ProvisionedKubeConfigPath, err)
		return nil
	}
	config, err := clientcmd.Load(b.Bytes())
	if err != nil {
		klog.Errorf("load config failed %+v", err)
		return nil
	}
	conf, err := clientcmd.NewNonInteractiveClientConfig(*config, "", &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		return fmt.Errorf("parse config failed %+v", err)
	}
	conf.Host = fmt.Sprintf("https://%s:6443", node.Spec.PrivateIP)
	cli, err := kubernetes.NewForConfig(conf)
	if err != nil {
		klog.Errorf("NewForConfig failed %+v", err)
		return nil
	}
	_, err = cli.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("list node failed %+v", err)
		return nil
	}
	c := client.MergeFrom(cluster.DeepCopy())
	cluster.Status.ControlPlaneStatus.CertData = base64.StdEncoding.EncodeToString(conf.CertData)
	cluster.Status.ControlPlaneStatus.CAData = base64.StdEncoding.EncodeToString(conf.CAData)
	cluster.Status.ControlPlaneStatus.KeyData = base64.StdEncoding.EncodeToString(conf.KeyData)
	cluster.Status.ControlPlaneStatus.Endpoints = make([]string, 0, len(nodes))
	for _, n := range nodes {
		cluster.Status.ControlPlaneStatus.Endpoints = append(cluster.Status.ControlPlaneStatus.Endpoints, fmt.Sprintf("https://%s:6443", n.Spec.PrivateIP))
	}
	cluster.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	if err := r.guaranteeService(ctx, cluster); err != nil {
		return err
	}
	err = r.Status().Patch(ctx, cluster, c)
	if err != nil {
		return fmt.Errorf("failed load config%+v", err)
	}
	return nil
}

func (r *ClusterReconciler) addFinalizer(ctx context.Context, cluster *v1.Cluster) error {
	if slice.Contains(cluster.Finalizers, v1.ClusterFinalizer) {
		return nil
	}
	cluster.Finalizers = append(cluster.Finalizers, v1.ClusterFinalizer)
	klog.Info("addFinalizer", cluster.Finalizers)
	err := r.Update(ctx, cluster)
	if err != nil {
		return fmt.Errorf("add cluster finalizer failed %+v", err)
	}
	return nil
}

func (r *ClusterReconciler) patchKubeControlPlanNodes(ctx context.Context, cluster *v1.Cluster) error {
	nodes := new(v1.NodeList)
	if err := r.List(ctx, nodes); err != nil {
		return err
	}
	patch := func(ctx context.Context, name string) error {
		node, err := r.getNode(ctx, nodes, name)
		if err != nil {
			klog.Errorf("patch machine node failed %+v", err)
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}
		return r.patchMachineNode(ctx, cluster, node)
	}
	for _, name := range cluster.Spec.ControlPlane.Nodes {
		if err := patch(ctx, name); err != nil {
			return fmt.Errorf("KubeControlPlane %+v", err)
		}
	}
	return nil
}

func (r *ClusterReconciler) patchMachineNode(ctx context.Context, cluster *v1.Cluster, node *v1.Node) error {
	if cluster.DeletionTimestamp.IsZero() {
		if node.Labels == nil {
			node.Labels = map[string]string{}
		}
		node.Spec.Cluster = &cluster.Name
		node.OwnerReferences = addOwnerReferences(node.OwnerReferences, cluster)
	} else if cluster.Status.ControlPlaneStatus.Phase == v1.DeletedPhase {
		node.Spec.Cluster = nil
		node.OwnerReferences = utils.RemoveOwnerReferences(node.OwnerReferences, cluster.UID)
		klog.Infof("nodes %s remove  owner references", node.Name)
	} else {
		return nil
	}
	return r.Update(ctx, node)
}

func (r *ClusterReconciler) generateSSHSecret(ctx context.Context, cluster *v1.Cluster) error {
	secret := new(corev1.Secret)
	err := r.Get(ctx, types.NamespacedName{
		Namespace: common.PrimusSafeNamespace,
		Name:      cluster.Name,
	}, secret)
	if err == nil || !errors.IsNotFound(err) {
		return err
	}

	node := new(v1.Node)
	err = r.Get(ctx, types.NamespacedName{Name: cluster.Spec.ControlPlane.Nodes[0]}, node)
	if err != nil {
		return err
	}
	username, err := r.ClusterBaseReconciler.getUsername(ctx, node, cluster)
	if err != nil {
		return nil
	}
	private, pub, err := secure.MakeSSHKeyPair()
	if err != nil {
		return err
	}
	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            cluster.Name,
			Namespace:       common.PrimusSafeNamespace,
			OwnerReferences: []metav1.OwnerReference{createKubernetesClusterOwnerReference(cluster)},
		},
		Data: map[string][]byte{
			utils.Username:     []byte(username),
			utils.Authorize:    private,
			utils.AuthorizePub: pub,
		},
		Type: "Opaque",
	}
	err = r.Create(ctx, secret)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}
	return nil
}

func (r *ClusterReconciler) podClear(ctx context.Context, cluster *v1.Cluster) error {
	labelSelector := client.MatchingLabels{v1.ClusterManageClusterLabel: cluster.Name}
	list := new(corev1.PodList)
	err := r.List(ctx, list, client.InNamespace(common.PrimusSafeNamespace), labelSelector)
	if err != nil {
		return err
	}
	for _, pod := range list.Items {
		klog.Infof("pod: %s, phase: %s", pod.Name, pod.Status.Phase)
		if pod.Status.Phase != corev1.PodSucceeded {
			continue
		}
		if time.Now().UTC().After(pod.CreationTimestamp.Add(time.Hour)) {
			if err = r.Delete(ctx, &pod); err != nil {
				klog.Errorf("cluster %s delete pod failed %+v", cluster.Name, err)
			}
		}
	}
	return nil
}

func (r *ClusterReconciler) guaranteeService(ctx context.Context, cluster *v1.Cluster) error {
	if cluster.Status.ControlPlaneStatus.Phase != v1.ReadyPhase && cluster.Status.ControlPlaneStatus.Phase != v1.CreatedPhase {
		return nil
	}
	nodes, err := r.getControllerPlaneNodes(ctx, cluster)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return nil
	}
	endpoint := new(corev1.Endpoints)
	address := make([]corev1.EndpointAddress, 0, len(nodes))
	for _, node := range nodes {
		address = append(address, corev1.EndpointAddress{
			IP: node.Spec.PrivateIP,
		})
	}
	err = r.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: common.PrimusSafeNamespace}, endpoint)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		endpoint = &corev1.Endpoints{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: common.PrimusSafeNamespace,
				OwnerReferences: []metav1.OwnerReference{
					createKubernetesClusterOwnerReference(cluster),
				},
			},
			Subsets: []corev1.EndpointSubset{
				{
					Addresses:         address,
					NotReadyAddresses: nil,
					Ports: []corev1.EndpointPort{
						{
							Name:     "https",
							Port:     6443,
							Protocol: "TCP",
						},
					},
				},
			},
		}
		err = r.Create(ctx, endpoint)
		if err != nil {
			return fmt.Errorf("create cluster endpoint failed %+v", err)
		}
	}
	service := new(corev1.Service)
	err = r.Get(ctx, types.NamespacedName{
		Name:      cluster.Name,
		Namespace: common.PrimusSafeNamespace,
	}, service)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		service = &corev1.Service{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:            cluster.Name,
				Namespace:       common.PrimusSafeNamespace,
				OwnerReferences: []metav1.OwnerReference{createKubernetesClusterOwnerReference(cluster)},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{
					Name:       "https",
					Protocol:   "TCP",
					Port:       443,
					TargetPort: intstr.IntOrString{IntVal: 6443},
				}},
				Type:            corev1.ServiceTypeClusterIP,
				SessionAffinity: corev1.ServiceAffinityNone,
			},
			Status: corev1.ServiceStatus{},
		}
		if err = r.Create(ctx, service); err != nil {
			return fmt.Errorf("create cluster service failed %+v", err)
		}
	}
	return nil
}
