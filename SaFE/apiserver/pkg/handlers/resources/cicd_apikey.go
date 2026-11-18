/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identity"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/apikey"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
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
	cryptoClient := cicdCrypto()
	if cryptoClient == nil {
		return commonerrors.NewInternalError("the crypto client is not available")
	}
	ciphertext, err := cryptoClient.Encrypt([]byte(plain))
	if err != nil {
		klog.ErrorS(err, "failed to encrypt CICD API key", "workload", workload.Name)
		return commonerrors.NewInternalError("failed to encrypt the CICD API key")
	}
	v1.SetAnnotation(workload, v1.UserEncryptedAPIKeyAnnotation, ciphertext)
	return nil
}

// resolveCICDAPIKeyPlain returns the workload owner's platform API key for the runners. Only the
// owner may create or resume the workload. Builtin mode, and a request the built-in mode at the end
// of the external chain authenticated, read or create the owner's platform key in the SaFE database.
// Otherwise external mode exchanges the credential the chain authenticated (the bearer, else the
// cookie) with identity for the owner's platform key.
func resolveCICDAPIKeyPlain(c *gin.Context, workload *v1.Workload, requestUser *v1.User) (string, error) {
	ownerId := v1.GetUserId(workload)
	if ownerId == "" || requestUser == nil || requestUser.Name != ownerId {
		return "", commonerrors.NewForbidden("only the owner can create or resume a CICD runner set")
	}
	if commonconfig.GetAuthMode() != commonconfig.AuthModeExternal || !authenticatedByExternalChain(c) {
		return cicdPlatformKeyForUser(c.Request.Context(), ownerId, v1.GetUserName(workload))
	}
	// The user type identity judges is the one the request carried with that credential,
	// not the one the external chain recorded on the context.
	if bearer := authority.GetBearerToken(c); bearer != "" {
		return identity.FetchPlatformAPIKey(c.Request.Context(), ownerId, bearer, "", c.GetHeader("UserType"))
	}
	cookieToken, _ := c.Cookie(authority.CookieToken)
	userType, _ := c.Cookie(authority.CookieUserType)
	return identity.FetchPlatformAPIKey(c.Request.Context(), ownerId, "", cookieToken, userType)
}

// authenticatedByExternalChain reports whether an authenticator of the external chain, not the
// built-in mode at its end, accepted the request.
func authenticatedByExternalChain(c *gin.Context) bool {
	userType := c.GetString(common.UserType)
	return userType == authority.UserTypeExternal || userType == authority.UserTypeDelegated
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

// cicdCrypto returns the shared crypto client, nil when the crypto key is unusable.
// A package var so tests can replace it.
var cicdCrypto = safecrypto.NewCrypto
