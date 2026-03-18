/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/klog/v2"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// apimKeyHMACSecret is used to compute deterministic HMAC-SHA256 hashes of APIM Keys.
// The encrypted APIM Key (AES-CBC) uses random IV so the same plaintext produces
// different ciphertexts, making direct comparison impossible. This deterministic hash
// enables composite unique constraint (apim_key_hash + key_alias) and efficient lookups.
const apimKeyHMACSecret = "safe-llm-gateway-apim-key-uniqueness"

// ComputeApimKeyHash computes a deterministic HMAC-SHA256 hash of an APIM Key.
// Used for the composite unique constraint (apim_key_hash, key_alias): the same APIM Key
// CAN be shared by different users, but the same user cannot bind the same APIM Key twice.
func ComputeApimKeyHash(apimKey string) string {
	mac := hmac.New(sha256.New, []byte(apimKeyHMACSecret))
	mac.Write([]byte(apimKey))
	return hex.EncodeToString(mac.Sum(nil))
}

const TLLMGatewayUserBinding = "llm_gateway_user_binding"

// LLMGatewayUserBinding represents a user's APIM Key -> LiteLLM Virtual Key binding.
type LLMGatewayUserBinding struct {
	UserEmail         string    `gorm:"column:user_email;primaryKey" json:"user_email"`
	ApimKey           string    `gorm:"column:apim_key" json:"apim_key"`                                               // Encrypted APIM Key (AES-CBC, non-deterministic)
	ApimKeyHash       string    `gorm:"column:apim_key_hash;uniqueIndex:idx_apimkey_alias;index" json:"apim_key_hash"` // HMAC-SHA256 hash, part of composite unique (apim_key_hash + key_alias)
	LiteLLMVirtualKey string    `gorm:"column:litellm_virtual_key" json:"litellm_virtual_key"`
	LiteLLMKeyHash    string    `gorm:"column:litellm_key_hash" json:"litellm_key_hash"`
	KeyAlias          string    `gorm:"column:key_alias;uniqueIndex:idx_apimkey_alias" json:"key_alias"` // Part of composite unique (apim_key_hash + key_alias)
	CreatedAt         time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (LLMGatewayUserBinding) TableName() string {
	return TLLMGatewayUserBinding
}

// LLMGatewayInterface defines database operations for LLM Gateway bindings.
type LLMGatewayInterface interface {
	CreateLLMBinding(ctx context.Context, binding *LLMGatewayUserBinding) error
	GetLLMBindingByEmail(ctx context.Context, email string) (*LLMGatewayUserBinding, error)
	GetLLMBindingByApimKeyHash(ctx context.Context, apimKeyHash string) (*LLMGatewayUserBinding, error)
	UpdateLLMBinding(ctx context.Context, binding *LLMGatewayUserBinding) error
	DeleteLLMBinding(ctx context.Context, email string) error
	ListLLMBindings(ctx context.Context, limit, offset int) ([]*LLMGatewayUserBinding, int64, error)
}

// CreateLLMBinding creates a new LLM Gateway user binding.
func (c *Client) CreateLLMBinding(ctx context.Context, binding *LLMGatewayUserBinding) error {
	if binding == nil {
		return commonerrors.NewBadRequest("binding cannot be nil")
	}

	db, err := c.GetGormDB()
	if err != nil {
		return err
	}

	result := db.WithContext(ctx).Create(binding)
	if result.Error != nil {
		klog.ErrorS(result.Error, "failed to create LLM binding", "email", binding.UserEmail)
		return result.Error
	}
	return nil
}

// GetLLMBindingByEmail retrieves an LLM binding by user email.
func (c *Client) GetLLMBindingByEmail(ctx context.Context, email string) (*LLMGatewayUserBinding, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}

	var binding LLMGatewayUserBinding
	result := db.WithContext(ctx).Where("user_email = ?", email).First(&binding)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil // Not found is not an error
		}
		return nil, result.Error
	}
	return &binding, nil
}

// GetLLMBindingByApimKeyHash retrieves an LLM binding by the deterministic HMAC hash of an APIM Key.
// Use ComputeApimKeyHash(plainApimKey) to compute the hash before calling this method.
func (c *Client) GetLLMBindingByApimKeyHash(ctx context.Context, apimKeyHash string) (*LLMGatewayUserBinding, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}

	var binding LLMGatewayUserBinding
	result := db.WithContext(ctx).Where("apim_key_hash = ?", apimKeyHash).First(&binding)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &binding, nil
}

// UpdateLLMBinding updates an existing LLM binding (matched by user_email).
func (c *Client) UpdateLLMBinding(ctx context.Context, binding *LLMGatewayUserBinding) error {
	if binding == nil {
		return commonerrors.NewBadRequest("binding cannot be nil")
	}

	db, err := c.GetGormDB()
	if err != nil {
		return err
	}

	result := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_email"}},
		DoUpdates: clause.AssignmentColumns([]string{"apim_key", "apim_key_hash", "litellm_virtual_key", "litellm_key_hash", "updated_at"}),
	}).Create(binding)
	if result.Error != nil {
		klog.ErrorS(result.Error, "failed to update LLM binding", "email", binding.UserEmail)
		return result.Error
	}
	return nil
}

// DeleteLLMBinding deletes an LLM binding by user email.
func (c *Client) DeleteLLMBinding(ctx context.Context, email string) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}

	result := db.WithContext(ctx).Where("user_email = ?", email).Delete(&LLMGatewayUserBinding{})
	if result.Error != nil {
		klog.ErrorS(result.Error, "failed to delete LLM binding", "email", email)
		return result.Error
	}
	return nil
}

// ListLLMBindings lists all LLM bindings with pagination.
func (c *Client) ListLLMBindings(ctx context.Context, limit, offset int) ([]*LLMGatewayUserBinding, int64, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, 0, err
	}

	var total int64
	db.WithContext(ctx).Model(&LLMGatewayUserBinding{}).Count(&total)

	var bindings []*LLMGatewayUserBinding
	result := db.WithContext(ctx).Order("created_at DESC").Limit(limit).Offset(offset).Find(&bindings)
	if result.Error != nil {
		return nil, 0, result.Error
	}
	return bindings, total, nil
}
