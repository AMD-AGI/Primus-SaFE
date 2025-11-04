package sql

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/sql/metrics"
	"github.com/AMD-AGI/primus-lens/core/pkg/trace"
	"gorm.io/gorm/logger"
)

type NullLogger struct {
}

func (n NullLogger) LogMode(level logger.LogLevel) logger.Interface {
	return n
}

func (n NullLogger) Info(ctx context.Context, s string, i ...interface{}) {
	log.GlobalLogger().Debugf("[GormLog][Info] %s %v", s, i)
}

func (n NullLogger) Warn(ctx context.Context, s string, i ...interface{}) {
	log.GlobalLogger().Debugf("[GormLog][Warn] %s %v", s, i)
}

func (n NullLogger) Error(ctx context.Context, s string, i ...interface{}) {
	log.GlobalLogger().Debugf("[GormLog][Error] %s %v", s, i)
}

func (n NullLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	sql, rowAffected := fc()
	timeUse := time.Now().Sub(begin)
	errStr := ""
	if err != nil {
		errStr = fmt.Sprintf("Error :%s", err.Error())
	}
	metrics.RecordQueryTimeUse(trace.TrimPackagePrefixes(trace.GetNearestCaller(2)), timeUse.Seconds())
	if timeUse.Seconds() > 5 {
		log.GlobalLogger().Warningf("[GormLog][Warning] %s.Slow SQL: %s. RowsAffected %d. Timeuse %d ms. %s", begin, sql, rowAffected, timeUse.Milliseconds(), errStr)
		metrics.RecordSlowQueryDuration(trace.TrimPackagePrefixes(trace.GetNearestCaller(2)), timeUse.Seconds())
		return
	}
	log.GlobalLogger().Debugf("[GormLog][Debug] %s. SQL: %s. RowsAffected %d. Timeuse %d ms. %s", begin, sql, rowAffected, timeUse.Milliseconds(), errStr)
}
