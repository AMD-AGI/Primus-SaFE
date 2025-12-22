# Primus-SaFE Documentation Plan

> Documentation structure based on knowledge base best practices

## Directory Structure

```
docs-source/
├── architecture/                           # System Architecture
│   ├── system-overview.md                  # System Overview (HIGH PRIORITY)
│   ├── module-interaction.md               # Module Interaction & Data Flow
│   └── deployment-topology.md              # Deployment Topology & Network Architecture
│
├── adr/                                    # Architecture Decision Records (HIGH PRIORITY)
│   ├── adr-001-kubernetes-platform.md      # Why Kubernetes as Base Platform
│   ├── adr-002-multi-module-architecture.md # Multi-Module Architecture Decision
│   ├── adr-003-victoriametrics-vs-prometheus.md # Metrics Storage Selection
│   ├── adr-004-juicefs-storage.md          # Distributed Storage Selection
│   ├── adr-005-opensearch-logging.md       # Logging System Selection
│   ├── adr-006-gang-scheduling.md          # Gang Scheduling Strategy
│   ├── adr-007-topology-aware-placement.md # Topology-Aware Scheduling Design
│   ├── adr-008-fault-tolerance-strategy.md # Fault Tolerance & Auto-Recovery Strategy
│   ├── adr-009-checkpoint-mechanism.md     # Checkpoint & Job Recovery Mechanism
│   └── adr-010-multi-tenant-isolation.md   # Multi-Tenant Isolation Design
│
├── design/                                 # Detailed Design Documents (HIGH PRIORITY)
│   ├── bootstrap/
│   │   ├── cluster-provisioning.md         # Kubernetes Cluster Auto-Provisioning
│   │   ├── harbor-registry.md              # Container Registry Design
│   │   └── higress-gateway.md              # API Gateway Design
│   │
│   ├── safe-core/
│   │   ├── apiserver-design.md             # API Server Design
│   │   ├── job-manager-design.md           # Job Manager Design (Job Lifecycle)
│   │   ├── resource-manager-design.md      # Resource Manager Design
│   │   ├── node-agent-design.md            # Node Agent Design
│   │   ├── webhooks-design.md              # Admission Webhooks Design
│   │   ├── queue-management.md             # Multi-Priority Queue Management
│   │   └── preemption-policy.md            # Preemption Policy Design
│   │
│   ├── lens/
│   │   ├── metrics-pipeline.md             # Metrics Collection & Storage Pipeline
│   │   ├── gpu-exporter-design.md          # GPU Metrics Exporter Design
│   │   ├── network-exporter-design.md      # Network Metrics Exporter Design
│   │   ├── workload-exporter-design.md     # Workload Metrics Exporter Design
│   │   ├── logging-pipeline.md             # Logging Collection Pipeline
│   │   ├── ai-advisor-design.md            # AI Advisor (Anomaly Detection) Design
│   │   └── telemetry-processor-design.md   # Telemetry Processor Design
│   │
│   ├── bench/
│   │   ├── preflight-checks.md             # Preflight System Design
│   │   ├── benchmark-framework.md          # Benchmark Framework Design
│   │   └── performance-baseline.md         # Performance Baseline Management
│   │
│   └── scheduler-plugins/
│       ├── topology-ip-sort.md             # TopologyIPSort Plugin Design
│       └── co-scheduling.md                # Co-Scheduling Design
│
├── api/                                    # API Documentation (HIGH PRIORITY)
│   ├── apiserver-rest-api.md               # SaFE API Server REST Interface
│   ├── lens-api.md                         # Lens API Interface
│   ├── job-submission-api.md               # Job Submission API
│   ├── resource-management-api.md          # Resource Management API
│   └── metrics-query-api.md                # Metrics Query API
│
├── guides/                                 # User Guides
│   ├── quick-start.md                      # Quick Start Guide
│   ├── cluster-deployment-guide.md         # Complete Cluster Deployment Guide
│   ├── job-submission-guide.md             # Job Submission Guide
│   ├── distributed-training-guide.md       # Distributed Training Configuration Guide
│   ├── monitoring-setup-guide.md           # Monitoring Setup Guide
│   ├── dashboard-usage-guide.md            # Grafana Dashboard Usage Guide
│   ├── multi-tenant-setup.md               # Multi-Tenant Configuration Guide
│   └── upgrade-guide.md                    # Upgrade Guide
│
├── concepts/                               # Concept Explanations
│   ├── job-lifecycle.md                    # Job Lifecycle
│   ├── scheduling-concepts.md              # Scheduling Concepts (Gang, Topology-Aware)
│   ├── fault-tolerance.md                  # Fault Tolerance & Auto-Recovery Concepts
│   ├── observability-model.md              # Observability Model
│   └── workspace-isolation.md              # Workspace Isolation Concepts
│
└── troubleshooting/                        # Troubleshooting (LOW PRIORITY, add incrementally)
    ├── job-scheduling-issues.md            # Job Scheduling Issues
    ├── gpu-detection-issues.md             # GPU Detection Issues
    ├── network-connectivity-issues.md      # Network Connectivity Issues
    ├── storage-performance-issues.md       # Storage Performance Issues
    ├── training-failures.md                # Training Failure Troubleshooting
    └── metrics-collection-issues.md        # Metrics Collection Issues
```

## Priority Guide

| Priority | Category | Count | Description |
|----------|----------|-------|-------------|
| **HIGH** | system-overview.md | 1 | Core system overview document |
| **HIGH** | ADR (first 5) | 5 | Key architecture decisions |
| **HIGH** | Core Module Design | 3 | apiserver, job-manager, metrics-pipeline |
| **HIGH** | API Documentation | 3 | Core API references |
| **MEDIUM** | Other Design Docs | 10+ | Detailed module designs |
| **MEDIUM** | User Guides | 5 | Deployment and usage guides |
| **LOW** | Troubleshooting | As needed | Add when issues are encountered |

## Suggested Writing Order

### Week 1: Foundation
1. `architecture/system-overview.md` - System overview
2. `adr/adr-001-kubernetes-platform.md` - Platform decision
3. `adr/adr-002-multi-module-architecture.md` - Architecture decision
4. `adr/adr-003-victoriametrics-vs-prometheus.md` - Metrics storage decision

### Week 2: Core Design
5. `design/safe-core/apiserver-design.md` - API Server design
6. `design/safe-core/job-manager-design.md` - Job Manager design
7. `design/lens/metrics-pipeline.md` - Metrics pipeline design
8. `adr/adr-006-gang-scheduling.md` - Gang scheduling decision

### Week 3: API & More Design
9. `api/apiserver-rest-api.md` - REST API documentation
10. `api/job-submission-api.md` - Job submission API
11. `design/safe-core/resource-manager-design.md` - Resource Manager design
12. `design/bench/preflight-checks.md` - Preflight system design

### Week 4: Guides & Polish
13. `guides/quick-start.md` - Quick start guide
14. `guides/cluster-deployment-guide.md` - Deployment guide
15. `concepts/job-lifecycle.md` - Job lifecycle concepts
16. Review and refine existing documents

## Document Template Reference

Each document should follow these guidelines:

### Architecture/Design Documents
```markdown
# [Component Name]

## Overview
One-paragraph summary of the component.

## Responsibilities
- Key responsibility 1
- Key responsibility 2

## Architecture
[Diagram or description of internal structure]

## Key Interfaces
[API/Interface descriptions]

## Dependencies
- Component A: [relationship]
- Component B: [relationship]

## Configuration
[Key configuration options]

## Keywords
component-name, related-topic-1, related-topic-2
```

### ADR Documents
```markdown
# ADR-XXX: [Decision Title]

## Status
Accepted | Proposed | Deprecated

## Context
What is the issue that we're seeing that motivates this decision?

## Decision
What is the change that we're proposing and/or doing?

## Consequences
What becomes easier or more difficult to do because of this change?

## Alternatives Considered
- Alternative 1: [pros/cons]
- Alternative 2: [pros/cons]

## Keywords
adr, decision-topic, technology-name
```

## Keywords
documentation, primus-safe, architecture, adr, design, api, guides

