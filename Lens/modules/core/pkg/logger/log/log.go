package log

import (
	"fmt"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/conf"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/logrus"
	"os"
)

type Fields map[string]interface{}

var globalLogger logger.Logger
var ErrorLoggerNotInitialize = fmt.Errorf("Logger not initialized")

func init() {
	_ = InitGlobalLogger(conf.DefaultConfig())
}

func InitGlobalLogger(conf *conf.LogConfig) (err error) {
	switch conf.Core {
	default:
		globalLogger, err = logrus.NewLogrusWrapper(conf)
		if err != nil {
			return err
		}
	}
	return nil
}

func GlobalLogger() logger.Logger {
	if globalLogger == nil {
		panic(ErrorLoggerNotInitialize)
	}
	return globalLogger
}

func SetGlobalLogger(logger logger.Logger) {
	globalLogger = logger
}

func Logf(level conf.Level, format string, v ...interface{}) {
	GlobalLogger().Logf(level, format, v...)
}

func Log(level conf.Level, v ...interface{}) {
	GlobalLogger().Log(level, v...)
}

func Info(args ...interface{}) {
	Log(conf.InfoLevel, args...)
}

func Infof(template string, args ...interface{}) {
	Logf(conf.InfoLevel, template, args...)
}

func Trace(args ...interface{}) {
	Log(conf.TraceLevel, args...)
}

func Tracef(template string, args ...interface{}) {
	Logf(conf.TraceLevel, template, args...)
}

func Debug(args ...interface{}) {
	Log(conf.DebugLevel, args...)
}

func Debugf(template string, args ...interface{}) {
	Logf(conf.DebugLevel, template, args...)
}

func Warn(args ...interface{}) {
	Log(conf.WarnLevel, args...)
}

func Warnf(template string, args ...interface{}) {
	Logf(conf.WarnLevel, template, args...)
}

func Error(args ...interface{}) {
	Log(conf.ErrorLevel, args...)
}

func Errorf(template string, args ...interface{}) {
	Logf(conf.ErrorLevel, template, args...)
}

func Fatal(args ...interface{}) {
	Log(conf.FatalLevel, args...)
	os.Exit(1)
}

func Fatalf(template string, args ...interface{}) {
	Logf(conf.FatalLevel, template, args...)
	os.Exit(1)
}
