# Kubernetes Scheduler Plugins

A collection of custom Kubernetes scheduler plugins that extend the default Kubernetes scheduler with advanced scheduling capabilities.

## Overview

This repository contains custom scheduler plugins built on top of the [scheduler-plugins](https://github.com/kubernetes-sigs/scheduler-plugins) framework. These plugins provide enhanced scheduling features for specific use cases and workloads.

## Available Plugins

### TopologyIPSort Plugin

A topology-aware scheduler plugin that optimizes pod placement based on node IP addresses and network topology.

**Features:**
- IP-based node sorting for network locality
- Pod group support for gang scheduling
- Priority-based queue sorting
- Co-scheduling for distributed workloads
- Support for Kubeflow training workloads

**Documentation:** [TopologyIPSort README](pkg/topologyipsort/README.md)

## Quick Start

### Prerequisites

- Kubernetes cluster (v1.24+)
- Helm 3.0+
- kubectl configured to access your cluster

### Installation

1. **Clone the repository:**
   ```bash
   git clone <repository-url>
   cd kube-scheduler-plugins
   ```

2. **Install using Helm:**
   ```bash
   helm install scheduler-plugins manifests/charts/scheduler-plugins/
   ```

3. **Verify installation:**
   ```bash
   kubectl get pods -n scheduler-plugins
   ```

### Configuration

Create a scheduler configuration that enables the desired plugins:

```yaml
apiVersion: kubescheduler.config.k8s.io/v1
kind: KubeSchedulerConfiguration
profiles:
- schedulerName: custom-scheduler
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

### Usage

To use the custom scheduler, specify the scheduler name in your pod specifications:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: example-pod
spec:
  schedulerName: custom-scheduler
  containers:
  - name: app
    image: nginx:latest
```

## Development

### Building

```bash
make build
```

### Testing

```bash
make test
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run specific plugin tests
go test ./pkg/topologyipsort/...
```

## Architecture

The plugins are built using the Kubernetes scheduler framework and extend the following extension points:

- **Score Plugin**: Evaluates and scores nodes for pod placement
- **Queue Sort Plugin**: Defines custom ordering for pods in the scheduling queue
- **Permit Plugin**: Controls pod admission and implements co-scheduling logic
- **Post Filter Plugin**: Provides diagnostics and failure handling

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

### Development Workflow

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Ensure all tests pass
6. Submit a pull request

### Code Style

- Follow Go coding standards
- Add comprehensive tests for new features
- Update documentation for API changes
- Use meaningful commit messages

## Troubleshooting

### Common Issues

1. **Plugin not loaded**: Check scheduler configuration and plugin registration
2. **Pods stuck in pending**: Verify scheduler is running and accessible
3. **Scheduling failures**: Check plugin logs for detailed error messages

### Debug Logging

Enable debug logging to troubleshoot issues:

```bash
kubectl logs -n scheduler-plugins deployment/scheduler-plugins-scheduler -f
```

## License

This project is licensed under the Apache License 2.0. See the [LICENSE](LICENSE) file for details.

## Support

- **Issues**: Report bugs and feature requests via GitHub issues
- **Discussions**: Join discussions in GitHub discussions
- **Documentation**: Check individual plugin READMEs for detailed documentation

## Related Projects

- [Kubernetes Scheduler Framework](https://kubernetes.io/docs/concepts/scheduling-eviction/kube-scheduler/#scheduling-framework)
- [scheduler-plugins](https://github.com/kubernetes-sigs/scheduler-plugins)
- [Kubeflow](https://www.kubeflow.org/)
