/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// CreateInferenceRequest represents the request to create an inference service.
type CreateInferenceRequest struct {
	DisplayName string               `json:"displayName" binding:"required"`
	Description string               `json:"description"`
	ModelForm   string               `json:"modelForm" binding:"required,oneof=API ModelSquare"`
	ModelName   string               `json:"modelName" binding:"required"`
	Instance    v1.InferenceInstance `json:"instance"`
	Resource    v1.InferenceResource `json:"resource" binding:"required"`
	Config      v1.InferenceConfig   `json:"config"`
}

// CreateInferenceResponse represents the response after creating an inference service.
type CreateInferenceResponse struct {
	InferenceId string `json:"inferenceId"`
}

// ListInferenceQuery represents query parameters for listing inferences.
type ListInferenceQuery struct {
	Limit     int    `form:"limit" binding:"omitempty,min=1"`
	Offset    int    `form:"offset" binding:"omitempty,min=0"`
	UserId    string `form:"userId" binding:"omitempty"` // Optional: filter by user ID
	ModelForm string `form:"modelForm" binding:"omitempty"`
	Phase     string `form:"phase" binding:"omitempty"`
}

// ListInferenceResponse represents the response for listing inferences.
type ListInferenceResponse struct {
	Total int             `json:"total"`
	Items []InferenceInfo `json:"items"`
}

// InferenceInfo represents basic inference information.
type InferenceInfo struct {
	InferenceId string    `json:"inferenceId"`
	DisplayName string    `json:"displayName"`
	ModelForm   string    `json:"modelForm"`
	ModelName   string    `json:"modelName"`
	Phase       string    `json:"phase"`
	Message     string    `json:"message"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// InferenceDetail represents detailed inference information.
type InferenceDetail struct {
	InferenceId string               `json:"inferenceId"`
	DisplayName string               `json:"displayName"`
	Description string               `json:"description"`
	UserId      string               `json:"userId"`
	UserName    string               `json:"userName"`
	ModelForm   string               `json:"modelForm"`
	ModelName   string               `json:"modelName"`
	Instance    v1.InferenceInstance `json:"instance"`
	Resource    v1.InferenceResource `json:"resource"`
	Phase       string               `json:"phase"`
	Message     string               `json:"message"`
	Events      []v1.InferenceEvent  `json:"events"`
	CreatedAt   time.Time            `json:"createdAt"`
	UpdatedAt   time.Time            `json:"updatedAt"`
}

// PatchInferenceRequest represents the request to update an inference service.
type PatchInferenceRequest struct {
	DisplayName *string               `json:"displayName"`
	Description *string               `json:"description"`
	Instance    *v1.InferenceInstance `json:"instance"`
}

// ChatRequest represents the request to chat with an inference model (streaming, no session saved).
// Frontend should prepare messages in OpenAI format before sending.
type ChatRequest struct {
	InferenceId      string                   `json:"inferenceId" binding:"required"`
	Messages         []map[string]interface{} `json:"messages" binding:"required"` // OpenAI format messages (prepared by frontend)
	Stream           bool                     `json:"stream"`                      // Enable streaming (SSE)
	Temperature      float64                  `json:"temperature"`                 // Controls randomness (0.0-2.0)
	TopP             float64                  `json:"topP"`                        // Nucleus sampling (0.0-1.0)
	MaxTokens        int                      `json:"maxTokens"`                   // Maximum tokens to generate
	FrequencyPenalty float64                  `json:"frequencyPenalty"`            // Penalize frequent tokens (-2.0 to 2.0)
	PresencePenalty  float64                  `json:"presencePenalty"`             // Penalize tokens based on presence (-2.0 to 2.0)
	N                int                      `json:"n"`                           // Number of completions to generate (1-10)
}

// SaveSessionRequest represents the request to save a chat session.
type SaveSessionRequest struct {
	Id           int64            `json:"id"` // Optional: if provided, update existing session
	ModelName    string           `json:"modelName" binding:"required"`
	DisplayName  string           `json:"displayName" binding:"required"`
	SystemPrompt string           `json:"systemPrompt"`
	Messages     []MessageHistory `json:"messages" binding:"required"` // Full chat history from frontend
}

// SaveSessionResponse represents the response after saving a session.
type SaveSessionResponse struct {
	Id int64 `json:"id"`
}

// ListPlaygroundSessionQuery represents query parameters for listing sessions.
type ListPlaygroundSessionQuery struct {
	Limit     int    `form:"limit" binding:"omitempty,min=1"`
	Offset    int    `form:"offset" binding:"omitempty,min=0"`
	ModelName string `form:"modelName" binding:"omitempty"`
}

// ListPlaygroundSessionResponse represents the response for listing sessions.
type ListPlaygroundSessionResponse struct {
	Total int                     `json:"total"`
	Items []PlaygroundSessionInfo `json:"items"`
}

// PlaygroundSessionInfo represents basic session information.
type PlaygroundSessionInfo struct {
	Id           int64     `json:"id"`
	ModelName    string    `json:"modelName"`
	DisplayName  string    `json:"displayName"`
	SystemPrompt string    `json:"systemPrompt"`
	MessageCount int       `json:"messageCount"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// PlaygroundSessionDetail represents detailed session information.
type PlaygroundSessionDetail struct {
	Id           int64            `json:"id"`
	ModelName    string           `json:"modelName"`
	DisplayName  string           `json:"displayName"`
	SystemPrompt string           `json:"systemPrompt"`
	Messages     []MessageHistory `json:"messages"`
	CreatedAt    time.Time        `json:"createdAt"`
	UpdatedAt    time.Time        `json:"updatedAt"`
}

// MessageHistory represents a chat message.
type MessageHistory struct {
	Role      string    `json:"role"` // system, user, assistant
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}
