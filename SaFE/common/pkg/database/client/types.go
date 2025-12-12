/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/lib/pq"
)

const (
	DESC = "desc"
	ASC  = "asc"

	CreateTime  = "create_time"
	CreatedTime = "created_at"
)

type Workload struct {
	Id             int64          `db:"id"`
	WorkloadId     string         `db:"workload_id"`
	DisplayName    string         `db:"display_name"`
	Workspace      string         `db:"workspace"`
	Cluster        string         `db:"cluster"`
	Resource       string         `db:"resource"`
	Image          string         `db:"image"`
	EntryPoint     string         `db:"entrypoint"`
	GVK            string         `db:"gvk"`
	Phase          sql.NullString `db:"phase"`
	UserName       sql.NullString `db:"username"`
	CreationTime   pq.NullTime    `db:"creation_time"`
	StartTime      pq.NullTime    `db:"start_time"`
	EndTime        pq.NullTime    `db:"end_time"`
	DeletionTime   pq.NullTime    `db:"deletion_time"`
	IsSupervised   bool           `db:"is_supervised"`
	IsTolerateAll  bool           `db:"is_tolerate_all"`
	IsDeleted      bool           `db:"is_deleted"`
	Priority       int            `db:"priority"`
	MaxRetry       int            `db:"max_retry"`
	QueuePosition  int            `db:"queue_position"`
	DispatchCount  int            `db:"dispatch_count"`
	TTLSecond      int            `db:"ttl_second"`
	Timeout        int            `db:"timeout"`
	Env            sql.NullString `db:"env"`
	Description    sql.NullString `db:"description"`
	Pods           sql.NullString `db:"pods"`
	Nodes          sql.NullString `db:"nodes"`
	Conditions     sql.NullString `db:"conditions"`
	CustomerLabels sql.NullString `db:"customer_labels"`
	Service        sql.NullString `db:"service"`
	Liveness       sql.NullString `db:"liveness"`
	Readiness      sql.NullString `db:"readiness"`
	UserId         sql.NullString `db:"user_id"`
	K8sObjectUid   sql.NullString `db:"k8s_object_uid"`
	WorkloadUId    sql.NullString `db:"workload_uid"`
	Ranks          sql.NullString `db:"ranks"`
	Dependencies   sql.NullString `db:"dependencies"`
	CronJobs       sql.NullString `db:"cron_jobs"`
	Secrets        sql.NullString `db:"secrets"`
	ScaleRunnerSet sql.NullString `db:"scale_runner_set"`
	ScaleRunnerId  sql.NullString `db:"scale_runner_id"`
}

// GetWorkloadFieldTags returns the WorkloadFieldTags value.
func GetWorkloadFieldTags() map[string]string {
	w := Workload{}
	return getFieldTags(w)
}

type Fault struct {
	Id             int64          `db:"id"`
	Uid            string         `db:"uid"`
	MonitorId      string         `db:"monitor_id"`
	Message        sql.NullString `db:"message"`
	Node           sql.NullString `db:"node"`
	Action         sql.NullString `db:"action"`
	Phase          sql.NullString `db:"phase"`
	Cluster        sql.NullString `db:"cluster"`
	CreationTime   pq.NullTime    `db:"creation_time"`
	UpdateTime     pq.NullTime    `db:"update_time"`
	DeletionTime   pq.NullTime    `db:"deletion_time"`
	IsAutoRepaired bool           `db:"is_auto_repaired"`
}

// GetFaultFieldTags returns the FaultFieldTags value.
func GetFaultFieldTags() map[string]string {
	f := Fault{}
	return getFieldTags(f)
}

type OpsJob struct {
	Id            int64          `db:"id"`
	JobId         string         `db:"job_id"`
	Cluster       string         `db:"cluster"`
	Inputs        []byte         `db:"inputs"`
	Type          string         `db:"type"`
	Timeout       int            `db:"timeout"`
	UserName      sql.NullString `db:"user_name"`
	Workspace     sql.NullString `db:"workspace"`
	CreationTime  pq.NullTime    `db:"creation_time"`
	StartTime     pq.NullTime    `db:"start_time"`
	EndTime       pq.NullTime    `db:"end_time"`
	DeletionTime  pq.NullTime    `db:"deletion_time"`
	Phase         sql.NullString `db:"phase"`
	Conditions    sql.NullString `db:"conditions"`
	Outputs       sql.NullString `db:"outputs"`
	Env           sql.NullString `db:"env"`
	IsDeleted     bool           `db:"is_deleted"`
	UserId        sql.NullString `db:"user_id"`
	Resource      sql.NullString `db:"resource"`
	Image         sql.NullString `db:"image"`
	EntryPoint    sql.NullString `db:"entrypoint"`
	IsTolerateAll bool           `db:"is_tolerate_all"`
	Hostpath      sql.NullString `db:"hostpath"`
}

// GetOpsJobFieldTags returns the OpsJobFieldTags value.
func GetOpsJobFieldTags() map[string]string {
	job := OpsJob{}
	return getFieldTags(job)
}

// getFieldTags retrieves FieldTags for internal use.
func getFieldTags(obj interface{}) map[string]string {
	result := make(map[string]string)
	t := reflect.TypeOf(obj)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		result[strings.ToLower(field.Name)] = field.Tag.Get("db")
	}
	return result
}

// generateCommand generates SQL command string using reflection
// Iterates through struct fields and builds column and value lists
// Skips fields with specified ignoreTag
// Returns formatted SQL command with columns and values
func generateCommand(obj interface{}, format, ignoreTag string) string {
	t := reflect.TypeOf(obj)
	columns := make([]string, 0, t.NumField())
	values := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("db")
		if tag == ignoreTag {
			continue
		}
		columns = append(columns, tag)
		values = append(values, fmt.Sprintf(":%s", tag))
	}
	cmd := fmt.Sprintf(format, strings.Join(columns, ", "), strings.Join(values, ", "))
	return cmd
}

// GetFieldTag returns the FieldTag value.
func GetFieldTag(tags map[string]string, name string) string {
	name = strings.ToLower(name)
	return tags[name]
}

type PublicKey struct {
	Id          int64       `db:"id"`
	UserId      string      `db:"user_id"`
	Description string      `db:"description"`
	PublicKey   string      `db:"public_key"`
	Status      bool        `db:"status"`
	CreateTime  pq.NullTime `db:"create_time"`
	UpdateTime  pq.NullTime `db:"update_time"`
	DeleteTime  pq.NullTime `db:"delete_time"`
}

// GetPublicKeyFieldTags returns the PublicKeyFieldTags value.
func GetPublicKeyFieldTags() map[string]string {
	f := PublicKey{}
	return getFieldTags(f)
}

type SshSessionRecords struct {
	Id               int64       `db:"id"`
	UserId           string      `db:"user_id"`
	SshType          string      `db:"ssh_type"`
	Namespace        string      `db:"namespace"`
	PodId            string      `db:"pod_id"`
	ContainerName    string      `db:"container_name"`
	DisconnectReason string      `db:"disconnect_reason"`
	DisconnectTime   pq.NullTime `db:"disconnect_time"`
	CreateTime       pq.NullTime `db:"create_time"`
}

// GetSshSessionRecordsFieldTags returns the SshSessionRecordsFieldTags value.
func GetSshSessionRecordsFieldTags() map[string]string {
	f := SshSessionRecords{}
	return getFieldTags(f)
}

type UserToken struct {
	UserId       string `db:"user_id"`
	SessionId    string `db:"session_id"`
	Token        string `db:"token"`
	CreationTime int64  `db:"creation_time"`
	ExpireTime   int64  `db:"expire_time"`
}

// GetUserTokenFieldTags returns the UserTokenFieldTags value.
func GetUserTokenFieldTags() map[string]string {
	token := UserToken{}
	return getFieldTags(token)
}

type Inference struct {
	Id           int64          `db:"id"`
	InferenceId  string         `db:"inference_id"`
	DisplayName  string         `db:"display_name"`
	Description  sql.NullString `db:"description"`
	UserId       string         `db:"user_id"`
	UserName     sql.NullString `db:"user_name"`
	ModelForm    string         `db:"model_form"`
	ModelName    string         `db:"model_name"`
	Instance     sql.NullString `db:"instance"`
	Resource     sql.NullString `db:"resource"`
	Config       sql.NullString `db:"config"`
	Phase        sql.NullString `db:"phase"`
	Events       sql.NullString `db:"events"`
	Message      sql.NullString `db:"message"`
	CreationTime pq.NullTime    `db:"creation_time"`
	UpdateTime   pq.NullTime    `db:"update_time"`
	DeletionTime pq.NullTime    `db:"deletion_time"`
	IsDeleted    bool           `db:"is_deleted"`
}

// GetInferenceFieldTags returns the InferenceFieldTags value.
func GetInferenceFieldTags() map[string]string {
	inf := Inference{}
	return getFieldTags(inf)
}

type PlaygroundSession struct {
	Id           int64       `db:"id"`
	UserId       string      `db:"user_id"`
	ModelName    string      `db:"model_name"`
	DisplayName  string      `db:"display_name"`
	SystemPrompt string      `db:"system_prompt"`
	Messages     string      `db:"messages"`
	CreationTime pq.NullTime `db:"creation_time"`
	UpdateTime   pq.NullTime `db:"update_time"`
	IsDeleted    bool        `db:"is_deleted"`
}

// GetPlaygroundSessionFieldTags returns the PlaygroundSessionFieldTags value.
func GetPlaygroundSessionFieldTags() map[string]string {
	session := PlaygroundSession{}
	return getFieldTags(session)
}

type Model struct {
	ID             string      `gorm:"column:id;primaryKey" json:"id" db:"id"`
	DisplayName    string      `gorm:"column:display_name" json:"displayName" db:"display_name"`
	Description    string      `gorm:"column:description" json:"description" db:"description"`
	Icon           string      `gorm:"column:icon" json:"icon" db:"icon"`
	Label          string      `gorm:"column:label" json:"label" db:"label"`
	Tags           string      `gorm:"column:tags" json:"tags" db:"tags"`
	MaxTokens      int         `gorm:"column:max_tokens" json:"maxTokens" db:"max_tokens"`
	Version        string      `gorm:"column:version" json:"version" db:"version"`
	SourceURL      string      `gorm:"column:source_url" json:"sourceURL" db:"source_url"`
	AccessMode     string      `gorm:"column:access_mode" json:"accessMode" db:"access_mode"`
	SourceToken    string      `gorm:"column:source_token" json:"sourceToken" db:"source_token"`
	Phase          string      `gorm:"column:phase" json:"phase" db:"phase"`
	Message        string      `gorm:"column:message" json:"message" db:"message"`
	InferenceID    string      `gorm:"column:inference_id" json:"inferenceID" db:"inference_id"`
	InferencePhase string      `gorm:"column:inference_phase" json:"inferencePhase" db:"inference_phase"`
	CreatedAt      pq.NullTime `gorm:"column:created_at;autoCreateTime" json:"createdAt" db:"created_at"`
	UpdatedAt      pq.NullTime `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt" db:"updated_at"`
	DeletionTime   pq.NullTime `gorm:"column:deletion_time" json:"deletionTime" db:"deletion_time"`
	IsDeleted      bool        `gorm:"column:is_deleted" json:"isDeleted" db:"is_deleted"`
}

func (Model) TableName() string {
	return "model"
}
