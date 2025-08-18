/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

type CreateNodeTemplateRequest struct {
	Name           string   `json:"name"`
	AddOnTemplates []string `json:"addOnTemplates"`
}

type CreateNodeTemplateResponse struct {
	Id string `json:"id"`
}

type ListNodeTemplateResponse struct {
	TotalCount int                        `json:"totalCount"`
	Items      []NodeTemplateResponseItem `json:"items"`
}

type NodeTemplateResponseItem struct {
	TemplateId     string   `json:"templateId"`
	AddOnTemplates []string `json:"addOnTemplates"`
}
