/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"fmt"
	"time"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/topic/posttrainingrun"
)

// endEvents maps the status a run ends with to its notification event.
var endEvents = map[string]string{
	string(v1.WorkloadSucceeded): posttrainingrun.EventSucceeded,
	string(v1.WorkloadFailed):    posttrainingrun.EventFailed,
	string(v1.WorkloadStopped):   posttrainingrun.EventStopped,
}

// notifyChanges sends the run events implied by the columns just written to run: it ended, or it was marked
// stalled while it was not before.
func (r *RunReconciler) notifyChanges(ctx context.Context, run *dbclient.PostTrainingRun, fields map[string]interface{}) {
	if status, ok := fields["status"].(string); ok {
		if event, ok := endEvents[status]; ok {
			r.notify(ctx, run, fields, event, nil)
		}
	}
	if since, ok := fields["stalled_since"]; ok && since != nil && !run.StalledSince.Valid {
		r.notify(ctx, run, fields, posttrainingrun.EventStalled, nil)
	}
}

// notify submits one event of run to its user when the user has notifications on. The notification table keeps
// one notification per run and event, so an event repeated by a later reconcile is not sent again.
func (r *RunReconciler) notify(ctx context.Context, run *dbclient.PostTrainingRun, fields map[string]interface{},
	event string, scores map[string]float64) {
	manager := notification.GetNotificationManager()
	if manager == nil || run.UserID == "" {
		return
	}
	user := &v1.User{}
	if err := r.Get(ctx, client.ObjectKey{Name: run.UserID}, user); err != nil {
		klog.ErrorS(err, "failed to get the user of a Post Training run", "run", run.RunID, "user", run.UserID)
		return
	}
	if !v1.IsUserEnableNotification(user) {
		return
	}
	info := runInfo(run, fields)
	info.Scores = scores
	data := map[string]interface{}{"topic": model.TopicPostTrainingRun, "event": event, "run": info,
		"users": []*v1.User{user}}
	if err := manager.SubmitNotification(ctx, model.TopicPostTrainingRun, run.RunID+"-"+event, data); err != nil {
		klog.ErrorS(err, "failed to submit a Post Training notification", "run", run.RunID, "event", event)
	}
}

// runInfo describes run for an email, preferring the values just written in fields.
func runInfo(run *dbclient.PostTrainingRun, fields map[string]interface{}) posttrainingrun.RunInfo {
	pick := func(column, current string) string {
		if v, ok := fields[column].(string); ok {
			return v
		}
		return current
	}
	info := posttrainingrun.RunInfo{
		RunID: run.RunID, DisplayName: run.DisplayName, BaseModel: run.BaseModelName, TrainType: run.TrainType,
		Status: pick("status", run.Status), FailedStage: pick("failed_stage", run.FailedStage),
		Message: pick("message", run.Message),
	}
	start, end := run.StartTime.Time, run.EndTime.Time
	if t, ok := fields["start_time"].(time.Time); ok {
		start = t
	}
	if t, ok := fields["end_time"].(time.Time); ok {
		end = t
	}
	if !start.IsZero() && end.After(start) {
		info.Duration = end.Sub(start).Round(time.Minute).String()
	}
	if m := loadSummary(pick("metric_summary", run.MetricSummary)).Main; m != nil {
		info.MainMetric = fmt.Sprintf("%s %.4g at step %d", m.Name, m.Value, m.Step)
	}
	return info
}
