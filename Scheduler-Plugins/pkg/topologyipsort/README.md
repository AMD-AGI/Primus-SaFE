# TopologyIPSort Plugin

## Overview

The TopologyIPSort plugin is a Kubernetes scheduler plugin that implements topology-aware pod scheduling based on node IP addresses. It provides intelligent pod placement by considering the network topology and IP address distribution across nodes in the cluster.

## Features

- **Topology-aware Scheduling**: Sorts nodes based on their IP addresses to optimize network locality
- **Pod Group Support**: Supports gang scheduling with PodGroup resources
- **Priority-based Queue Sorting**: Implements custom queue sorting logic for pods
- **Co-scheduling**: Ensures all pods in a group can be scheduled together
- **Replica Type Support**: Handles different replica types (master, worker) in distributed training workloads

## Architecture

The plugin implements multiple scheduler framework extension points:

- **Score Plugin**: Scores nodes based on IP topology
- **Queue Sort Plugin**: Sorts pods in the scheduling queue
- **Permit Plugin**: Controls pod admission with co-scheduling logic
- **Post Filter Plugin**: Provides scheduling failure diagnostics

## Configuration

### Scheduler Configuration

Add the plugin to your scheduler configuration:

```yaml
apiVersion: kubescheduler.config.k8s.io/v1
kind: KubeSchedulerConfiguration
profiles:
- schedulerName: topology-ip-scheduler
  plugins:
    multiPoint:
      enabled:
      - name: TopologyIPSort
    queueSort:
      enabled:
      - name: TopologyIPSort
      disabled:
      - name: "*"
    score:
      enabled:
      - name: TopologyIPSort
      disabled:
      - name: "*"
```

### Pod Annotations

The plugin supports topology configuration through pod annotations:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: example-pod
  annotations:
    scheduling.x-k8s.io.tp: "2"  # Topology count
    scheduling.x-k8s.io.ep: "3"  # Endpoint count
    scheduling.x-k8s.io.cp: "4"  # Connection count
    scheduling.x-k8s.io.pp: "5"  # Path count
spec:
  # ... pod spec
```

### Pod Group Support

For gang scheduling, create a PodGroup:

```yaml
apiVersion: scheduling.x-k8s.io/v1alpha1
kind: PodGroup
metadata:
  name: example-group
  namespace: default
spec:
  minMember: 3
```

Then label your pods with the PodGroup:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: example-pod
  labels:
    scheduling.x-k8s.io/pod-group: example-group
spec:
  # ... pod spec
```

## Usage Examples

### Basic Pod Scheduling

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: topology-pod
  namespace: default
spec:
  containers:
  - name: app
    image: nginx:latest
  schedulerName: topology-ip-scheduler
```

### Distributed Training Workload

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: training-master
  namespace: default
  labels:
    scheduling.x-k8s.io/pod-group: training-job
    training.kubeflow.org/replica-type: master
    training.kubeflow.org/replica-index: "0"
spec:
  containers:
  - name: training
    image: training:latest
  schedulerName: topology-ip-scheduler
---
apiVersion: v1
kind: Pod
metadata:
  name: training-worker-0
  namespace: default
  labels:
    scheduling.x-k8s.io/pod-group: training-job
    training.kubeflow.org/replica-type: worker
    training.kubeflow.org/replica-index: "0"
spec:
  containers:
  - name: training
    image: training:latest
  schedulerName: topology-ip-scheduler
```

## Algorithm Details

### IP Index Calculation

The plugin calculates an IP index for each node by converting the internal IP address to an integer:

```go
func getIPIndex(node *framework.NodeInfo) int {
    for _, addr := range node.Node().Status.Addresses {
        if addr.Type == corev1.NodeInternalIP {
            ips := strings.Split(addr.Address, ".")
            if len(ips) < 4 {
                continue
            }
            pow := 0
            for i, ip := range ips {
                val, err := strconv.Atoi(ip)
                if err != nil {
                    continue
                }
                val = val << (24 - i*8)
                pow += val
            }
            return pow
        }
    }
    return 0
}
```

### Scoring Logic

1. **Node Filtering**: Filters nodes that can accommodate the pod
2. **IP-based Sorting**: Sorts nodes by their IP index
3. **Unit Calculation**: Calculates scheduling unit based on topology annotations
4. **Score Assignment**: Assigns maximum score to the best node, minimum to others

### Queue Sorting

The plugin implements custom queue sorting logic:

1. **Priority-based**: Higher priority pods come first
2. **Pod Group Ordering**: Pods within the same group are ordered by replica type and index
3. **Creation Time**: PodGroups are ordered by creation timestamp

### Co-scheduling

The permit plugin ensures gang scheduling:

1. **Group Validation**: Verifies all pods in the group are available
2. **Resource Check**: Ensures sufficient resources for all pods
3. **MinMember Validation**: Confirms the minimum member requirement is met

## Performance Considerations

- **IP Calculation**: IP index calculation is O(1) per node
- **Node Sorting**: Sorting is O(n log n) where n is the number of nodes
- **Pod Group Lookup**: Uses cached client for efficient PodGroup retrieval
- **Memory Usage**: Minimal additional memory overhead

## Troubleshooting

### Common Issues

1. **Pod Stuck in Pending**: Check if PodGroup exists and has correct MinMember
2. **Scheduling Failures**: Verify node resources and topology annotations
3. **Queue Sorting Issues**: Check pod priorities and labels

### Debug Logging

Enable debug logging to troubleshoot scheduling issues:

```bash
kubectl logs -n kube-system deployment/scheduler-plugins-scheduler -f
```

Look for logs with the prefix "TopologyIPSort" to understand scheduling decisions.

## Contributing

To contribute to the TopologyIPSort plugin:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Submit a pull request

## License

This project is licensed under the Apache License 2.0. 