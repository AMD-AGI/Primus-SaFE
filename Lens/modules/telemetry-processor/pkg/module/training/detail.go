package training

import (
	"context"
	"github.com/AMD-AGI/primus-lens/core/pkg/model"
	"github.com/gin-gonic/gin"
)

func Log(ctx *gin.Context) {
	req := &model.TrainingLogEvent{}
	err := ctx.ShouldBindJSON(req)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

}

func processTrainingLogEvent(ctx context.Context, event *model.TrainingLogEvent) error {

}
