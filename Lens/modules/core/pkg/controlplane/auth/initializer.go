// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"context"
	"encoding/json"
	"os"
	"time"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// RootPasswordSecretName is the name of the secret containing root password
	RootPasswordSecretName = "primus-lens-root-credentials"
	// RootPasswordSecretKey is the key in the secret containing the password
	RootPasswordSecretKey = "password"
	// RootPasswordSecretNamespaceEnv is the env var for the namespace
	RootPasswordSecretNamespaceEnv = "POD_NAMESPACE"
	// DefaultSecretNamespace is the default namespace for the secret
	DefaultSecretNamespace = "primus-lens"
)

// Initializer handles system initialization
type Initializer struct {
	facade       cpdb.FacadeInterface
	safeDetector *SafeDetector
	k8sClient    client.Client
}

// InitializationStatus represents the current initialization status
type InitializationStatus struct {
	SystemInitialized bool     `json:"systemInitialized"`
	AuthInitialized   bool     `json:"authInitialized"`
	AuthMode          AuthMode `json:"authMode"`
	SafeDetected      bool     `json:"safeDetected"`
	SuggestedMode     AuthMode `json:"suggestedMode"`
	RootUserExists    bool     `json:"rootUserExists"`
}

// InitializationResult represents the result of initialization
type InitializationResult struct {
	Success             bool     `json:"success"`
	AuthMode            AuthMode `json:"authMode"`
	RootPasswordGenerated bool   `json:"rootPasswordGenerated"`
	RootPassword        string   `json:"rootPassword,omitempty"` // Only returned if generated
	Message             string   `json:"message"`
}

// NewInitializer creates a new Initializer
func NewInitializer(safeDetector *SafeDetector) *Initializer {
	return &Initializer{
		facade:       cpdb.GetFacade(),
		safeDetector: safeDetector,
		k8sClient:    nil,
	}
}

// NewInitializerWithK8s creates a new Initializer with K8s client
func NewInitializerWithK8s(safeDetector *SafeDetector, k8sClient client.Client) *Initializer {
	return &Initializer{
		facade:       cpdb.GetFacade(),
		safeDetector: safeDetector,
		k8sClient:    k8sClient,
	}
}

// SetK8sClient sets the K8s client for the initializer
func (i *Initializer) SetK8sClient(k8sClient client.Client) {
	i.k8sClient = k8sClient
}

// GetStatus returns the current initialization status
func (i *Initializer) GetStatus(ctx context.Context) (*InitializationStatus, error) {
	status := &InitializationStatus{}

	// Check system.initialized
	systemInit, err := i.getConfigBool(ctx, ConfigKeySystemInitialized)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	status.SystemInitialized = systemInit

	// Check auth.initialized
	authInit, err := i.getConfigBool(ctx, ConfigKeyAuthInitialized)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	status.AuthInitialized = authInit

	// Get current auth mode
	authMode, err := i.getConfigString(ctx, ConfigKeyAuthMode)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if authMode != "" {
		status.AuthMode = AuthMode(authMode)
	} else {
		status.AuthMode = AuthModeNone
	}

	// Check if root user exists
	rootUser, err := i.facade.GetUser().GetByUsername(ctx, RootUsername)
	status.RootUserExists = err == nil && rootUser != nil

	// Detect SaFE environment
	if i.safeDetector != nil {
		result, err := i.safeDetector.DetectSaFE(ctx)
		if err == nil {
			status.SafeDetected = result.ShouldEnableSafeMode
		}
	}

	// Suggest mode based on detection
	if status.SafeDetected {
		status.SuggestedMode = AuthModeSaFE
	} else {
		status.SuggestedMode = AuthModeNone
	}

	return status, nil
}

// Initialize performs the initial system setup
func (i *Initializer) Initialize(ctx context.Context, opts *InitializeOptions) (*InitializationResult, error) {
	result := &InitializationResult{}

	// Check if already initialized
	status, err := i.GetStatus(ctx)
	if err != nil {
		return nil, err
	}

	if status.SystemInitialized {
		result.Success = false
		result.Message = "System is already initialized"
		return result, nil
	}

	// Create root user
	rootPassword, generated, err := i.createRootUser(ctx, opts.RootPassword)
	if err != nil {
		return nil, err
	}

	result.RootPasswordGenerated = generated
	if generated {
		result.RootPassword = rootPassword
		log.Warnf("Root user created with generated password: %s", rootPassword)
		log.Warn("Please change the root password on first login!")
	}

	// Determine auth mode
	authMode := opts.AuthMode
	if authMode == "" {
		if status.SafeDetected {
			authMode = AuthModeSaFE
		} else {
			authMode = AuthModeNone
		}
	}

	// Set auth mode
	if err := i.setConfigString(ctx, ConfigKeyAuthMode, string(authMode), "auth"); err != nil {
		return nil, err
	}

	// Mark system as initialized
	if err := i.setConfigBool(ctx, ConfigKeySystemInitialized, true, "system"); err != nil {
		return nil, err
	}

	// Mark auth as initialized
	if err := i.setConfigBool(ctx, ConfigKeyAuthInitialized, true, "auth"); err != nil {
		return nil, err
	}

	// If SaFE detected and mode is safe, enable integration
	if authMode == AuthModeSaFE && status.SafeDetected {
		if err := i.setConfigBool(ctx, ConfigKeySafeIntegrationEnabled, true, "auth"); err != nil {
			return nil, err
		}
		if err := i.setConfigBool(ctx, ConfigKeySafeIntegrationAutoDetected, true, "auth"); err != nil {
			return nil, err
		}
	}

	result.Success = true
	result.AuthMode = authMode
	result.Message = "System initialized successfully"

	log.Infof("System initialized with auth mode: %s", authMode)

	return result, nil
}

// InitializeOptions contains options for initialization
type InitializeOptions struct {
	// AuthMode to set (if empty, will be auto-detected)
	AuthMode AuthMode `json:"authMode"`
	// RootPassword to set (if empty, will be generated)
	RootPassword string `json:"rootPassword"`
}

// createRootUser creates the root user if it doesn't exist
func (i *Initializer) createRootUser(ctx context.Context, password string) (string, bool, error) {
	// Check if root user already exists in database
	existingUser, err := i.facade.GetUser().GetByUsername(ctx, RootUsername)
	if err == nil && existingUser != nil {
		log.Info("Root user already exists in database, skipping creation")
		return "", false, nil // Root user already exists
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return "", false, err
	}

	// Get password from various sources in order of priority:
	// 1. Provided password parameter
	// 2. Environment variable LENS_ROOT_PASSWORD
	// 3. Existing Kubernetes Secret (created by another pod)
	// 4. Generate new password and store in Secret
	passwordGenerated := false
	passwordFromSecret := false

	if password == "" {
		password = os.Getenv(EnvRootPassword)
	}

	// If still no password, try to get from existing Secret (another pod may have created it)
	if password == "" && i.k8sClient != nil {
		secretPassword, exists, err := i.getPasswordFromSecret(ctx)
		if err != nil {
			log.Warnf("Failed to check existing secret: %v", err)
		} else if exists {
			password = secretPassword
			passwordFromSecret = true
			log.Info("Using password from existing Kubernetes Secret (created by another pod)")
		}
	}

	// If still no password, generate one
	if password == "" {
		var err error
		password, err = GenerateRandomPassword()
		if err != nil {
			return "", false, err
		}
		passwordGenerated = true
		log.Info("Generated random password for root user")
	}

	// Hash password
	passwordHash, err := HashPassword(password)
	if err != nil {
		return "", false, err
	}

	// Create root user in database
	now := time.Now()
	rootUser := &model.LensUsers{
		ID:                 RootUserID,
		Username:           RootUsername,
		Email:              RootEmail,
		DisplayName:        RootDisplayName,
		AuthType:           string(AuthTypeLocal),
		Status:             string(UserStatusActive),
		IsAdmin:            true,
		IsRoot:             true,
		PasswordHash:       passwordHash,
		MustChangePassword: passwordGenerated, // Force password change if generated
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := i.facade.GetUser().Create(ctx, rootUser); err != nil {
		// If creation failed due to duplicate (race condition with another pod),
		// that's ok - another pod created it first
		if isDuplicateError(err) {
			log.Info("Root user was created by another pod, skipping")
			return "", false, nil
		}
		return "", false, err
	}

	log.Info("Root user created successfully")

	// If password was generated and not from Secret, store it in Secret
	if passwordGenerated && !passwordFromSecret && i.k8sClient != nil {
		if err := i.savePasswordToSecret(ctx, password); err != nil {
			log.Errorf("Failed to save password to Secret: %v", err)
			// Still return the password so it can be logged as fallback
			log.Warnf("=======================================================")
			log.Warnf("FAILED TO SAVE PASSWORD TO SECRET")
			log.Warnf("Root password (save this securely): %s", password)
			log.Warnf("=======================================================")
		} else {
			log.Infof("Root password saved to Kubernetes Secret: %s/%s", i.getSecretNamespace(), RootPasswordSecretName)
		}
	}

	if passwordGenerated {
		return password, true, nil
	}
	return "", false, nil
}

// getPasswordFromSecret retrieves the root password from Kubernetes Secret
func (i *Initializer) getPasswordFromSecret(ctx context.Context) (string, bool, error) {
	if i.k8sClient == nil {
		return "", false, nil
	}

	secret := &corev1.Secret{}
	err := i.k8sClient.Get(ctx, types.NamespacedName{
		Namespace: i.getSecretNamespace(),
		Name:      RootPasswordSecretName,
	}, secret)

	if err != nil {
		if errors.IsNotFound(err) {
			return "", false, nil // Secret doesn't exist
		}
		return "", false, err
	}

	// Secret exists, get the password
	if passwordBytes, ok := secret.Data[RootPasswordSecretKey]; ok {
		return string(passwordBytes), true, nil
	}

	return "", false, nil
}

// savePasswordToSecret saves the root password to a Kubernetes Secret
func (i *Initializer) savePasswordToSecret(ctx context.Context, password string) error {
	if i.k8sClient == nil {
		return nil
	}

	namespace := i.getSecretNamespace()

	// Try to create the secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      RootPasswordSecretName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "primus-lens",
				"app.kubernetes.io/component": "auth",
				"app.kubernetes.io/managed-by": "primus-lens-api",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			RootPasswordSecretKey: []byte(password),
			"username":            []byte(RootUsername),
			"created_at":          []byte(time.Now().Format(time.RFC3339)),
		},
	}

	err := i.k8sClient.Create(ctx, secret)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// Secret already exists (created by another pod), that's ok
			log.Info("Root password Secret already exists (created by another pod)")
			return nil
		}
		return err
	}

	return nil
}

// getSecretNamespace returns the namespace for the root password secret
func (i *Initializer) getSecretNamespace() string {
	// Try to get from environment (pod's namespace)
	if ns := os.Getenv(RootPasswordSecretNamespaceEnv); ns != "" {
		return ns
	}
	return DefaultSecretNamespace
}

// isDuplicateError checks if the error is a duplicate key error
func isDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "duplicate key") || contains(errStr, "UNIQUE constraint")
}

// contains checks if s contains substr
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Helper methods for config operations

func (i *Initializer) getConfigBool(ctx context.Context, key string) (bool, error) {
	config, err := i.facade.GetSystemConfig().Get(ctx, key)
	if err != nil {
		return false, err
	}

	// Parse the value from ExtType
	if val, ok := config.Value["value"]; ok {
		switch v := val.(type) {
		case bool:
			return v, nil
		case string:
			return v == "true", nil
		}
	}

	// Try direct bool value
	if val, ok := config.Value[""].(bool); ok {
		return val, nil
	}

	return false, nil
}

func (i *Initializer) setConfigBool(ctx context.Context, key string, value bool, category string) error {
	valueJSON, _ := json.Marshal(value)
	config := &model.LensSystemConfigs{
		Key:       key,
		Value:     model.ExtType{"value": value},
		Category:  category,
		UpdatedAt: time.Now(),
	}

	_ = valueJSON // unused but for clarity
	return i.facade.GetSystemConfig().Set(ctx, config)
}

func (i *Initializer) getConfigString(ctx context.Context, key string) (string, error) {
	config, err := i.facade.GetSystemConfig().Get(ctx, key)
	if err != nil {
		return "", err
	}

	// Parse the value from ExtType
	if val, ok := config.Value["value"]; ok {
		if str, ok := val.(string); ok {
			return str, nil
		}
	}

	return "", nil
}

func (i *Initializer) setConfigString(ctx context.Context, key string, value string, category string) error {
	config := &model.LensSystemConfigs{
		Key:       key,
		Value:     model.ExtType{"value": value},
		Category:  category,
		UpdatedAt: time.Now(),
	}

	return i.facade.GetSystemConfig().Set(ctx, config)
}

// EnsureInitialized ensures the system is initialized
// This should be called during application startup
func (i *Initializer) EnsureInitialized(ctx context.Context) error {
	status, err := i.GetStatus(ctx)
	if err != nil {
		return err
	}

	if status.SystemInitialized {
		log.Info("System already initialized")
		return nil
	}

	// Auto-initialize with defaults
	log.Info("System not initialized, performing auto-initialization...")

	result, err := i.Initialize(ctx, &InitializeOptions{})
	if err != nil {
		return err
	}

	if result.RootPasswordGenerated {
		log.Warnf("=======================================================")
		log.Warnf("ROOT USER CREATED WITH GENERATED PASSWORD")
		log.Warnf("Username: %s", RootUsername)
		if i.k8sClient != nil {
			log.Warnf("Password stored in Secret: %s/%s", i.getSecretNamespace(), RootPasswordSecretName)
			log.Warnf("To retrieve: kubectl get secret %s -n %s -o jsonpath='{.data.password}' | base64 -d",
				RootPasswordSecretName, i.getSecretNamespace())
		} else {
			// Fallback: log the password only if K8s client is not available
			log.Warnf("Password: %s", result.RootPassword)
			log.Warnf("WARNING: K8s client not available, password logged above. Please secure it!")
		}
		log.Warnf("Please change this password on first login!")
		log.Warnf("=======================================================")
	}

	return nil
}
