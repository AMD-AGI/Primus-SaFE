/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identity"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/apikey"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	safecrypto "github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

// attachCICDEncryptedAPIKey resolves the workload owner's platform API key for ScaleSet/GithubRunner
// CICD workloads, encrypts it with the shared crypto secret, and stores the ciphertext on the
// workload annotation. Ephemeral runners inherit USER_APIKEY from the scale-set template.
func (h *Handler) attachCICDEncryptedAPIKey(c *gin.Context, workload *v1.Workload, requestUser *v1.User) error {
	if !commonworkload.IsCICDProxyRoot(workload) {
		return nil
	}
	plain, err := resolveCICDAPIKeyPlain(c, workload, requestUser)
	if err != nil {
		return err
	}
	if plain == "" {
		return commonerrors.NewBadRequest("failed to resolve an API key for the CICD workload")
	}
	ciphertext, err := safecrypto.NewCrypto().Encrypt([]byte(plain))
	if err != nil {
		klog.ErrorS(err, "failed to encrypt CICD API key", "workload", workload.Name)
		return commonerrors.NewInternalError("failed to encrypt the CICD API key")
	}
	v1.SetAnnotation(workload, v1.UserEncryptedAPIKeyAnnotation, ciphertext)
	return nil
}

// resolveCICDAPIKeyPlain returns the plaintext API key the workload owner's runners use. Only the
// owner may create or resume the workload. Builtin mode reads or creates the owner's platform key
// in the SaFE database. External mode uses the owner's own credential: an ak- bearer as-is, or a
// session exchanged with identity for the platform key.
func resolveCICDAPIKeyPlain(c *gin.Context, workload *v1.Workload, requestUser *v1.User) (string, error) {
	ownerId := v1.GetUserId(workload)
	if ownerId == "" || requestUser == nil || requestUser.Name != ownerId {
		return "", commonerrors.NewForbidden("only the owner can create or resume a CICD runner set")
	}
	if commonconfig.GetAuthMode() != commonconfig.AuthModeExternal {
		return cicdPlatformKeyForUser(c.Request.Context(), ownerId, v1.GetUserName(workload))
	}
	bearer := authority.GetBearerToken(c)
	if strings.HasPrefix(bearer, authority.ApiKeyPrefix) {
		return bearer, nil
	}
	// The user type identity judges is the one the request carried (cookie or UserType header),
	// not the one the external chain recorded on the context.
	var userType string
	cookieToken, err := c.Cookie(authority.CookieToken)
	if err == nil {
		userType, _ = c.Cookie(authority.CookieUserType)
	} else {
		userType = c.GetHeader("UserType")
	}
	return identity.FetchPlatformAPIKey(c.Request.Context(), bearer, cookieToken, userType)
}

// cicdPlatformKeyForUser reads or creates a user's platform key in the SaFE database.
// A package var so tests can replace the database lookup.
var cicdPlatformKeyForUser = func(ctx context.Context, userId, userName string) (string, error) {
	if !commonconfig.IsDBEnable() {
		return "", commonerrors.NewInternalError("database is not enabled")
	}
	db := dbclient.NewClient()
	if db == nil {
		return "", commonerrors.NewInternalError("database client is not available")
	}
	return apikey.GetOrCreatePlatformKey(ctx, db, userId, userName, true)
}
