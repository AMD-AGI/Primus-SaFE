package image_handlers

import (
	"context"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/gin-gonic/gin"
	"strconv"
	"time"
)

func (h *ImageHandler) createImageRegistry(c *gin.Context) (*model.RegistryInfo, error) {
	body := &CreateRegistryRequest{}
	if err := c.ShouldBindJSON(&body); err != nil {
		return nil, commonerrors.NewBadRequest("invalid body: " + err.Error())
	}
	if err := body.Validate(true); err != nil {
		return nil, err
	}
	result, err := h.upsertImageRegistryInfo(c, body)
	if err != nil {
		return nil, err
	}
	err = h.refreshImageImportSecrets(c)
	if err != nil {
		return nil, err
	}

	err = h.refreshImagePullSecrets(c)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (h *ImageHandler) updateImageRegistry(c *gin.Context) (*model.RegistryInfo, error) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return nil, commonerrors.NewBadRequest("invalid id: " + err.Error())
	}
	body := &CreateRegistryRequest{}
	if err := c.ShouldBindJSON(&body); err != nil {
		return nil, commonerrors.NewBadRequest("invalid body: " + err.Error())
	}
	if err := body.Validate(false); err != nil {
		return nil, err
	}
	body.Id = int32(id)
	result, err := h.upsertImageRegistryInfo(c, body)
	if err != nil {
		return nil, err
	}
	err = h.refreshImageImportSecrets(c)
	if err != nil {
		return nil, err
	}
	err = h.refreshImagePullSecrets(c)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (h *ImageHandler) deleteImageRegistry(c *gin.Context) (interface{}, error) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return nil, commonerrors.NewBadRequest("invalid id: " + err.Error())
	}
	existInfo, err := h.dbClient.GetRegistryInfoById(c, int32(id))
	if err != nil {
		return nil, err
	}
	if existInfo == nil {
		return nil, nil
	}
	err = h.dbClient.DeleteRegistryInfo(c, int32(id))
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *ImageHandler) listImageRegistry(c *gin.Context) ([]*model.RegistryInfo, error) {
	page := &Pagination{}
	if err := c.ShouldBindQuery(page); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	return h.dbClient.ListRegistryInfos(c, page.PageNum, page.PageSize)
}

func (h *ImageHandler) upsertImageRegistryInfo(ctx context.Context, req *CreateRegistryRequest) (*model.RegistryInfo, error) {
	newInfo, err := h.cvtCreateRegistryRequestToRegistryInfo(req)
	if err != nil {
		return nil, err
	}
	var existInfo *model.RegistryInfo
	if req.Id != 0 {
		existInfo, err = h.dbClient.GetRegistryInfoById(ctx, req.Id)
		if err != nil {
			return nil, err
		}
	}
	if existInfo != nil {
		newInfo.ID = existInfo.ID
		newInfo.UpdatedAt = time.Now()
	}
	err = h.dbClient.UpsertRegistryInfo(ctx, newInfo)
	if err != nil {
		return nil, err
	}
	return existInfo, nil
}

func (h *ImageHandler) cvtCreateRegistryRequestToRegistryInfo(req *CreateRegistryRequest) (*model.RegistryInfo, error) {
	password := ""
	if req.Password != "" {
		encPassword, err := crypto.NewCrypto().Encrypt([]byte(req.Password))
		if err != nil {
			return nil, err
		}
		password = encPassword
	}
	userName := ""
	if req.UserName != "" {
		encUserName, err := crypto.NewCrypto().Encrypt([]byte(req.UserName))
		if err != nil {
			return nil, err
		}
		userName = encUserName
	}
	return &model.RegistryInfo{
		Name:     req.Name,
		URL:      req.Url,
		Username: userName,
		Password: password,
		Default:  req.Default,
	}, nil
}
