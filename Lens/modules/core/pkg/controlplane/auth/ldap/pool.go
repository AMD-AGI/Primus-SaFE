// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package ldap

import (
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/go-ldap/ldap/v3"
)

// ConnectionPool manages a pool of LDAP connections
type ConnectionPool struct {
	config    *Config
	pool      chan *ldap.Conn
	mu        sync.Mutex
	closed    bool
	tlsConfig *tls.Config
}

// NewConnectionPool creates a new LDAP connection pool
func NewConnectionPool(config *Config) (*ConnectionPool, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	poolSize := config.GetPoolSize()

	pool := &ConnectionPool{
		config: config,
		pool:   make(chan *ldap.Conn, poolSize),
	}

	// Configure TLS if needed
	if config.UseSSL || config.UseStartTLS {
		pool.tlsConfig = &tls.Config{
			ServerName:         config.Host,
			InsecureSkipVerify: config.SkipTLSVerify,
		}
	}

	// Pre-populate the pool with connections
	for i := 0; i < poolSize; i++ {
		conn, err := pool.createConnection()
		if err != nil {
			// Close any connections we've created
			pool.Close()
			return nil, fmt.Errorf("failed to create initial connection: %w", err)
		}
		pool.pool <- conn
	}

	log.Infof("LDAP connection pool created with %d connections to %s:%d",
		poolSize, config.Host, config.GetPort())

	return pool, nil
}

// createConnection creates a new LDAP connection
func (p *ConnectionPool) createConnection() (*ldap.Conn, error) {
	var conn *ldap.Conn
	var err error

	address := fmt.Sprintf("%s:%d", p.config.Host, p.config.GetPort())
	timeout := time.Duration(p.config.GetConnTimeout()) * time.Second

	if p.config.UseSSL {
		// Connect with TLS
		conn, err = ldap.DialTLS("tcp", address, p.tlsConfig)
	} else {
		// Connect without TLS
		conn, err = ldap.DialURL(fmt.Sprintf("ldap://%s", address),
			ldap.DialWithDialer(&net.Dialer{Timeout: timeout}))
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to LDAP server: %w", err)
	}

	// Set connection timeout
	conn.SetTimeout(timeout)

	// Start TLS if configured (and not already using LDAPS)
	if p.config.UseStartTLS && !p.config.UseSSL {
		if err := conn.StartTLS(p.tlsConfig); err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to start TLS: %w", err)
		}
	}

	// Bind with service account
	if err := conn.Bind(p.config.BindDN, p.config.GetBindPassword()); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to bind with service account: %w", err)
	}

	return conn, nil
}

// Get retrieves a connection from the pool
func (p *ConnectionPool) Get() (*ldap.Conn, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, fmt.Errorf("connection pool is closed")
	}
	p.mu.Unlock()

	select {
	case conn := <-p.pool:
		// Verify connection is still valid
		if !p.isConnectionValid(conn) {
			conn.Close()
			// Create a new connection
			newConn, err := p.createConnection()
			if err != nil {
				return nil, err
			}
			return newConn, nil
		}
		return conn, nil
	default:
		// Pool is empty, create a new connection
		return p.createConnection()
	}
}

// Put returns a connection to the pool
func (p *ConnectionPool) Put(conn *ldap.Conn) {
	if conn == nil {
		return
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		conn.Close()
		return
	}
	p.mu.Unlock()

	// Re-bind with service account before returning to pool
	if err := conn.Bind(p.config.BindDN, p.config.GetBindPassword()); err != nil {
		// Connection is no longer valid, close it
		conn.Close()
		return
	}

	select {
	case p.pool <- conn:
		// Connection returned to pool
	default:
		// Pool is full, close the connection
		conn.Close()
	}
}

// isConnectionValid checks if a connection is still valid
func (p *ConnectionPool) isConnectionValid(conn *ldap.Conn) bool {
	// Try to perform a simple operation to check connection
	// We use a very short timeout for this check
	conn.SetTimeout(2 * time.Second)
	defer conn.SetTimeout(time.Duration(p.config.GetConnTimeout()) * time.Second)

	// Try to search for the base DN with a scope of base
	searchRequest := ldap.NewSearchRequest(
		p.config.BaseDN,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		1,
		2,
		false,
		"(objectClass=*)",
		[]string{"dn"},
		nil,
	)

	_, err := conn.Search(searchRequest)
	return err == nil
}

// Close closes all connections in the pool
func (p *ConnectionPool) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()

	close(p.pool)
	for conn := range p.pool {
		conn.Close()
	}

	log.Info("LDAP connection pool closed")
}

// Stats returns pool statistics
func (p *ConnectionPool) Stats() PoolStats {
	return PoolStats{
		Available: len(p.pool),
		PoolSize:  p.config.GetPoolSize(),
	}
}

// PoolStats contains pool statistics
type PoolStats struct {
	Available int `json:"available"`
	PoolSize  int `json:"poolSize"`
}
