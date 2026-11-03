/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttrainingrun

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/url"
	"slices"

	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

const (
	EventSucceeded = "succeeded"
	EventFailed    = "failed"
	EventStopped   = "stopped"
	EventStalled   = "stalled"
	EventEvaluated = "evaluated"
)

var events = []string{EventSucceeded, EventFailed, EventStopped, EventStalled, EventEvaluated}

// Topic sends run-level emails of Post Training: the run succeeded, failed or was stopped, was first marked
// stalled, or finished its automatic evaluation.
type Topic struct{}

// TopicData is the notification payload the Post Training controller submits.
type TopicData struct {
	Event string     `json:"event"`
	Run   RunInfo    `json:"run"`
	Users []*v1.User `json:"users,omitempty"`
}

// RunInfo describes the run in the email.
type RunInfo struct {
	RunID       string `json:"runId"`
	DisplayName string `json:"displayName"`
	BaseModel   string `json:"baseModel"`
	TrainType   string `json:"trainType"`
	Status      string `json:"status"`
	FailedStage string `json:"failedStage,omitempty"`
	Message     string `json:"message,omitempty"`
	Duration    string `json:"duration,omitempty"`
	// MainMetric is the latest value of the train type's summary metric, e.g. "train/loss 0.4213 at step 340".
	MainMetric string `json:"mainMetric,omitempty"`
	// Scores are the benchmark scores of the automatic evaluation.
	Scores map[string]float64 `json:"scores,omitempty"`
}

// emailData is what the email template renders.
type emailData struct {
	Heading     string
	Run         RunInfo
	StatusColor string
	URL         string
}

// Name returns the topic name.
func (t *Topic) Name() string {
	return model.TopicPostTrainingRun
}

// Filter accepts the run events that send an email.
func (t *Topic) Filter(data map[string]interface{}) bool {
	event, _ := data["event"].(string)
	return slices.Contains(events, event)
}

// BuildMessage renders the email of one run event for the users in the payload.
func (t *Topic) BuildMessage(_ context.Context, data map[string]interface{}) ([]*model.Message, error) {
	d := &TopicData{}
	if err := commonutils.TransMapToStruct(data, d); err != nil {
		return nil, fmt.Errorf("failed to convert data to TopicData: %w", err)
	}
	var to []string
	for _, user := range d.Users {
		if email := v1.GetUserEmail(user); email != "" {
			to = append(to, email)
		}
	}
	if len(to) == 0 {
		klog.Warningf("No email recipients found for Post Training run %s", d.Run.RunID)
		return nil, nil
	}
	heading := headings[d.Event]
	content, err := render(emailData{Heading: heading, Run: d.Run, StatusColor: statusColor(d.Event), URL: runURL(d.Run.RunID)})
	if err != nil {
		return nil, err
	}
	return []*model.Message{{Email: &model.EmailMessage{
		To:      to,
		Title:   fmt.Sprintf("[Primus Safe] Post Training run %s ( %s )", heading, d.Run.DisplayName),
		Content: content,
	}}}, nil
}

var headings = map[string]string{
	EventSucceeded: "succeeded",
	EventFailed:    "failed",
	EventStopped:   "stopped",
	EventStalled:   "may be stalled",
	EventEvaluated: "evaluated",
}

// statusColor returns the color of the event in the email.
func statusColor(event string) string {
	switch event {
	case EventSucceeded, EventEvaluated:
		return "#2f855a"
	case EventFailed:
		return "#c53030"
	case EventStopped, EventStalled:
		return "#d69e2e"
	}
	return "#4a5568"
}

// runURL links to the run's detail page.
func runURL(runID string) string {
	return fmt.Sprintf("%s/post-training/detail?runId=%s", commonconfig.GetSystemHost(), url.QueryEscape(runID))
}

// render executes the email template.
func render(data emailData) (string, error) {
	tmpl, err := template.New("post_training_run").Parse(emailTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse email template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render email template: %w", err)
	}
	return buf.String(), nil
}
