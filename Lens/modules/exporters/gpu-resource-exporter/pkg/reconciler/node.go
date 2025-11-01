package reconciler

import (
	"context"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type NodeReconciler struct {
	clientSets *clientsets.K8SClientSet
}

func NewNodeReconciler() *NodeReconciler {
	n := &NodeReconciler{
		clientSets: clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet,
	}
	go func() {
		_ = n.start(context.Background())
	}()
	return n
}

func (n *NodeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {

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
	return ctrl.NewControllerManagedBy(mgr).For(&corev1.Node{}).
		Complete(n)
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

/*
apiVersion: v1
kind: Service
metadata:
  creationTimestamp: "2025-09-25T04:01:18Z"
  labels:
    app.kubernetes.io/managed-by: prometheus-operator
    app.kubernetes.io/name: kubelet
    k8s-app: kubelet
  name: prometheus-kube-prometheus-kubelet
  namespace: kube-system
  resourceVersion: "2952"
  uid: ab373925-b46c-49c1-a098-b07c88752b8f
spec:
  clusterIP: None
  clusterIPs:
  - None
  internalTrafficPolicy: Cluster
  ipFamilies:
  - IPv4
  - IPv6
  ipFamilyPolicy: RequireDualStack
  ports:
  - name: https-metrics
    port: 10250
    protocol: TCP
    targetPort: 10250
  - name: http-metrics
    port: 10255
    protocol: TCP
    targetPort: 10255
  - name: cadvisor
    port: 4194
    protocol: TCP
    targetPort: 4194
  sessionAffinity: None
  type: ClusterIP
status:
*/
