/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"encoding/base64"
	"strconv"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

func (h *ImageHandler) updateImportProgress(c *gin.Context) (interface{}, error) {
	data, err := base64.URLEncoding.DecodeString(c.Param("name"))
	if err != nil {
		return nil, commonerrors.NewBadRequest("invalid image name: " + err.Error())
	}
	imageName := string(data)
	imageItem, err := h.dbClient.GetImageByTag(c, imageName)
	if err != nil {
		klog.ErrorS(err, "get image by image tag error")
		return nil, commonerrors.NewInternalError("Database Error")
	}
	if imageItem == nil {
		klog.Errorf("image not found, imageTag: %s \n", imageName)
		return nil, commonerrors.NewNotFound("get image by imagetag", imageName)
	}

	importImage, err := h.dbClient.GetImportImageByImageID(c, imageItem.ID)
	if err != nil {
		klog.Errorf("GetImportImageByID(%d) error: %s \n", imageItem.ID, err)
		return nil, commonerrors.NewInternalError("Database Error")
	}
	if importImage == nil {
		klog.Errorf("import image not found, imageID: %d \n", imageItem.ID)
		return nil, commonerrors.NewNotFound("get import image by id", strconv.Itoa(int(imageItem.ID)))
	}

	if err := c.ShouldBindJSON(&importImage.Layer); err != nil {
		klog.ErrorS(err, "json marshal upstream data error, name: ", imageName)
		return nil, commonerrors.NewInternalError("json marshal error")
	}

	if err := h.dbClient.UpdateImageImportJob(c, importImage); err != nil {
		klog.ErrorS(err, "update import image error")
		return nil, commonerrors.NewInternalError("Database Error")
	}

	return nil, nil
}
