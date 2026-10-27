/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package fake provides in-memory implementations of the identityexchange
// stores for tests of this package and of the code that integrates it.
package fake

import (
	"context"
	"database/sql"
	"sync"

	"github.com/lib/pq"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identityexchange"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// Store is an in-memory identityexchange.Store.
type Store struct {
	mu       sync.Mutex
	Clients  map[string]*dbclient.ServiceClient
	Bindings []*dbclient.IdentityBinding
	Audit    []*dbclient.AuditLog
	nextId   int64
}

var _ identityexchange.Store = (*Store)(nil)

// NewStore returns an empty store.
func NewStore() *Store { return &Store{Clients: map[string]*dbclient.ServiceClient{}} }

func (s *Store) GetServiceClient(_ context.Context, id string) (*dbclient.ServiceClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.Clients[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	cp := *c
	return &cp, nil
}

func (s *Store) InsertServiceClient(_ context.Context, sc *dbclient.ServiceClient) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextId++
	sc.Id = s.nextId
	cp := *sc
	s.Clients[sc.ClientId] = &cp
	return nil
}

func (s *Store) ListServiceClients(context.Context) ([]*dbclient.ServiceClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*dbclient.ServiceClient
	for _, c := range s.Clients {
		cp := *c
		out = append(out, &cp)
	}
	return out, nil
}

func (s *Store) SetServiceClientDisabled(_ context.Context, id string, disabled bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.Clients[id]
	if ok {
		c.Disabled = disabled
	}
	return ok, nil
}

func (s *Store) GetIdentityBinding(_ context.Context, clientId, issuer, subject string) (*dbclient.IdentityBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range s.Bindings {
		if !b.Deleted && b.ClientId == clientId && b.Issuer == issuer && b.Subject == subject {
			cp := *b
			return &cp, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (s *Store) HasIdentityBindingForUser(_ context.Context, clientId, userId string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range s.Bindings {
		if !b.Deleted && b.ClientId == clientId && b.UserId == userId {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) InsertIdentityBinding(_ context.Context, b *dbclient.IdentityBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.Bindings {
		if !e.Deleted && e.ClientId == b.ClientId && e.Issuer == b.Issuer && e.Subject == b.Subject {
			return &pq.Error{Code: "23505"}
		}
	}
	s.nextId++
	b.Id = s.nextId
	cp := *b
	s.Bindings = append(s.Bindings, &cp)
	return nil
}

func (s *Store) ListIdentityBindingsByUser(_ context.Context, userId string) ([]*dbclient.IdentityBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*dbclient.IdentityBinding
	for _, b := range s.Bindings {
		if !b.Deleted && b.UserId == userId {
			cp := *b
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (s *Store) DeleteIdentityBinding(_ context.Context, id int64, userId string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range s.Bindings {
		if b.Id == id && b.UserId == userId && !b.Deleted {
			b.Deleted = true
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) InsertAuditLog(_ context.Context, l *dbclient.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *l
	s.Audit = append(s.Audit, &cp)
	return nil
}

// AuditRows returns a copy of the audit rows written so far.
func (s *Store) AuditRows() []*dbclient.AuditLog {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*dbclient.AuditLog(nil), s.Audit...)
}

// Users is an in-memory identityexchange.UserStore.
type Users struct {
	mu sync.Mutex
	M  map[string]*v1.User
}

var _ identityexchange.UserStore = (*Users)(nil)

// NewUsers returns an empty user store.
func NewUsers() *Users { return &Users{M: map[string]*v1.User{}} }

// Add registers a user.
func (u *Users) Add(id, name, email string, roles ...v1.UserRole) *v1.User {
	user := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: id, Annotations: map[string]string{
		v1.UserNameAnnotation: name, v1.UserEmailAnnotation: email,
	}}}
	user.Spec.Roles = roles
	u.mu.Lock()
	u.M[id] = user
	u.mu.Unlock()
	return user
}

func (u *Users) GetUser(_ context.Context, id string) (*v1.User, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if user, ok := u.M[id]; ok {
		return user.DeepCopy(), nil
	}
	return nil, identityexchange.ErrUserNotFound
}
