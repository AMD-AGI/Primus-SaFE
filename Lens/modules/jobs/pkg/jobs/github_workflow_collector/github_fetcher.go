package github_workflow_collector

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/github"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GithubFetcher fetches GitHub data (commits, workflow runs) and stores them
type GithubFetcher struct {
	clientSets    *clientsets.K8SClientSet
	clientManager *github.ClientManager
}

// NewGithubFetcher creates a new GithubFetcher
func NewGithubFetcher(clientSets *clientsets.K8SClientSet) *GithubFetcher {
	return &GithubFetcher{
		clientSets:    clientSets,
		clientManager: github.GetGlobalManager(),
	}
}

// FetchAndStoreGithubData fetches commit and workflow run details from GitHub and stores them
func (f *GithubFetcher) FetchAndStoreGithubData(ctx context.Context, config *model.GithubWorkflowConfigs, run *model.GithubWorkflowRuns) error {
	if f.clientManager == nil {
		log.Debugf("GithubFetcher: client manager not initialized, skipping GitHub data fetch")
		return nil
	}

	// Get runner set info to find the secret
	runnerSetInfo, err := f.getRunnerSetInfo(ctx, config.RunnerSetNamespace, config.RunnerSetName)
	if err != nil {
		log.Warnf("GithubFetcher: failed to get runner set info for %s/%s: %v", config.RunnerSetNamespace, config.RunnerSetName, err)
		return nil // Don't fail the job, just skip GitHub data
	}

	if runnerSetInfo == nil || runnerSetInfo.GithubConfigSecret == "" {
		log.Debugf("GithubFetcher: no GitHub secret configured for runner set %s/%s", config.RunnerSetNamespace, config.RunnerSetName)
		return nil
	}

	// Get GitHub client using the secret
	client, err := f.clientManager.GetClientForSecret(ctx, config.RunnerSetNamespace, runnerSetInfo.GithubConfigSecret)
	if err != nil {
		log.Warnf("GithubFetcher: failed to get GitHub client: %v", err)
		return nil
	}

	owner := config.GithubOwner
	repo := config.GithubRepo

	// If owner/repo not in config, try to get from runner set
	if owner == "" {
		owner = runnerSetInfo.GithubOwner
	}
	if repo == "" {
		repo = runnerSetInfo.GithubRepo
	}

	if owner == "" {
		log.Debugf("GithubFetcher: no GitHub owner configured, skipping")
		return nil
	}

	// Fetch commit details if we have a SHA
	if run.HeadSha != "" {
		if err := f.fetchAndStoreCommit(ctx, client, owner, repo, run); err != nil {
			log.Warnf("GithubFetcher: failed to fetch commit %s: %v", run.HeadSha, err)
		}
	}

	// Fetch workflow run details if we have a GitHub run ID
	if run.GithubRunID > 0 {
		if err := f.fetchAndStoreWorkflowRun(ctx, client, owner, repo, run); err != nil {
			log.Warnf("GithubFetcher: failed to fetch workflow run %d: %v", run.GithubRunID, err)
		}
	}

	return nil
}

// fetchAndStoreCommit fetches commit details from GitHub and stores them
func (f *GithubFetcher) fetchAndStoreCommit(ctx context.Context, client *github.Client, owner, repo string, run *model.GithubWorkflowRuns) error {
	// Check if we already have this commit
	commitFacade := database.GetFacade().GetGithubWorkflowCommit()
	existing, err := commitFacade.GetByRunID(ctx, run.ID)
	if err != nil {
		return err
	}
	if existing != nil {
		log.Debugf("GithubFetcher: commit for run %d already exists, skipping", run.ID)
		return nil
	}

	// Fetch from GitHub
	commitInfo, err := client.GetCommit(ctx, owner, repo, run.HeadSha)
	if err != nil {
		return err
	}

	// Convert to model
	parentSHAs, _ := json.Marshal(commitInfo.Parents)
	
	// Limit files to avoid storing too much data
	files := commitInfo.Files
	if len(files) > 100 {
		files = files[:100] // Only store first 100 files
	}
	filesJSON, _ := json.Marshal(files)

	commit := &model.GithubWorkflowCommits{
		RunID:   run.ID,
		SHA:     commitInfo.SHA,
		Message: commitInfo.Message,
		HTMLURL: commitInfo.HTMLURL,
		ParentSHAs: model.ExtJSON(parentSHAs),
		Files:      model.ExtJSON(filesJSON),
	}

	if commitInfo.Author != nil {
		commit.AuthorName = commitInfo.Author.Name
		commit.AuthorEmail = commitInfo.Author.Email
		commit.AuthorDate = commitInfo.Author.Date
	}

	if commitInfo.Committer != nil {
		commit.CommitterName = commitInfo.Committer.Name
		commit.CommitterEmail = commitInfo.Committer.Email
		commit.CommitterDate = commitInfo.Committer.Date
	}

	if commitInfo.Stats != nil {
		commit.Additions = commitInfo.Stats.Additions
		commit.Deletions = commitInfo.Stats.Deletions
		commit.FilesChanged = len(commitInfo.Files)
	}

	if err := commitFacade.Upsert(ctx, commit); err != nil {
		return err
	}

	log.Debugf("GithubFetcher: stored commit %s for run %d", commit.SHA, run.ID)
	return nil
}

// fetchAndStoreWorkflowRun fetches workflow run details from GitHub and stores them
func (f *GithubFetcher) fetchAndStoreWorkflowRun(ctx context.Context, client *github.Client, owner, repo string, run *model.GithubWorkflowRuns) error {
	// Check if we already have this workflow run
	detailsFacade := database.GetFacade().GetGithubWorkflowRunDetails()
	existing, err := detailsFacade.GetByRunID(ctx, run.ID)
	if err != nil {
		return err
	}
	if existing != nil {
		log.Debugf("GithubFetcher: workflow run details for run %d already exists, skipping", run.ID)
		return nil
	}

	// Fetch from GitHub (with jobs)
	runInfo, err := client.GetWorkflowRunWithJobs(ctx, owner, repo, run.GithubRunID)
	if err != nil {
		return err
	}

	// Convert jobs to JSON
	jobsJSON, _ := json.Marshal(runInfo.Jobs)

	details := &model.GithubWorkflowRunDetails{
		RunID:                  run.ID,
		GithubRunID:            runInfo.ID,
		GithubRunNumber:        runInfo.RunNumber,
		GithubRunAttempt:       runInfo.RunAttempt,
		WorkflowID:             runInfo.WorkflowID,
		WorkflowName:           runInfo.WorkflowName,
		WorkflowPath:           runInfo.WorkflowPath,
		Status:                 runInfo.Status,
		Conclusion:             runInfo.Conclusion,
		HTMLURL:                runInfo.HTMLURL,
		JobsURL:                runInfo.JobsURL,
		LogsURL:                runInfo.LogsURL,
		ArtifactsURL:           runInfo.ArtifactsURL,
		CreatedAtGithub:        runInfo.CreatedAt,
		UpdatedAtGithub:        runInfo.UpdatedAt,
		DurationSeconds:        runInfo.DurationSeconds,
		Event:                  runInfo.Event,
		HeadSHA:                runInfo.HeadSHA,
		HeadBranch:             runInfo.HeadBranch,
		HeadRepositoryFullName: runInfo.HeadRepository,
		BaseSHA:                runInfo.BaseSHA,
		BaseBranch:             runInfo.BaseBranch,
		Jobs:                   model.ExtJSON(jobsJSON),
	}

	if runInfo.RunStartedAt != nil {
		details.RunStartedAt = *runInfo.RunStartedAt
	}
	if runInfo.RunCompletedAt != nil {
		details.RunCompletedAt = *runInfo.RunCompletedAt
	}

	if runInfo.TriggerActor != nil {
		details.TriggerActor = runInfo.TriggerActor.Login
		details.TriggerActorID = runInfo.TriggerActor.ID
	}

	if runInfo.PullRequest != nil {
		details.PullRequestNumber = runInfo.PullRequest.Number
		details.PullRequestTitle = runInfo.PullRequest.Title
		details.PullRequestURL = runInfo.PullRequest.URL
	}

	if err := detailsFacade.Upsert(ctx, details); err != nil {
		return err
	}

	log.Debugf("GithubFetcher: stored workflow run details for run %d (GitHub run %d)", run.ID, runInfo.ID)
	return nil
}

// InitGithubClientManager initializes the global GitHub client manager
func InitGithubClientManager(clientSets *clientsets.K8SClientSet) {
	if clientSets.Clientsets == nil {
		log.Warnf("GithubFetcher: k8s clientset not initialized for GitHub client manager")
		return
	}
	github.InitGlobalManager(clientSets.Clientsets)
	log.Info("GithubFetcher: initialized global GitHub client manager")
}

// RunnerSetInfo holds basic info about a runner set
type RunnerSetInfo struct {
	UID                string
	Name               string
	Namespace          string
	GithubConfigSecret string
	GithubOwner        string
	GithubRepo         string
}

// AutoScalingRunnerSet GVR
var autoScalingRunnerSetGVR = schema.GroupVersionResource{
	Group:    "actions.github.com",
	Version:  "v1alpha1",
	Resource: "autoscalingrunnersets",
}

// getRunnerSetInfo returns basic info about a runner set by querying the K8s API
func (f *GithubFetcher) getRunnerSetInfo(ctx context.Context, namespace, name string) (*RunnerSetInfo, error) {
	if f.clientSets.Dynamic == nil {
		return nil, nil
	}

	obj, err := f.clientSets.Dynamic.Resource(autoScalingRunnerSetGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	info := &RunnerSetInfo{
		UID:       string(obj.GetUID()),
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		return info, nil
	}

	if secret, ok, _ := unstructured.NestedString(spec, "githubConfigSecret"); ok {
		info.GithubConfigSecret = secret
	}

	if url, ok, _ := unstructured.NestedString(spec, "githubConfigUrl"); ok {
		info.GithubOwner, info.GithubRepo = parseGitHubURL(url)
	}

	return info, nil
}

// parseGitHubURL parses a GitHub URL and extracts owner and repo
func parseGitHubURL(url string) (owner, repo string) {
	url = strings.TrimSuffix(url, "/")
	parts := strings.Split(url, "/")

	if len(parts) < 4 {
		return "", ""
	}

	ghIndex := -1
	for i, part := range parts {
		if strings.Contains(part, "github.com") {
			ghIndex = i
			break
		}
	}

	if ghIndex < 0 || ghIndex+1 >= len(parts) {
		return "", ""
	}

	owner = parts[ghIndex+1]
	if ghIndex+2 < len(parts) {
		repo = parts[ghIndex+2]
	}

	return owner, repo
}

