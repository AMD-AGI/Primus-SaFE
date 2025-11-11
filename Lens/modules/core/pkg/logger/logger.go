package logger

import (
	"context"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/common"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/conf"
	"github.com/go-logr/logr"
	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
)

type Logger interface {
	logr.LogSink
	// Init initialises options
	InitLogger(conf *conf.LogConfig) error
	// Options The Logger options
	Config() *conf.LogConfig
	// Fields set fields to always be logged
	WithFields(fields map[string]interface{}) Logger
	WithField(field string, data interface{}) Logger
	WithError(error) Logger
	// Log writes a log entry
	Log(level conf.Level, v ...interface{})
	// Logf writes a formatted log entry
	Logf(level conf.Level, format string, v ...interface{})
	// String returns the name of logger
	WithContext(ctx context.Context) Logger
	Fatalf(format string, args ...interface{})
	Fatal(args ...interface{})
	Errorf(format string, args ...interface{})
	Errorln(args ...interface{})
	Infof(format string, args ...interface{})
	Infoln(args ...interface{})
	Debugf(format string, args ...interface{})
	Debug(args ...interface{})
	Warningf(format string, args ...interface{})
	Warning(args ...interface{})
	Tracef(format string, args ...interface{})
	Trace(args ...interface{})
	AddContextHook(h common.ContextHook)
}

var registeredContextKey = []string{
	"platform",
	"method",
}

func RegisterContextKey(key string) {
	registeredContextKey = append(registeredContextKey, key) //TODO Lock
}

func transferDefaultFieldsFromContext(ctx context.Context) map[string]interface{} {
	result := map[string]interface{}{}
	for _, s := range registeredContextKey {
		if ctx.Value(s) != nil {
			result[s] = ctx.Value(s)
		}
	}
	// trace
	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		return result
	}
	if spanContext, ok := span.Context().(jaeger.SpanContext); ok {
		result["trace_id"] = spanContext.TraceID().String()
		result["span_id"] = spanContext.SpanID().String()
	}
	return result
}

func CtxToMap(ctx context.Context) map[string]interface{} {

	return nil
}
