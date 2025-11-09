package api

import (
	"encoding/json"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/gin-gonic/gin"
	"math"
	"strconv"
	"time"
)

type GrafanaMetricsPoint struct {
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Timestamp int64   `json:"timestamp"`
}

func GetWorkloadTrainingPerformance(ctx *gin.Context) {
	workloadUid := ctx.Param("uid")
	if workloadUid == "" {
		_ = ctx.Error(errors.NewError().
			WithCode(errors.RequestParameterInvalid).
			WithMessage("workloadUid is required"))
		return
	}

	startStr := ctx.Query("start")
	endStr := ctx.Query("end")

	startMs, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil {
		_ = ctx.Error(errors.NewError().
			WithCode(errors.RequestParameterInvalid).
			WithMessage("invalid start time format"))
		return
	}

	endMs, err := strconv.ParseInt(endStr, 10, 64)
	if err != nil {
		_ = ctx.Error(errors.NewError().
			WithCode(errors.RequestParameterInvalid).
			WithMessage("invalid end time format"))
		return
	}

	startTime := time.UnixMilli(startMs)
	endTime := time.UnixMilli(endMs)
	performances, err := database.GetFacade().GetTraining().ListTrainingPerformanceByWorkloadIdsAndTimeRange(
		ctx, []string{workloadUid}, startTime, endTime,
	)
	if err != nil {
		_ = ctx.Error(errors.NewError().WithCode(errors.InternalError).WithMessage(err.Error()))
		return
	}

	series := map[string]*model.GrafanaMetricsSeries{}
	for _, p := range performances {
		for key, value := range p.Performance {
			valueFloat := convertToFloat(value)
			if math.IsNaN(valueFloat) {
				continue
			}
			if _, ok := series[key]; !ok {
				series[key] = &model.GrafanaMetricsSeries{
					Name:   key,
					Points: [][2]float64{},
				}
			}
			series[key].Points = append(series[key].Points,
				[2]float64{valueFloat, float64(p.CreatedAt.UnixMilli())})
		}
	}

	// flatten to rows for Infinity
	var flatResult = []GrafanaMetricsPoint{}
	for name, s := range series {
		for _, pt := range s.Points {
			flatResult = append(flatResult, GrafanaMetricsPoint{
				Metric:    name,
				Value:     pt[0],
				Timestamp: int64(pt[1]),
			})
		}
	}

	ctx.JSON(200, flatResult)
}

func convertToFloat(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int8:
		return float64(v)
	case int16:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case uint:
		return float64(v)
	case uint8:
		return float64(v)
	case uint16:
		return float64(v)
	case uint32:
		return float64(v)
	case uint64:
		return float64(v)
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}
