/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

// runNodeSSH runs one command on a node. Tests replace it.
var runNodeSSH = defaultRunNodeSSH

func defaultRunNodeSSH(ctx context.Context, cli client.Client, node *v1.Node, cmd string) error {
	sshClient, err := utils.GetSSHClient(ctx, cli, node)
	if err != nil {
		return err
	}
	defer sshClient.Close()
	session, err := sshClient.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	if out, err := session.CombinedOutput(cmd); err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// adoptedSSHUserPattern limits the secret username before it is placed in a remote command.
var adoptedSSHUserPattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`)

// adoptClusterContents creates admin NodeFlavor, Workspace and Node objects for a cluster
// that already has a Kubernetes control plane. Existing admin objects are left unchanged.
func (r *ClusterReconciler) adoptClusterContents(ctx context.Context, cluster *v1.Cluster) error {
	if !isAdoptedClusterPending(cluster) || !cluster.IsReady() {
		return nil
	}
	factory, err := utils.GetK8sClientFactory(r.clientManager, cluster.Name)
	if err != nil || factory == nil || !factory.IsValid() {
		return fmt.Errorf("data plane client for adopted cluster %s is not ready", cluster.Name)
	}
	nodeList, err := factory.ClientSet().CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	authorizeErr := r.authorizeAdoptedWorkers(ctx, cluster, nodeList.Items)
	if err = r.syncAdoptedInventory(ctx, cluster, factory.ClientSet(), nodeList.Items); err != nil {
		return err
	}
	return authorizeErr
}

// authorizeAdoptedWorkers appends the management public key to the SSH user of each pending worker.
// The command runs on a control plane node, which already holds a key the workers accept.
// A secret with only a password has no key to install, so the step is skipped.
func (r *ClusterReconciler) authorizeAdoptedWorkers(ctx context.Context, cluster *v1.Cluster, nodes []corev1.Node) error {
	ips, err := r.pendingAdoptedWorkerIPs(ctx, cluster, nodes)
	if err != nil || len(ips) == 0 {
		return err
	}
	secret, err := r.getAdoptedSSHSecret(ctx, cluster)
	if err != nil {
		return err
	}
	pub, err := sshPublicKeyFromSecret(secret)
	if err != nil {
		return err
	}
	if pub == "" {
		klog.Infof("ssh secret of adopted cluster %s has no key, skip authorizing workers", cluster.Name)
		return nil
	}
	username := string(secret.Data[utils.Username])
	if username == "" {
		username = utils.Root
	}
	if !adoptedSSHUserPattern.MatchString(username) {
		return fmt.Errorf("invalid ssh username %q in secret %s/%s", username, secret.Namespace, secret.Name)
	}
	cmd := adoptedWorkerAuthorizeCommand(pub, username, ips)
	controlPlanes, err := r.getControllerPlaneNodes(ctx, cluster)
	if err != nil {
		return err
	}
	var lastErr error
	for _, node := range controlPlanes {
		if err = runNodeSSH(ctx, r.Client, node, cmd); err != nil {
			klog.ErrorS(err, "failed to authorize workers from control plane", "node", node.Name)
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr == nil {
		return fmt.Errorf("no control plane node available to authorize workers of %s", cluster.Name)
	}
	return lastErr
}

// getAdoptedSSHSecret returns the SSH secret referenced by the adopted cluster control plane.
func (r *ClusterReconciler) getAdoptedSSHSecret(ctx context.Context, cluster *v1.Cluster) (*corev1.Secret, error) {
	ref := cluster.Spec.ControlPlane.SSHSecret
	if ref == nil || ref.Name == "" {
		return nil, fmt.Errorf("adopted cluster %s has no ssh secret", cluster.Name)
	}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}, secret); err != nil {
		return nil, err
	}
	return secret, nil
}

// sshPublicKeyFromSecret returns the authorized public key, or derives it from the private key.
// It returns an empty key without error when the secret only holds a password.
func sshPublicKeyFromSecret(secret *corev1.Secret) (string, error) {
	if pub := strings.TrimSpace(string(secret.Data[utils.AuthorizePub])); pub != "" {
		return pub, nil
	}
	key := secret.Data[utils.Authorize]
	if len(key) == 0 {
		if len(secret.Data[utils.Password]) > 0 {
			return "", nil
		}
		return "", fmt.Errorf("ssh secret %s/%s has no key or password", secret.Namespace, secret.Name)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("ssh secret %s/%s has an invalid private key: %v", secret.Namespace, secret.Name, err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))), nil
}

// bootstrapAdoptedNodes builds in-memory control plane nodes from the adopt IPs of a pending cluster.
// They are never persisted; they only carry the SSH address and secret used to read the kubeconfig.
func bootstrapAdoptedNodes(cluster *v1.Cluster) []*v1.Node {
	if !isAdoptedClusterPending(cluster) || cluster.Spec.ControlPlane.SSHSecret == nil {
		return nil
	}
	ips := v1.GetAdoptControlPlaneIPs(cluster)
	nodes := make([]*v1.Node, 0, len(ips))
	for _, ip := range ips {
		nodes = append(nodes, &v1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: ip},
			Spec: v1.NodeSpec{
				PrivateIP: ip,
				Port:      pointer.Int32(22),
				SSHSecret: cluster.Spec.ControlPlane.SSHSecret,
			},
		})
	}
	return nodes
}

// syncAdoptedInventory creates missing flavors, workspaces and nodes from the data plane.
func (r *ClusterReconciler) syncAdoptedInventory(ctx context.Context, cluster *v1.Cluster,
	clientSet kubernetes.Interface, nodes []corev1.Node) error {
	for i := range nodes {
		flavor := adoptedFlavor(cluster, &nodes[i])
		if err := r.ensureAdoptedFlavor(ctx, flavor); err != nil {
			return err
		}
	}
	grouped := groupNodesByWorkspace(nodes)
	for name, members := range grouped {
		pvcs, err := clientSet.CoreV1().PersistentVolumeClaims(name).List(ctx, metav1.ListOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		var items []corev1.PersistentVolumeClaim
		if pvcs != nil {
			items = pvcs.Items
		}
		if err = r.ensureAdoptedWorkspace(ctx, cluster, name, members, items); err != nil {
			return err
		}
	}
	for i := range nodes {
		if err := r.ensureAdoptedNode(ctx, cluster, &nodes[i]); err != nil {
			return err
		}
	}
	return r.syncAdoptedControlPlane(ctx, cluster, nodes)
}

// syncAdoptedControlPlane records the data plane control plane nodes on the cluster.
func (r *ClusterReconciler) syncAdoptedControlPlane(ctx context.Context, cluster *v1.Cluster, nodes []corev1.Node) error {
	names := adoptedControlPlaneNames(nodes)
	if len(names) == 0 || sameStringSet(cluster.Spec.ControlPlane.Nodes, names) {
		return nil
	}
	original := client.MergeFrom(cluster.DeepCopy())
	cluster.Spec.ControlPlane.Nodes = names
	return r.Patch(ctx, cluster, original)
}

func (r *ClusterReconciler) ensureAdoptedFlavor(ctx context.Context, flavor *v1.NodeFlavor) error {
	if flavor == nil || flavor.Name == "" {
		return nil
	}
	existing := &v1.NodeFlavor{}
	err := r.Get(ctx, types.NamespacedName{Name: flavor.Name}, existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	return r.Create(ctx, flavor)
}

func (r *ClusterReconciler) ensureAdoptedWorkspace(ctx context.Context, cluster *v1.Cluster, name string,
	members []corev1.Node, pvcs []corev1.PersistentVolumeClaim) error {
	if name == "" {
		return nil
	}
	existing := &v1.Workspace{}
	err := r.Get(ctx, types.NamespacedName{Name: name}, existing)
	if err == nil {
		if existing.Spec.Cluster != cluster.Name {
			klog.Infof("skip adopted workspace %s, it belongs to cluster %s", name, existing.Spec.Cluster)
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				v1.DisplayNameLabel: name,
				v1.ClusterIdLabel:   cluster.Name,
			},
		},
		Spec: v1.WorkspaceSpec{
			Cluster:     cluster.Name,
			NodeFlavor:  adoptedWorkspaceFlavor(cluster, members),
			Replica:     len(members),
			QueuePolicy: v1.QueueFifoPolicy,
			Volumes:     volumesFromPVCs(pvcs),
		},
	}
	v1.SetAnnotation(workspace, v1.WorkspaceAdoptAnnotation, v1.TrueStr)
	return r.Create(ctx, workspace)
}

func (r *ClusterReconciler) ensureAdoptedNode(ctx context.Context, cluster *v1.Cluster, k8sNode *corev1.Node) error {
	if k8sNode == nil || k8sNode.Name == "" {
		return nil
	}
	workspace, err := r.adoptedNodeWorkspace(ctx, cluster, k8sNode)
	if err != nil {
		return err
	}
	existing := &v1.Node{}
	err = r.Get(ctx, types.NamespacedName{Name: k8sNode.Name}, existing)
	if err == nil {
		if existing.GetSpecCluster() != "" && existing.GetSpecCluster() != cluster.Name {
			klog.Infof("skip adopted node %s, it belongs to cluster %s", k8sNode.Name, existing.GetSpecCluster())
			return nil
		}
		original := client.MergeFrom(existing.DeepCopy())
		changed := false
		if existing.Labels == nil {
			existing.Labels = map[string]string{}
		}
		if existing.GetSpecCluster() == "" {
			existing.Spec.Cluster = pointer.String(cluster.Name)
			existing.Labels[v1.ClusterIdLabel] = cluster.Name
			changed = true
		}
		if isK8sControlPlaneNode(k8sNode) && !v1.IsControlPlane(existing) {
			existing.Labels[v1.KubernetesControlPlane] = ""
			changed = true
		}
		if workspace != "" && existing.GetSpecWorkspace() == "" {
			existing.Spec.Workspace = pointer.String(workspace)
			existing.Labels[v1.WorkspaceIdLabel] = workspace
			changed = true
		}
		if !changed {
			return nil
		}
		return r.Patch(ctx, existing, original)
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	ip := internalIP(k8sNode)
	flavorName := adoptedFlavor(cluster, k8sNode).Name
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: k8sNode.Name,
			Labels: map[string]string{
				v1.DisplayNameLabel: k8sNode.Name,
				v1.NodeIdLabel:      k8sNode.Name,
				v1.ClusterIdLabel:   cluster.Name,
			},
		},
		Spec: v1.NodeSpec{
			Cluster:    pointer.String(cluster.Name),
			Hostname:   pointer.String(k8sNode.Name),
			PrivateIP:  ip,
			Port:       pointer.Int32(22),
			SSHSecret:  cluster.Spec.ControlPlane.SSHSecret,
			NodeFlavor: &corev1.ObjectReference{Name: flavorName, Kind: v1.NodeFlavorKind, APIVersion: v1.SchemeGroupVersion.String()},
		},
	}
	if workspace != "" {
		node.Spec.Workspace = pointer.String(workspace)
		node.Labels[v1.WorkspaceIdLabel] = workspace
	}
	if isK8sControlPlaneNode(k8sNode) {
		node.Labels[v1.KubernetesControlPlane] = ""
	}
	return r.Create(ctx, node)
}

// adoptedNodeWorkspace returns the workspace named by the data plane node label when that
// workspace exists in the admin plane and belongs to the cluster, otherwise an empty name.
func (r *ClusterReconciler) adoptedNodeWorkspace(ctx context.Context, cluster *v1.Cluster, k8sNode *corev1.Node) (string, error) {
	name := v1.GetWorkspaceId(k8sNode)
	if name == "" {
		return "", nil
	}
	workspace := &v1.Workspace{}
	if err := r.Get(ctx, types.NamespacedName{Name: name}, workspace); err != nil {
		return "", client.IgnoreNotFound(err)
	}
	if workspace.Spec.Cluster != cluster.Name {
		return "", nil
	}
	return name, nil
}

// legacyControlPlaneLabel is the role label used before node-role.kubernetes.io/control-plane.
const legacyControlPlaneLabel = "node-role.kubernetes.io/master"

// isK8sControlPlaneNode reports a data plane node that serves the Kubernetes control plane.
func isK8sControlPlaneNode(node *corev1.Node) bool {
	if node == nil || len(node.Labels) == 0 {
		return false
	}
	if _, ok := node.Labels[v1.KubernetesControlPlane]; ok {
		return true
	}
	_, ok := node.Labels[legacyControlPlaneLabel]
	return ok
}

// adoptedControlPlaneNames returns the data plane control plane node names.
func adoptedControlPlaneNames(nodes []corev1.Node) []string {
	names := make([]string, 0)
	for i := range nodes {
		if isK8sControlPlaneNode(&nodes[i]) && nodes[i].Name != "" {
			names = append(names, nodes[i].Name)
		}
	}
	sort.Strings(names)
	return names
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	a := append([]string(nil), left...)
	b := append([]string(nil), right...)
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// adoptedFlavor names a flavor from the node label, the cluster annotation, or node capacity.
// A plain Kubernetes node has none of the SaFE labels, so nodes with the same capacity share one flavor.
func adoptedFlavor(cluster *v1.Cluster, node *corev1.Node) *v1.NodeFlavor {
	if node == nil {
		return nil
	}
	cpu := node.Status.Capacity[corev1.ResourceCPU]
	if cpu.IsZero() {
		cpu = resource.MustParse("1")
	}
	memory := node.Status.Capacity[corev1.ResourceMemory]
	if memory.IsZero() {
		memory = resource.MustParse("1Gi")
	}
	name := v1.GetNodeFlavorId(node)
	if name == "" && cluster != nil {
		name = v1.GetAnnotation(cluster, v1.ClusterAdoptNodeFlavorAnnotation)
	}
	gpu, gpuResource := adoptedGPU(node)
	if name == "" {
		name = capacityFlavorName(cpu, memory, gpu, gpuResource)
	}
	flavor := &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.NodeFlavorSpec{
			Cpu:    v1.CpuChip{Quantity: cpu},
			Memory: memory,
		},
	}
	if gpuResource != "" {
		flavor.Spec.Gpu = &v1.GpuChip{ResourceName: gpuResource, Quantity: gpu}
	}
	return flavor
}

func adoptedGPU(node *corev1.Node) (resource.Quantity, string) {
	if node == nil {
		return resource.Quantity{}, ""
	}
	if gpu, ok := node.Status.Capacity[corev1.ResourceName(common.AmdGpu)]; ok && !gpu.IsZero() {
		return gpu, common.AmdGpu
	}
	if gpu, ok := node.Status.Capacity[corev1.ResourceName(common.NvidiaGpu)]; ok && !gpu.IsZero() {
		return gpu, common.NvidiaGpu
	}
	return resource.Quantity{}, ""
}

func capacityFlavorName(cpu, memory resource.Quantity, gpu resource.Quantity, gpuResource string) string {
	name := "adopted-c" + quantityToken(cpu) + "-m" + quantityToken(memory)
	if gpuResource != "" && !gpu.IsZero() {
		vendor := "gpu"
		switch gpuResource {
		case common.AmdGpu:
			vendor = "amd"
		case common.NvidiaGpu:
			vendor = "nvidia"
		}
		name += "-" + vendor + quantityToken(gpu)
	}
	return name
}

func quantityToken(q resource.Quantity) string {
	var b strings.Builder
	for _, r := range strings.ToLower(q.String()) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "0"
	}
	return b.String()
}

func groupNodesByWorkspace(nodes []corev1.Node) map[string][]corev1.Node {
	grouped := map[string][]corev1.Node{}
	for i := range nodes {
		name := v1.GetWorkspaceId(&nodes[i])
		if name == "" {
			continue
		}
		grouped[name] = append(grouped[name], nodes[i])
	}
	return grouped
}

func adoptedWorkspaceFlavor(cluster *v1.Cluster, nodes []corev1.Node) string {
	counts := map[string]int{}
	for i := range nodes {
		name := adoptedFlavor(cluster, &nodes[i]).Name
		counts[name]++
	}
	best, bestCount := "", 0
	for name, count := range counts {
		if count > bestCount || (count == bestCount && (best == "" || name < best)) {
			best, bestCount = name, count
		}
	}
	return best
}

func volumesFromPVCs(pvcs []corev1.PersistentVolumeClaim) []v1.WorkspaceVolume {
	volumes := make([]v1.WorkspaceVolume, 0)
	for i := range pvcs {
		pvc := &pvcs[i]
		if !strings.HasPrefix(pvc.Name, string(v1.PFS)+"-") {
			continue
		}
		id, err := strconv.Atoi(strings.TrimPrefix(pvc.Name, string(v1.PFS)+"-"))
		if err != nil {
			continue
		}
		storageClass := ""
		if pvc.Spec.StorageClassName != nil {
			storageClass = *pvc.Spec.StorageClassName
		}
		if storageClass == "" {
			storageClass = "manual"
		}
		capacity := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		if capacity.IsZero() {
			capacity = resource.MustParse("1Gi")
		}
		access := corev1.ReadWriteMany
		if len(pvc.Spec.AccessModes) > 0 {
			access = pvc.Spec.AccessModes[0]
		}
		volumes = append(volumes, v1.WorkspaceVolume{
			Id:           id,
			Type:         v1.PFS,
			MountPath:    "/mnt/" + pvc.Name,
			AccessMode:   access,
			Capacity:     capacity.String(),
			StorageClass: storageClass,
		})
	}
	sort.Slice(volumes, func(i, j int) bool { return volumes[i].Id < volumes[j].Id })
	return volumes
}

// pendingAdoptedWorkerIPs returns worker addresses whose admin Node is missing or not machine-ready.
// Machine-ready workers and workers of another cluster are left alone, so later reconciles only
// repeat the SSH fan-out for workers that still cannot be reached.
func (r *ClusterReconciler) pendingAdoptedWorkerIPs(ctx context.Context, cluster *v1.Cluster, nodes []corev1.Node) ([]string, error) {
	controlPlanes := map[string]bool{}
	if cluster != nil {
		for _, name := range cluster.Spec.ControlPlane.Nodes {
			controlPlanes[name] = true
		}
	}
	ips := make([]string, 0)
	for i := range nodes {
		if controlPlanes[nodes[i].Name] || isK8sControlPlaneNode(&nodes[i]) {
			continue
		}
		ip := internalIP(&nodes[i])
		if net.ParseIP(ip) == nil {
			continue
		}
		existing := &v1.Node{}
		err := r.Get(ctx, types.NamespacedName{Name: nodes[i].Name}, existing)
		if err == nil {
			owner := existing.GetSpecCluster()
			if existing.IsMachineReady() || (owner != "" && cluster != nil && owner != cluster.Name) {
				continue
			}
		} else if !apierrors.IsNotFound(err) {
			return nil, err
		}
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	return ips, nil
}

func internalIP(node *corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	return ""
}

// adoptedWorkerAuthorizeCommand builds the remote script that installs one public key for the
// given user on every worker. It reports the workers that failed and exits non-zero if any did.
func adoptedWorkerAuthorizeCommand(pub, username string, ips []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "pub=$(cat <<'EOF'\n%s\nEOF\n)\nfailed=\"\"\n", strings.TrimSpace(pub))
	for _, ip := range ips {
		if net.ParseIP(ip) == nil {
			continue
		}
		fmt.Fprintf(&b, "ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=8 %s@%s "+
			"\"mkdir -p ~/.ssh && chmod 700 ~/.ssh && touch ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys && "+
			"(grep -qxF \\\"$pub\\\" ~/.ssh/authorized_keys || echo \\\"$pub\\\" >> ~/.ssh/authorized_keys)\" "+
			"|| failed=\"$failed %s\"\n", username, ip, ip)
	}
	b.WriteString("if [ -n \"$failed\" ]; then echo \"failed to authorize workers:$failed\" >&2; exit 1; fi\n")
	return b.String()
}

// releaseAdoptedCluster deletes the admin records of an adopted cluster in order: Addons, Nodes,
// Workspaces, then the Cluster finalizer. The data plane is neither reset nor modified.
func (r *ClusterReconciler) releaseAdoptedCluster(ctx context.Context, cluster *v1.Cluster) (ctrlruntime.Result, error) {
	for _, list := range []func(context.Context, string) ([]client.Object, error){
		r.listClusterAddons, r.listClusterNodes, r.listClusterWorkspaces,
	} {
		objs, err := list(ctx, cluster.Name)
		if err != nil {
			return ctrlruntime.Result{}, err
		}
		if len(objs) == 0 {
			continue
		}
		for _, obj := range objs {
			if !obj.GetDeletionTimestamp().IsZero() {
				continue
			}
			if err = r.Delete(ctx, obj); client.IgnoreNotFound(err) != nil {
				return ctrlruntime.Result{}, err
			}
		}
		return ctrlruntime.Result{RequeueAfter: 3 * time.Second}, nil
	}
	if err := r.clientManager.Delete(cluster.Name); err != nil {
		klog.ErrorS(err, "failed to delete cluster clients", "cluster", cluster.Name)
	}
	return ctrlruntime.Result{}, utils.RemoveFinalizer(ctx, r.Client, cluster, v1.ClusterFinalizer)
}

// listClusterAddons returns the Addons that target the cluster.
func (r *ClusterReconciler) listClusterAddons(ctx context.Context, clusterName string) ([]client.Object, error) {
	list := &v1.AddonList{}
	if err := r.List(ctx, list); err != nil {
		return nil, err
	}
	objs := make([]client.Object, 0)
	for i := range list.Items {
		if list.Items[i].Spec.Cluster != nil && list.Items[i].Spec.Cluster.Name == clusterName {
			objs = append(objs, &list.Items[i])
		}
	}
	return objs, nil
}

// listClusterNodes returns the admin Nodes bound to the cluster.
func (r *ClusterReconciler) listClusterNodes(ctx context.Context, clusterName string) ([]client.Object, error) {
	list := &v1.NodeList{}
	if err := r.List(ctx, list); err != nil {
		return nil, err
	}
	objs := make([]client.Object, 0)
	for i := range list.Items {
		if getClusterId(&list.Items[i]) == clusterName {
			objs = append(objs, &list.Items[i])
		}
	}
	return objs, nil
}

// listClusterWorkspaces returns the Workspaces of the cluster.
func (r *ClusterReconciler) listClusterWorkspaces(ctx context.Context, clusterName string) ([]client.Object, error) {
	list := &v1.WorkspaceList{}
	if err := r.List(ctx, list); err != nil {
		return nil, err
	}
	objs := make([]client.Object, 0)
	for i := range list.Items {
		if list.Items[i].Spec.Cluster == clusterName {
			objs = append(objs, &list.Items[i])
		}
	}
	return objs, nil
}
