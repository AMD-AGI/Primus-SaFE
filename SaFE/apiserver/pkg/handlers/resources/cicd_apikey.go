/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"strings"

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

// attachCICDEncryptedAPIKey resolves the caller's API key for ScaleSet/GithubRunner CICD
// workloads, encrypts it with the shared crypto secret, and stores the ciphertext on the
// workload annotation. Ephemeral runners inherit USER_APIKEY from the scale-set template.
// Bearer ak-... is used as-is; a session/token is exchanged for a primary/platform key
// (identity in external mode, SaFE DB in builtin mode).
func (h *Handler) attachCICDEncryptedAPIKey(c *gin.Context, workload *v1.Workload, requestUser *v1.User) error {
	if !commonworkload.IsCICDScalingRunnerSet(workload) && !commonworkload.IsCICDGithubRunner(workload) {
		return nil
	}
	plain, err := resolveCICDAPIKeyPlain(c, requestUser)
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

// resolveCICDAPIKeyPlain returns the plaintext API key for the CreateWorkload caller.
func resolveCICDAPIKeyPlain(c *gin.Context, requestUser *v1.User) (string, error) {
	bearer := extractBearerToken(c.GetHeader("Authorization"))
	if strings.HasPrefix(bearer, "ak-") {
		return bearer, nil
	}
	cookieToken, _ := c.Cookie(authority.CookieToken)
	userType := c.GetString(common.UserType)
	if userType == "" {
		if ut, err := c.Cookie(authority.CookieUserType); err == nil {
			userType = ut
		}
	}
	if commonconfig.GetAuthMode() == commonconfig.AuthModeExternal {
		return identity.FetchPrimaryAPIKey(c.Request.Context(), bearer, cookieToken, userType)
	}
	if requestUser == nil || requestUser.Name == "" {
		return "", commonerrors.NewUnauthorized("the request user is unknown")
	}
	if !commonconfig.IsDBEnable() {
		return "", commonerrors.NewInternalError("database is not enabled")
	}
	db := dbclient.NewClient()
	if db == nil {
		return "", commonerrors.NewInternalError("database client is not available")
	}
	return apikey.GetOrCreatePlatformKey(c.Request.Context(), db, requestUser.Name, v1.GetUserName(requestUser), true)
}

func extractBearerToken(authHeader string) string {
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
