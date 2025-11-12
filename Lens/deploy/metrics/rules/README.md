# Alert Rules for Primus Lens

This directory contains VMRule resources that define alert rules for the Primus Lens monitoring system.

## Files

- `vmrule-hardware.yaml`: Hardware-related alerts (GPU, network, storage)
- `vmrule-training.yaml`: Training workload alerts (performance, resources, checkpoints)
- `vmrule-system.yaml`: System and Kubernetes alerts (nodes, pods, workloads)

## Deployment

### Prerequisites

1. VictoriaMetrics Operator installed
2. VMAlert deployed and configured
3. Telemetry Processor running and accessible

### Apply Alert Rules

```bash
# Apply all alert rules
kubectl apply -f vmrule-hardware.yaml
kubectl apply -f vmrule-training.yaml
kubectl apply -f vmrule-system.yaml

# Or apply all at once
kubectl apply -f .
```

### Verify Alert Rules

```bash
# Check if VMRule resources are created
kubectl get vmrules -n primus-lens

# Check VMAlert logs
kubectl logs -n primus-lens -l app.kubernetes.io/name=vmalert

# Access VMAlert UI (port-forward if needed)
kubectl port-forward -n primus-lens svc/primus-lens-vmalert 8080:8080
# Open http://localhost:8080
```

## Alert Categories

### Hardware Alerts

- **GPU Alerts**: Utilization, memory, temperature, power
- **Network Alerts**: InfiniBand errors, RDMA latency, bandwidth saturation
- **Storage Alerts**: Disk space, I/O latency

### Training Alerts

- **Performance Alerts**: Throughput degradation, training hanging, loss anomalies
- **Resource Alerts**: Memory pressure, gradient norm anomalies, batch time
- **Checkpoint Alerts**: Checkpoint failures, long checkpoint times

### System Alerts

- **Node Alerts**: Node down, CPU/memory/load high
- **Pod Alerts**: Crash looping, not ready, OOM, CPU throttling
- **Workload Alerts**: Pods pending, replica mismatch, job failures
- **Container Alerts**: Container killed, frequent restarts

## Customization

### Modifying Thresholds

Edit the rule expressions and thresholds according to your environment:

```yaml
# Example: Adjust GPU utilization threshold
- alert: GPUHighUtilization
  expr: gpu_utilization > 95  # Change this value
  for: 5m                      # Change duration
```

### Adding New Rules

1. Add a new rule to the appropriate file or create a new file
2. Ensure it has the correct labels:
   ```yaml
   labels:
     app: primus-lens
     component: alerts
     category: <your-category>
   ```
3. Apply the updated YAML file

### Testing Rules

Before deploying to production:

1. Use VMAlert's `/api/v1/rules` endpoint to check rule syntax
2. Test with sample metrics using `/api/v1/query`
3. Create test alerts and verify they appear in telemetry-processor

## Alert Flow

```
1. VMAlert evaluates rules against metrics
   ↓
2. Rules trigger based on conditions
   ↓
3. Alerts sent to telemetry-processor webhook
   ↓
4. Telemetry-processor processes and enriches alerts
   ↓
5. Alerts stored in database
   ↓
6. Correlation analysis performed
   ↓
7. Notifications sent to configured channels
```

## Troubleshooting

### Rules Not Loading

1. Check VMRule labels match VMAlert's `ruleSelector`
2. Verify namespace matches VMAlert namespace
3. Check VMAlert logs for parsing errors

### Alerts Not Firing

1. Verify metrics are available: Check Prometheus/VictoriaMetrics
2. Test rule expression: Use Prometheus query UI
3. Check evaluation interval: Alerts may take time to fire
4. Review `for` duration: Alert must be active for specified time

### Alerts Not Reaching Telemetry Processor

1. Verify VMAlert notifier configuration
2. Check telemetry-processor logs for incoming requests
3. Test webhook endpoint manually:
   ```bash
   curl -X POST http://telemetry-processor:8989/v1/alerts/metric \
     -H "Content-Type: application/json" \
     -d '{"alerts":[...]}'
   ```

## Best Practices

1. **Use Appropriate Severity Levels**:
   - `critical`: Requires immediate action
   - `high`: Requires attention soon
   - `warning`: Should be investigated
   - `info`: For awareness only

2. **Set Reasonable Thresholds**:
   - Test in non-production first
   - Adjust based on baseline metrics
   - Avoid alert fatigue

3. **Add Helpful Annotations**:
   - `summary`: Brief description
   - `description`: Detailed information with values
   - `runbook_url`: Link to resolution steps

4. **Group Related Alerts**:
   - Use `group_by` in route configuration
   - Set appropriate `group_wait` and `group_interval`

5. **Monitor Alert Volume**:
   - Use alert statistics to track alert frequency
   - Adjust rules that fire too often
   - Remove rules that never fire

## Examples

### Creating a Custom Alert Rule

```yaml
apiVersion: operator.victoriametrics.com/v1beta1
kind: VMRule
metadata:
  name: custom-alerts
  namespace: primus-lens
  labels:
    app: primus-lens
    component: alerts
spec:
  groups:
    - name: custom_group
      interval: 60s
      rules:
        - alert: CustomAlert
          expr: custom_metric > 100
          for: 5m
          labels:
            severity: warning
            team: ml-ops
          annotations:
            summary: "Custom metric exceeded threshold"
            description: "Custom metric is {{ $value }}"
```

## Integration with Telemetry Processor

All alerts configured here are automatically sent to the telemetry processor, which:

1. Standardizes alert format
2. Enriches with workload/pod/node context
3. Performs correlation analysis
4. Routes to appropriate notification channels
5. Stores in database for historical analysis

See the telemetry-processor documentation for more details on alert processing.

