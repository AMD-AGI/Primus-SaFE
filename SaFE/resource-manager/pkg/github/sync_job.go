/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"context"
	"encoding/json"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// SyncJob fetches GitHub API data for unsynced workflow runs and stores it in SaFE DB.
// It runs periodically in job-manager.
type SyncJob struct {
	store       *Store
	k8sClients  map[string]kubernetes.Interface
	batchSize   int
	interval    time.Duration
}

func NewSyncJob(store *Store, batchSize int, interval time.Duration) *SyncJob {
	if batchSize <= 0 {
		batchSize = 20
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &SyncJob{
		store:      store,
		k8sClients: make(map[string]kubernetes.Interface),
		batchSize:  batchSize,
		interval:   interval,
	}
}

func (j *SyncJob) RegisterK8sClient(cluster string, client kubernetes.Interface) {
	j.k8sClients[cluster] = client
}

func (j *SyncJob) Start(ctx context.Context) {
	klog.Info("[github-sync] starting sync job")
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			klog.Info("[github-sync] stopped")
			return
		case <-ticker.C:
			j.syncBatch(ctx)
		}
	}
}

func (j *SyncJob) syncBatch(ctx context.Context) {
	runs, err := j.store.GetUnsyncedRuns(ctx, j.batchSize)
	if err != nil {
		klog.V(1).Infof("[github-sync] get unsynced runs: %v", err)
		return
	}
	if len(runs) == 0 {
		return
	}

	klog.V(2).Infof("[github-sync] processing %d unsynced runs", len(runs))

	for _, run := range runs {
		if run.GithubOwner == "" || run.GithubRepo == "" {
			j.store.MarkSynced(ctx, run.ID)
			continue
		}

		token := j.getGitHubToken(ctx, run.Cluster, run.GithubOwner, run.GithubRepo)
		client := NewGitHubClient(token)

		synced := true

		if err := j.syncRunDetails(ctx, client, &run); err != nil {
			klog.V(1).Infof("[github-sync] run %d details: %v", run.GithubRunID, err)
			synced = false
		}

		if run.HeadSHA != "" {
			if err := j.syncCommit(ctx, client, &run); err != nil {
				klog.V(1).Infof("[github-sync] run %d commit: %v", run.GithubRunID, err)
			}
		}

		if run.Status == "completed" {
			if err := j.syncJobs(ctx, client, &run); err != nil {
				klog.V(1).Infof("[github-sync] run %d jobs: %v", run.GithubRunID, err)
				synced = false
			}
		} else {
			synced = false
		}

		if synced {
			j.store.MarkSynced(ctx, run.ID)
			klog.V(2).Infof("[github-sync] synced run %d (workflow=%s)", run.GithubRunID, run.WorkflowName)
		}
	}
}

func (j *SyncJob) syncRunDetails(ctx context.Context, client *GitHubClient, run *WorkflowRunRecord) error {
	ghRun, err := client.GetWorkflowRun(ctx, run.GithubOwner, run.GithubRepo, run.GithubRunID)
	if err != nil {
		return err
	}

	prNumber := 0
	if len(ghRun.PullRequests) > 0 {
		prNumber = ghRun.PullRequests[0].Number
	}

	rawData, _ := json.Marshal(ghRun)

	return j.store.UpsertRunDetails(ctx, int(run.ID), run.GithubRunID,
		ghRun.HTMLURL, ghRun.JobsURL, ghRun.LogsURL,
		ghRun.Event, ghRun.TriggeringActor.Login, prNumber, ghRun.Path, rawData)
}

func (j *SyncJob) syncCommit(ctx context.Context, client *GitHubClient, run *WorkflowRunRecord) error {
	commit, err := client.GetCommit(ctx, run.GithubOwner, run.GithubRepo, run.HeadSHA)
	if err != nil {
		return err
	}

	return j.store.UpsertCommit(ctx,
		commit.SHA, run.GithubOwner, run.GithubRepo,
		commit.Commit.Message, commit.Commit.Author.Name, commit.Commit.Author.Email,
		commit.Commit.Author.Date,
		commit.Stats.Additions, commit.Stats.Deletions, len(commit.Files))
}

func (j *SyncJob) syncJobs(ctx context.Context, client *GitHubClient, run *WorkflowRunRecord) error {
	jobs, err := client.GetAllJobs(ctx, run.GithubOwner, run.GithubRepo, run.GithubRunID)
	if err != nil {
		return err
	}

	for _, job := range jobs {
		if err := j.store.UpsertJob(ctx, int(run.ID), job.ID,
			job.Name, job.Status, job.Conclusion,
			job.RunnerName, job.RunnerGroupName,
			job.StartedAt, job.CompletedAt, nil); err != nil {
			klog.V(1).Infof("[github-sync] upsert job %d: %v", job.ID, err)
			continue
		}

		for _, step := range job.Steps {
			var durSec int
			if step.StartedAt != nil && step.CompletedAt != nil {
				durSec = int(step.CompletedAt.Sub(*step.StartedAt).Seconds())
			}
			j.store.UpsertStep(ctx, int(job.ID), step.Number,
				step.Name, step.Status, step.Conclusion,
				step.StartedAt, step.CompletedAt, durSec)
		}
	}

	return nil
}

// getGitHubToken retrieves the GitHub token from K8s Secret in the target cluster.
func (j *SyncJob) getGitHubToken(ctx context.Context, cluster, owner, repo string) string {
	k8sClient, ok := j.k8sClients[cluster]
	if !ok {
		return ""
	}

	secrets, err := k8sClient.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return ""
	}

	for _, secret := range secrets.Items {
		if secret.Type != corev1.SecretTypeOpaque {
			continue
		}
		for _, key := range []string{"github_token", "token", "GITHUB_TOKEN"} {
			if v, ok := secret.Data[key]; ok {
				return string(v)
			}
		}
	}
	return ""
}
