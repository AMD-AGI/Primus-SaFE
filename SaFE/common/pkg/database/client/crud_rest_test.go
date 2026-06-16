/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"testing"
	"time"

	sqrl "github.com/Masterminds/squirrel"

	dbmodel "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

func TestA2AServiceRegistryCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()
	q := sqrl.Eq{"deleted": false}

	_ = c.UpsertA2AService(ctx, &A2AServiceRegistry{})
	_, _ = c.GetA2AService(ctx, "svc")
	_, _ = c.GetA2AServiceByK8s(ctx, "ns", "svc")
	_, _ = c.SelectA2AServices(ctx, q, []string{"id"}, 10, 0)
	_, _ = c.CountA2AServices(ctx, q)
	_, _ = c.ListActiveA2AServices(ctx)
	_ = c.SetA2AServiceDeleted(ctx, "svc")
	_ = c.UpdateA2AHealth(ctx, "svc", "healthy")
}

func TestA2ACallLogCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()
	q := sqrl.Eq{"id": 1}

	_ = c.InsertA2ACallLog(ctx, &A2ACallLog{})
	_, _ = c.SelectA2ACallLogs(ctx, q, []string{"id"}, 10, 0)
	_, _ = c.CountA2ACallLogs(ctx, q)
}

func TestLLMGatewayCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()

	_ = c.WithLLMBindingAdvisoryLock(ctx, "a@b", func(context.Context) error { return nil })
	_ = c.CreateLLMBinding(ctx, &LLMGatewayUserBinding{})
	_, _ = c.GetLLMBindingByEmail(ctx, "a@b")
	_, _ = c.GetLLMBindingByApimKeyHash(ctx, "hash")
	_ = c.UpdateLLMBinding(ctx, &LLMGatewayUserBinding{})
	_ = c.DeleteLLMBinding(ctx, "a@b")
	_, _, _ = c.ListLLMBindings(ctx, 10, 0)
}

func TestNodeStatisticCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 80)
	ctx := context.Background()

	_, _ = c.GetNodeStatisticByID(ctx, 1)
	_, _ = c.GetNodeStatisticByClusterAndNode(ctx, "c", "n")
	_, _ = c.GetNodeStatisticsByCluster(ctx, "c")
	_, _ = c.GetNodeStatisticsByNodeNames(ctx, "c", []string{"n1", "n2"})
	_, _ = c.GetNodeGpuUtilizationMap(ctx, "c", []string{"n1"})
	_ = c.CreateNodeStatistic(ctx, &dbmodel.NodeStatistic{})
	_ = c.UpdateNodeStatistic(ctx, &dbmodel.NodeStatistic{})
	_ = c.UpsertNodeStatistic(ctx, &dbmodel.NodeStatistic{})
	_ = c.DeleteNodeStatistic(ctx, 1)
	_ = c.DeleteNodeStatisticByClusterAndNode(ctx, "c", "n")
	_ = c.DeleteNodeStatisticsByCluster(ctx, "c")
}

func TestWorkloadStatisticCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 80)
	ctx := context.Background()

	_, _ = c.GetWorkloadStatisticByID(ctx, 1)
	_, _ = c.GetWorkloadStatisticByWorkloadID(ctx, "w")
	_, _ = c.GetWorkloadStatisticsByWorkloadID(ctx, "w")
	_, _ = c.GetWorkloadStatisticByWorkloadUID(ctx, "u")
	_, _ = c.GetWorkloadStatisticsByWorkloadUID(ctx, "u")
	_, _ = c.GetWorkloadStatisticsByClusterAndWorkspace(ctx, "c", "ws")
	_, _ = c.GetWorkloadStatisticsByType(ctx, "train")
	_ = c.UpsertWorkloadStatistic(ctx, &dbmodel.WorkloadStatistic{})
	_ = c.UpdateWorkloadStatistic(ctx, &dbmodel.WorkloadStatistic{})
	_ = c.DeleteWorkloadStatistic(ctx, 1)
	_ = c.DeleteWorkloadStatisticsByWorkloadID(ctx, "w")
	_ = c.CreateWorkloadStatistic(ctx, &dbmodel.WorkloadStatistic{})
}

func TestEmailOutboxCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()

	_ = c.CreateEmailOutbox(ctx, &dbmodel.EmailOutbox{})
	_, _ = c.ListPendingEmailOutbox(ctx, 10)
	_, _ = c.ListPendingEmailOutboxAfter(ctx, 0, 10)
	_, _ = c.DispatchEmailOutbox(ctx, 1)
	_ = c.AckEmailOutbox(ctx, 1)
	_, _ = c.ResetStaleDispatched(ctx, time.Minute)
	_ = c.FailEmailOutbox(ctx, 1, "err")
	_, _ = c.GetEmailOutbox(ctx, 1)
}

func TestImageImportJobCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()

	_, _ = c.GetImageImportJobByJobName(ctx, "job")
	_, _ = c.GetImageImportJobByTag(ctx, "tag")
	_, _ = c.GetImageImportJobByID(ctx, 1)
	_ = c.UpsertImageImportJob(ctx, &dbmodel.ImageImportJob{})
	_, _ = c.GetImportImageByImageID(ctx, 1)
	_ = c.UpdateImageImportJob(ctx, &dbmodel.ImageImportJob{})
}

func TestImageDigestCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()

	_ = c.UpsertImageDigest(ctx, &dbmodel.ImageDigest{})
	_, _ = c.GetImageDigestById(ctx, 1)
	_ = c.DeleteImageDigest(ctx, 1)
}

func TestRegistryInfoCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()

	_ = c.UpsertRegistryInfo(ctx, &dbmodel.RegistryInfo{})
	_, _ = c.GetRegistryInfoById(ctx, 1)
	_, _ = c.GetDefaultRegistryInfo(ctx)
	_, _ = c.GetRegistryInfoByUrl(ctx, "http://r")
	_ = c.DeleteRegistryInfo(ctx, 1)
	_, _ = c.ListRegistryInfos(ctx, 1, 10)
}

func TestPlaygroundSessionCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()
	q := sqrl.Eq{"deleted": false}

	_ = c.InsertPlaygroundSession(ctx, &PlaygroundSession{})
	_ = c.UpdatePlaygroundSession(ctx, &PlaygroundSession{})
	_, _ = c.SelectPlaygroundSessions(ctx, q, []string{"id"}, 10, 0)
	_, _ = c.CountPlaygroundSessions(ctx, q)
	_ = c.SetPlaygroundSessionDeleted(ctx, 1)
	_, _ = c.GetPlaygroundSession(ctx, 1)
}

func TestAuditLogCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()
	q := sqrl.Eq{"id": 1}

	_ = c.InsertAuditLog(ctx, &AuditLog{})
	_ = c.BatchInsertAuditLogs(ctx, []*AuditLog{{}, {}})
	_, _ = c.SelectAuditLogs(ctx, q, []string{"id"}, 10, 0)
	_, _ = c.CountAuditLogs(ctx, q)
}

func TestPublicKeyCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()
	q := sqrl.Eq{"deleted": false}

	_ = c.InsertPublicKey(ctx, &PublicKey{})
	_, _ = c.SelectPublicKeys(ctx, q, []string{"id"}, 10, 0)
	_, _ = c.CountPublicKeys(ctx, q)
	_ = c.DeletePublicKey(ctx, "u", 1)
	_ = c.SetPublicKeyStatus(ctx, "u", 1, true)
	_ = c.SetPublicKeyDescription(ctx, "u", 1, "desc")
	_, _ = c.GetPublicKeyByUserId(ctx, "u")
}

func TestUserTokenCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()

	_ = c.UpsertUserToken(ctx, &UserToken{})
	_, _ = c.SelectUserTokens(ctx, sqrl.Eq{"id": 1}, []string{"id"}, 10, 0)
}

func TestSshSessionRecordsCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()

	_, _ = c.InsertSshSessionRecord(ctx, &SshSessionRecords{})
	_ = c.SetSshDisconnect(ctx, 1, "closed")
}

func TestNotificationCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()

	_ = c.SubmitNotification(ctx, &dbmodel.Notification{})
	_ = c.UpdateNotification(ctx, &dbmodel.Notification{})
	_, _ = c.ListUnprocessedNotifications(ctx)
}
