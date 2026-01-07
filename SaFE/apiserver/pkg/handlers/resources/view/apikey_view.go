/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

// CreateApiKeyRequest represents the request body for creating an API key
type CreateApiKeyRequest struct {
	// Name is the display name for the API key (required, can be duplicated)
	Name string `json:"name" binding:"required,max=100"`
	// TTLDays is the number of days until the API key expires (required, max 366)
	TTLDays int `json:"ttlDays" binding:"required,min=1,max=366"`
	// Whitelist is an optional list of IP addresses or CIDR ranges
	Whitelist []string `json:"whitelist,omitempty"`
}

// CreateApiKeyResponse represents the response after creating an API key
// The apiKey field is only returned once during creation
type CreateApiKeyResponse struct {
	// Id is the unique identifier of the API key
	Id int64 `json:"id"`
	// Name is the display name
	Name string `json:"name"`
	// UserId is the ID of the user who owns this key
	UserId string `json:"userId"`
	// ApiKey is the actual API key value (only returned during creation)
	ApiKey string `json:"apiKey"`
	// ExpirationTime is when the key expires (RFC3339 format)
	ExpirationTime string `json:"expirationTime"`
	// CreationTime is when the key was created (RFC3339 format)
	CreationTime string `json:"creationTime"`
	// Whitelist is the list of allowed IPs/CIDRs
	Whitelist []string `json:"whitelist"`
	// Deleted indicates if the key has been deleted
	Deleted bool `json:"deleted"`
}

// ListApiKeyRequest represents the query parameters for listing API keys
type ListApiKeyRequest struct {
	// Offset is the pagination offset
	Offset int `form:"offset" binding:"omitempty,min=0"`
	// Limit is the pagination limit
	Limit int `form:"limit" binding:"omitempty,min=1"`
	// SortBy is the field to sort by (e.g., creationTime, expirationTime)
	SortBy string `form:"sortBy" binding:"omitempty"`
	// Order is the sort order (desc or asc)
	Order string `form:"order" binding:"omitempty,oneof=desc asc"`
	// UserId is set internally (not from query params)
	UserId string `form:"-"`
}

// ListApiKeyResponse represents the response for listing API keys
type ListApiKeyResponse struct {
	// TotalCount is the total number of keys
	TotalCount int `json:"totalCount"`
	// Items is the list of API keys
	Items []ApiKeyResponseItem `json:"items"`
}

// ApiKeyResponseItem represents an API key in list responses
// Note: The actual API key value is NOT returned for security
type ApiKeyResponseItem struct {
	// Id is the unique identifier
	Id int64 `json:"id"`
	// Name is the display name
	Name string `json:"name"`
	// UserId is the ID of the user who owns this key
	UserId string `json:"userId"`
	// KeyHint is the partial key for display (e.g., "ak-XX****YYYY")
	KeyHint string `json:"keyHint"`
	// ExpirationTime is when the key expires (RFC3339 format)
	ExpirationTime string `json:"expirationTime"`
	// CreationTime is when the key was created (RFC3339 format)
	CreationTime string `json:"creationTime"`
	// Whitelist is the list of allowed IPs/CIDRs
	Whitelist []string `json:"whitelist"`
	// Deleted indicates if the key has been deleted
	Deleted bool `json:"deleted"`
	// DeletionTime is when the key was deleted (RFC3339 format, null if not deleted)
	DeletionTime *string `json:"deletionTime"`
}
