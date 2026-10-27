/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	// TServiceClient stores the service clients allowed to exchange tokens.
	TServiceClient = "service_clients"
	// TIdentityBinding stores the user-confirmed (client, issuer, subject) -> user bindings.
	TIdentityBinding = "identity_bindings"
)

// ServiceClient is a registered caller of the token exchange endpoint.
type ServiceClient struct {
	Id               int64          `db:"id"`
	ClientId         string         `db:"client_id"`
	Name             string         `db:"name"`
	SecretHash       string         `db:"secret_hash"`
	AuthMethod       string         `db:"auth_method"`       // "client_secret_basic" (reserved: "private_key_jwt")
	AllowedAudiences string         `db:"allowed_audiences"` // JSON array
	AllowedScopes    string         `db:"allowed_scopes"`    // JSON array
	Whitelist        string         `db:"whitelist"`         // JSON array of IP/CIDR, empty = no restriction
	MaxTtlSeconds    int            `db:"max_ttl_seconds"`   // upper bound of the token lifetime for this client
	Disabled         bool           `db:"disabled"`
	CreatedBy        string         `db:"created_by"`
	CreationTime     pq.NullTime    `db:"creation_time"`
	PublicKey        sql.NullString `db:"public_key"` // reserved for private_key_jwt
}

// IdentityBinding maps an external identity (issuer, subject) seen by a service
// client to the SaFE user who confirmed it.
type IdentityBinding struct {
	Id           int64       `db:"id"`
	ClientId     string      `db:"client_id"`
	Issuer       string      `db:"issuer"`
	Subject      string      `db:"subject"`
	UserId       string      `db:"user_id"`
	CreationTime pq.NullTime `db:"creation_time"`
	Deleted      bool        `db:"deleted"`
	DeletionTime pq.NullTime `db:"deletion_time"`
}

// InsertServiceClient registers a service client.
func (c *Client) InsertServiceClient(ctx context.Context, sc *ServiceClient) error {
	if sc == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := generateCommand(*sc, `INSERT INTO `+TServiceClient+` (%s) VALUES (%s) RETURNING id`, "id")
	rows, err := db.NamedQueryContext(ctx, cmd, sc)
	if err != nil {
		return fmt.Errorf("failed to insert service client: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return rows.Scan(&sc.Id)
	}
	return nil
}

// GetServiceClient returns the service client with the given client id.
func (c *Client) GetServiceClient(ctx context.Context, clientId string) (*ServiceClient, error) {
	if clientId == "" {
		return nil, commonerrors.NewBadRequest("client id is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}
	var sc ServiceClient
	if err = db.GetContext(ctx, &sc, `SELECT * FROM `+TServiceClient+` WHERE client_id=$1 LIMIT 1`, clientId); err != nil {
		return nil, err
	}
	return &sc, nil
}

// ListServiceClients returns every registered service client.
func (c *Client) ListServiceClients(ctx context.Context) ([]*ServiceClient, error) {
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}
	var list []*ServiceClient
	err = db.SelectContext(ctx, &list, `SELECT * FROM `+TServiceClient+` ORDER BY id`)
	return list, err
}

// SetServiceClientDisabled enables or disables a service client. It reports
// whether a row was changed.
func (c *Client) SetServiceClientDisabled(ctx context.Context, clientId string, disabled bool) (bool, error) {
	db, err := c.getDB()
	if err != nil {
		return false, err
	}
	res, err := db.ExecContext(ctx, `UPDATE `+TServiceClient+` SET disabled=$1 WHERE client_id=$2`, disabled, clientId)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// InsertIdentityBinding stores a binding. The unique index over the active
// rows turns a second claim of the same (client, issuer, subject) into an error.
func (c *Client) InsertIdentityBinding(ctx context.Context, b *IdentityBinding) error {
	if b == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := generateCommand(*b, `INSERT INTO `+TIdentityBinding+` (%s) VALUES (%s) RETURNING id`, "id")
	rows, err := db.NamedQueryContext(ctx, cmd, b)
	if err != nil {
		return fmt.Errorf("failed to insert identity binding: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return rows.Scan(&b.Id)
	}
	return nil
}

// GetIdentityBinding returns the active binding of an external identity for a client.
func (c *Client) GetIdentityBinding(ctx context.Context, clientId, issuer, subject string) (*IdentityBinding, error) {
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}
	var b IdentityBinding
	err = db.GetContext(ctx, &b, `SELECT * FROM `+TIdentityBinding+
		` WHERE client_id=$1 AND issuer=$2 AND subject=$3 AND deleted=false LIMIT 1`, clientId, issuer, subject)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// HasIdentityBindingForUser reports whether the user has an active binding to the client.
func (c *Client) HasIdentityBindingForUser(ctx context.Context, clientId, userId string) (bool, error) {
	db, err := c.getDB()
	if err != nil {
		return false, err
	}
	var n int
	err = db.GetContext(ctx, &n, `SELECT COUNT(*) FROM `+TIdentityBinding+
		` WHERE client_id=$1 AND user_id=$2 AND deleted=false`, clientId, userId)
	return n > 0, err
}

// ListIdentityBindingsByUser returns the user's active bindings.
func (c *Client) ListIdentityBindingsByUser(ctx context.Context, userId string) ([]*IdentityBinding, error) {
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}
	var list []*IdentityBinding
	err = db.SelectContext(ctx, &list, `SELECT * FROM `+TIdentityBinding+
		` WHERE user_id=$1 AND deleted=false ORDER BY id`, userId)
	return list, err
}

// DeleteIdentityBinding soft-deletes one of the user's bindings. It reports
// whether a row was changed; a binding owned by someone else is never touched.
func (c *Client) DeleteIdentityBinding(ctx context.Context, id int64, userId string) (bool, error) {
	db, err := c.getDB()
	if err != nil {
		return false, err
	}
	res, err := db.ExecContext(ctx, `UPDATE `+TIdentityBinding+
		` SET deleted=true, deletion_time=now() WHERE id=$1 AND user_id=$2 AND deleted=false`, id, userId)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
