/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cicdlog

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/concurrent"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

type Scope struct {
	Cluster    string
	Kind       string
	Workspace  string
	Name       string
	RunnerID   string
	ScaleSetID string
}

type SearchOptions struct {
	Queries    []Query
	ErrorsOnly bool
	Observe    func(time.Time, *error)
}

func ScopeFromWorkload(workload *v1.Workload) Scope {
	return Scope{
		Cluster: v1.GetClusterId(workload), Kind: workload.SpecKind(), Workspace: workload.Spec.Workspace, Name: workload.Name,
		RunnerID: v1.GetLabel(workload, v1.CICDScaleRunnerIdLabel), ScaleSetID: v1.GetCICDRunnerScaleSetId(workload),
	}
}

func ARCControllerQueries(scope Scope, query Query) ([]Query, error) {
	query.TermFilters = map[string]string{"kubernetes.namespace_name": common.CICDArcNamespace}
	query.PrefixFilters = map[string]string{"kubernetes.pod_name": commonconfig.GetCICDControllerName()}
	query.NodeNames, query.PodNames = "", ""
	query.UseK8sLabel = false
	query.Keywords = []string{scope.Kind, scope.Workspace}
	switch scope.Kind {
	case common.CICDScaleRunnerSetKind:
		query.Keywords = append(query.Keywords, scope.Name)
		return []Query{query}, nil
	case common.CICDEphemeralRunnerKind:
		second := query
		query.Keywords = append(append([]string{}, query.Keywords...), scope.RunnerID)
		second.Keywords = append(append([]string{}, second.Keywords...), scope.ScaleSetID)
		return []Query{query, second}, nil
	default:
		return nil, commonerrors.NewBadRequest("the workload is not a CICD workload")
	}
}

func SearchARCControllerLogs(ctx context.Context, scope Scope, options SearchOptions) ([]commonsearch.OpenSearchLogResponse, error) {
	if !commonconfig.IsOpenSearchEnable() {
		return nil, fmt.Errorf("ARC log backend is disabled")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	searchClient := commonsearch.GetOpensearchClient(scope.Cluster)
	if searchClient == nil {
		return nil, commonerrors.NewInternalError("ARC log cluster client is unavailable")
	}
	indices := make(chan int, len(options.Queries))
	for i := range options.Queries {
		indices <- i
	}
	close(indices)
	result := make([]commonsearch.OpenSearchLogResponse, len(options.Queries))
	_, err := concurrent.Exec(len(options.Queries), func() error {
		i := <-indices
		query := options.Queries[i]
		body := BuildSearchBody(&query, scope.Name)
		if options.ErrorsOnly {
			body = buildARCErrorSearchBody(query, scope)
		}
		start := time.Now()
		raw, err := searchClient.SearchByTimeRangeContext(ctx, query.SinceTime, query.UntilTime, "", "/_search", body)
		if options.Observe != nil {
			options.Observe(start, &err)
		}
		if err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &result[i]); err != nil {
			return fmt.Errorf("ARC log response could not be decoded")
		}
		result[i].NormalizeMessages()
		return nil
	})
	return result, err
}

func buildARCErrorSearchBody(query Query, scope Scope) []byte {
	query.Order, query.Offset, query.Limit = "desc", 0, 20
	query.DisableOutput = true
	textQuery := query
	textQuery.Keywords = append(append([]string{}, query.Keywords...), scope.Name)
	query.Keywords = nil
	request := buildSearchRequest(&query, scope.Name)
	textMatch := &commonsearch.OpenSearchRequest{}
	BuildKeywords(textMatch, &textQuery)
	request.Query.Bool.Must = append(request.Query.Bool.Must, commonsearch.OpenSearchField{
		"bool": map[string]interface{}{"minimum_should_match": 1, "should": []interface{}{
			map[string]interface{}{"bool": map[string]interface{}{"must": textMatch.Query.Bool.Must}},
			arcObjectTerms("controllerKind", scope), arcObjectTerms("kind", scope),
			map[string]interface{}{"bool": map[string]interface{}{"filter": []interface{}{
				map[string]interface{}{"term": map[string]string{scope.Kind + ".name.keyword": scope.Name}},
				map[string]interface{}{"term": map[string]string{scope.Kind + ".namespace.keyword": scope.Workspace}},
			}}},
		}},
	}, commonsearch.OpenSearchField{
		"bool": map[string]interface{}{"minimum_should_match": 1, "should": []interface{}{
			KeywordMatchAnyField("error", 0), map[string]interface{}{"terms": map[string]interface{}{"level.keyword": []string{"error", "ERROR"}}},
			map[string]interface{}{"exists": map[string]string{"field": "error"}},
		}},
	})
	return jsonutils.MarshalSilently(request)
}

func arcObjectTerms(kindField string, scope Scope) map[string]interface{} {
	return map[string]interface{}{"bool": map[string]interface{}{"filter": []interface{}{
		map[string]interface{}{"term": map[string]string{kindField + ".keyword": scope.Kind}},
		map[string]interface{}{"term": map[string]string{"name.keyword": scope.Name}},
		map[string]interface{}{"term": map[string]string{"namespace.keyword": scope.Workspace}},
	}}}
}
