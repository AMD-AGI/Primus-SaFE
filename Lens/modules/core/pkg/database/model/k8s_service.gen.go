// K8s Service model for gateway-exporter
// This file stores Kubernetes Service information for gateway traffic metrics enrichment

package model

import (
	"time"
)

const TableNameK8sService = "k8s_services"

// K8sService represents a Kubernetes Service in the database
type K8sService struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	UID         string    `gorm:"column:uid;type:varchar(64);uniqueIndex" json:"uid"`
	Name        string    `gorm:"column:name;type:varchar(253);index" json:"name"`
	Namespace   string    `gorm:"column:namespace;type:varchar(253);index" json:"namespace"`
	ClusterIP   string    `gorm:"column:cluster_ip;type:varchar(45)" json:"cluster_ip"`
	ServiceType string    `gorm:"column:service_type;type:varchar(32)" json:"service_type"`
	Selector    ExtType   `gorm:"column:selector;type:jsonb;default:'{}'" json:"selector"`
	Ports       ExtJSON   `gorm:"column:ports;type:jsonb;default:'[]'" json:"ports"`
	Labels      ExtType   `gorm:"column:labels;type:jsonb;default:'{}'" json:"labels"`
	Annotations ExtType   `gorm:"column:annotations;type:jsonb;default:'{}'" json:"annotations"`
	Deleted     bool      `gorm:"column:deleted;default:false" json:"deleted"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updated_at"`
}

// TableName returns the table name for K8sService
func (*K8sService) TableName() string {
	return TableNameK8sService
}

// ServicePort represents a port exposed by a Service
// Used for JSON serialization within the Ports field
type ServicePort struct {
	Name       string `json:"name"`
	Port       int    `json:"port"`
	TargetPort string `json:"target_port"`
	Protocol   string `json:"protocol"`
	NodePort   int    `json:"node_port,omitempty"`
}
