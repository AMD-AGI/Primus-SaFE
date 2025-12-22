-- K8s Services Database Schema
-- This file defines the database schema for storing Kubernetes Service information
-- Used by gateway-exporter for traffic metrics enrichment

-- Table: k8s_services
-- Stores Kubernetes Service information for gateway traffic correlation
CREATE TABLE IF NOT EXISTS k8s_services (
    id BIGSERIAL PRIMARY KEY,
    uid VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(253) NOT NULL,
    namespace VARCHAR(253) NOT NULL,
    cluster_ip VARCHAR(45),
    service_type VARCHAR(32),
    selector JSONB DEFAULT '{}',
    ports JSONB DEFAULT '[]',
    labels JSONB DEFAULT '{}',
    annotations JSONB DEFAULT '{}',
    deleted BOOLEAN DEFAULT false,
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for k8s_services
CREATE UNIQUE INDEX IF NOT EXISTS idx_k8s_services_uid ON k8s_services(uid);
CREATE INDEX IF NOT EXISTS idx_k8s_services_name ON k8s_services(name);
CREATE INDEX IF NOT EXISTS idx_k8s_services_namespace ON k8s_services(namespace);
CREATE INDEX IF NOT EXISTS idx_k8s_services_namespace_name ON k8s_services(namespace, name);
CREATE INDEX IF NOT EXISTS idx_k8s_services_deleted ON k8s_services(deleted);
CREATE INDEX IF NOT EXISTS idx_k8s_services_updated_at ON k8s_services(updated_at DESC);

-- Comments for documentation
COMMENT ON TABLE k8s_services IS 'Kubernetes Service information storage for gateway traffic correlation';
COMMENT ON COLUMN k8s_services.uid IS 'Kubernetes Service UID (unique identifier)';
COMMENT ON COLUMN k8s_services.name IS 'Service name';
COMMENT ON COLUMN k8s_services.namespace IS 'Service namespace';
COMMENT ON COLUMN k8s_services.cluster_ip IS 'Service ClusterIP address';
COMMENT ON COLUMN k8s_services.service_type IS 'Service type (ClusterIP, NodePort, LoadBalancer, ExternalName)';
COMMENT ON COLUMN k8s_services.selector IS 'Service label selector as JSON';
COMMENT ON COLUMN k8s_services.ports IS 'Service ports configuration as JSON array';
COMMENT ON COLUMN k8s_services.labels IS 'Service labels as JSON';
COMMENT ON COLUMN k8s_services.annotations IS 'Service annotations as JSON';
COMMENT ON COLUMN k8s_services.deleted IS 'Whether the service has been deleted from Kubernetes';
COMMENT ON COLUMN k8s_services.created_at IS 'Timestamp when the service was first created';
COMMENT ON COLUMN k8s_services.updated_at IS 'Timestamp when the service was last updated';

-- Table: service_pod_references
-- Stores the relationship between Services and their backend Pods
CREATE TABLE IF NOT EXISTS service_pod_references (
    id BIGSERIAL PRIMARY KEY,
    service_uid VARCHAR(64) NOT NULL,
    service_name VARCHAR(253) NOT NULL,
    service_namespace VARCHAR(253) NOT NULL,
    pod_uid VARCHAR(64) NOT NULL,
    pod_name VARCHAR(253) NOT NULL,
    pod_ip VARCHAR(45),
    pod_labels JSONB DEFAULT '{}',
    workload_id VARCHAR(253),
    workload_owner VARCHAR(253),
    workload_type VARCHAR(64),
    node_name VARCHAR(253),
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for service_pod_references
CREATE INDEX IF NOT EXISTS idx_service_pod_refs_service_uid ON service_pod_references(service_uid);
CREATE INDEX IF NOT EXISTS idx_service_pod_refs_service_name ON service_pod_references(service_name);
CREATE INDEX IF NOT EXISTS idx_service_pod_refs_service_namespace ON service_pod_references(service_namespace);
CREATE INDEX IF NOT EXISTS idx_service_pod_refs_namespace_name ON service_pod_references(service_namespace, service_name);
CREATE INDEX IF NOT EXISTS idx_service_pod_refs_pod_uid ON service_pod_references(pod_uid);
CREATE INDEX IF NOT EXISTS idx_service_pod_refs_workload_id ON service_pod_references(workload_id);
CREATE INDEX IF NOT EXISTS idx_service_pod_refs_updated_at ON service_pod_references(updated_at DESC);

-- Composite unique index to prevent duplicate service-pod relationships
CREATE UNIQUE INDEX IF NOT EXISTS idx_service_pod_refs_unique ON service_pod_references(service_uid, pod_uid);

-- Comments for documentation
COMMENT ON TABLE service_pod_references IS 'Service to Pod relationship mapping for gateway traffic correlation';
COMMENT ON COLUMN service_pod_references.service_uid IS 'Reference to the Service UID';
COMMENT ON COLUMN service_pod_references.service_name IS 'Service name (denormalized for faster queries)';
COMMENT ON COLUMN service_pod_references.service_namespace IS 'Service namespace (denormalized for faster queries)';
COMMENT ON COLUMN service_pod_references.pod_uid IS 'Pod UID';
COMMENT ON COLUMN service_pod_references.pod_name IS 'Pod name';
COMMENT ON COLUMN service_pod_references.pod_ip IS 'Pod IP address';
COMMENT ON COLUMN service_pod_references.pod_labels IS 'Pod labels as JSON';
COMMENT ON COLUMN service_pod_references.workload_id IS 'Primus-SaFE workload identifier';
COMMENT ON COLUMN service_pod_references.workload_owner IS 'Workload owner (user name)';
COMMENT ON COLUMN service_pod_references.workload_type IS 'Workload type (deployment, statefulset, job, etc.)';
COMMENT ON COLUMN service_pod_references.node_name IS 'Node where the Pod is running';
COMMENT ON COLUMN service_pod_references.created_at IS 'Timestamp when the reference was first created';
COMMENT ON COLUMN service_pod_references.updated_at IS 'Timestamp when the reference was last updated';

