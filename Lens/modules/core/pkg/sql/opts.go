package sql

import (
	"github.com/AMD-AGI/primus-lens/core/pkg/sql/callbacks"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func WithLogger(logger logger.Interface) opts {
	return func(db *gorm.DB) {
		db.Logger = logger
	}
}

func WithErrorStackCallback() opts {
	return func(db *gorm.DB) {
		db.Callback().Delete().Register("error", callbacks.CreateErrorSolveCallback(callbacks.ErrorWithStack))
		db.Callback().Create().Register("error", callbacks.CreateErrorSolveCallback(callbacks.ErrorWithStack))
		db.Callback().Update().Register("error", callbacks.CreateErrorSolveCallback(callbacks.ErrorWithStack))
		db.Callback().Query().Register("error", callbacks.CreateErrorSolveCallback(callbacks.ErrorWithStack))
		db.Callback().Raw().Register("error", callbacks.CreateErrorSolveCallback(callbacks.ErrorWithStack))
		db.Callback().Row().Register("error", callbacks.CreateErrorSolveCallback(callbacks.ErrorWithStack))
	}
}

func WithRestErrorStackCallback() opts {
	return func(db *gorm.DB) {
		db.Callback().Delete().Register("error", callbacks.CreateErrorSolveCallback(callbacks.RestErrorWithStack))
		db.Callback().Create().Register("error", callbacks.CreateErrorSolveCallback(callbacks.RestErrorWithStack))
		db.Callback().Update().Register("error", callbacks.CreateErrorSolveCallback(callbacks.RestErrorWithStack))
		db.Callback().Query().Register("error", callbacks.CreateErrorSolveCallback(callbacks.RestErrorWithStack))
		db.Callback().Raw().Register("error", callbacks.CreateErrorSolveCallback(callbacks.RestErrorWithStack))
		db.Callback().Row().Register("error", callbacks.CreateErrorSolveCallback(callbacks.RestErrorWithStack))
	}
}
