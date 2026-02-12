/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package opensearch

const (
	TimeField       = "@timestamp"
	MessageField    = "message"
	StreamField     = "stream"
	MaxDocsPerQuery = 10000
)

type OpenSearchField map[string]interface{}

type OpenSearchSpanNearQuery struct {
	Slop    int               `json:"slop"`
	InOrder bool              `json:"in_order,omitempty"`
	Clauses []OpenSearchField `json:"clauses,omitempty"`
}

type OpenSearchQuery struct {
	Bool struct {
		// and search
		Must   []OpenSearchField `json:"must,omitempty"`
		Filter []OpenSearchField `json:"filter,omitempty"`
	} `json:"bool,omitempty"`
}

type OpenSearchRequest struct {
	// Specify the fields to return
	Source []string          `json:"_source,omitempty"`
	Query  OpenSearchQuery   `json:"query"`
	Sort   []OpenSearchField `json:"sort,omitempty"`
	From   int               `json:"from"`
	Size   int               `json:"size"`
}

type OpenSearchScrollRequest struct {
	Scroll   string `json:"scroll,omitempty"`
	ScrollId string `json:"scroll_id,omitempty"`
}

type OpenSearchLogDoc struct {
	// unique document id
	Id     string `json:"_id"`
	Source struct {
		Timestamp string `json:"@timestamp"`
		Stream    string `json:"stream,omitempty"`
		Message   string `json:"message,omitempty"`
		// for context search
		Line       int `json:"line,omitempty"`
		Kubernetes struct {
			PodName string `json:"pod_name,omitempty"`
			Labels  struct {
				DispatchCount string `json:"primus-safe_workload_dispatch_count,omitempty"`
				ReplicaIndex  string `json:"training_kubeflow_org/replica-index,omitempty"`
				ReplicaType   string `json:"training_kubeflow_org/replica-type,omitempty"`
				JobName       string `json:"training_kubeflow_org/job-name,omitempty"`
				WorkloadId    string `json:"primus-safe_workload_id,omitempty"`
			} `json:"labels,omitempty"`
			Host          string `json:"host,omitempty"`
			ContainerName string `json:"container_name,omitempty"`
		} `json:"kubernetes,omitempty"`
	} `json:"_source"`
}

type OpenSearchLogHits struct {
	Total struct {
		Value int `json:"value"`
	} `json:"total"`
	Hits []OpenSearchLogDoc `json:"hits"`
}

type OpenSearchLogResponse struct {
	// Query duration, in milliseconds
	Took int64 `json:"took,omitempty"`
	// Total number of returned documents
	Hits     OpenSearchLogHits `json:"hits"`
	ScrollId string            `json:"_scroll_id,omitempty"`
}

type OpenSearchEventDoc struct {
	// unique document id
	Id     string `json:"_id"`
	Source struct {
		Timestamp          string `json:"@timestamp"`
		Reason             string `json:"reason,omitempty"`
		Type               string `json:"type,omitempty"`
		Message            string `json:"message,omitempty"`
		ReportingComponent string `json:"reportingComponent,omitempty"`
		InvolvedObject     struct {
			Kind      string `json:"kind"`
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"involvedObject"`
	} `json:"_source"`
}

type OpenSearchEventHits struct {
	Total struct {
		Value int `json:"value"`
	} `json:"total"`
	Hits []OpenSearchEventDoc `json:"hits"`
}

type OpenSearchEventResponse struct {
	// Query duration, in milliseconds
	Took int64 `json:"took,omitempty"`
	// Total number of returned documents
	Hits OpenSearchEventHits `json:"hits"`
}
