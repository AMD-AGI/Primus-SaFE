# Primus Lens Documentation Center

Welcome to the Primus Lens Documentation Center. This directory provides a complete index of system documentation.

## WandB Integration Documentation

### 📘 [WandB Integration Technical Documentation](./wandb-integration-technical.md)

**Target Audience**: Developers, System Architects, DevOps Engineers

**Content Overview**:
- System architecture and data flow details
- wandb-exporter implementation principles
- telemetry-processor processing logic
- API query interface design
- Database models and storage structure
- Monitoring metrics and troubleshooting
- Complete API reference

**Use Cases**:
- Understanding system internals
- Secondary development and customization
- System integration and deployment
- Performance optimization and tuning
- Problem diagnosis and debugging

### 📗 [WandB Integration User Guide](./wandb-integration-user-guide.md)

**Target Audience**: Training Engineers, Algorithm Researchers, Data Scientists

**Content Overview**:
- Quick start guide (3 steps to get started)
- Common usage scenarios
  - Single machine training
  - Distributed training (single node multi-GPU)
  - Multi-node distributed training
  - PyTorch Lightning
  - Primus Framework
- Metrics viewing and visualization
- Advanced configuration options
- FAQ
- Best practices

**Use Cases**:
- Integrating WandB in training tasks
- Viewing and analyzing training metrics
- Configuring and debugging WandB integration
- Solving common usage issues

## How to Choose Documentation

### Which documentation should I read?

**If you are a training user** and want to:
- ✅ Use WandB in training code
- ✅ View training metrics and visualizations
- ✅ Configure environment variables
- ✅ Solve usage issues

👉 Please read the **[User Guide](./wandb-integration-user-guide.md)**

---

**If you are a developer/operations personnel** and need to:
- ✅ Understand system architecture and implementation principles
- ✅ Perform system deployment and configuration
- ✅ Develop new features or customizations
- ✅ Debug system issues and optimize performance
- ✅ Integrate with other systems

👉 Please read the **[Technical Documentation](./wandb-integration-technical.md)**

---

**If you are a new user**, suggested reading order:
1. First, quickly browse the "Quick Start" section of the **[User Guide](./wandb-integration-user-guide.md)**
2. Find the corresponding example based on your usage scenario
3. If you encounter problems, check the "FAQ" section
4. If you need in-depth understanding, then read the **[Technical Documentation](./wandb-integration-technical.md)**

## Documentation Quick Index

### Quick Start

- [3 Steps to Get Started](./wandb-integration-user-guide.md#quick-start) - User Guide
- [Install WandB Exporter](./wandb-integration-user-guide.md#step-1-install-wandb-exporter) - User Guide
- [Configure Environment Variables](./wandb-integration-user-guide.md#step-2-configure-environment-variables) - User Guide

### Usage Scenarios

- [Single Machine Training](./wandb-integration-user-guide.md#scenario-1-single-machine-training) - User Guide
- [Distributed Training](./wandb-integration-user-guide.md#scenario-2-distributed-training-single-node-multi-gpu) - User Guide
- [Multi-Node Training](./wandb-integration-user-guide.md#scenario-3-multi-node-distributed-training) - User Guide
- [PyTorch Lightning](./wandb-integration-user-guide.md#scenario-4-using-pytorch-lightning) - User Guide
- [Primus Framework](./wandb-integration-user-guide.md#scenario-5-using-primus-framework) - User Guide

### Technical Details

- [System Architecture](./wandb-integration-technical.md#system-architecture) - Technical Documentation
- [Data Flow Details](./wandb-integration-technical.md#overview) - Technical Documentation
- [Auto-Interception Mechanism](./wandb-integration-technical.md#11-auto-interception-mechanism) - Technical Documentation
- [Framework Detection Principles](./wandb-integration-technical.md#12-framework-detection-and-data-collection) - Technical Documentation
- [Data Processing Flow](./wandb-integration-technical.md#23-metrics-data-processing) - Technical Documentation
- [API Interface Design](./wandb-integration-technical.md#31-api-endpoint-design) - Technical Documentation
- [Database Models](./wandb-integration-technical.md#233-database-model) - Technical Documentation

### Configuration and Deployment

- [Environment Variable Configuration](./wandb-integration-user-guide.md#configuration-options) - User Guide
- [Advanced Configuration](./wandb-integration-user-guide.md#advanced-configuration) - User Guide
- [System Deployment](./wandb-integration-technical.md#part-4-configuration-and-deployment) - Technical Documentation
- [Monitoring Metrics](./wandb-integration-technical.md#24-monitoring-metrics) - Technical Documentation

### API Reference

- [Query API](./wandb-integration-technical.md#31-api-endpoint-design) - Technical Documentation
- [Data Formats](./wandb-integration-technical.md#a-complete-data-format-examples) - Technical Documentation
- [Route Table](./wandb-integration-technical.md#b-complete-api-route-table) - Technical Documentation

### Troubleshooting

- [FAQ](./wandb-integration-user-guide.md#faq) - User Guide
- [Monitoring and Troubleshooting](./wandb-integration-technical.md#part-5-monitoring-and-troubleshooting) - Technical Documentation
- [Performance Tuning](./wandb-integration-technical.md#532-performance-tuning) - Technical Documentation

### Best Practices

- [Naming Conventions](./wandb-integration-user-guide.md#1-naming-conventions) - User Guide
- [Metrics Organization](./wandb-integration-user-guide.md#2-metrics-organization) - User Guide
- [Error Handling](./wandb-integration-user-guide.md#5-error-handling) - User Guide
- [Distributed Optimization](./wandb-integration-user-guide.md#6-distributed-training-optimization) - User Guide

## Version Information

| Documentation | Version | Last Updated | Status |
|------|------|---------|------|
| WandB Integration Technical Documentation | 1.0 | 2024-12-03 | ✅ Latest |
| WandB Integration User Guide | 1.0 | 2024-12-03 | ✅ Latest |

## Contribution Guidelines

If you find errors in the documentation or wish to improve it, please:

1. Submit an Issue on GitHub
2. Or directly submit a Pull Request
3. Contact the documentation maintenance team

## Additional Resources

- **Lens Project Homepage**: [GitHub Repository](https://github.com/AMD-AGI/Primus-SaFE)
- **WandB Official Documentation**: https://docs.wandb.ai/
- **PyTorch Distributed Training**: https://pytorch.org/tutorials/beginner/dist_overview.html
- **Kubernetes Official Documentation**: https://kubernetes.io/docs/

## Language Versions

- [English Documentation](./README.md) (Current)
- [中文文档](./README-ZH.md)

---

**Documentation Maintenance**: Primus Lens Team  
**Last Updated**: 2024-12-03

