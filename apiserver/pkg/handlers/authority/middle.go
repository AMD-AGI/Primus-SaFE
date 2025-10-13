package authority

import (
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/gin-gonic/gin"
	"strings"
)

func Prepare(_ ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(common.Name, strings.TrimSpace(c.Param(common.Name)))
	}
}

func Authorize(_ ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		err := ParseCookie(c)
		if err != nil {
			apiutils.AbortWithApiError(c, err)
		}
	}
}
