package custom_handlers

import (
	"fmt"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"strconv"
	"strings"
	"time"
)

func (h *Handler) CreatePublicKey(c *gin.Context) {
	handle(c, h.createPublicKey)
}

func (h *Handler) ListPublicKeys(c *gin.Context) {
	handle(c, h.listPublicKeys)
}

func (h *Handler) DeletePublicKey(c *gin.Context) {
	handle(c, h.deletePublicKey)
}

func (h *Handler) SetPublicKeyStatus(c *gin.Context) {
	handle(c, h.setPublicKeyStatus)
}

func (h *Handler) SetPublicKeyDescription(c *gin.Context) {
	handle(c, h.setPublicKeyDescription)
}

func (h *Handler) createPublicKey(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	req := &types.CreatePublicKeyRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "fail to parse create public key request", "body", string(body))
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

	if err := h.dbClient.DeletePublicKey(c.Request.Context(), c.GetString(common.UserId), int64(publicKeyId)); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *Handler) setPublicKeyStatus(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	req := &types.SetPublicKeyStatusRequest{}
	body, err := getBodyFromRequest(c.Request, req)
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

	err = h.dbClient.SetPublicKeyStatus(c.Request.Context(), c.GetString(common.UserId), int64(publicKeyId), req.Status)
	return nil, err
}

func (h *Handler) setPublicKeyDescription(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	req := &types.SetPublicKeyDescriptionRequest{}
	body, err := getBodyFromRequest(c.Request, req)
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

	err = h.dbClient.SetPublicKeyDescription(c.Request.Context(), c.GetString(common.UserId), int64(publicKeyId), req.Description)
	return nil, err
}

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
