package logrus

import (
	"context"
	"fmt"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/common"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/conf"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/formatter"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"os"
	"runtime"
	"strings"
)

type Close func()

const callerKey = "caller"

type LogrusWrapper struct {
	contextHooks []common.ContextHook
	logrusLogger *logrus.Logger
	fields       logrus.Fields
	callerSkip   int
	fnClose      Close // fnClose is used to release resources when Close()
	conf         *conf.LogConfig
}

func (wrapper *LogrusWrapper) Init(info logr.RuntimeInfo) {
	// wrapper.callerSkip += info.CallDepth
}

func (wrapper *LogrusWrapper) Enabled(level int) bool {
	if wrapper == nil || wrapper.conf == nil {
		return true
	}
	return int(wrapper.conf.Level) < level
}

func (wrapper *LogrusWrapper) Info(level int, msg string, keysAndValues ...any) {
	((wrapper.WithValues(keysAndValues)).(*LogrusWrapper)).Infoln(msg)
}

func (wrapper *LogrusWrapper) Error(err error, msg string, keysAndValues ...any) {
	((wrapper.WithValues(keysAndValues)).(*LogrusWrapper)).WithError(err).Errorln(msg)
}

func (wrapper *LogrusWrapper) WithValues(keysAndValues ...any) logr.LogSink {
	// Default: one key per value
	var orphanValue any
	if len(keysAndValues)%2 != 0 {
		orphanValue = keysAndValues[len(keysAndValues)-1]
		keysAndValues = keysAndValues[:len(keysAndValues)-1]
	}
	for i := 0; i < len(keysAndValues)/2; i += 2 {
		wrapper.WithField(fmt.Sprintf("%+v", keysAndValues[i]), keysAndValues[i+1])
	}
	if orphanValue != nil {
		wrapper.WithField("orphan", orphanValue)
	}
	return wrapper
}

func (wrapper *LogrusWrapper) WithName(name string) logr.LogSink {
	return wrapper.WithField("name", name)
}

func NewLogrusWrapper(options *conf.LogConfig) (*LogrusWrapper, error) {
	logrusLogger := logrus.New()
	logrusLogger.SetLevel(convertToLogrusLevel(options.Level))
	logrusLogger.SetFormatter(convertToLogrusFormatter(options.Formatter))
	writers := make([]io.Writer, 0, len(options.Outputs))
	closers := []io.Closer{}
	for _, v := range options.Outputs {
		if v.Type == conf.OutputTypeStdout {
			writers = append(writers, os.Stdout)
		}

		if v.Type == conf.OutputTypeStderr {
			writers = append(writers, os.Stderr)
		}

		if v.Type == conf.OutputTypeFile && v.File != nil {
			file, err := os.OpenFile(*v.File, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0666)
			if err != nil {
				return nil, errors.Wrapf(err, "fail to open file %v", v.File)
			}

			writers = append(writers, file)
			closers = append(closers, file)
		}

		if v.Type == conf.OutputTypeRotateFile && v.RotateFile != nil {

			v.RotateFile.SetDefaults()

			rotateFile := &lumberjack.Logger{
				Filename:   v.RotateFile.FileName,
				MaxSize:    v.RotateFile.MaxSize,
				MaxBackups: v.RotateFile.MaxBackups,
				MaxAge:     v.RotateFile.MaxAge,
				Compress:   v.RotateFile.Compress,
			}

			writers = append(writers, rotateFile)
			closers = append(closers, rotateFile)
		}

	}

	mv := io.MultiWriter(writers...)
	logrusLogger.SetOutput(mv)
	if options.SetReportCaller {
		logrusLogger.SetReportCaller(true)
	}
	wrapper := &LogrusWrapper{
		conf:         options,
		logrusLogger: logrusLogger,
		callerSkip:   3,
		fields:       map[string]interface{}{},
	}
	wrapper.fnClose = func() {
		for _, v := range closers {
			v.Close()
		}
	}
	return wrapper, nil
}

func (wrapper *LogrusWrapper) injectBaseKeys() *logrus.Entry {
	baseFields := common.GetBaseFields()
	baseFields[callerKey] = caller(wrapper.callerSkip)
	return wrapper.logrusLogger.WithFields(baseFields)
}

func (wrapper *LogrusWrapper) Logf(level conf.Level, format string, v ...interface{}) {
	wrapper.logrusLogger.Logf(conf.LoggerToLogrusLevel(level), format, v...)
}
func (wrapper *LogrusWrapper) Log(level conf.Level, v ...interface{}) {
	wrapper.logrusLogger.Log(conf.LoggerToLogrusLevel(level), v...)
}

func (wrapper *LogrusWrapper) Debugf(format string, args ...interface{}) {
	baseEntry := wrapper.injectBaseKeys()
	if len(wrapper.fields) > 0 {
		baseEntry.WithFields(wrapper.fields).Debugf(format, args...)
	} else {
		baseEntry.Debugf(format, args...)
	}
}

func (wrapper *LogrusWrapper) Config() *conf.LogConfig {
	return wrapper.conf
}

func (wrapper *LogrusWrapper) InitLogger(conf *conf.LogConfig) error {
	wrapper.conf = conf
	return nil
}

func (wrapper *LogrusWrapper) Tracef(format string, args ...interface{}) {
	baseEntry := wrapper.injectBaseKeys()
	if len(wrapper.fields) > 0 {
		baseEntry.WithFields(wrapper.fields).Tracef(format, args...)
	} else {
		baseEntry.Tracef(format, args...)
	}
}

func (wrapper *LogrusWrapper) Infof(format string, args ...interface{}) {
	baseEntry := wrapper.injectBaseKeys()
	if len(wrapper.fields) > 0 {
		baseEntry.WithFields(wrapper.fields).Infof(format, args...)
	} else {
		baseEntry.Infof(format, args...)
	}
}

func (wrapper *LogrusWrapper) Warningf(format string, args ...interface{}) {
	baseEntry := wrapper.injectBaseKeys()
	if len(wrapper.fields) > 0 {
		baseEntry.WithFields(wrapper.fields).Warningf(format, args...)
	} else {
		baseEntry.Warningf(format, args...)
	}
}

func (wrapper *LogrusWrapper) Errorf(format string, args ...interface{}) {
	baseEntry := wrapper.injectBaseKeys()
	if len(wrapper.fields) > 0 {
		baseEntry.WithFields(wrapper.fields).Errorf(format, args...)
	} else {
		baseEntry.Errorf(format, args...)
	}
}

func (wrapper *LogrusWrapper) Fatalf(format string, args ...interface{}) {
	baseEntry := wrapper.injectBaseKeys()
	if len(wrapper.fields) > 0 {
		baseEntry.WithFields(wrapper.fields).Fatalf(format, args...)
	} else {
		baseEntry.Fatalf(format, args...)
	}
}

func (wrapper *LogrusWrapper) Trace(args ...interface{}) {
	baseEntry := wrapper.injectBaseKeys()
	if len(wrapper.fields) > 0 {
		baseEntry.WithFields(wrapper.fields).Trace(args...)
	} else {
		baseEntry.Trace(args...)
	}
}

func (wrapper *LogrusWrapper) Debug(args ...interface{}) {
	baseEntry := wrapper.injectBaseKeys()
	if len(wrapper.fields) > 0 {
		baseEntry.WithFields(wrapper.fields).Debug(args...)
	} else {
		baseEntry.Debug(args...)
	}
}

func (wrapper *LogrusWrapper) Infoln(args ...interface{}) {
	baseEntry := wrapper.injectBaseKeys()
	if len(wrapper.fields) > 0 {
		baseEntry.WithFields(wrapper.fields).Info(args...)
	} else {
		baseEntry.Info(args...)
	}
}

func (wrapper *LogrusWrapper) Warning(args ...interface{}) {
	baseEntry := wrapper.injectBaseKeys()
	if len(wrapper.fields) > 0 {
		baseEntry.WithFields(wrapper.fields).Warning(args...)
	} else {
		baseEntry.Warning(args...)
	}
}

func (wrapper *LogrusWrapper) Errorln(args ...interface{}) {
	baseEntry := wrapper.injectBaseKeys()
	if len(wrapper.fields) > 0 {
		baseEntry.WithFields(wrapper.fields).Error(args...)
	} else {
		baseEntry.Error(args...)
	}
}

func (wrapper *LogrusWrapper) Fatal(args ...interface{}) {
	baseEntry := wrapper.injectBaseKeys()
	if len(wrapper.fields) > 0 {
		baseEntry.WithFields(wrapper.fields).Fatal(args...)
	} else {
		baseEntry.Fatal(args...)
	}
}

func (warpper *LogrusWrapper) WithError(err error) logger.Logger {
	return warpper.WithField("error", err)
}

func (wrapper *LogrusWrapper) WithField(key string, value interface{}) logger.Logger {
	result := &LogrusWrapper{
		logrusLogger: wrapper.logrusLogger,
		conf:         wrapper.conf,
		callerSkip:   wrapper.callerSkip,
	}

	// Merge wrapper.fields and key:value into data
	// key:value may override existing items in wrapper.fields
	data := make(map[string]interface{}, len(wrapper.fields)+1)
	for k, v := range wrapper.fields {
		data[k] = v
	}
	data[key] = value

	result.fields = logrus.Fields(data)
	return result
}

func (wrapper *LogrusWrapper) WithFields(fields map[string]interface{}) logger.Logger {
	result := &LogrusWrapper{
		conf:         wrapper.conf,
		logrusLogger: wrapper.logrusLogger,
		fnClose:      wrapper.fnClose,
		fields:       map[string]interface{}{},
	}

	// Merge wrapper.fields and key:value into data
	// fields may override existing items in wrapper.fields
	data := make(map[string]interface{}, len(wrapper.fields)+len(fields))
	for k, v := range wrapper.fields {
		data[k] = v
	}
	for k, v := range fields {
		data[k] = v
	}

	result.fields = logrus.Fields(data)
	return result
}

func (wrapper *LogrusWrapper) WithContext(ctx context.Context) logger.Logger {
	result := &LogrusWrapper{
		logrusLogger: wrapper.logrusLogger,
		fnClose:      wrapper.fnClose,
		callerSkip:   wrapper.callerSkip,
		conf:         wrapper.conf,
		fields:       map[string]interface{}{},
	}
	for s, i := range wrapper.fields {
		result.fields[s] = i
	}
	if ctx != nil {
		fields := wrapper.execContextHooks(ctx)
		for _, field := range fields {
			result.fields[field.Key] = field.Interface
		}
	}
	return result
}

func (l *LogrusWrapper) execContextHooks(ctx context.Context) []common.Field {
	if ctx == nil || len(l.contextHooks) == 0 {
		return nil
	}
	var fields []common.Field
	for _, h := range l.contextHooks {
		fields = append(fields, h(ctx)...)
	}
	return fields
}
func (wrapper *LogrusWrapper) AddCallerSkip(skip int) logger.Logger {
	return &LogrusWrapper{
		conf:         wrapper.conf,
		logrusLogger: wrapper.logrusLogger,
		fnClose:      wrapper.fnClose,
		callerSkip:   wrapper.callerSkip + skip,
		fields:       wrapper.fields,
	}
}

func (wrapper *LogrusWrapper) AddContextHook(h common.ContextHook) {
	wrapper.contextHooks = append(wrapper.contextHooks, h)
}

func (wrapper *LogrusWrapper) Flush() {
	// Refer to: https://github.com/sirupsen/logrus/issues/435
	// logrus doesn't provide any flush or sync method. If you
	// don't want to lost message, just sleep a time before exit
	wrapper.Infof("If you see this message and `Flush` is the last logger's method called in your application, it means no log lost.")
}

func (wrapper *LogrusWrapper) Close() {
	if wrapper.fnClose != nil {
		wrapper.fnClose()
	}
}

func convertToLogrusLevel(l conf.Level) logrus.Level {
	var level logrus.Level
	switch l {
	case conf.TraceLevel:
		level = logrus.TraceLevel
	case conf.DebugLevel:
		level = logrus.DebugLevel
	case conf.InfoLevel:
		level = logrus.InfoLevel
	case conf.WarnLevel:
		level = logrus.WarnLevel
	case conf.ErrorLevel:
		level = logrus.ErrorLevel
	case conf.FatalLevel:
		level = logrus.FatalLevel
	default:
		level = logrus.DebugLevel
	}

	return level
}

func convertToLogrusFormatter(f conf.Formatter) logrus.Formatter {
	var fmt logrus.Formatter
	switch f {
	case conf.JSONFormater:
		fmt = &logrus.JSONFormatter{TimestampFormat: "2006-01-02T15:04:05.000Z0700"}
	case conf.ConsoleFormater:
		fmt = &logrus.TextFormatter{TimestampFormat: "2006-01-02T15:04:05.000"}
	case conf.StructuredFormater:
		fmt = &formatter.StructuredFormatter{}
	default:
		fmt = &logrus.TextFormatter{TimestampFormat: "2006-01-02T15:04:05.000"}
	}
	return fmt
}

func caller(skip int) string {
	const callerOffset = 1
	_, file, line, _ := runtime.Caller(skip + callerOffset)
	idx := strings.LastIndexByte(file, '/')
	if idx == -1 {
		return fmt.Sprintf("%s:%d", file, line)
	}
	idx = strings.LastIndexByte(file[:idx], '/')
	if idx == -1 {
		return fmt.Sprintf("%s:%d", file, line)
	}
	return fmt.Sprintf("%s:%d", file[idx+1:], line)
}
