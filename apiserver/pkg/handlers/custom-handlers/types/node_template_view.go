/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

type CreateNodeTemplateRequest struct {
	// Used to generate the node template id, which will do normalization processing, such as lowercase
	Name string `json:"name"`
	// List of addon-template id
	AddOnTemplates []string `json:"addOnTemplates"`
}

type CreateNodeTemplateResponse struct {
	// The NodeTemplate id
	Id string `json:"id"`
}

type ListNodeTemplateResponse struct {
	// The total number of node templates, not limited by pagination
	TotalCount int                        `json:"totalCount"`
	Items      []NodeTemplateResponseItem `json:"items"`
}

type NodeTemplateResponseItem struct {
	// The NodeTemplate id
	TemplateId string `json:"templateId"`
	// List of addon-template id
	AddOnTemplates []string `json:"addOnTemplates"`
}
