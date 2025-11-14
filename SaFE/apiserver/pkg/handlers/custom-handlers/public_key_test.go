/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// TestCvtToPublicKeyResponse tests the conversion from database PublicKey to response item
func TestCvtToPublicKeyResponse(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		pubKey   *dbclient.PublicKey
		validate func(*testing.T, types.ListPublicKeysResponseItem)
	}{
		{
			name: "complete public key",
			pubKey: &dbclient.PublicKey{
				Id:          101,
				UserId:      "user-123",
				Description: "My SSH key for production",
				PublicKey:   "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDexample user@host",
				Status:      true,
				CreateTime:  pq.NullTime{Time: now, Valid: true},
				UpdateTime:  pq.NullTime{Time: now.Add(1 * time.Hour), Valid: true},
			},
			validate: func(t *testing.T, result types.ListPublicKeysResponseItem) {
				assert.Equal(t, int64(101), result.Id)
				assert.Equal(t, "user-123", result.UserId)
				assert.Equal(t, "My SSH key for production", result.Description)
				assert.Contains(t, result.PublicKey, "ssh-rsa")
				assert.True(t, result.Status)
				assert.NotEmpty(t, result.CreateTime)
				assert.NotEmpty(t, result.UpdateTime)
			},
		},
		{
			name: "public key without times",
			pubKey: &dbclient.PublicKey{
				Id:          202,
				UserId:      "user-456",
				Description: "Development key",
				PublicKey:   "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAexample dev@machine",
				Status:      true,
				CreateTime:  pq.NullTime{Valid: false},
				UpdateTime:  pq.NullTime{Valid: false},
			},
			validate: func(t *testing.T, result types.ListPublicKeysResponseItem) {
				assert.Equal(t, int64(202), result.Id)
				assert.Equal(t, "user-456", result.UserId)
				assert.Equal(t, "Development key", result.Description)
				assert.Contains(t, result.PublicKey, "ssh-ed25519")
				assert.True(t, result.Status)
				assert.Empty(t, result.CreateTime)
				assert.Empty(t, result.UpdateTime)
			},
		},
		{
			name: "disabled public key",
			pubKey: &dbclient.PublicKey{
				Id:          303,
				UserId:      "user-789",
				Description: "Old key - disabled",
				PublicKey:   "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABold user@oldhost",
				Status:      false, // Disabled
				CreateTime:  pq.NullTime{Time: now.Add(-30 * 24 * time.Hour), Valid: true},
				UpdateTime:  pq.NullTime{Time: now, Valid: true},
			},
			validate: func(t *testing.T, result types.ListPublicKeysResponseItem) {
				assert.Equal(t, int64(303), result.Id)
				assert.Equal(t, "user-789", result.UserId)
				assert.Equal(t, "Old key - disabled", result.Description)
				assert.False(t, result.Status) // Disabled
			},
		},
		{
			name: "public key with empty description",
			pubKey: &dbclient.PublicKey{
				Id:          404,
				UserId:      "user-000",
				Description: "",
				PublicKey:   "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAgen user@host",
				Status:      true,
				CreateTime:  pq.NullTime{Time: now, Valid: true},
				UpdateTime:  pq.NullTime{Time: now, Valid: true},
			},
			validate: func(t *testing.T, result types.ListPublicKeysResponseItem) {
				assert.Equal(t, int64(404), result.Id)
				assert.Empty(t, result.Description)
				assert.True(t, result.Status)
			},
		},
		{
			name: "ECDSA public key",
			pubKey: &dbclient.PublicKey{
				Id:          505,
				UserId:      "user-ecdsa",
				Description: "ECDSA key for automation",
				PublicKey:   "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYexample automation@server",
				Status:      true,
				CreateTime:  pq.NullTime{Time: now, Valid: true},
				UpdateTime:  pq.NullTime{Time: now, Valid: true},
			},
			validate: func(t *testing.T, result types.ListPublicKeysResponseItem) {
				assert.Equal(t, int64(505), result.Id)
				assert.Equal(t, "user-ecdsa", result.UserId)
				assert.Contains(t, result.PublicKey, "ecdsa-sha2-nistp256")
				assert.Equal(t, "ECDSA key for automation", result.Description)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cvtToPublicKeyResponse(tt.pubKey)
			tt.validate(t, result)
		})
	}
}
