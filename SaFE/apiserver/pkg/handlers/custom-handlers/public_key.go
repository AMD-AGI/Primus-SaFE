/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

// CreatePublicKey handles the creation of a new public key.
func (h *Handler) CreatePublicKey(c *gin.Context) {
	handle(c, h.createPublicKey)
}

// ListPublicKeys handles listing all public keys for the user.
func (h *Handler) ListPublicKeys(c *gin.Context) {
	handle(c, h.listPublicKeys)
}

// DeletePublicKey handles the deletion of a public key by ID.
func (h *Handler) DeletePublicKey(c *gin.Context) {
	handle(c, h.deletePublicKey)
}

// SetPublicKeyStatus handles updating the status of a public key.
func (h *Handler) SetPublicKeyStatus(c *gin.Context) {
	handle(c, h.setPublicKeyStatus)
}

// SetPublicKeyDescription handles updating the description of a public key.
func (h *Handler) SetPublicKeyDescription(c *gin.Context) {
	handle(c, h.setPublicKeyDescription)
}

// createPublicKey creates a new public key record in the database.
func (h *Handler) createPublicKey(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	req := &types.CreatePublicKeyRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "fail to parse create public key request", "body", string(body))
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: common.PublicKeyKind,
		Verb:         v1.CreateVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	nowTime := dbutils.NullMetaV1Time(&metav1.Time{Time: time.Now().UTC()})
	publicKey := &dbclient.PublicKey{
		UserId:      c.GetString(common.UserId),
		Description: req.Description,
		PublicKey:   req.PublicKey,
		Status:      true,
		CreateTime:  nowTime,
		UpdateTime:  nowTime,
	}
	err = h.dbClient.InsertPublicKey(c.Request.Context(), publicKey)

	return nil, err
}

// listPublicKeys lists all public keys for the current user.
func (h *Handler) listPublicKeys(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}

	query, err := parseListPublicKeyQuery(c)
	if err != nil {
		klog.ErrorS(err, "fail to parse list public key request", "body", query)
		return nil, err
	}

	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: common.PublicKeyKind,
		Verb:         v1.ListVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	query.UserId = c.GetString(common.UserId)
	dbSql, orderBy, err := cvtToListPublicKeysSql(query)
	if err != nil {
		return nil, err
	}
	pubKeys, err := h.dbClient.SelectPublicKeys(ctx, dbSql, orderBy, query.Limit, query.Offset)
	if err != nil {
		return nil, err
	}
	totalCount, err := h.dbClient.CountPublicKeys(ctx, dbSql)
	if err != nil {
		return nil, err
	}
	result := &types.ListPublicKeysResponse{
		TotalCount: totalCount,
	}
	for _, k := range pubKeys {
		result.Items = append(result.Items, cvtToPublicKeyResponse(k))
	}
	return result, nil
}

// deletePublicKey deletes a public key by its ID.
func (h *Handler) deletePublicKey(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	id := c.Param("id")
	if id == "" {
		return nil, fmt.Errorf("id is empty")
	}
	publicKeyId, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}

	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: common.PublicKeyKind,
		Verb:         v1.DeleteVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	if err := h.dbClient.DeletePublicKey(c.Request.Context(), c.GetString(common.UserId), int64(publicKeyId)); err != nil {
		return nil, err
	}
	return nil, nil
}

// setPublicKeyStatus sets the status of a public key.
func (h *Handler) setPublicKeyStatus(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	req := &types.SetPublicKeyStatusRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "fail to parse create public key request", "body", string(body))
		return nil, err
	}
	id := c.Param("id")
	if id == "" {
		return nil, fmt.Errorf("id is empty")
	}
	publicKeyId, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}

	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: common.PublicKeyKind,
		Verb:         v1.UpdateVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	err = h.dbClient.SetPublicKeyStatus(c.Request.Context(), c.GetString(common.UserId), int64(publicKeyId), req.Status)
	return nil, err
}

// setPublicKeyDescription sets the description of a public key.
func (h *Handler) setPublicKeyDescription(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	req := &types.SetPublicKeyDescriptionRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "fail to parse create public key request", "body", string(body))
		return nil, err
	}
	id := c.Param("id")
	if id == "" {
		return nil, fmt.Errorf("id is empty")
	}
	publicKeyId, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}

	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: common.PublicKeyKind,
		Verb:         v1.UpdateVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	err = h.dbClient.SetPublicKeyDescription(c.Request.Context(), c.GetString(common.UserId), int64(publicKeyId), req.Description)
	return nil, err
}

// cvtToPublicKeyResponse converts a PublicKey model to a response item.
func cvtToPublicKeyResponse(k *dbclient.PublicKey) types.ListPublicKeysResponseItem {
	result := types.ListPublicKeysResponseItem{
		Id:          k.Id,
		UserId:      k.UserId,
		Description: k.Description,
		PublicKey:   k.PublicKey,
		Status:      k.Status,
		CreateTime:  dbutils.ParseNullTimeToString(k.CreateTime),
		UpdateTime:  dbutils.ParseNullTimeToString(k.UpdateTime),
	}
	return result
}

// parseListPublicKeyQuery parses the query parameters for listing public keys.
func parseListPublicKeyQuery(c *gin.Context) (*types.ListPublicKeysRequest, error) {
	query := &types.ListPublicKeysRequest{}
	err := c.ShouldBindWith(&query, binding.Query)
	if err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	if query.Limit <= 0 {
		query.Limit = types.DefaultQueryLimit
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

// cvtToListPublicKeysSql converts the query to SQL conditions and order by clause.
func cvtToListPublicKeysSql(query *types.ListPublicKeysRequest) (sqrl.Sqlizer, []string, error) {
	dbTags := dbclient.GetPublicKeyFieldTags()
	dbSql := sqrl.And{
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "DeleteTime"): nil},
	}
	if userId := strings.TrimSpace(query.UserId); userId != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "UserId"): userId})
	}

	orderBy := buildListPublicKeysOrderBy(query, dbTags)
	return dbSql, orderBy, nil
}

// buildListPublicKeysOrderBy builds the order by clause for listing public keys.
func buildListPublicKeysOrderBy(query *types.ListPublicKeysRequest, dbTags map[string]string) []string {
	var nullOrder string
	if query.Order == dbclient.DESC {
		nullOrder = "NULLS FIRST"
	} else {
		nullOrder = "NULLS LAST"
	}
	createTime := dbclient.GetFieldTag(dbTags, "CreateTime")

	var orderBy []string
	isSortByCreationTime := false
	if query.SortBy != "" {
		sortBy := strings.TrimSpace(query.SortBy)
		sortBy = dbclient.GetFieldTag(dbTags, sortBy)
		if sortBy != "" {
			if stringutil.StrCaseEqual(query.SortBy, createTime) {
				isSortByCreationTime = true
			}
			orderBy = append(orderBy, fmt.Sprintf("%s %s %s", sortBy, query.Order, nullOrder))
		}
	}
	if !isSortByCreationTime {
		orderBy = append(orderBy, fmt.Sprintf("%s %s", createTime, dbclient.DESC))
	}
	return orderBy
}
