// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

// FacadeInterface defines the Control Plane Facade interface
type FacadeInterface interface {
	// GetUser returns the User Facade interface
	GetUser() UserFacadeInterface
	// GetSession returns the Session Facade interface
	GetSession() SessionFacadeInterface
	// GetAuthProvider returns the AuthProvider Facade interface
	GetAuthProvider() AuthProviderFacadeInterface
	// GetSystemConfig returns the SystemConfig Facade interface
	GetSystemConfig() SystemConfigFacadeInterface
	// GetLoginAudit returns the LoginAudit Facade interface
	GetLoginAudit() LoginAuditFacadeInterface
}

// Facade is the unified entry point for Control Plane database operations
type Facade struct {
	User         UserFacadeInterface
	Session      SessionFacadeInterface
	AuthProvider AuthProviderFacadeInterface
	SystemConfig SystemConfigFacadeInterface
	LoginAudit   LoginAuditFacadeInterface
}

// NewFacade creates a new Control Plane Facade instance
func NewFacade() *Facade {
	return &Facade{
		User:         NewUserFacade(),
		Session:      NewSessionFacade(),
		AuthProvider: NewAuthProviderFacade(),
		SystemConfig: NewSystemConfigFacade(),
		LoginAudit:   NewLoginAuditFacade(),
	}
}

// GetUser returns the User Facade interface
func (f *Facade) GetUser() UserFacadeInterface {
	return f.User
}

// GetSession returns the Session Facade interface
func (f *Facade) GetSession() SessionFacadeInterface {
	return f.Session
}

// GetAuthProvider returns the AuthProvider Facade interface
func (f *Facade) GetAuthProvider() AuthProviderFacadeInterface {
	return f.AuthProvider
}

// GetSystemConfig returns the SystemConfig Facade interface
func (f *Facade) GetSystemConfig() SystemConfigFacadeInterface {
	return f.SystemConfig
}

// GetLoginAudit returns the LoginAudit Facade interface
func (f *Facade) GetLoginAudit() LoginAuditFacadeInterface {
	return f.LoginAudit
}

// Global default Facade instance
var defaultFacade = NewFacade()

// GetFacade returns the default Control Plane Facade instance
func GetFacade() FacadeInterface {
	return defaultFacade
}
