package cluster

import (
	"io"
	golog "log"
	"os"

	"github.com/datarhei/core/v16/log"

	hclog "github.com/hashicorp/go-hclog"
)

// Mimic the hashicorp logger

type hclogger struct {
	logger log.Logger
	level  hclog.Level
	name   string
	args   []interface{}
}

func NewLogger(logger log.Logger, lvl hclog.Level) hclog.Logger {
	return &hclogger{
		logger: logger,
		level:  lvl,
	}
}

func (l *hclogger) Log(level hclog.Level, msg string, args ...interface{}) {
	fields := l.argFields(args...)

	logger := l.logger.WithFields(fields)

	switch level {
	case hclog.Trace:
		logger = logger.Debug()
	case hclog.Debug:
		logger = logger.Debug()
	case hclog.Info:
		logger = logger.Info()
	case hclog.Warn:
		logger = logger.Warn()
	case hclog.Error:
		logger = logger.Error()
	}

	logger.Log(msg)
}

func (l *hclogger) argFields(args ...interface{}) log.Fields {
	if len(args)%2 != 0 {
		args = args[:len(args)-1]
	}

	fields := log.Fields{}

	for i := 0; i < len(args); i = i + 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}

		fields[key] = args[i+1]
	}

	return fields
}

func (l *hclogger) Trace(msg string, args ...interface{}) {
	l.Log(hclog.Trace, msg, args...)
}

func (l *hclogger) Debug(msg string, args ...interface{}) {
	l.Log(hclog.Debug, msg, args...)
}

func (l *hclogger) Info(msg string, args ...interface{}) {
	l.Log(hclog.Info, msg, args...)
}

func (l *hclogger) Warn(msg string, args ...interface{}) {
	l.Log(hclog.Warn, msg, args...)
}

func (l *hclogger) Error(msg string, args ...interface{}) {
	l.Log(hclog.Error, msg, args...)
}

func (l *hclogger) IsTrace() bool {
	return l.level == hclog.Trace
}

func (l *hclogger) IsDebug() bool {
	return l.level == hclog.Debug
}

func (l *hclogger) IsInfo() bool {
	return l.level == hclog.Info
}

func (l *hclogger) IsWarn() bool {
	return l.level == hclog.Warn
}

func (l *hclogger) IsError() bool {
	return l.level == hclog.Error
}

func (l *hclogger) ImpliedArgs() []interface{} {
	return l.args
}

func (l *hclogger) With(args ...interface{}) hclog.Logger {
	fields := l.argFields(args)

	return &hclogger{
		logger: l.logger.WithFields(fields),
		level:  l.level,
		name:   l.name,
		args:   args,
	}
}

func (l *hclogger) Name() string {
	return l.name
}

func (l *hclogger) Named(name string) hclog.Logger {
	if len(l.name) != 0 {
		name = l.name + "." + name
	}

	return &hclogger{
		logger: l.logger.WithField("logname", name),
		level:  l.level,
		name:   name,
		args:   l.args,
	}
}

func (l *hclogger) ResetNamed(name string) hclog.Logger {
	return &hclogger{
		logger: l.logger.WithField("logname", name),
		level:  l.level,
		name:   name,
		args:   l.args,
	}
}

func (l *hclogger) SetLevel(lvl hclog.Level) {
	level := log.Linfo

	switch lvl {
	case hclog.Trace:
		level = log.Ldebug
	case hclog.Debug:
		level = log.Ldebug
	case hclog.Info:
		level = log.Linfo
	case hclog.Warn:
		level = log.Lwarn
	case hclog.Error:
		level = log.Lerror
	}

	l.logger = l.logger.WithOutput(log.NewSyncWriter(log.NewConsoleWriter(os.Stderr, level, true)))
	l.level = lvl
}

func (l *hclogger) GetLevel() hclog.Level {
	return l.level
}

func (l *hclogger) StandardLogger(opts *hclog.StandardLoggerOptions) *golog.Logger {
	return golog.New(io.Discard, "", 0)
}

func (l *hclogger) StandardWriter(opts *hclog.StandardLoggerOptions) io.Writer {
	return io.Discard
}
