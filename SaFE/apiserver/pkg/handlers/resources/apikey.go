/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/lib/pq"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// CreateApiKey handles the creation of a new API key for the authenticated user.
func (h *Handler) CreateApiKey(c *gin.Context) {
	handle(c, h.createApiKey)
}

// ListApiKey handles listing API keys for the authenticated user.
func (h *Handler) ListApiKey(c *gin.Context) {
	handle(c, h.listApiKey)
}

// DeleteApiKey handles the soft deletion of an API key.
func (h *Handler) DeleteApiKey(c *gin.Context) {
	handle(c, h.deleteApiKey)
}

// createApiKey implements the API key creation logic.
func (h *Handler) createApiKey(c *gin.Context) (interface{}, error) {
	if h.dbClient == nil {
		return nil, commonerrors.NewInternalError("database is not enabled")
	}

	userId := c.GetString(common.UserId)
	if userId == "" {
		return nil, commonerrors.NewUnauthorized("user not authenticated")
	}
	userName := c.GetString(common.UserName)

	req := &view.CreateApiKeyRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse create api key request", "body", string(body))
		return nil, err
	}

	// Validate request
	if req.Name == "" {
		return nil, commonerrors.NewBadRequest("name is required")
	}
	if req.TTLDays < 1 || req.TTLDays > authority.MaxTTLDays {
		return nil, commonerrors.NewBadRequest("ttlDays must be between 1 and 366")
	}

	// Validate and deduplicate whitelist
	var whitelist []string
	if len(req.Whitelist) > 0 {
		var err error
		whitelist, err = authority.ValidateAndDeduplicateWhitelist(req.Whitelist)
		if err != nil {
			return nil, commonerrors.NewBadRequest("invalid whitelist: " + err.Error())
		}
	}

	// Generate API key
	apiKey, err := authority.GenerateApiKey()
	if err != nil {
		klog.ErrorS(err, "failed to generate api key")
		return nil, commonerrors.NewInternalError("failed to generate API key")
	}

	// Calculate expiration time
	now := time.Now().UTC()
	expirationTime := now.AddDate(0, 0, req.TTLDays)

	// Serialize whitelist to JSON
	whitelistJSON := "[]"
	if len(whitelist) > 0 {
		whitelistBytes, err := json.Marshal(whitelist)
		if err != nil {
			klog.ErrorS(err, "failed to marshal whitelist")
			return nil, commonerrors.NewInternalError("failed to process whitelist")
		}
		whitelistJSON = string(whitelistBytes)
	}

	// Hash the API key for secure storage (only store hash, not plaintext)
	hashedApiKey := authority.HashApiKey(apiKey, authority.GetApiKeySecret())
	// Generate key hint for display (e.g., "XX-YYYY")
	keyHint := authority.GenerateKeyHint(apiKey)

	// Create database record with hashed API key
	record := &dbclient.ApiKey{
		Name:           req.Name,
		UserId:         userId,
		UserName:       userName,
		ApiKey:         hashedApiKey, // Store hash, not plaintext
		KeyHint:        keyHint,      // Store hint for display
		ExpirationTime: pq.NullTime{Time: expirationTime, Valid: true},
		CreationTime:   pq.NullTime{Time: now, Valid: true},
		Whitelist:      whitelistJSON,
		Deleted:        false,
	}

	if err := h.dbClient.InsertApiKey(c.Request.Context(), record); err != nil {
		klog.ErrorS(err, "failed to insert api key", "name", req.Name, "userId", userId)
		return nil, commonerrors.NewInternalError("failed to create API key")
	}

	klog.Infof("created api key, id: %d, name: %s, userId: %s, expiration: %s",
		record.Id, req.Name, userId, expirationTime.Format(time.RFC3339))

	return &view.CreateApiKeyResponse{
		Id:             record.Id,
		Name:           req.Name,
		UserId:         userId,
		ApiKey:         apiKey, // Only returned during creation
		ExpirationTime: timeutil.FormatRFC3339(expirationTime),
		CreationTime:   timeutil.FormatRFC3339(now),
		Whitelist:      whitelist, // Use deduplicated whitelist
		Deleted:        false,
	}, nil
}

// listApiKey implements the API key listing logic.
func (h *Handler) listApiKey(c *gin.Context) (interface{}, error) {
	if h.dbClient == nil {
		return nil, commonerrors.NewInternalError("database is not enabled")
	}

	userId := c.GetString(common.UserId)
	if userId == "" {
		return nil, commonerrors.NewUnauthorized("user not authenticated")
	}

	// Parse query parameters
	req, err := parseListApiKeyQuery(c)
	if err != nil {
		return nil, err
	}
	req.UserId = userId

	// Build query: filter by user_id, exclude deleted only (include expired keys)
	tags := dbclient.GetApiKeyFieldTags()
	query := sqrl.And{
		sqrl.Eq{dbclient.GetFieldTag(tags, "UserId"): req.UserId},
		sqrl.Eq{dbclient.GetFieldTag(tags, "Deleted"): false},
	}

	// Build order by: expired keys at the end
	orderBy := buildListApiKeyOrderBy(req, tags)
	// Add expiration sorting: non-expired first (expiration_time > now() DESC)
	expirationField := dbclient.GetFieldTag(tags, "ExpirationTime")
	expiredSort := fmt.Sprintf("(%s > NOW()) DESC", expirationField)
	orderBy = append([]string{expiredSort}, orderBy...)

	// Get total count
	totalCount, err := h.dbClient.CountApiKeys(c.Request.Context(), query)
	if err != nil {
		klog.ErrorS(err, "failed to count api keys", "userId", userId)
		return nil, commonerrors.NewInternalError("failed to list API keys")
	}

	// Get paginated records
	records, err := h.dbClient.SelectApiKeys(c.Request.Context(), query, orderBy, req.Limit, req.Offset)
	if err != nil {
		klog.ErrorS(err, "failed to select api keys", "userId", userId)
		return nil, commonerrors.NewInternalError("failed to list API keys")
	}

	items := make([]view.ApiKeyResponseItem, 0, len(records))
	for _, record := range records {
		item := convertToApiKeyResponseItem(record)
		items = append(items, item)
	}

	return &view.ListApiKeyResponse{
		TotalCount: totalCount,
		Items:      items,
	}, nil
}

// deleteApiKey implements the API key deletion logic (soft delete).
func (h *Handler) deleteApiKey(c *gin.Context) (interface{}, error) {
	if h.dbClient == nil {
		return nil, commonerrors.NewInternalError("database is not enabled")
	}

	userId := c.GetString(common.UserId)
	if userId == "" {
		return nil, commonerrors.NewUnauthorized("user not authenticated")
	}

	// Get API key ID from path parameter
	idStr := c.Param("id")
	if idStr == "" {
		return nil, commonerrors.NewBadRequest("id is required")
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, commonerrors.NewBadRequest("invalid id format")
	}

	// Verify the API key belongs to the user and exists
	record, err := h.dbClient.GetApiKeyById(c.Request.Context(), id)
	if err != nil {
		return nil, commonerrors.NewNotFoundWithMessage("API key not found")
	}
	if record.UserId != userId {
		return nil, commonerrors.NewForbidden("not authorized to delete this API key")
	}
	if record.Deleted {
		return nil, commonerrors.NewBadRequest("API key already deleted")
	}

	// Perform soft delete
	if err := h.dbClient.SetApiKeyDeleted(c.Request.Context(), userId, id); err != nil {
		klog.ErrorS(err, "failed to delete api key", "id", id, "userId", userId)
		return nil, commonerrors.NewInternalError("failed to delete API key")
	}

	klog.Infof("deleted api key, id: %d, userId: %s", id, userId)
	return nil, nil
}

// parseListApiKeyQuery parses query parameters for listing API keys.
func parseListApiKeyQuery(c *gin.Context) (*view.ListApiKeyRequest, error) {
	query := &view.ListApiKeyRequest{}
	err := c.ShouldBindWith(&query, binding.Query)
	if err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	if query.Limit <= 0 {
		query.Limit = view.DefaultQueryLimit
	}
	if query.Order == "" {
		query.Order = dbclient.DESC
	}
	if query.SortBy == "" {
		query.SortBy = dbclient.CreateTime
	} else {
		query.SortBy = strings.ToLower(query.SortBy)
	}
	return query, nil
}

// buildListApiKeyOrderBy builds the ORDER BY clause for listing API keys.
func buildListApiKeyOrderBy(req *view.ListApiKeyRequest, dbTags map[string]string) []string {
	var orderBy []string
	if req.SortBy != "" {
		sortBy := dbclient.GetFieldTag(dbTags, req.SortBy)
		if sortBy != "" {
			orderBy = append(orderBy, sortBy+" "+req.Order)
		}
	}
	// Always add creation_time as secondary sort
	creationTime := dbclient.GetFieldTag(dbTags, "CreationTime")
	if len(orderBy) == 0 || !strings.Contains(orderBy[0], creationTime) {
		orderBy = append(orderBy, creationTime+" "+dbclient.DESC)
	}
	return orderBy
}

// convertToApiKeyResponseItem converts a database record to a response item
// Note: API key value is NOT included in list responses for security
func convertToApiKeyResponseItem(record *dbclient.ApiKey) view.ApiKeyResponseItem {
	item := view.ApiKeyResponseItem{
		Id:      record.Id,
		Name:    record.Name,
		UserId:  record.UserId,
		KeyHint: record.KeyHint, // Already in display format: "ak-XX****YYYY"
		Deleted: record.Deleted,
	}

	if record.ExpirationTime.Valid {
		item.ExpirationTime = timeutil.FormatRFC3339(record.ExpirationTime.Time)
	}
	if record.CreationTime.Valid {
		item.CreationTime = timeutil.FormatRFC3339(record.CreationTime.Time)
	}
	if record.DeletionTime.Valid {
		t := timeutil.FormatRFC3339(record.DeletionTime.Time)
		item.DeletionTime = &t
	}

	// Parse whitelist from JSON
	if record.Whitelist != "" && record.Whitelist != "[]" && record.Whitelist != "null" {
		var whitelist []string
		if err := json.Unmarshal([]byte(record.Whitelist), &whitelist); err == nil {
			item.Whitelist = whitelist
		}
	}
	if item.Whitelist == nil {
		item.Whitelist = []string{}
	}

	return item
}
