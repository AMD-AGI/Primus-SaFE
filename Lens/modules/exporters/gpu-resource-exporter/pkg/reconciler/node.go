package reconciler

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/node"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/sql"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/k8sUtil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	defaultGPUVendor = metadata.GpuVendorAMD
)

type NodeReconciler struct {
	clientSets       *clientsets.K8SClientSet
	storageClientSet *clientsets.StorageClientSet
}

func NewNodeReconciler() *NodeReconciler {
	n := &NodeReconciler{
		clientSets:       clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet,
		storageClientSet: clientsets.GetClusterManager().GetCurrentClusterClients().StorageClientSet,
	}
	go func() {
		_ = n.start(context.Background())
	}()
	go func() {
		_ = n.startNodeCleanup(context.Background())
	}()
	return n
}

func (n *NodeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	if n.clientSets == nil {
		n.clientSets = clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet
	}
	if n.storageClientSet == nil {
		n.storageClientSet = clientsets.GetClusterManager().GetCurrentClusterClients().StorageClientSet
	}

	// Get the node
	k8sNode := &corev1.Node{}
	err := n.clientSets.ControllerRuntimeClient.Get(ctx, types.NamespacedName{Name: req.Name}, k8sNode)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return reconcile.Result{}, nil
		}
		log.Errorf("Error getting node %s: %v", req.Name, err)
		return reconcile.Result{}, err
	}

	// Process node info
	err = n.reconcileNodeInfo(ctx, k8sNode)
	if err != nil {
		log.Errorf("Failed to reconcile node info for %s: %v", k8sNode.Name, err)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (n *NodeReconciler) start(ctx context.Context) error {
	for {
		err := n.do(ctx)
		if err != nil {
			log.Errorf("failed to reconcile node related resources: %v", err)
		}
		time.Sleep(30 * time.Second)
	}
}

func (n *NodeReconciler) do(ctx context.Context) error {
	nodes := &corev1.NodeList{}
	err := n.clientSets.ControllerRuntimeClient.List(ctx, nodes)
	if err != nil {
		return err
	}
	desiredSvc := n.desiredKubeletService()
	err = n.clientSets.ControllerRuntimeClient.Create(ctx, desiredSvc)
	if err != nil {
		// ignore already exists error
		if client.IgnoreAlreadyExists(err) != nil {
			return err
		}
		err = n.clientSets.ControllerRuntimeClient.Update(ctx, desiredSvc)
		if err != nil {
			return err
		}
	}
	desiredEndpoints := n.desireKubeletServiceEndpoint(nodes)
	err = n.clientSets.ControllerRuntimeClient.Create(ctx, desiredEndpoints)
	if err != nil {
		// ignore already exists error
		if client.IgnoreAlreadyExists(err) != nil {
			return err
		}
		err = n.clientSets.ControllerRuntimeClient.Update(ctx, desiredEndpoints)
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return n.isGPUNode(e.Object.(*corev1.Node))
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return n.isGPUNode(e.ObjectNew.(*corev1.Node))
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return n.isGPUNode(e.Object.(*corev1.Node))
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return n.isGPUNode(e.Object.(*corev1.Node))
			},
		}).
		Complete(n)
}

// isGPUNode checks if a node has GPU resources
func (n *NodeReconciler) isGPUNode(node *corev1.Node) bool {
	resourceName := metadata.GetResourceName(defaultGPUVendor)
	_, hasGPU := node.Status.Capacity[corev1.ResourceName(resourceName)]
	return hasGPU
}

func (n *NodeReconciler) desiredKubeletService() *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "primus-lens-kubelet-service",
			Namespace: "kube-system",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "primus-lens",
				"app.kubernetes.io/name":       "kubelet",
				"k8s-app":                      "kubelet",
			},
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Ports: []corev1.ServicePort{
				{
					Name:       "https-metrics",
					Port:       10250,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(10250),
				},
				{
					Name:       "http-metrics",
					Port:       10255,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(10255),
				},
				{
					Name:       "cadvisor",
					Port:       4194,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(4194),
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

func (n *NodeReconciler) desireKubeletServiceEndpoint(nodes *corev1.NodeList) *corev1.Endpoints {
	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "primus-lens-kubelet-service",
			Namespace: "kube-system",
		},
		Subsets: []corev1.EndpointSubset{},
	}
	addresses := []corev1.EndpointAddress{}
	for _, node := range nodes.Items {
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP || addr.Type == corev1.NodeExternalIP {
				addresses = append(addresses, corev1.EndpointAddress{
					IP:       addr.Address,
					NodeName: &node.Name,
					TargetRef: &corev1.ObjectReference{
						Kind: "Node",
						Name: node.Name,
						UID:  node.UID,
					},
				})
			}
		}
	}
	subset := corev1.EndpointSubset{
		Addresses: addresses,
		Ports: []corev1.EndpointPort{
			{
				Name: "https-metrics",
				Port: 10250,
			},
			{
				Name: "http-metrics",
				Port: 10255,
			},
			{
				Name: "cadvisor",
				Port: 4194,
			},
		},
	}
	endpoints.Subsets = append(endpoints.Subsets, subset)
	return endpoints
}

// reconcileNodeInfo updates node information in the database
func (n *NodeReconciler) reconcileNodeInfo(ctx context.Context, k8sNode *corev1.Node) error {
	// Get existing node from database
	existDBNode, err := database.GetFacade().GetNode().GetNodeByName(ctx, k8sNode.Name)
	if err != nil {
		return err
	}

	// Build new node data
	newDBNode := &model.Node{
		ID:                0,
		Name:              k8sNode.Name,
		Address:           n.getNodeAddress(k8sNode),
		GpuName:           node.GetGpuDeviceName(*k8sNode, defaultGPUVendor),
		Status:            k8sUtil.NodeStatus(*k8sNode),
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
		CPU:               "", // TODO CPU information awaiting agent retrieval
		CPUCount:          int32(node.GetCPUCount(*k8sNode)),
		Memory:            node.GetMemorySizeHumanReadable(*k8sNode),
		K8sVersion:        "1.23.1",
		K8sStatus:         k8sUtil.NodeStatus(*k8sNode),
		Os:                k8sNode.Status.NodeInfo.OSImage,
		KubeletVersion:    k8sNode.Status.NodeInfo.KubeletVersion,
		ContainerdVersion: k8sNode.Status.NodeInfo.ContainerRuntimeVersion,
		Taints:            n.convertTaintsToExtType(k8sNode.Spec.Taints),
		Labels:            n.convertMapToExtType(k8sNode.Labels),
		Annotations:       n.convertMapToExtType(k8sNode.Annotations),
	}

	// Get driver version from node exporter
	nodeExporterClient, err := clientsets.GetOrInitNodeExportersClient(ctx, k8sNode.Name, n.clientSets.ControllerRuntimeClient)
	if err != nil {
		log.Errorf("Failed to init node exporter client for node %s: %v", k8sNode.Name, err)
	} else {
		driverVer, err := nodeExporterClient.GetDriverVersion(ctx)
		if err == nil {
			newDBNode.DriverVersion = driverVer
		} else {
			if existDBNode != nil {
				newDBNode.DriverVersion = existDBNode.DriverVersion
			}
			log.Errorf("Failed to get driver version from %s: %v", k8sNode.Name, err)
		}
	}

	// Determine if this is a create or update
	isCreate := false
	if existDBNode == nil {
		existDBNode = newDBNode
		isCreate = true
	} else {
		// Skip update if last update was less than 10 seconds ago
		if time.Now().Before(existDBNode.UpdatedAt.Add(10 * time.Second)) {
			return nil
		}
		newDBNode.ID = existDBNode.ID
		newDBNode.CreatedAt = existDBNode.CreatedAt
		newDBNode.UpdatedAt = time.Now()
		existDBNode = newDBNode
	}

	// Get GPU allocation information
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	allocatable, capacity, err := gpu.GetNodeGpuAllocation(ctx, n.clientSets, k8sNode.Name, clusterName, defaultGPUVendor)
	if err != nil {
		log.Errorf("Failed to get node GPU allocation for %s: %v", k8sNode.Name, err)
		return err
	}
	existDBNode.GpuCount = int32(capacity)
	existDBNode.GpuAllocation = int32(allocatable)

	// Get GPU utilization
	usage, err := gpu.CalculateNodeGpuUsage(ctx, k8sNode.Name, n.storageClientSet, defaultGPUVendor)
	if err == nil {
		existDBNode.GpuUtilization = usage
	} else {
		log.Warnf("Failed to get GPU utilization for %s: %v", k8sNode.Name, err)
	}

	// Save to database
	if existDBNode.ID == 0 {
		err = database.GetFacade().GetNode().CreateNode(ctx, existDBNode)
		if err != nil {
			log.Errorf("Failed to create node %s in database: %v", k8sNode.Name, err)
			return err
		}
		if isCreate {
			log.Infof("Created node %s in database", k8sNode.Name)
		}
	} else {
		err = database.GetFacade().GetNode().UpdateNode(ctx, existDBNode)
		if err != nil {
			log.Errorf("Failed to update node %s in database: %v", k8sNode.Name, err)
			return err
		}
		log.Debugf("Updated node %s in database", k8sNode.Name)
	}

	return nil
}

// getNodeAddress returns the first available address from the node
func (n *NodeReconciler) getNodeAddress(k8sNode *corev1.Node) string {
	if len(k8sNode.Status.Addresses) > 0 {
		return k8sNode.Status.Addresses[0].Address
	}
	return ""
}

// convertTaintsToExtType converts Kubernetes taints to ExtType
func (n *NodeReconciler) convertTaintsToExtType(taints []corev1.Taint) model.ExtType {
	if len(taints) == 0 {
		return model.ExtType{}
	}

	result := make(model.ExtType)
	taintsList := make([]map[string]interface{}, 0, len(taints))

	for _, taint := range taints {
		taintMap := map[string]interface{}{
			"key":    taint.Key,
			"value":  taint.Value,
			"effect": string(taint.Effect),
		}
		if taint.TimeAdded != nil {
			taintMap["timeAdded"] = taint.TimeAdded.Time
		}
		taintsList = append(taintsList, taintMap)
	}

	result["taints"] = taintsList
	return result
}

// convertMapToExtType converts map[string]string to ExtType for labels and annotations
func (n *NodeReconciler) convertMapToExtType(m map[string]string) model.ExtType {
	if len(m) == 0 {
		return model.ExtType{}
	}

	result := make(model.ExtType, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// startNodeCleanup periodically checks for nodes that exist in DB but not in the cluster
func (n *NodeReconciler) startNodeCleanup(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// Run immediately on start
	if err := n.cleanupOrphanedNodes(ctx); err != nil {
		log.Errorf("Initial node cleanup failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := n.cleanupOrphanedNodes(ctx); err != nil {
				log.Errorf("Failed to cleanup orphaned nodes: %v", err)
			}
		}
	}
}

// cleanupOrphanedNodes removes nodes from DB that no longer exist in the cluster
func (n *NodeReconciler) cleanupOrphanedNodes(ctx context.Context) error {
	if n.clientSets == nil {
		n.clientSets = clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet
	}

	// Get all nodes from the cluster
	k8sNodes := &corev1.NodeList{}
	err := n.clientSets.ControllerRuntimeClient.List(ctx, k8sNodes)
	if err != nil {
		return err
	}

	// Build a set of existing node names
	k8sNodeNames := make(map[string]bool)
	for _, node := range k8sNodes.Items {
		k8sNodeNames[node.Name] = true
	}

	// Get all nodes from the database
	dbNodes, _, err := database.GetFacade().GetNode().SearchNode(ctx, filter.NodeFilter{})
	if err != nil {
		return err
	}

	// Delete nodes that don't exist in the cluster
	deletedCount := 0
	for _, dbNode := range dbNodes {
		if !k8sNodeNames[dbNode.Name] {
			log.Infof("Deleting orphaned node from database: %s", dbNode.Name)
			err := n.deleteNodeByName(ctx, dbNode.Name)
			if err != nil {
				log.Errorf("Failed to delete node %s: %v", dbNode.Name, err)
			} else {
				deletedCount++
			}
		}
	}

	if deletedCount > 0 {
		log.Infof("Cleaned up %d orphaned nodes from database", deletedCount)
	}

	return nil
}

// deleteNodeByName deletes a node from the database by name
func (n *NodeReconciler) deleteNodeByName(ctx context.Context, nodeName string) error {
	db := sql.GetDefaultDB()
	return db.WithContext(ctx).Where("name = ?", nodeName).Delete(&model.Node{}).Error
}
