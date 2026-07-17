/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"testing"
)

func TestPingUninitialized(t *testing.T) {
	// nil receiver -> guard returns an error, never dereferences db.
	var c *Client
	if err := c.Ping(context.Background()); err == nil {
		t.Error("Ping on nil client should return an error")
	}

	// non-nil client without a db connection -> guard returns an error too.
	if err := (&Client{}).Ping(context.Background()); err == nil {
		t.Error("Ping on client without db should return an error")
	}
}
