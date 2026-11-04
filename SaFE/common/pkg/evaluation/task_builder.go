/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package evaluation

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	commonworkspace "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workspace"
)

const (
	ServiceTypeRemoteAPI     = "remote_api"
	ServiceTypeLocalWorkload = "local_workload"

	// DefaultTimeoutSecond is the timeout of an evaluation job that sets none.
	DefaultTimeoutSecond = 7200
	defaultConcurrency   = 32
)

// Benchmark is one benchmark of an evaluation task.
type Benchmark struct {
	DatasetId       string `json:"datasetId"`
	DatasetName     string `json:"datasetName,omitempty"`
	DatasetLocalDir string `json:"datasetLocalDir,omitempty"`
	EvalType        string `json:"evalType,omitempty"`
	Limit           *int   `json:"limit,omitempty"`
}

// TaskRequest is what an evaluation task is built from.
type TaskRequest struct {
	Name        string
	ServiceID   string
	ServiceType string
	// Benchmarks is the JSON array of Benchmark; each needs a datasetId.
	Benchmarks string
	Workspace  string
	// Judge is the optional JSON {"serviceId", "serviceType"} of the LLM-as-Judge model.
	Judge string
	// Concurrency defaults to 32 when it is not positive.
	Concurrency   int
	TimeoutSecond int
	UserID        string
	UserName      string
	// PostTrainingRunID and ModelID link the task to a Post Training run and the evaluated model.
	PostTrainingRunID string
	ModelID           string
}

// Task is a built evaluation task: its evaluation_task row is stored, and Inputs are the parameters of its OpsJob.
type Task struct {
	TaskID    string
	Cluster   string
	Workspace string
	Inputs    []v1.Parameter
}

// SecretGetter reads an admin-plane Secret by name.
type SecretGetter func(ctx context.Context, name string) (*corev1.Secret, error)

// judgeConfig is the judge model of an LLM-as-Judge evaluation.
type judgeConfig struct {
	ServiceId   string `json:"serviceId"`   // Model ID or Workload ID
	ServiceType string `json:"serviceType"` // "remote_api" or "local_workload"
}

// BuildTask validates req, resolves the evaluated service, the judge and the benchmark datasets, stores the
// evaluation_task row and returns the inputs of its OpsJob.
func BuildTask(ctx context.Context, cli client.Client, db dbclient.Interface, getSecret SecretGetter,
	req TaskRequest) (*Task, error) {
	concurrency := req.Concurrency
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}
	if req.ServiceID == "" {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("%s is required", v1.ParameterEvalServiceId))
	}
	if req.ServiceType == "" {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("%s is required", v1.ParameterEvalServiceType))
	}
	if req.Benchmarks == "" {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("%s is required", v1.ParameterEvalBenchmarks))
	}
	serviceId, serviceType, workspaceId := req.ServiceID, req.ServiceType, req.Workspace

	// Parse and enrich benchmarks with dataset info
	var benchmarks []Benchmark
	if err := json.Unmarshal([]byte(req.Benchmarks), &benchmarks); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid benchmarks JSON: %v", err))
	}
	for i, b := range benchmarks {
		if b.DatasetId == "" {
			return nil, commonerrors.NewBadRequest("datasetId is required in benchmarks")
		}
		dataset, err := db.GetDataset(ctx, b.DatasetId)
		if err != nil {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("dataset not found: %s", b.DatasetId))
		}
		if dataset.DatasetType != "evaluation" {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("dataset %s is not an evaluation type dataset", b.DatasetId))
		}
		benchmarks[i].DatasetName = dataset.DisplayName
		// DatasetLocalDir will be set after we know the workspace
	}

	taskId := fmt.Sprintf("eval-task-%s", uuid.New().String()[:8])

	// Get service info and construct endpoint
	var serviceName, modelEndpoint, modelName, modelAPIKey, clusterId string
	if serviceType == ServiceTypeRemoteAPI {
		dbModel, err := db.GetModelByID(ctx, serviceId)
		if err != nil {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("model not found: %s", serviceId))
		}
		serviceName = dbModel.DisplayName
		modelEndpoint = dbModel.SourceURL
		modelAPIKey = modelAPIKeyOf(ctx, cli, getSecret, serviceId)

		// Use ModelName if available, otherwise fallback to DisplayName
		if dbModel.ModelName != "" {
			modelName = dbModel.ModelName
		} else {
			modelName = dbModel.DisplayName
		}

		// The evaluation workload needs to run in a cluster that has the dataset
		if len(benchmarks) > 0 && workspaceId == "" {
			dataset, err := db.GetDataset(ctx, benchmarks[0].DatasetId)
			if err == nil && dataset != nil && dataset.LocalPaths != "" {
				var localPaths []dbclient.DatasetLocalPathDB
				if err := json.Unmarshal([]byte(dataset.LocalPaths), &localPaths); err == nil {
					for _, lp := range localPaths {
						if lp.Status == dbclient.DatasetStatusReady && lp.Workspace != "" {
							workspaceId = lp.Workspace
							klog.InfoS("Got workspace from dataset localPaths for remote_api evaluation",
								"datasetId", benchmarks[0].DatasetId, "workspace", workspaceId)
							break
						}
					}
				}
			}
		}
		if workspaceId != "" && clusterId == "" {
			ws := &v1.Workspace{}
			if err := cli.Get(ctx, client.ObjectKey{Name: workspaceId}, ws); err == nil {
				clusterId = ws.Spec.Cluster
				klog.InfoS("Got clusterId from workspace for remote_api evaluation",
					"workspace", workspaceId, "cluster", clusterId)
			}
		}
	} else {
		workload, err := db.GetWorkload(ctx, serviceId)
		if err != nil {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("workload not found: %s", serviceId))
		}
		serviceName = workload.DisplayName
		clusterId = workload.Cluster

		modelEndpoint = fmt.Sprintf("http://%s:8000/v1", workload.WorkloadId)

		// Parse real model name: priority is --served-model-name from entryPoint > env > displayName
		modelName = servedModelName(workload.EntryPoint, workload.EntryPoints)
		if modelName == "" {
			modelName = modelNameFromEnv(workload.Env)
		}
		if modelName == "" {
			modelName = workload.DisplayName
		}
		if workspaceId == "" {
			workspaceId = workload.Workspace
		}
	}

	// Set DatasetLocalDir for benchmarks based on workspace volume path
	volumeMountPath := "/wekafs"
	if workspaceId != "" {
		ws := &v1.Workspace{}
		if err := cli.Get(ctx, client.ObjectKey{Name: workspaceId}, ws); err == nil {
			if path := commonworkspace.GetNfsPathFromWorkspace(ws); path != "" {
				volumeMountPath = path
			}
		}
	}
	for i := range benchmarks {
		benchmarks[i].DatasetLocalDir = fmt.Sprintf("%s/datasets/%s", volumeMountPath, benchmarks[i].DatasetName)
	}
	enriched, err := json.Marshal(benchmarks)
	if err != nil {
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to serialize benchmarks: %v", err))
	}
	benchmarksJSON := string(enriched)

	// Parse judge config to get judge service info for task record
	var judge judgeConfig
	var judgeServiceName string
	if req.Judge != "" {
		if err := json.Unmarshal([]byte(req.Judge), &judge); err != nil || judge.ServiceId == "" {
			judge = judgeConfig{}
		}
		if judge.ServiceType == ServiceTypeRemoteAPI {
			if judgeDbModel, err := db.GetModelByID(ctx, judge.ServiceId); err == nil && judgeDbModel != nil {
				judgeServiceName = judgeDbModel.DisplayName
			}
		} else if judge.ServiceType == ServiceTypeLocalWorkload {
			if judgeWorkload, err := db.GetWorkload(ctx, judge.ServiceId); err == nil && judgeWorkload != nil {
				judgeServiceName = judgeWorkload.DisplayName
			}
		}
	}

	task := &dbclient.EvaluationTask{
		TaskId:            taskId,
		TaskName:          req.Name,
		ServiceId:         serviceId,
		ServiceType:       serviceType,
		ServiceName:       serviceName,
		Benchmarks:        benchmarksJSON,
		EvalParams:        "{}", // EvalParams is deprecated, always set to empty JSON
		Status:            dbclient.EvaluationTaskStatusPending,
		JudgeServiceId:    sql.NullString{String: judge.ServiceId, Valid: judge.ServiceId != ""},
		JudgeServiceType:  sql.NullString{String: judge.ServiceType, Valid: judge.ServiceType != ""},
		JudgeServiceName:  sql.NullString{String: judgeServiceName, Valid: judgeServiceName != ""},
		Timeout:           req.TimeoutSecond,
		Concurrency:       concurrency,
		Workspace:         workspaceId,
		UserId:            req.UserID,
		UserName:          req.UserName,
		CreationTime:      pq.NullTime{Time: time.Now().UTC(), Valid: true},
		PostTrainingRunId: sql.NullString{String: req.PostTrainingRunID, Valid: req.PostTrainingRunID != ""},
		ModelId:           sql.NullString{String: req.ModelID, Valid: req.ModelID != ""},
	}
	if err := db.UpsertEvaluationTask(ctx, task); err != nil {
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to create evaluation task: %v", err))
	}

	inputs := []v1.Parameter{
		{Name: v1.ParameterEvalTaskId, Value: taskId},
		{Name: v1.ParameterEvalServiceId, Value: serviceId},
		{Name: v1.ParameterEvalServiceType, Value: serviceType},
		{Name: v1.ParameterEvalBenchmarks, Value: benchmarksJSON},
		{Name: v1.ParameterModelEndpoint, Value: modelEndpoint},
		{Name: v1.ParameterModelName, Value: modelName},
	}
	if modelAPIKey != "" {
		inputs = append(inputs, v1.Parameter{Name: v1.ParameterModelApiKey, Value: modelAPIKey})
	}
	if clusterId != "" {
		inputs = append(inputs, v1.Parameter{Name: v1.ParameterCluster, Value: clusterId})
	}
	if workspaceId != "" {
		inputs = append(inputs, v1.Parameter{Name: v1.ParameterWorkspace, Value: workspaceId})
	}
	if judge.ServiceId != "" {
		inputs = append(inputs, judgeInputs(ctx, cli, db, getSecret, judge)...)
	}
	inputs = append(inputs, v1.Parameter{Name: v1.ParameterEvalConcurrency, Value: fmt.Sprint(concurrency)})
	return &Task{TaskID: taskId, Cluster: clusterId, Workspace: workspaceId, Inputs: inputs}, nil
}

// judgeInputs resolves the judge model's name, endpoint and API key into OpsJob inputs.
func judgeInputs(ctx context.Context, cli client.Client, db dbclient.Interface, getSecret SecretGetter,
	judge judgeConfig) []v1.Parameter {
	var judgeModelName, judgeEndpoint, judgeAPIKey string
	if judge.ServiceType == ServiceTypeRemoteAPI {
		judgeDbModel, err := db.GetModelByID(ctx, judge.ServiceId)
		if err != nil {
			klog.ErrorS(err, "failed to get judge model from database", "serviceId", judge.ServiceId)
		} else if judgeDbModel != nil {
			judgeModelName = judgeDbModel.ModelName
			judgeEndpoint = judgeDbModel.SourceURL
			judgeAPIKey = modelAPIKeyOf(ctx, cli, getSecret, judgeDbModel.ID)
		}
	} else if judge.ServiceType == ServiceTypeLocalWorkload {
		judgeWorkload, err := db.GetWorkload(ctx, judge.ServiceId)
		if err != nil {
			klog.ErrorS(err, "failed to get judge workload from database", "serviceId", judge.ServiceId)
		} else if judgeWorkload != nil {
			k8sWorkload := &v1.Workload{}
			if err := cli.Get(ctx, client.ObjectKey{
				Name:      judgeWorkload.WorkloadId,
				Namespace: judgeWorkload.Workspace,
			}, k8sWorkload); err != nil {
				klog.ErrorS(err, "failed to get K8s Workload for judge", "workloadId", judgeWorkload.WorkloadId)
			} else {
				judgeEndpoint = fmt.Sprintf("http://%s.%s.svc.cluster.local:8000/v1",
					commonworkload.GetK8sServiceName(k8sWorkload), judgeWorkload.Workspace)
				entryPoint := ""
				if len(k8sWorkload.Spec.EntryPoints) > 0 {
					entryPoint = k8sWorkload.Spec.EntryPoints[0]
				}
				judgeModelName = servedModelName(entryPoint, judgeWorkload.EntryPoints)
				if judgeModelName == "" {
					judgeModelName = judgeWorkload.DisplayName
				}
			}
		}
	}
	var inputs []v1.Parameter
	if judgeModelName != "" {
		inputs = append(inputs, v1.Parameter{Name: v1.ParameterJudgeModel, Value: judgeModelName})
	}
	if judgeEndpoint != "" {
		inputs = append(inputs, v1.Parameter{Name: v1.ParameterJudgeEndpoint, Value: judgeEndpoint})
	}
	if judgeAPIKey != "" {
		inputs = append(inputs, v1.Parameter{Name: v1.ParameterJudgeApiKey, Value: judgeAPIKey})
	}
	return inputs
}

// modelAPIKeyOf reads the API key of a remote_api model from the Secret its Model CR references, or "".
func modelAPIKeyOf(ctx context.Context, cli client.Client, getSecret SecretGetter, modelId string) string {
	k8sModel := &v1.Model{}
	if err := cli.Get(ctx, client.ObjectKey{Name: modelId}, k8sModel); err != nil {
		klog.ErrorS(err, "failed to get K8s Model CR", "modelId", modelId)
		return ""
	}
	if k8sModel.Spec.Source.ApiKey == nil || k8sModel.Spec.Source.ApiKey.Name == "" {
		return ""
	}
	secret, err := getSecret(ctx, k8sModel.Spec.Source.ApiKey.Name)
	if err != nil {
		klog.ErrorS(err, "failed to get secret for model API key", "secretName", k8sModel.Spec.Source.ApiKey.Name,
			"modelId", modelId)
		return ""
	}
	for _, key := range []string{"api-key", "apiKey", "token", "api_key"} {
		if val, ok := secret.Data[key]; ok && len(val) > 0 {
			return string(val)
		}
	}
	return ""
}

// servedModelName parses the --served-model-name parameter from entryPoint or entryPoints.
// This is the actual model name registered in vLLM/inference service.
func servedModelName(entryPoint string, entryPoints sql.NullString) string {
	if entryPoint != "" {
		if decoded, err := base64.StdEncoding.DecodeString(entryPoint); err == nil {
			if name := ServedModelNameFromCmd(string(decoded)); name != "" {
				return name
			}
		}
	}
	if entryPoints.Valid && entryPoints.String != "" {
		var points []string
		if err := json.Unmarshal([]byte(entryPoints.String), &points); err == nil {
			for _, ep := range points {
				if decoded, err := base64.StdEncoding.DecodeString(ep); err == nil {
					if name := ServedModelNameFromCmd(string(decoded)); name != "" {
						return name
					}
				}
			}
		}
	}
	return ""
}

// ServedModelNameFromCmd extracts the --served-model-name value from a command string.
func ServedModelNameFromCmd(cmd string) string {
	const flag = "--served-model-name"
	idx := strings.Index(cmd, flag)
	if idx == -1 {
		return ""
	}
	rest := strings.TrimLeft(cmd[idx+len(flag):], " =")
	if parts := strings.Fields(rest); len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// modelNameFromEnv parses the real model name from workload env JSON.
// It looks for PRIMUS_SOURCE_MODEL, MODEL_NAME or SERVED_MODEL_NAME, in that order.
func modelNameFromEnv(env sql.NullString) string {
	if !env.Valid || env.String == "" {
		return ""
	}
	var envMap map[string]string
	if err := json.Unmarshal([]byte(env.String), &envMap); err != nil {
		return ""
	}
	for _, key := range []string{"PRIMUS_SOURCE_MODEL", "MODEL_NAME", "SERVED_MODEL_NAME"} {
		if modelName := envMap[key]; modelName != "" {
			return modelName
		}
	}
	return ""
}
