/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"k8s.io/klog/v2"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TA2AServiceRegistry = "a2a_service_registry"
)

var (
	insertA2AServiceFormat = `INSERT INTO ` + TA2AServiceRegistry + ` (%s) VALUES (%s) RETURNING id`
)

func (c *Client) UpsertA2AService(ctx context.Context, svc *A2AServiceRegistry) error {
	if svc == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}

	existing, _ := c.getA2AServiceIncludeDeleted(ctx, svc.ServiceName)
	if existing != nil {
		svc.Id = existing.Id
		nowTime := time.Now().UTC()
		cmd := fmt.Sprintf(`UPDATE %s SET display_name=$1, description=$2, endpoint=$3, a2a_path_prefix=$4, a2a_agent_card=$5, a2a_skills=$6, a2a_health=$7, a2a_last_seen=$8, k8s_namespace=$9, k8s_service=$10, discovery_source=$11, status=$12, workload_id=$13, updated_at=$14 WHERE service_name=$15`, TA2AServiceRegistry)
		_, err = db.ExecContext(ctx, cmd,
			svc.DisplayName, svc.Description, svc.Endpoint, svc.A2APathPrefix,
			svc.A2AAgentCard, svc.A2ASkills, svc.A2AHealth, svc.A2ALastSeen,
			svc.K8sNamespace, svc.K8sService, svc.DiscoverySource, svc.Status,
			svc.WorkloadId, nowTime, svc.ServiceName)
		return err
	}

	cmd := generateCommand(*svc, insertA2AServiceFormat, "id")
	rows, err := db.NamedQueryContext(ctx, cmd, svc)
	if err != nil {
		return fmt.Errorf("failed to insert a2a service: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		if err := rows.Scan(&svc.Id); err != nil {
			return fmt.Errorf("failed to scan a2a service id: %v", err)
		}
	}
	return nil
}

func (c *Client) getA2AServiceIncludeDeleted(ctx context.Context, serviceName string) (*A2AServiceRegistry, error) {
	if serviceName == "" {
		return nil, commonerrors.NewBadRequest("service name is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}
	cmd := fmt.Sprintf(`SELECT * FROM %s WHERE service_name=$1 LIMIT 1`, TA2AServiceRegistry)
	var svc A2AServiceRegistry
	err = db.GetContext(ctx, &svc, cmd, serviceName)
	if err != nil {
		return nil, err
	}
	return &svc, nil
}

func (c *Client) GetA2AService(ctx context.Context, serviceName string) (*A2AServiceRegistry, error) {
	if serviceName == "" {
		return nil, commonerrors.NewBadRequest("service name is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}
	cmd := fmt.Sprintf(`SELECT * FROM %s WHERE service_name=$1 AND status!='deleted' LIMIT 1`, TA2AServiceRegistry)
	var svc A2AServiceRegistry
	err = db.GetContext(ctx, &svc, cmd, serviceName)
	if err != nil {
		return nil, err
	}
	return &svc, nil
}

func (c *Client) GetA2AServiceByK8s(ctx context.Context, namespace, service string) (*A2AServiceRegistry, error) {
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}
	cmd := fmt.Sprintf(`SELECT * FROM %s WHERE k8s_namespace=$1 AND k8s_service=$2 AND status!='deleted' LIMIT 1`, TA2AServiceRegistry)
	var svc A2AServiceRegistry
	err = db.GetContext(ctx, &svc, cmd, namespace, service)
	if err != nil {
		return nil, err
	}
	return &svc, nil
}

func (c *Client) SelectA2AServices(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*A2AServiceRegistry, error) {
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}
	builder := sqrl.Select("*").PlaceholderFormat(sqrl.Dollar).From(TA2AServiceRegistry).Where(query)
	if offset > 0 || limit > 0 {
		builder = builder.Limit(uint64(limit)).Offset(uint64(offset))
	}
	sql, args, err := builder.OrderBy(orderBy...).ToSql()
	if err != nil {
		return nil, err
	}
	var services []*A2AServiceRegistry
	if c.RequestTimeout > 0 {
		ctx2, cancel := context.WithTimeout(ctx, c.RequestTimeout)
		defer cancel()
		err = db.SelectContext(ctx2, &services, sql, args...)
	} else {
		err = db.SelectContext(ctx, &services, sql, args...)
	}
	return services, err
}

func (c *Client) CountA2AServices(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}
	sql, args, err := sqrl.Select("COUNT(*)").PlaceholderFormat(sqrl.Dollar).From(TA2AServiceRegistry).Where(query).ToSql()
	if err != nil {
		return 0, err
	}
	var cnt int
	err = db.GetContext(ctx, &cnt, sql, args...)
	return cnt, err
}

func (c *Client) ListActiveA2AServices(ctx context.Context) ([]*A2AServiceRegistry, error) {
	return c.SelectA2AServices(ctx, sqrl.Eq{"status": "active"}, []string{"service_name ASC"}, 0, 0)
}

func (c *Client) SetA2AServiceDeleted(ctx context.Context, serviceName string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	nowTime := time.Now().UTC()
	cmd := fmt.Sprintf(`UPDATE %s SET status='deleted', updated_at=$1 WHERE service_name=$2`, TA2AServiceRegistry)
	result, err := db.ExecContext(ctx, cmd, nowTime, serviceName)
	if err != nil {
		klog.ErrorS(err, "failed to delete a2a service", "serviceName", serviceName)
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return commonerrors.NewNotFoundWithMessage("A2A service not found")
	}
	return nil
}

func (c *Client) UpdateA2AHealth(ctx context.Context, serviceName, health string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	nowTime := time.Now().UTC()
	cmd := fmt.Sprintf(`UPDATE %s SET a2a_health=$1, a2a_last_seen=$2, updated_at=$3 WHERE service_name=$4 AND status!='deleted'`, TA2AServiceRegistry)
	_, err = db.ExecContext(ctx, cmd, health, nowTime, nowTime, serviceName)
	return err
}
