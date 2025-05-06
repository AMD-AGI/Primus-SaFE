/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package log

const (
	TimeField    = "@timestamp"
	MessageField = "message"
	StreamField  = "stream"
)

type OpenSearchField map[string]interface{}

type OpenSearchSpanNearQuery struct {
	Slop    int               `json:"slop"`
	InOrder bool              `json:"in_order,omitempty"`
	Clauses []OpenSearchField `json:"clauses,omitempty"`
}

type OpenSearchQuery struct {
	Bool struct {
		// The bool query supports AND (must), OR (should), and filter (must match) clauses
		Must   []OpenSearchField `json:"must,omitempty"`
		Filter []OpenSearchField `json:"filter,omitempty"`
	} `json:"bool,omitempty"`
}

type OpenSearchRequest struct {
	Source []string          `json:"_source"`
	Query  OpenSearchQuery   `json:"query"`
	Sort   []OpenSearchField `json:"sort,omitempty"`
	From   int               `json:"from"`
	Size   int               `json:"size"`
}

type OpenSearchScrollRequest struct {
	Scroll   string `json:"scroll,omitempty"`
	ScrollId string `json:"scroll_id,omitempty"`
}

type OpenSearchDoc struct {
	// Document unique identifier
	Id     string `json:"_id"`
	Source struct {
		Timestamp  string `json:"@timestamp"`
		Stream     string `json:"stream,omitempty"`
		Message    string `json:"message,omitempty"`
		Line       int    `json:"line,omitempty"`
		Kubernetes struct {
			PodName string `json:"pod_name,omitempty"`
			Labels  struct {
				DispatchCount string `json:"workload_dispatch_count,omitempty"`
				ReplicaIndex  string `json:"training_kubeflow_org/replica-index,omitempty"`
				ReplicaType   string `json:"training_kubeflow_org/replica-type,omitempty"`
				JobName       string `json:"training_kubeflow_org/job-name,omitempty"`
				WorkloadId    string `json:"workload_id,omitempty"`
			} `json:"labels,omitempty"`
			Host          string `json:"host,omitempty"`
			ContainerName string `json:"container_name,omitempty"`
		} `json:"kubernetes,omitempty"`
	} `json:"_source"`
}

type OpenSearchHits struct {
	Total struct {
		Value int `json:"value"`
	} `json:"total"`
	Hits []OpenSearchDoc `json:"hits"`
}

type OpenSearchResponse struct {
	// The query takes time, unit: milliseconds
	Took int64 `json:"took,omitempty"`
	// The total number of returned documents
	Hits     OpenSearchHits `json:"hits"`
	ScrollId string         `json:"_scroll_id,omitempty"`
}
