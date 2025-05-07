/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"

	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/secure"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
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
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type ClusterReconciler struct {
	*BaseReconciler
}

func SetupClusterController(mgr manager.Manager) error {
	r := &ClusterReconciler{
		&BaseReconciler{
			Client: mgr.GetClient(),
		},
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Cluster{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Cluster Controller successfully")
	return nil
}

func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	startTime := time.Now().UTC()
	defer func() {
		klog.V(4).Infof("Finished node reconcile %s cost (%v)", req.Name, time.Since(startTime))
	}()
	klog.Infof("%+s", req.Name)
	cluster := new(v1.Cluster)
	err := r.Get(ctx, req.NamespacedName, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrlruntime.Result{}, nil
		}
		return ctrlruntime.Result{}, err
	}
	if err := r.ensureClusterControlePlane(ctx, cluster); err != nil {
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

func (r *ClusterReconciler) ensureClusterControlePlane(ctx context.Context, cluster *v1.Cluster) error {
	if err := r.addFinalizer(ctx, cluster); err != nil {
		return err
	}
	if len(cluster.Spec.ControlPlane.Nodes) == 0 {
		return nil
	}
	if cluster.Spec.ControlPlane.SSHSecret == nil {
		err := r.generateSSHSecret(ctx, cluster)
		if err != nil {
			return err
		}
	}

	if cluster.Status.ControlePlaneStatus.Phase == "" {
		if err := r.fetchProvisionedClusterKubeConfig(ctx, cluster); err != nil {
			return err
		}
		if cluster.Status.ControlePlaneStatus.Phase != "" {
			return nil
		}
	}

	if err := r.patchKubeControlPlanNodes(ctx, cluster); err != nil {
		return err
	}

	hostsContent, err := r.generateHosts(ctx, cluster, nil)
	if err != nil {
		if !cluster.DeletionTimestamp.IsZero() {
			klog.Infof("delete %s finalizer", KubernetesFinalizer)
			var ok bool
			cluster.Finalizers, ok = slice.RemoveString(cluster.Finalizers, KubernetesFinalizer)
			if !ok {
				return nil
			}
			return r.Update(ctx, cluster)
		}
		return err
	}
	if cluster.Status.ControlePlaneStatus.Phase != v1.ReadyPhase && hostsContent == nil && cluster.DeletionTimestamp.IsZero() {
		klog.Infof("cluster %s Kube control plane nodes not ready, plase wait", cluster.Name)
		return nil
	}
	if !cluster.DeletionTimestamp.IsZero() {
		return r.reset(ctx, cluster, hostsContent)
	}

	phase := cluster.Status.ControlePlaneStatus.Phase
	if phase == "" || phase == v1.PendingPhase || phase == v1.CreatingPhase || phase == v1.CreationFailed {

		pod, err := r.ensureInitWorkerPodCreated(ctx, cluster, hostsContent)
		if err != nil {
			return err
		}
		if pod != nil {
			c := client.MergeFrom(cluster.DeepCopy())
			cluster.Status.ControlePlaneStatus.Phase = v1.CreatingPhase
			if pod.Status.Phase == corev1.PodSucceeded {
				cluster.Status.ControlePlaneStatus.Phase = v1.CreatedPhase
			} else if pod.Status.Phase == corev1.PodFailed {
				cluster.Status.ControlePlaneStatus.Phase = v1.CreationFailed
			}
			if err := r.Status().Patch(ctx, cluster, c); err != nil {
				return err
			}
		} else {
			return nil
		}
	}

	if err := r.fetchProvisionedClusterKubeConfig(ctx, cluster); err != nil {
		return err
	}

	if err := r.ensureService(ctx, cluster); err != nil {
		return err
	}
	if cluster.Status.ControlePlaneStatus.Phase == v1.ReadyPhase {
		if err = r.podClear(ctx, cluster); err != nil {
			return err
		}
	}
	return nil
}

func (r *ClusterReconciler) reset(ctx context.Context, cluster *v1.Cluster, hostsContent *HostTemplateContent) error {
	if cluster.Status.ControlePlaneStatus.Phase == v1.DeletedPhase {
		if err := r.patchKubeControlPlanNodes(ctx, cluster); err != nil {
			return err
		}
		pod, _ := r.ensurePod(ctx, cluster.Name, v1.ClusterCreateAction)
		if pod != nil {
			for _, m := range hostsContent.Controllers {
				pod.OwnerReferences = removeOwnerReferences(pod.OwnerReferences, m.UID)
			}
			klog.Infof("remove machine node OwnerReferences %+v", pod.OwnerReferences)
			if err := r.Update(ctx, pod); err != nil {
				return err
			}
		}
		var ok bool
		cluster.Finalizers, ok = slice.RemoveString(cluster.Finalizers, KubernetesFinalizer)
		if !ok {
			return nil
		}
		return r.Update(ctx, cluster)

	}
	c := client.MergeFrom(cluster.DeepCopy())
	if cluster.Status.ControlePlaneStatus.Phase == v1.CreationFailed || cluster.Status.ControlePlaneStatus.Phase == v1.PendingPhase {
		cluster.Status.ControlePlaneStatus.Phase = v1.DeletedPhase
	} else {
		pod, err := r.ensureResetWorkPodCreated(ctx, cluster, hostsContent)
		if err != nil {
			return err
		}
		_, err = r.ensureHostsConfigMapCreated(ctx, cluster.Name, metav1.OwnerReference{
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
			cluster.Status.ControlePlaneStatus.Phase = v1.DeletedPhase
		} else if pod.Status.Phase == corev1.PodFailed {
			cluster.Status.ControlePlaneStatus.Phase = v1.DeleteFailedPhase
		} else {
			cluster.Status.ControlePlaneStatus.Phase = v1.DeletingPhase
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
	return r.BaseReconciler.getUsername(ctx, node, cluster)
}

func (r *ClusterReconciler) ensureInitWorkerPodCreated(ctx context.Context, cluster *v1.Cluster, hostsContent *HostTemplateContent) (*corev1.Pod, error) {
	labelSelector := client.MatchingLabels{v1.ClusterManageActionLabel: string(v1.ClusterCreateAction), v1.ClusterManageClusterLabel: cluster.Name}
	list := new(corev1.PodList)
	err := r.List(ctx, list, client.InNamespace(common.PrimusSafeNamespace), labelSelector)
	if err != nil {
		return nil, err
	}
	if len(list.Items) > 0 {
		pod := list.Items[0]
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == cluster.Kind && owner.UID == cluster.UID {
				_, err = r.ensureHostsConfigMapCreated(ctx, cluster.Name, metav1.OwnerReference{
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
				return &pod, nil
			}
		}
		return nil, r.Delete(ctx, &pod)
	}
	username, err := r.getUsername(ctx, cluster)
	if err != nil {
		return nil, err
	}
	cmd := GetKubeSprayCreateCMD(username, GetKubeSprayEnv(cluster))
	pod := generateWorkerPod(v1.ClusterCreateAction, cluster, username, cmd, getKubesprayImage(cluster), cluster.Name, hostsContent)
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
	err = r.List(ctx, list, client.InNamespace(common.PrimusSafeNamespace), labelSelector)
	if err != nil {
		return nil, err
	}

	_, err = r.ensureHostsConfigMapCreated(ctx, cluster.Name, metav1.OwnerReference{
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

func (r *ClusterReconciler) ensureResetWorkPodCreated(ctx context.Context, cluster *v1.Cluster, hostsContent *HostTemplateContent) (*corev1.Pod, error) {
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
	cmd := GetKubeSprayResetCMD(username, GetKubeSprayEnv(cluster))
	pod := generateWorkerPod(v1.ClusterResetAction, cluster, username, cmd, getKubesprayImage(cluster), cluster.Name, hostsContent)
	if err := r.Create(ctx, pod); err != nil {
		return nil, err
	}
	return pod, nil
}

func (r *ClusterReconciler) getNodes(ctx context.Context, names []string) []*v1.Node {
	nodes := make([]*v1.Node, 0, len(names))
	list := new(v1.NodeList)
	if err := r.List(ctx, list); err != nil {
		return nodes
	}
	for _, v := range names {
		node, err := r.getNode(ctx, list, v)
		if err != nil {
			klog.Error(err)
		}
		nodes = append(nodes, node)
	}
	return nodes
}
func (r *ClusterReconciler) fetchProvisionedClusterKubeConfig(ctx context.Context, cluster *v1.Cluster) error {
	if cluster.Status.ControlePlaneStatus.Phase != v1.CreatedPhase && cluster.Status.ControlePlaneStatus.Phase != "" {
		return nil
	}
	if len(cluster.Status.ControlePlaneStatus.CAData) != 0 && len(cluster.Status.ControlePlaneStatus.CertData) != 0 && len(cluster.Status.ControlePlaneStatus.KeyData) != 0 {
		return nil
	}
	nodes := r.getNodes(ctx, cluster.Spec.ControlPlane.Nodes)
	if len(nodes) == 0 {
		return nil
	}
	node := nodes[0]
	if node == nil {
		return nil
	}
	secret := new(corev1.Secret)
	err := r.Get(ctx, types.NamespacedName{
		Namespace: node.Spec.SSHSecret.Namespace,
		Name:      node.Spec.SSHSecret.Name,
	}, secret)
	if err != nil {
		return err
	}
	sshConfig, err := getSHHConfig(secret)
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
		klog.Infof("cat %s failed  %v", ProvisionedKubeConfigPath, err)
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
	conf.Host = fmt.Sprintf("https://%s:6443", nodes[0].Spec.PrivateIP)
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
	crypto := crypto.Instance()
	cluster.Status.ControlePlaneStatus.CertData, err = crypto.Encrypt([]byte(base64.StdEncoding.EncodeToString(conf.CertData)))
	if err != nil {
		return fmt.Errorf("cert  encrypt error %+v", err)
	}
	cluster.Status.ControlePlaneStatus.CAData, err = crypto.Encrypt([]byte(base64.StdEncoding.EncodeToString(conf.CAData)))
	if err != nil {
		return fmt.Errorf("ca encrypt error %+v", err)
	}
	cluster.Status.ControlePlaneStatus.KeyData, err = crypto.Encrypt([]byte(base64.StdEncoding.EncodeToString(conf.KeyData)))
	if err != nil {
		return fmt.Errorf("key encrypt error %+v", err)
	}
	cluster.Status.ControlePlaneStatus.Endpoints = make([]string, 0, len(nodes))
	for _, n := range nodes {
		cluster.Status.ControlePlaneStatus.Endpoints = append(cluster.Status.ControlePlaneStatus.Endpoints, fmt.Sprintf("https://%s:6443", n.Spec.PrivateIP))
	}
	cluster.Status.ControlePlaneStatus.Phase = v1.ReadyPhase
	if err := r.ensureService(ctx, cluster); err != nil {
		return err
	}
	err = r.Status().Patch(ctx, cluster, c)
	if err != nil {
		return fmt.Errorf("failed load config%+v", err)
	}
	return nil
}

func (r *ClusterReconciler) addFinalizer(ctx context.Context, cluster *v1.Cluster) error {
	if slice.ContainsString(cluster.Finalizers, KubernetesFinalizer) {
		return nil
	}
	cluster.Finalizers = append(cluster.Finalizers, KubernetesFinalizer)
	klog.Info("addFinalizer", cluster.Finalizers)
	err := r.Update(ctx, cluster)
	if err != nil {
		return fmt.Errorf("add kebespray finalizer failed %+v", err)
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
		node.Labels[v1.ClusterManageNodeClusterLabel] = cluster.Name
		node.Labels[v1.ClusterNameLabel] = cluster.Name
		node.Spec.Cluster = &cluster.Name
		node.OwnerReferences = addOwnerReferences(node.OwnerReferences, cluster)
	} else if cluster.Status.ControlePlaneStatus.Phase == v1.DeletedPhase {
		if _, ok := node.Labels[v1.ClusterManageNodeClusterLabel]; ok {
			delete(node.Labels, v1.ClusterManageNodeClusterLabel)
		}
		if _, ok := node.Labels[v1.ClusterNameLabel]; ok {
			delete(node.Labels, v1.ClusterNameLabel)
		}
		node.Spec.Cluster = nil
		node.OwnerReferences = removeOwnerReferences(node.OwnerReferences, cluster.UID)
		klog.Infof("machine nodes %s remove  owner references", node.Name)
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
	klog.Infof("%+v", err)
	username, err := r.BaseReconciler.getUsername(ctx, node, cluster)
	if err != nil {
		return nil
	}
	klog.Infof("%+v", err)
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
			Username:     []byte(username),
			Authorize:    private,
			AuthorizePub: pub,
		},
		Type: "Opaque",
	}
	err = r.Create(ctx, secret)
	klog.Infof("%+v", err)
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
	err := r.List(ctx, list, client.InNamespace(""), labelSelector)
	if err != nil {
		return err
	}
	machines := new(v1.NodeList)
	labelSelector = client.MatchingLabels{v1.ClusterManageNodeClusterLabel: cluster.Name}
	err = r.List(ctx, machines, labelSelector)
	if err != nil {
		return err
	}
	getMachineNode := func(name string) *v1.Node {
		for i := range machines.Items {
			if machines.Items[i].Name == name {
				return machines.Items[i].DeepCopy()
			}
		}
		return nil
	}
	for _, pod := range list.Items {
		klog.Info(pod.Name, pod.Status.Phase)
		if pod.Status.Phase != corev1.PodSucceeded {
			continue
		}
		if _, ok := pod.Labels[v1.ClusterManageScaleDownLabel]; !ok && pod.Labels[v1.ClusterManageActionLabel] == string(v1.ClusterScaleDownAction) {
			machine := getMachineNode(pod.Labels[v1.ClusterManageNodeLabel])
			if machine != nil {
				if _, ok := machine.Labels[v1.ClusterManageNodeClusterLabel]; ok {
					delete(machine.Labels, v1.ClusterManageNodeClusterLabel)
				}
				machine.OwnerReferences = removeOwnerReferences(machine.OwnerReferences, cluster.UID)
				klog.Infof("machine nodes %s remove  owner references", machine.Name)
				if err := r.Update(ctx, machine); err != nil {
					return err
				}
			}
			p := client.MergeFrom(pod.DeepCopy())
			if pod.Labels == nil {
				pod.Labels = map[string]string{}
			}
			pod.Labels[v1.ClusterManageScaleDownLabel] = "true"
			if err = r.Patch(ctx, &pod, p); err != nil {
				klog.Errorf("kubernetes cluster %s scale down machine node %s faild %+v", cluster.Name, pod.Labels[v1.ClusterManageNodeLabel], err)
			}
		}
		if time.Now().UTC().After(pod.CreationTimestamp.Add(time.Hour)) {
			if err = r.Delete(ctx, &pod); err != nil {
				klog.Errorf("kubernetes cluster %s delete scale down pod failed %+v", cluster.Name, err)
			}
		}
	}
	return nil
}

func (r *ClusterReconciler) ensureService(ctx context.Context, cluster *v1.Cluster) error {
	klog.Infof("ensureService %s Phase %s", cluster.Name, cluster.Status.ControlePlaneStatus.Phase)
	if cluster.Status.ControlePlaneStatus.Phase != v1.ReadyPhase && cluster.Status.ControlePlaneStatus.Phase != v1.CreatedPhase {
		return nil
	}
	nodes := r.getNodes(ctx, cluster.Spec.ControlPlane.Nodes)
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
	err := r.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: common.PrimusSafeNamespace}, endpoint)
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
				Namespace:       "",
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
