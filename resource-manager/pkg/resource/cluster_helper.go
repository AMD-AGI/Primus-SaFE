/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

type ClusterBaseReconciler struct {
	client.Client
}

func (r *ClusterBaseReconciler) getUsername(ctx context.Context, node *v1.Node, cluster *v1.Cluster) (string, error) {
	if cluster.Spec.ControlPlane.SSHSecret != nil {
		secret := new(corev1.Secret)
		err := r.Get(ctx, apitypes.NamespacedName{
			Namespace: cluster.Spec.ControlPlane.SSHSecret.Namespace,
			Name:      cluster.Spec.ControlPlane.SSHSecret.Name,
		}, secret)
		if err != nil {
			return "", err
		}
		if data, ok := secret.Data[Username]; ok {
			return string(data), nil
		}
	}
	if node.Spec.SSHSecret != nil {
		secret := new(corev1.Secret)
		err := r.Get(ctx, apitypes.NamespacedName{Name: node.Spec.SSHSecret.Name, Namespace: node.Spec.SSHSecret.Namespace}, secret)
		if err != nil {
			return "", err
		}
		if data, ok := secret.Data[Username]; ok {
			return string(data), nil
		} else {
			return "root", nil
		}
	}
	secret := new(corev1.Secret)
	err := r.Get(ctx, apitypes.NamespacedName{
		Namespace: common.PrimusSafeNamespace,
		Name:      cluster.Name,
	}, secret)
	if err != nil {
		return "", err
	}
	username := "root"
	if data, ok := secret.Data[Username]; ok {
		username = string(data)
	}
	return username, nil
}

func (r *ClusterBaseReconciler) getNode(ctx context.Context, nodes *v1.NodeList, name string) (*v1.Node, error) {
	for _, n := range nodes.Items {
		if n.Name == name {
			return n.DeepCopy(), nil
		}
	}
	node := new(v1.Node)
	err := r.Get(ctx, apitypes.NamespacedName{
		Name: name,
	}, node)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (r *ClusterBaseReconciler) generateHosts(ctx context.Context, cluster *v1.Cluster, worker *v1.Node) (*HostTemplateContent, error) {
	controllers := make([]*v1.Node, 0, len(cluster.Spec.ControlPlane.Nodes))
	for _, v := range cluster.Spec.ControlPlane.Nodes {
		node := new(v1.Node)
		err := r.Get(ctx, apitypes.NamespacedName{Name: v}, node)
		if err != nil {
			return nil, err
		}
		if !isReadyMachineNode(node) {
			klog.Infof("machine node %s not ready status is %s", node.Name, node.Status.MachineStatus.Phase)
			continue
		}
		controllers = append(controllers, node)
	}

	if len(controllers) != len(cluster.Spec.ControlPlane.Nodes) {
		return nil, nil
	}

	hostsContent := &HostTemplateContent{
		NodeAndIP:     make([]string, 0, len(controllers)+1),
		MasterName:    make([]string, 0, len(controllers)),
		NodeName:      make([]string, 0, 1),
		EtcdNodeName:  make([]string, 0, len(controllers)),
		Hosts:         make([]string, 0, len(controllers)+1),
		PodHostsAlias: map[string]string{},
		Labels:        map[string]string{},
		ClusterName:   cluster.Name,
		Controllers:   controllers,
	}
	if cluster.Spec.ClusterID != nil {
		hostsContent.ClusterID = *cluster.Spec.ClusterID
	} else {
		hostsContent.ClusterID = "1.0.0.1"
	}
	count := 0
	for _, machine := range controllers {
		hostname := machine.Status.MachineStatus.HostName
		publicIP := machine.Spec.PublicIP
		if publicIP == "" {
			publicIP = machine.Spec.PrivateIP
		}
		username, err := r.getUsername(ctx, machine, cluster)
		if err != nil {
			return nil, err
		}
		nodeAndIp := fmt.Sprintf("%s ansible_host=%s ip=%s ansible_ssh_user=%s main_access_ip=%s", hostname, publicIP, machine.Spec.PrivateIP, username, machine.Spec.PrivateIP)
		hostsContent.MasterName = append(hostsContent.MasterName, hostname)
		hostsContent.EtcdNodeName = append(hostsContent.EtcdNodeName, hostname)
		hostsContent.NodeAndIP = append(hostsContent.NodeAndIP, nodeAndIp)
		if hostname != publicIP {
			hostsContent.Hosts = append(hostsContent.Hosts, fmt.Sprintf("%s %s", publicIP, hostname))
			hostsContent.PodHostsAlias[hostname] = publicIP
		}
		if l, ok := getNodeLabelsString(machine); ok {
			hostsContent.Labels[machine.Name] = l
		}
		count++
	}
	if worker != nil {
		node := new(v1.Node)
		err := r.Get(ctx, apitypes.NamespacedName{Name: worker.Name}, node)
		if err != nil {
			return nil, err
		}
		hostname := node.Status.MachineStatus.HostName
		publicIP := node.Spec.PublicIP
		if publicIP == "" {
			publicIP = node.Spec.PrivateIP
		}
		username, err := r.getUsername(ctx, node, cluster)
		if err != nil {
			return nil, err
		}
		nodeAndIp := fmt.Sprintf("%s ansible_host=%s ip=%s ansible_ssh_user=%s", hostname, publicIP, node.Spec.PrivateIP, username)
		hostsContent.NodeName = append(hostsContent.NodeName, hostname)
		hostsContent.NodeAndIP = append(hostsContent.NodeAndIP, nodeAndIp)
		if hostname != publicIP {
			hostsContent.Hosts = append(hostsContent.Hosts, fmt.Sprintf("%s %s", publicIP, hostname))
			hostsContent.PodHostsAlias[hostname] = publicIP
		}

		if l, ok := getNodeLabelsString(node); ok {
			hostsContent.Labels[node.Name] = l
		}
		count++
	}
	if len(hostsContent.NodeName) == 0 {
		hostsContent.NodeName = append(hostsContent.NodeName, hostsContent.MasterName...)
	}
	return hostsContent, nil
}

func (r *ClusterBaseReconciler) guaranteeHostsConfigMapCreated(ctx context.Context, name string, owner metav1.OwnerReference, hostsContent *HostTemplateContent) (*corev1.ConfigMap, error) {
	kubesprayHostData := &strings.Builder{}
	tmpl := template.Must(template.New("").Parse(kubesprayHostsTemplate))
	if err := tmpl.Execute(kubesprayHostData, hostsContent); err != nil {
		return nil, err
	}

	hostData := &strings.Builder{}

	cm := new(corev1.ConfigMap)
	err := r.Get(ctx, apitypes.NamespacedName{
		Namespace: common.PrimusSafeNamespace,
		Name:      name,
	}, cm)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}
		cm = &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: common.PrimusSafeNamespace,
				OwnerReferences: []metav1.OwnerReference{
					owner,
				},
			},
			Data: map[string]string{
				HostsYaml: strings.TrimSpace(kubesprayHostData.String()),
				Hosts:     strings.TrimSpace(hostData.String()),
			},
		}
		if err := r.Client.Create(ctx, cm); err != nil {
			return nil, err
		}
	} else {
		c := client.MergeFrom(cm.DeepCopy())
		cm.Data[HostsYaml] = strings.TrimSpace(kubesprayHostData.String())
		cm.Data[Hosts] = strings.TrimSpace(hostData.String())
		err = r.Patch(ctx, cm, c)
		if err != nil {
			return nil, err
		}
	}
	klog.Info("hostsContent.Hosts length", len(hostsContent.Hosts))
	return cm, nil
}

func (r *ClusterBaseReconciler) getPod(ctx context.Context, labelSelector client.MatchingLabels) (*corev1.Pod, error) {
	list := new(corev1.PodList)
	err := r.List(ctx, list, client.InNamespace(common.PrimusSafeNamespace), labelSelector)
	if err != nil {
		return nil, err
	}
	if len(list.Items) > 0 {
		return &list.Items[0], nil
	}
	return nil, nil
}

func (r *ClusterBaseReconciler) getPodList(ctx context.Context, labelSelector client.MatchingLabels) ([]corev1.Pod, error) {
	list := new(corev1.PodList)
	err := r.List(ctx, list, client.InNamespace(common.PrimusSafeNamespace), labelSelector)
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (r *ClusterBaseReconciler) getCluster(ctx context.Context, clusterName string) (*v1.Cluster, error) {
	if clusterName == "" {
		return nil, nil
	}
	cluster := new(v1.Cluster)
	err := r.Get(ctx, apitypes.NamespacedName{Name: clusterName}, cluster)
	if err != nil {
		return nil, err
	}
	return cluster, nil
}

//go:embed kubespray_hosts.template
var kubesprayHostsTemplate string

const (
	ProvisionedKubeConfigPath = "/etc/kubernetes/admin.conf"
	Username                  = "username"
	Password                  = "password"
	Root                      = "root"
	ClusterKubeSprayHosts     = "cluster-kube-spray-hosts"
	ClusterSecret             = "cluster-secret"
	Hosts                     = "hosts"
	HostsYaml                 = "hosts.yaml"
	Authorize                 = "authorize"
	AuthorizePub              = "authorize.pub"
	HarborCA                  = "HarborCa"
)

var DefaultKubeletConfigArgs = map[string]string{
	"node-status-update-frequency": "60s",
}

type HostTemplateContent struct {
	NodeAndIP     []string
	MasterName    []string
	NodeName      []string
	EtcdNodeName  []string
	Hosts         []string
	PodHostsAlias map[string]string
	Labels        map[string]string
	ClusterName   string
	ClusterID     string
	Controllers   []*v1.Node
}

func getSHHConfig(secret *corev1.Secret) (*ssh.ClientConfig, error) {
	var username string
	if data, ok := secret.Data[Username]; ok {
		username = string(data)
	} else {
		username = Root
	}
	sshConfig := &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	if sshPrivateKeyData, ok := secret.Data[Authorize]; ok {
		signer, err := ssh.ParsePrivateKey(sshPrivateKeyData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse SSH private key: %v", err)
		}
		sshConfig.Auth = append(sshConfig.Auth, ssh.PublicKeys(signer))
	} else if password, ok := secret.Data[Password]; ok {
		sshConfig.Auth = append(sshConfig.Auth, ssh.Password(string(password)))
	} else {
		return nil, fmt.Errorf("ssh private key or password not found in secret")
	}
	return sshConfig, nil
}

func generateWorkerPod(action v1.ClusterManageAction, cluster *v1.Cluster, username, cmd, image, config string, hostsContent *HostTemplateContent) *corev1.Pod {
	name := cluster.Name + "-" + string(action)
	hostsAlias := make([]corev1.HostAlias, 0, len(hostsContent.PodHostsAlias))
	for hostname, ip := range hostsContent.PodHostsAlias {
		hostsAlias = append(hostsAlias, corev1.HostAlias{
			IP: ip,
			Hostnames: []string{
				hostname,
			},
		})
	}
	mode := pointer.Int32(0400)

	if len(cluster.Spec.ControlPlane.KubeApiServerArgs) > 0 {
		replace := "kube_kubeadm_apiserver_extra_args:"
		for k, v := range cluster.Spec.ControlPlane.KubeApiServerArgs {
			replace = fmt.Sprintf("%s \n  %s: %s", replace, k, v)
		}
		cmd = fmt.Sprintf("sed -i \"/^kube_kubeadm_apiserver_extra_args: /d\" roles/kubernetes/control-plane/defaults/main/main.yml && echo \"%s\" >> roles/kubernetes/control-plane/defaults/main/main.yml && %s", replace, cmd)
	}

	kubeletArgs := "kubelet_config_extra_args:"
	for k, v := range DefaultKubeletConfigArgs {
		if _, ok := cluster.Spec.ControlPlane.KubeletConfigArgs[k]; ok {
			continue
		}
		kubeletArgs = fmt.Sprintf("%s \n  %s: %s", kubeletArgs, k, v)
	}

	for k, v := range cluster.Spec.ControlPlane.KubeletConfigArgs {
		kubeletArgs = fmt.Sprintf("%s \n  %s: %s", kubeletArgs, k, v)
	}
	cmd = fmt.Sprintf("sed -i \"/^kubelet_config_extra_args: /d\" roles/kubernetes/node/defaults/main.yml && echo \"%s\" >> roles/kubernetes/node/defaults/main.yml && %s", kubeletArgs, cmd)

	sshSecretName := cluster.Name
	if cluster.Spec.ControlPlane.SSHSecret != nil {
		sshSecretName = cluster.Spec.ControlPlane.SSHSecret.Name
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				v1.ClusterManageClusterLabel: cluster.Name,
				v1.ClusterManageActionLabel:  string(action),
			},
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
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    string(action),
					Command: []string{"/bin/bash", "-c"},
					Args:    []string{cmd},
					Image:   image,
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      ClusterKubeSprayHosts,
							ReadOnly:  true,
							MountPath: "kubespray/hosts",
						},
						{
							Name:      ClusterSecret,
							ReadOnly:  true,
							MountPath: "kubespray/.ssh",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: ClusterKubeSprayHosts,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: config,
							},
							Items: []corev1.KeyToPath{
								{
									Key:  HostsYaml,
									Path: HostsYaml,
								},
								{
									Key:  Hosts,
									Path: Hosts,
								},
							},
						},
					},
				},
				{
					Name: ClusterSecret,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  sshSecretName,
							DefaultMode: mode,
							Items: []corev1.KeyToPath{
								{
									Key:  Authorize,
									Path: Authorize,
								},
							},
						},
					},
				},
			},
			HostAliases:   hostsAlias,
			RestartPolicy: corev1.RestartPolicyNever,
		},
		Status: corev1.PodStatus{},
	}
	if cluster.Spec.ControlPlane.ImageSecret != nil {
		pod.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: cluster.Spec.ControlPlane.ImageSecret.Name,
			},
		}
	}
	return pod
}

func generateScaleWorkerPod(action v1.ClusterManageAction, cluster *v1.Cluster, node *v1.Node, useName, cmd, image, config string, hostsContent *HostTemplateContent) *corev1.Pod {
	pod := generateWorkerPod(action, cluster, useName, cmd, image, config, hostsContent)
	name := fmt.Sprintf("%s-%s-%s", cluster.Name, node.Name, action)
	if len(name) > 58 {
		name = name[:58]
	}
	pod.Name = name
	pod.Labels[v1.ClusterManageNodeLabel] = node.Name
	pod.OwnerReferences = append(pod.OwnerReferences, metav1.OwnerReference{
		APIVersion: node.APIVersion,
		Kind:       node.Kind,
		Name:       node.Name,
		UID:        node.UID,
	})
	return pod
}
func getKubeSprayCreateCMD(user, env string) string {
	return fmt.Sprintf("ansible-playbook -i hosts/hosts.yaml --private-key .ssh/%s cluster.yml --become-user=root %s -b -vvv", Authorize, env)
}

func getKubeSprayHostsCMD(user string) string {
	cmd := fmt.Sprintf("ansible all -i hosts/hosts.yaml --private-key .ssh/%s -m copy -a \"src=inventory/hosts dest=/etc/hosts mode=u+x\" --become-user=root -b -vvv", Authorize)
	if user == "" || user == "root" {
		return cmd
	}
	return fmt.Sprintf("groupadd -r kubespray && useradd -r -g kubespray %s && mkdir -p /home/%s && chmod -R 777 /home/%s && su %s -c '%s'",
		user, user, user, user, cmd)
}

func getKubeSprayEnv(cluster *v1.Cluster) string {
	cmd := ""
	if cluster.Spec.ControlPlane.KubeVersion != nil {
		cmd = fmt.Sprintf("%s -e kube_version=%s", cmd, *cluster.Spec.ControlPlane.KubeVersion)
	}
	if cluster.Spec.ControlPlane.KubePodsSubnet != nil {
		cmd = fmt.Sprintf("%s -e kube_pods_subnet=%s", cmd, *cluster.Spec.ControlPlane.KubePodsSubnet)
	}
	if cluster.Spec.ControlPlane.KubeServiceAddress != nil {
		cmd = fmt.Sprintf("%s -e kube_service_addresses=%s", cmd, *cluster.Spec.ControlPlane.KubeServiceAddress)
	}
	if cluster.Spec.ControlPlane.KubeProxyMode != nil {
		cmd = fmt.Sprintf("%s -e kube_proxy_mode=%s", cmd, *cluster.Spec.ControlPlane.KubeProxyMode)
	}
	if cluster.Spec.ControlPlane.KubeNetworkPlugin != nil {
		cmd = fmt.Sprintf("%s -e kube_network_plugin=%s", cmd, *cluster.Spec.ControlPlane.KubeNetworkPlugin)
	}
	if cluster.Spec.ControlPlane.NodeLocalDNSIP != nil {
		cmd = fmt.Sprintf("%s -e nodelocaldns_ip=%s", cmd, *cluster.Spec.ControlPlane.NodeLocalDNSIP)
	}
	if cluster.Spec.ControlPlane.KubeletLogFilesMaxSize != nil {
		cmd = fmt.Sprintf("%s -e kubelet_logfiles_max_size=%s", cmd, cluster.Spec.ControlPlane.KubeletLogFilesMaxSize.String())
	}

	if cluster.Spec.ControlPlane.KubeNetworkNodePrefix != nil {
		cmd = fmt.Sprintf("%s -e kube_network_node_prefix=%d", cmd, *cluster.Spec.ControlPlane.KubeNetworkNodePrefix)
	}
	cmd = fmt.Sprintf("%s -e auto_renew_certificates=true -e nginx_image_repo=public.ecr.aws/docker/library/nginx", cmd)
	cmd = fmt.Sprintf("%s -e kube_controller_node_monitor_grace_period=5m -e kube_apiserver_pod_eviction_not_ready_timeout_seconds=60 -e kube_apiserver_pod_eviction_unreachable_timeout_seconds=60", cmd)
	return cmd
}

func getKubeSprayResetCMD(user, env string) string {
	return fmt.Sprintf("ansible-playbook -i hosts/hosts.yaml --private-key .ssh/%s reset.yml -e reset_confirmation=yes %s --become-user=root -b -vvv", Authorize, env)
}

func getKubesprayImage(cluster *v1.Cluster) string {
	if cluster.Spec.ControlPlane.KubeSprayImage != nil && *cluster.Spec.ControlPlane.KubeSprayImage != "" {
		return *cluster.Spec.ControlPlane.KubeSprayImage
	}
	return "quay.io/kubespray/kubespray:v2.24.0"
}

func addOwnerReferences(references []metav1.OwnerReference, cluster *v1.Cluster) []metav1.OwnerReference {
	for _, r := range references {
		if r.UID == cluster.UID {
			return references
		}
	}
	references = append(references, metav1.OwnerReference{
		APIVersion: cluster.APIVersion,
		Kind:       cluster.Kind,
		Name:       cluster.Name,
		UID:        cluster.UID,
	})
	return references
}

func getNodeLabelsString(node *v1.Node) (string, bool) {
	if node.Labels == nil {
		return "", false
	}
	labels := node.Labels
	delete(node.Labels, v1.ClusterManageNodeClusterLabel)
	if node.Spec.NodeFlavor != nil {
		labels[v1.NodeFlavorIdLabel] = node.Spec.NodeFlavor.Name
	}
	l, err := json.Marshal(labels)
	if err != nil {
		return "", false
	}
	return string(l), true
}

func createKubernetesClusterOwnerReference(cluster *v1.Cluster) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion:         cluster.APIVersion,
		Kind:               cluster.Kind,
		Name:               cluster.Name,
		UID:                cluster.UID,
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
	}
}

func isReadyMachineNode(node *v1.Node) bool {
	if node.Status.MachineStatus.Phase == "" {
		return false
	}
	if node.Status.MachineStatus.Phase == v1.NodeNotReady {
		return false
	}
	if node.Status.MachineStatus.Phase == v1.NodeManagedFailed {
		return false
	}
	return true
}

func guaranteeControllerPlane(cluster *v1.Cluster) bool {
	if !cluster.DeletionTimestamp.IsZero() {
		return true
	}
	switch phase := cluster.Status.ControlPlaneStatus.Phase; phase {
	case "", v1.PendingPhase, v1.CreatingPhase, v1.CreationFailed:
		return true
	}
	return false
}
