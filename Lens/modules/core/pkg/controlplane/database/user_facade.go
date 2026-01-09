// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
)

// UserFacadeInterface defines User database operations
type UserFacadeInterface interface {
	Create(ctx context.Context, user *model.LensUsers) error
	GetByID(ctx context.Context, id string) (*model.LensUsers, error)
	GetByUsername(ctx context.Context, username string) (*model.LensUsers, error)
	GetByEmail(ctx context.Context, email string) (*model.LensUsers, error)
	Update(ctx context.Context, user *model.LensUsers) error
	UpdateLastLogin(ctx context.Context, userID string) error
	GetRootUser(ctx context.Context) (*model.LensUsers, error)
	List(ctx context.Context, offset, limit int) ([]*model.LensUsers, int64, error)
	ListByAuthType(ctx context.Context, authType string, offset, limit int) ([]*model.LensUsers, int64, error)
	Delete(ctx context.Context, id string) error
	ExistsByUsername(ctx context.Context, username string) (bool, error)
	CreateFromLDAP(ctx context.Context, username, email, displayName string, isAdmin bool) (*model.LensUsers, error)
}

// UserFacade implements UserFacadeInterface
type UserFacade struct {
	BaseFacade
}

// NewUserFacade creates a new UserFacade
func NewUserFacade() *UserFacade {
	return &UserFacade{}
}

// Create creates a new user
func (f *UserFacade) Create(ctx context.Context, user *model.LensUsers) error {
	return f.getDB().WithContext(ctx).Create(user).Error
}

// GetByID gets a user by ID
func (f *UserFacade) GetByID(ctx context.Context, id string) (*model.LensUsers, error) {
	var user model.LensUsers
	err := f.getDB().WithContext(ctx).Where("id = ?", id).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByUsername gets a user by username
func (f *UserFacade) GetByUsername(ctx context.Context, username string) (*model.LensUsers, error) {
	var user model.LensUsers
	err := f.getDB().WithContext(ctx).Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByEmail gets a user by email
func (f *UserFacade) GetByEmail(ctx context.Context, email string) (*model.LensUsers, error) {
	var user model.LensUsers
	err := f.getDB().WithContext(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Update updates a user
func (f *UserFacade) Update(ctx context.Context, user *model.LensUsers) error {
	user.UpdatedAt = time.Now()
	return f.getDB().WithContext(ctx).Save(user).Error
}

// UpdateLastLogin updates the last login time for a user
func (f *UserFacade) UpdateLastLogin(ctx context.Context, userID string) error {
	now := time.Now()
	return f.getDB().WithContext(ctx).
		Model(&model.LensUsers{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"last_login_at": now,
			"updated_at":    now,
		}).Error
}

// GetRootUser gets the root user
func (f *UserFacade) GetRootUser(ctx context.Context) (*model.LensUsers, error) {
	var user model.LensUsers
	err := f.getDB().WithContext(ctx).Where("is_root = ?", true).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// List lists users with pagination
func (f *UserFacade) List(ctx context.Context, offset, limit int) ([]*model.LensUsers, int64, error) {
	var users []*model.LensUsers
	var count int64

	db := f.getDB().WithContext(ctx).Model(&model.LensUsers{})

	if err := db.Count(&count).Error; err != nil {
		return nil, 0, err
	}

	if err := db.Offset(offset).Limit(limit).Order("created_at DESC").Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, count, nil
}

// ListByAuthType lists users by authentication type with pagination
func (f *UserFacade) ListByAuthType(ctx context.Context, authType string, offset, limit int) ([]*model.LensUsers, int64, error) {
	var users []*model.LensUsers
	var count int64

	db := f.getDB().WithContext(ctx).Model(&model.LensUsers{}).Where("auth_type = ?", authType)

	if err := db.Count(&count).Error; err != nil {
		return nil, 0, err
	}

	if err := db.Offset(offset).Limit(limit).Order("created_at DESC").Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, count, nil
}

// Delete deletes a user by ID
func (f *UserFacade) Delete(ctx context.Context, id string) error {
	return f.getDB().WithContext(ctx).Where("id = ?", id).Delete(&model.LensUsers{}).Error
}

// ExistsByUsername checks if a user with the given username exists
func (f *UserFacade) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	var count int64
	err := f.getDB().WithContext(ctx).Model(&model.LensUsers{}).Where("username = ?", username).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateFromLDAP creates a new user from LDAP authentication
func (f *UserFacade) CreateFromLDAP(ctx context.Context, username, email, displayName string, isAdmin bool) (*model.LensUsers, error) {
	now := time.Now()
	user := &model.LensUsers{
		ID:          generateUserID(),
		Username:    username,
		Email:       email,
		DisplayName: displayName,
		AuthType:    "ldap",
		Status:      "active",
		IsAdmin:     isAdmin,
		IsRoot:      false,
		CreatedAt:   now,
		UpdatedAt:   now,
		LastLoginAt: now,
	}

	if err := f.getDB().WithContext(ctx).Create(user).Error; err != nil {
		return nil, err
	}

	return user, nil
}

// generateUserID generates a unique user ID
func generateUserID() string {
	return "user-" + time.Now().Format("20060102150405") + "-" + randomHex(8)
}

// randomHex generates a random hex string of the given length
func randomHex(n int) string {
	bytes := make([]byte, n/2)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// Ensure UserFacade implements UserFacadeInterface
var _ UserFacadeInterface = (*UserFacade)(nil)
