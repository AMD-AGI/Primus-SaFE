/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

// CreatePublicKeyRequest represents the request to create a new public key.
type CreatePublicKeyRequest struct {
	Name        string `json:"name"`        // The name of the public key.
	Description string `json:"description"` // The description of the public key.
	PublicKey   string `json:"publicKey"`   // The actual public key string.
}

// ListPublicKeysRequest represents the query parameters for listing public keys.
type ListPublicKeysRequest struct {
	Offset int    `form:"offset" binding:"omitempty,min=0"`         // The offset for pagination.
	Limit  int    `form:"limit" binding:"omitempty,min=1"`          // The limit for pagination.
	SortBy string `form:"sortBy" binding:"omitempty"`               // The field to sort by.
	Order  string `form:"order" binding:"omitempty,oneof=desc asc"` // The sort order, either desc or asc.
	UserId string `form:"-"`                                        // The user ID (internal use).
}

// ListPublicKeysResponse represents the response for listing public keys.
type ListPublicKeysResponse struct {
	TotalCount int                          `json:"totalCount"` // The total number of public keys.
	Items      []ListPublicKeysResponseItem `json:"items"`      // The list of public key items.
}

// ListPublicKeysResponseItem represents a single public key item in the response.
type ListPublicKeysResponseItem struct {
	Id          int64  `json:"id"`          // The unique ID of the public key.
	UserId      string `json:"userId"`      // The user ID associated with the public key.
	Description string `json:"description"` // The description of the public key.
	PublicKey   string `json:"publicKey"`   // The actual public key string.
	Status      bool   `json:"status"`      // The status of the public key (active/inactive).
	CreateTime  string `json:"createTime"`  // The creation time of the public key.
	UpdateTime  string `json:"updateTime"`  // The last update time of the public key.
}

// SetPublicKeyStatusRequest represents the request to set the status of a public key.
type SetPublicKeyStatusRequest struct {
	Status bool `json:"status"` // The new status of the public key.
}

// SetPublicKeyDescriptionRequest represents the request to set the description of a public key.
type SetPublicKeyDescriptionRequest struct {
	Description string `json:"description"` // The new description for the public key.
}
