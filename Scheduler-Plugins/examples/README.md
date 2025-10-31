# TopologyIPSort Plugin Examples

This directory contains example configurations and manifests for using the TopologyIPSort scheduler plugin.

## Quick Start

### 1. Deploy the Scheduler

First, deploy the scheduler with TopologyIPSort plugin enabled:

```bash
# Using Helm
helm install scheduler-plugins manifests/charts/scheduler-plugins/

# Or using the provided config
kubectl apply -f topologyipsort-scheduler-config.yaml
```

### 2. Create a PodGroup

For gang scheduling, create a PodGroup first:

```bash
kubectl apply -f topologyipsort-podgroup-example.yaml
```

### 3. Deploy Pods

Deploy pods that use the topology-aware scheduler:

```bash
kubectl apply -f topologyipsort-pod-example.yaml
```

## Example Configurations

### Basic Scheduler Configuration

The `topologyipsort-scheduler-config.yaml` file shows how to configure the scheduler with the TopologyIPSort plugin enabled for all extension points.

### Simple Pod Example

The `topologyipsort-pod-example.yaml` file demonstrates:
- Basic pod configuration with topology annotations
- Pod group labeling for gang scheduling
- Replica type labeling for distributed workloads
- Resource requests and limits

### Distributed Training Example

The `topologyipsort-podgroup-example.yaml` file shows:
- PodGroup configuration with minimum member requirements
- Multiple pods in a distributed training job
- Master and worker replica types
- Proper labeling for gang scheduling

## Configuration Options

### Topology Annotations

Configure topology parameters using pod annotations:

```yaml
annotations:
  scheduling.x-k8s.io.tp: "2"  # Topology count
  scheduling.x-k8s.io.ep: "3"  # Endpoint count
  scheduling.x-k8s.io.cp: "4"  # Connection count
  scheduling.x-k8s.io.pp: "5"  # Path count
```

### Pod Group Labels

Enable gang scheduling with PodGroup labels:

```yaml
labels:
  scheduling.x-k8s.io/pod-group: "your-group-name"
```

### Replica Type Labels

For distributed workloads, use replica type labels:

```yaml
labels:
  training.kubeflow.org/replica-type: "master"  # or "worker"
  training.kubeflow.org/replica-index: "0"
```

## Verification

### Check Pod Status

```bash
kubectl get pods -o wide
```

### Check Scheduler Logs

```bash
kubectl logs -n scheduler-plugins deployment/scheduler-plugins-scheduler -f
```

Look for logs with "TopologyIPSort" prefix to see scheduling decisions.

### Verify Pod Group Status

```bash
kubectl get podgroups
kubectl describe podgroup example-group
```

## Troubleshooting

### Common Issues

1. **Pods stuck in Pending**: Check if PodGroup exists and MinMember is met
2. **Scheduling failures**: Verify node resources and topology annotations
3. **Plugin not loaded**: Check scheduler configuration and plugin registration

### Debug Commands

```bash
# Check scheduler configuration
kubectl get configmap -n scheduler-plugins scheduler-config -o yaml

# Check pod events
kubectl describe pod <pod-name>

# Check node resources
kubectl describe nodes
```

## Advanced Usage

### Custom Topology Configuration

Adjust topology parameters based on your network layout:

```yaml
annotations:
  scheduling.x-k8s.io.tp: "4"  # More topology levels
  scheduling.x-k8s.io.ep: "8"  # More endpoints
  scheduling.x-k8s.io.cp: "16" # More connections
  scheduling.x-k8s.io.pp: "32" # More paths
```

### Resource Requirements

Set appropriate resource requirements for your workloads:

```yaml
resources:
  requests:
    memory: "1Gi"
    cpu: "500m"
  limits:
    memory: "2Gi"
    cpu: "1000m"
```

### Scheduling Timeout

Configure scheduling timeout for PodGroups:

```yaml
spec:
  scheduleTimeoutSeconds: 600  # 10 minutes
``` 