// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

func ReceiveHttpLogs(ctx *gin.Context) {
	bodyData, err := ctx.GetRawData()
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	err = parseAndProcess(ctx, bodyData, ctx.ClientIP())
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, nil))
}

func parseAndProcess(ctx context.Context, bodyData []byte, agentIp string) error {
	var logData []*PodLog
	err := json.Unmarshal(bodyData, &logData)
	if err != nil {
		return errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid log data").WithError(err)
	}
	for _, logItem := range logData {
		timeStamp, err := convertTimeStamp(logItem.Date)
		if err != nil {
			log.GlobalLogger().WithContext(ctx).Errorf("convert timestamp failed, err: %v", err)
			continue
		}
		logItem.Time = timeStamp
		latency := time.Since(timeStamp).Seconds()
		host := agentIp
		if logItem.Kubernetes != nil {
			host = logItem.Kubernetes.Host
		}
		if logItem.Log != "" && logItem.Message == "" {
			logItem.Message = logItem.Log
		}
		logConsumeLatencySummary.WithLabelValues(host).Observe(latency)
		logConsumeLatencyHistogram.WithLabelValues(host).Observe(latency)
		err = executeWorkloadLog(ctx, logItem)
		if err != nil {
			log.GlobalLogger().WithContext(ctx).Errorf("execute workload log failed, err: %v", err)
		}
	}
	return nil
}
func convertTimeStamp(timestamp float64) (time.Time, error) {
	timeStr := fmt.Sprintf("%f", timestamp)
	parts := strings.Split(timeStr, ".")
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("invalid timestamp format")
	}

	// Convert the seconds part to int64
	seconds, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid seconds part: %v", err)
	}

	// Convert the fractional part to nanoseconds
	fractional, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid fractional part: %v", err)
	}

	// Convert the timestamp to time.Time
	t := time.Unix(seconds, fractional)

	return t, nil
}
