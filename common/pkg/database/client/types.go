/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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
	CreateTime     pq.NullTime    `db:"create_time"`
	StartTime      pq.NullTime    `db:"start_time"`
	EndTime        pq.NullTime    `db:"end_time"`
	DeleteTime     pq.NullTime    `db:"delete_time"`
	IsSupervised   bool           `db:"is_supervised"`
	IsTolerateAll  bool           `db:"is_tolerate_all"`
	IsDeleted      bool           `db:"is_deleted"`
	Priority       int            `db:"priority"`
	MaxRetry       int            `db:"max_retry"`
	SchedulerOrder int            `db:"scheduler_order"`
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
}

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
	CreateTime     pq.NullTime    `db:"create_time"`
	UpdateTime     pq.NullTime    `db:"update_time"`
	DeleteTime     pq.NullTime    `db:"delete_time"`
	IsAutoRepaired bool           `db:"is_auto_repaired"`
}

func GetFaultFieldTags() map[string]string {
	f := Fault{}
	return getFieldTags(f)
}

type OpsJob struct {
	Id         int64          `db:"id"`
	JobId      string         `db:"job_id"`
	Cluster    string         `db:"cluster"`
	Inputs     []byte         `db:"inputs"`
	Type       string         `db:"type"`
	Timeout    int            `db:"timeout"`
	UserName   sql.NullString `db:"user_name"`
	Workspace  sql.NullString `db:"workspace"`
	CreateTime pq.NullTime    `db:"create_time"`
	StartTime  pq.NullTime    `db:"start_time"`
	EndTime    pq.NullTime    `db:"end_time"`
	DeleteTime pq.NullTime    `db:"delete_time"`
	Phase      sql.NullString `db:"phase"`
	Conditions sql.NullString `db:"conditions"`
	Outputs    sql.NullString `db:"outputs"`
	Env        sql.NullString `db:"env"`
	IsDeleted  bool           `db:"is_deleted"`
	UserId     sql.NullString `db:"user_id"`
	Resource   sql.NullString `db:"resource"`
	Image      sql.NullString `db:"image"`
	EntryPoint sql.NullString `db:"entrypoint"`
}

func GetOpsJobFieldTags() map[string]string {
	job := OpsJob{}
	return getFieldTags(job)
}

func getFieldTags(obj interface{}) map[string]string {
	result := make(map[string]string)
	t := reflect.TypeOf(obj)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		result[strings.ToLower(field.Name)] = field.Tag.Get("db")
	}
	return result
}

func genInsertCommand(obj interface{}, format, ignoreTag string) string {
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

func GetFieldTag(tags map[string]string, name string) string {
	name = strings.ToLower(name)
	return tags[name]
}
