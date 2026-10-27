/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identityexchange

import (
	"context"
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// Store is the persistence the package needs. *dbclient.Client implements it.
// "Not found" is reported as sql.ErrNoRows by the Get methods.
type Store interface {
	GetServiceClient(ctx context.Context, clientId string) (*dbclient.ServiceClient, error)
	InsertServiceClient(ctx context.Context, sc *dbclient.ServiceClient) error
	ListServiceClients(ctx context.Context) ([]*dbclient.ServiceClient, error)
	SetServiceClientDisabled(ctx context.Context, clientId string, disabled bool) (bool, error)

	GetIdentityBinding(ctx context.Context, clientId, issuer, subject string) (*dbclient.IdentityBinding, error)
	HasIdentityBindingForUser(ctx context.Context, clientId, userId string) (bool, error)
	InsertIdentityBinding(ctx context.Context, b *dbclient.IdentityBinding) error
	ListIdentityBindingsByUser(ctx context.Context, userId string) ([]*dbclient.IdentityBinding, error)
	DeleteIdentityBinding(ctx context.Context, id int64, userId string) (bool, error)

	InsertAuditLog(ctx context.Context, auditLog *dbclient.AuditLog) error
}

var _ Store = (*dbclient.Client)(nil)

// ErrUserNotFound is returned by UserStore when the user does not exist.
var ErrUserNotFound = errors.New("user not found")

// UserStore reads user records. Roles and restriction state come from here on
// every request; nothing about them is cached in a token.
type UserStore interface {
	GetUser(ctx context.Context, userId string) (*v1.User, error)
}

// k8sUserStore reads the User custom resources.
type k8sUserStore struct{ client.Reader }

// NewUserStore returns a UserStore backed by the User custom resources.
func NewUserStore(r client.Reader) UserStore { return &k8sUserStore{r} }

func (s *k8sUserStore) GetUser(ctx context.Context, userId string) (*v1.User, error) {
	user := &v1.User{}
	if err := s.Get(ctx, client.ObjectKey{Name: userId}, user); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}
