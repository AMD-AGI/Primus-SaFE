/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

const (
	pendingEvidencePodLimit     = 5
	pendingEvidencePodListLimit = 50
	pendingEvidenceEventLimit   = 5
	pendingEvidenceEventList    = 30
	pendingEvidenceLogPods      = 3
	pendingEvidenceLogTail      = 400
	pendingEvidenceLogLines     = 30
	pendingEvidenceLineLimit    = 300
	pendingEvidenceLimit        = 6000
	pendingReasonSentenceLimit  = 400

	jobManagerAppLabelKey   = "primus-safe-app-name"
	jobManagerAppLabelValue = "job-manager"
)

// pendingReasonSummarizer turns pending evidence into one sentence billed to the caller.
type pendingReasonSummarizer interface {
	SummarizePendingReason(ctx context.Context, email, ntid, evidence string) (string, error)
}

// errPendingSummaryUnavailable is returned when no summarizer or caller identity is available.
var errPendingSummaryUnavailable = errors.New("pending reason summarizer is unavailable")

// fetchPodLogs reads the tail of one pod log. Tests replace it.
var fetchPodLogs = func(ctx context.Context, clientSet kubernetes.Interface, namespace, name string, tail int64) ([]byte, error) {
	opts := &corev1.PodLogOptions{TailLines: &tail}
	return clientSet.CoreV1().Pods(namespace).GetLogs(name, opts).DoRaw(ctx)
}

type pendingReasonAction int

const (
	pendingReasonNone pendingReasonAction = iota
	pendingReasonMessage
	pendingReasonCollect
)

// SetPendingReasonSummarizer installs the LiteLLM summarizer used by the pending-reason API.
// A nil summarizer leaves the scheduler message or the strongest collected fact.
func (h *Handler) SetPendingReasonSummarizer(s pendingReasonSummarizer) {
	if h == nil {
		return
	}
	h.pendingSummarizer = s
}

// GetWorkloadPendingReason explains why a workload is Pending.
// The reason is empty when the workload is not Pending.
func (h *Handler) GetWorkloadPendingReason(c *gin.Context) {
	handle(c, h.getWorkloadPendingReason)
}

// getWorkloadPendingReason authorizes the caller, reads the live workload, and explains Pending.
func (h *Handler) getWorkloadPendingReason(c *gin.Context) (interface{}, error) {
	workload, err := h.getAndAuthWorkload(c)
	if err != nil {
		return nil, err
	}
	// Queue position and message are written on the control-plane object.
	// The auth lookup may be a database projection without that status.
	live, err := h.getAdminWorkload(c.Request.Context(), workload.Name)
	if err != nil {
		return nil, err
	}
	email, ntid := h.pendingReasonCaller(c)
	return &view.WorkloadPendingReasonResponse{
		Reason: h.resolvePendingReason(c.Request.Context(), live, email, ntid),
	}, nil
}

// pendingReasonCaller returns the email and NTID used to bill a summary.
// A lookup failure still returns the identity already on the request.
func (h *Handler) pendingReasonCaller(c *gin.Context) (string, string) {
	user, err := h.getAndSetUsername(c)
	if err != nil {
		klog.ErrorS(err, "failed to resolve caller for pending reason")
	}
	if user != nil {
		email := strings.TrimSpace(v1.GetUserEmail(user))
		ntid := v1.NtidFromPreferredName(v1.GetAnnotation(user, v1.UserPreferredNameAnnotation))
		if email == "" {
			email = strings.TrimSpace(v1.GetUserName(user))
		}
		if email == "" {
			email = user.Name
		}
		return email, ntid
	}
	if name := strings.TrimSpace(c.GetString(common.UserName)); name != "" {
		return name, ""
	}
	return strings.TrimSpace(c.GetString(common.UserId)), ""
}

// resolvePendingReason selects the message, a summary, or collected evidence for one workload.
func (h *Handler) resolvePendingReason(ctx context.Context, w *v1.Workload, email, ntid string) string {
	if w == nil {
		return ""
	}
	action := pendingReasonActionFor(w.Status.Phase, w.Status.QueuePosition, w.Status.Message)
	return applyPendingReason(action, w.Status.Message,
		func() (string, string) { return h.collectPendingEvidence(ctx, w) },
		func(text string) (string, error) {
			if h.pendingSummarizer == nil || strings.TrimSpace(email) == "" {
				return "", errPendingSummaryUnavailable
			}
			summary, err := h.pendingSummarizer.SummarizePendingReason(ctx, email, ntid, text)
			if err != nil {
				klog.ErrorS(err, "failed to summarize pending reason", "workload", w.Name)
			}
			return summary, err
		},
	)
}

// pendingReasonActionFor chooses how a Pending workload is explained.
// A queue position of 1 or more keeps a non-empty scheduler message as written, since no pod exists yet.
// Otherwise the pods are collected, and a non-empty message is included as one piece of evidence.
func pendingReasonActionFor(phase v1.WorkloadPhase, queuePosition int, message string) pendingReasonAction {
	if phase != v1.WorkloadPending {
		return pendingReasonNone
	}
	if queuePosition >= 1 && strings.TrimSpace(message) != "" {
		return pendingReasonMessage
	}
	return pendingReasonCollect
}

// applyPendingReason runs one decision. collect returns the model prompt and the raw fallback sentence.
// summarize may be nil. A summary failure returns the fallback.
func applyPendingReason(action pendingReasonAction, message string, collect func() (string, string), summarize func(string) (string, error)) string {
	message = strings.TrimSpace(message)
	switch action {
	case pendingReasonMessage:
		return message
	case pendingReasonCollect:
		if collect == nil {
			return ""
		}
		evidence, fallback := collect()
		if message != "" {
			evidence = "scheduler message: " + message + "\n" + evidence
			fallback = message
		}
		if strings.TrimSpace(evidence) == "" {
			return ""
		}
		if strings.TrimSpace(fallback) == "" {
			fallback = firstLine(evidence)
		}
		return summaryOr(evidence, fallback, summarize)
	default:
		return ""
	}
}

// summaryOr asks summarize for one sentence and returns fallback when that fails.
func summaryOr(text, fallback string, summarize func(string) (string, error)) string {
	fallback = firstSentence(fallback)
	if summarize == nil {
		return fallback
	}
	summary, err := summarize(text)
	summary = firstSentence(summary)
	if err != nil || summary == "" {
		return fallback
	}
	return summary
}

// collectPendingEvidence gathers provisioning status, data-plane pod facts, and job-manager lines.
// The second result is the strongest single fact, used when the summary is unavailable.
func (h *Handler) collectPendingEvidence(ctx context.Context, w *v1.Workload) (string, string) {
	if w == nil {
		return "", ""
	}
	var lines []string
	var fallback string
	if line := provisioningLine(w); line != "" {
		lines = append(lines, line)
		fallback = line
	}
	podLines, podFallback := h.pendingPodEvidence(ctx, w)
	lines = append(lines, podLines...)
	fallback = preferFallback(fallback, podFallback)
	logLines := h.jobManagerLogLines(ctx, w.Name)
	for _, line := range logLines {
		lines = append(lines, "job-manager "+line)
	}
	if fallback == "" && len(logLines) > 0 {
		fallback = logLines[0]
	}
	if len(lines) == 0 {
		return "", ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Workload %s is Pending, queuePosition %d.\n", w.Name, w.Status.QueuePosition)
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return clipText(b.String(), pendingEvidenceLimit), firstSentence(fallback)
}

// provisioningLine returns the external placement condition, if the workload has one.
func provisioningLine(w *v1.Workload) string {
	if w == nil || w.Status.ExternalExecution == nil {
		return ""
	}
	msg := strings.TrimSpace(w.Status.ExternalExecution.ProvisioningCondition)
	if msg == "" {
		return ""
	}
	return clipText("provisioning "+msg, pendingEvidenceLineLimit)
}

// pendingPodEvidence reads Pending pods and their scheduling events from the data-plane cluster.
func (h *Handler) pendingPodEvidence(ctx context.Context, w *v1.Workload) ([]string, string) {
	clusterID := v1.GetClusterId(w)
	if h.clientManager == nil || clusterID == "" || w.Spec.Workspace == "" || w.Name == "" {
		return nil, ""
	}
	factory, err := commonutils.GetK8sClientFactory(h.clientManager, clusterID)
	if err != nil {
		klog.ErrorS(err, "failed to get data plane client", "cluster", clusterID, "workload", w.Name)
		return nil, ""
	}
	clientSet := factory.ClientSet()
	if clientSet == nil {
		return nil, ""
	}
	list, err := clientSet.CoreV1().Pods(w.Spec.Workspace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set{v1.WorkloadIdLabel: w.Name}.String(),
		Limit:         pendingEvidencePodListLimit,
	})
	if err != nil {
		klog.ErrorS(err, "failed to list workload pods", "workload", w.Name)
		return nil, ""
	}
	pending := make([]corev1.Pod, 0, len(list.Items))
	for i := range list.Items {
		if list.Items[i].Status.Phase == corev1.PodPending {
			pending = append(pending, list.Items[i])
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].Name < pending[j].Name })
	if len(pending) > pendingEvidencePodLimit {
		pending = pending[:pendingEvidencePodLimit]
	}
	var lines []string
	var fallback string
	for i := range pending {
		podLines, podFallback := describePendingPod(&pending[i])
		lines = append(lines, podLines...)
		fallback = preferFallback(fallback, podFallback)
		evLines, evFallback := podSchedulingEvents(ctx, clientSet, &pending[i])
		lines = append(lines, evLines...)
		fallback = preferFallback(fallback, evFallback)
	}
	return lines, fallback
}

// describePendingPod formats container waiting reasons and an unschedulable condition.
// The fallback prefers a container waiting reason over the scheduling condition.
func describePendingPod(pod *corev1.Pod) ([]string, string) {
	if pod == nil {
		return nil, ""
	}
	var lines []string
	var fallback string
	statuses := make([]corev1.ContainerStatus, 0, len(pod.Status.InitContainerStatuses)+len(pod.Status.ContainerStatuses))
	statuses = append(statuses, pod.Status.InitContainerStatuses...)
	statuses = append(statuses, pod.Status.ContainerStatuses...)
	for i := range statuses {
		waiting := statuses[i].State.Waiting
		if waiting == nil {
			continue
		}
		line := joinReason(fmt.Sprintf("pod/%s container %s waiting", pod.Name, statuses[i].Name),
			strings.TrimSpace(waiting.Reason), strings.TrimSpace(waiting.Message))
		lines = append(lines, line)
		if fallback == "" {
			fallback = line
		}
	}
	for i := range pod.Status.Conditions {
		cond := &pod.Status.Conditions[i]
		if cond.Type != corev1.PodScheduled || cond.Status == corev1.ConditionTrue {
			continue
		}
		line := joinReason(fmt.Sprintf("pod/%s unschedulable", pod.Name),
			strings.TrimSpace(cond.Reason), strings.TrimSpace(cond.Message))
		lines = append(lines, line)
		if fallback == "" {
			fallback = line
		}
	}
	return lines, fallback
}

// podSchedulingEvents returns the newest warning or scheduling events for one pod.
func podSchedulingEvents(ctx context.Context, clientSet kubernetes.Interface, pod *corev1.Pod) ([]string, string) {
	if clientSet == nil || pod == nil || pod.Name == "" {
		return nil, ""
	}
	selector := fields.AndSelectors(
		fields.OneTermEqualSelector("involvedObject.name", pod.Name),
		fields.OneTermEqualSelector("involvedObject.kind", "Pod"),
	).String()
	list, err := clientSet.CoreV1().Events(pod.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: selector,
		Limit:         pendingEvidenceEventList,
	})
	if err != nil || list == nil {
		if err != nil {
			klog.ErrorS(err, "failed to list pod events", "pod", pod.Name)
		}
		return nil, ""
	}
	events := make([]*corev1.Event, 0, len(list.Items))
	for i := range list.Items {
		ev := &list.Items[i]
		if ev.InvolvedObject.Name != pod.Name || ev.InvolvedObject.Kind != "Pod" {
			continue
		}
		if ev.InvolvedObject.UID != "" && pod.UID != "" && ev.InvolvedObject.UID != pod.UID {
			continue
		}
		if !isPendingSignalEvent(ev) {
			continue
		}
		events = append(events, ev)
	}
	sort.Slice(events, func(i, j int) bool {
		return eventLastTime(events[i]).After(eventLastTime(events[j]))
	})
	if len(events) > pendingEvidenceEventLimit {
		events = events[:pendingEvidenceEventLimit]
	}
	var lines []string
	var fallback string
	for _, ev := range events {
		line := joinReason("event", strings.TrimSpace(ev.Reason), strings.TrimSpace(ev.Message))
		lines = append(lines, line)
		if fallback == "" {
			fallback = line
		}
	}
	return lines, fallback
}

// isPendingSignalEvent reports whether an event can explain why a pod is still pending.
func isPendingSignalEvent(ev *corev1.Event) bool {
	if ev == nil {
		return false
	}
	if ev.Type == corev1.EventTypeWarning {
		return true
	}
	blob := strings.ToLower(ev.Reason + " " + ev.Message)
	for _, needle := range []string{
		"scale", "schedul", "quota", "unschedul", "image", "pull", "backoff", "preempt", "lease", "pool",
	} {
		if strings.Contains(blob, needle) {
			return true
		}
	}
	return false
}

// eventLastTime is the newest timestamp recorded on an event.
func eventLastTime(ev *corev1.Event) time.Time {
	if ev == nil {
		return time.Time{}
	}
	if !ev.LastTimestamp.IsZero() {
		return ev.LastTimestamp.Time
	}
	if !ev.EventTime.IsZero() {
		return ev.EventTime.Time
	}
	if !ev.FirstTimestamp.IsZero() {
		return ev.FirstTimestamp.Time
	}
	return ev.CreationTimestamp.Time
}

// jobManagerLogLines returns recent job-manager log lines that mention the workload.
func (h *Handler) jobManagerLogLines(ctx context.Context, workloadID string) []string {
	if h.clientSet == nil || workloadID == "" {
		return nil
	}
	deps, err := h.clientSet.AppsV1().Deployments(common.PrimusSafeNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set{jobManagerAppLabelKey: jobManagerAppLabelValue}.String(),
	})
	if err != nil {
		klog.ErrorS(err, "failed to list job-manager deployments", "workload", workloadID)
		return nil
	}
	var pods []corev1.Pod
	for i := range deps.Items {
		sel := deps.Items[i].Spec.Selector
		if sel == nil || len(sel.MatchLabels) == 0 {
			continue
		}
		list, err := h.clientSet.CoreV1().Pods(common.PrimusSafeNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: labels.Set(sel.MatchLabels).String(),
		})
		if err != nil {
			klog.ErrorS(err, "failed to list job-manager pods", "workload", workloadID)
			continue
		}
		pods = append(pods, list.Items...)
	}
	sort.Slice(pods, func(i, j int) bool {
		return pods[i].CreationTimestamp.Time.After(pods[j].CreationTimestamp.Time)
	})
	if len(pods) > pendingEvidenceLogPods {
		pods = pods[:pendingEvidenceLogPods]
	}
	var lines []string
	for i := range pods {
		if len(lines) >= pendingEvidenceLogLines {
			break
		}
		raw, err := fetchPodLogs(ctx, h.clientSet, pods[i].Namespace, pods[i].Name, pendingEvidenceLogTail)
		if err != nil {
			klog.ErrorS(err, "failed to read job-manager log", "pod", pods[i].Name, "workload", workloadID)
			continue
		}
		lines = append(lines, filterWorkloadLogLines(string(raw), workloadID, pendingEvidenceLogLines-len(lines))...)
	}
	return lines
}

// filterWorkloadLogLines keeps lines that mention the workload, up to limit.
func filterWorkloadLogLines(raw, workloadID string, limit int) []string {
	if limit <= 0 || workloadID == "" {
		return nil
	}
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, workloadID) {
			continue
		}
		out = append(out, clipText(line, pendingEvidenceLineLimit))
		if len(out) >= limit {
			break
		}
	}
	return out
}

// preferFallback keeps the more specific fact. A container wait outranks an
// unschedulable condition, which outranks an event, which outranks provisioning.
func preferFallback(current, next string) string {
	if next == "" {
		return current
	}
	if current == "" || evidenceRank(next) > evidenceRank(current) {
		return next
	}
	return current
}

// evidenceRank scores one collected fact. Higher is more specific.
func evidenceRank(line string) int {
	switch {
	case strings.Contains(line, " waiting "):
		return 4
	case strings.Contains(line, "unschedulable"):
		return 3
	case strings.HasPrefix(line, "event "):
		return 2
	case strings.HasPrefix(line, "provisioning "):
		return 1
	default:
		return 0
	}
}

// joinReason formats a prefix with an optional reason and message.
func joinReason(prefix, reason, message string) string {
	var line string
	switch {
	case reason != "" && message != "":
		line = prefix + " " + reason + ": " + message
	case reason != "":
		line = prefix + " " + reason
	case message != "":
		line = prefix + ": " + message
	default:
		line = prefix
	}
	return clipText(line, pendingEvidenceLineLimit)
}

// firstSentence collapses a model reply or a fact into one clipped line.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	return clipText(s, pendingReasonSentenceLimit)
}

// firstLine returns the first non-empty line.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

// clipText shortens s to at most n runes.
func clipText(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return strings.TrimSpace(string(runes[:n]))
}
