// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"context"
	"time"

	"gorm.io/gorm"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	cpmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

const (
	// AuthTypeSafe is the auth type for users synced from SaFE
	AuthTypeSafe = "safe"
)

// UserSyncService syncs users from SaFE User CRD to Lens Control Plane
type UserSyncService struct {
	k8sClient client.Client // K8s client to read SaFE User CRD
	lensDB    *gorm.DB      // Lens Control Plane database (read-write)
}

// NewUserSyncService creates a new user sync service
func NewUserSyncService(k8sClient client.Client, lensDB *gorm.DB) *UserSyncService {
	return &UserSyncService{
		k8sClient: k8sClient,
		lensDB:    lensDB,
	}
}

// Name returns the task name
func (s *UserSyncService) Name() string {
	return "user-sync"
}

// Run executes the user sync task
func (s *UserSyncService) Run(ctx context.Context) error {
	log.Debug("Starting user sync from SaFE User CRD")

	// 1. Get all Users from SaFE CRD
	safeUsers, err := s.listSafeUsers(ctx)
	if err != nil {
		log.Errorf("Failed to list SaFE users: %v", err)
		return err
	}

	if len(safeUsers) == 0 {
		log.Debug("No users found in SaFE")
		return nil
	}

	log.Debugf("Found %d users in SaFE", len(safeUsers))

	// 2. Sync each user to Lens Control Plane
	syncedCount := 0
	updatedCount := 0
	for _, user := range safeUsers {
		created, err := s.syncUser(ctx, &user)
		if err != nil {
			log.Errorf("Failed to sync user %s: %v", user.Name, err)
			continue
		}
		if created {
			syncedCount++
		} else {
			updatedCount++
		}
	}

	// 3. Mark users as disabled if they no longer exist in SaFE
	disabledCount, err := s.disableDeletedUsers(ctx, safeUsers)
	if err != nil {
		log.Errorf("Failed to disable deleted users: %v", err)
	}

	log.Infof("User sync completed: created=%d, updated=%d, disabled=%d", syncedCount, updatedCount, disabledCount)
	return nil
}

// listSafeUsers retrieves all users from SaFE User CRD
func (s *UserSyncService) listSafeUsers(ctx context.Context) ([]primusSafeV1.User, error) {
	userList := &primusSafeV1.UserList{}
	if err := s.k8sClient.List(ctx, userList); err != nil {
		return nil, err
	}
	return userList.Items, nil
}

// syncUser syncs a single user from SaFE to Lens
// Returns true if a new user was created, false if updated
func (s *UserSyncService) syncUser(ctx context.Context, safeUser *primusSafeV1.User) (bool, error) {
	userFacade := cpdb.GetFacade().GetUser()

	// Check if user already exists in Lens (by username)
	existing, err := userFacade.GetByUsername(ctx, safeUser.Name)
	if err != nil && err != gorm.ErrRecordNotFound {
		return false, err
	}

	// Determine if user is admin based on roles
	isAdmin := safeUser.IsSystemAdmin() || safeUser.IsSystemAdminReadonly()

	// Determine user status
	status := "active"
	if safeUser.IsRestricted() {
		status = "disabled"
	}

	now := time.Now()

	// Check if existing user was found (handle GORM callback issue)
	if existing != nil && existing.ID != "" {
		// Update existing user if needed
		needsUpdate := false
		if existing.IsAdmin != isAdmin {
			existing.IsAdmin = isAdmin
			needsUpdate = true
		}
		if existing.Status != status {
			existing.Status = status
			needsUpdate = true
		}

		if needsUpdate {
			existing.UpdatedAt = now
			return false, userFacade.Update(ctx, existing)
		}
		return false, nil
	}

	// Create new user in Lens Control Plane
	user := &cpmodel.LensUsers{
		ID:        safeUser.Name, // Use SaFE username as ID
		Username:  safeUser.Name,
		AuthType:  AuthTypeSafe,
		Status:    status,
		IsAdmin:   isAdmin,
		IsRoot:    false, // SaFE users are never root
		CreatedAt: now,
		UpdatedAt: now,
	}

	return true, userFacade.Create(ctx, user)
}

// disableDeletedUsers marks users as disabled if they no longer exist in SaFE
func (s *UserSyncService) disableDeletedUsers(ctx context.Context, activeUsers []primusSafeV1.User) (int64, error) {
	// Build set of active usernames
	activeSet := make(map[string]bool)
	for _, u := range activeUsers {
		activeSet[u.Name] = true
	}

	// Get all SaFE-synced users from Lens (get up to 10000 users)
	userFacade := cpdb.GetFacade().GetUser()
	syncedUsers, _, err := userFacade.ListByAuthType(ctx, AuthTypeSafe, 0, 10000)
	if err != nil {
		return 0, err
	}

	// Disable users that are no longer in SaFE
	var disabledCount int64
	for _, user := range syncedUsers {
		if !activeSet[user.Username] && user.Status != "disabled" {
			user.Status = "disabled"
			user.UpdatedAt = time.Now()
			if err := userFacade.Update(ctx, user); err != nil {
				log.Errorf("Failed to disable user %s: %v", user.Username, err)
				continue
			}
			disabledCount++
		}
	}

	return disabledCount, nil
}
