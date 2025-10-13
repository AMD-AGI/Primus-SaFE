package ssh_handlers

import (
	"fmt"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/gin-gonic/gin"
)

func InitWebShellRouters(e *gin.Engine, h *SshHandler) {
	group := e.Group(common.PrimusRouterCustomRootPath, authority.Authorize(), authority.Prepare())
	{
		group.GET(fmt.Sprintf("workloads/:%s/pods/:%s/webshell", common.Name, common.PodId), h.WebShell)
	}
}
