// Package log provides an opiniated logging facility as it provides only 4 log levels.
package log

import (
	"fmt"
	"maps"
	"reflect"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/encoding/json"
)

// LogLevel represents a log level
type Level uint

var components = []string{}
var lock = sync.Mutex{}

func registerComponent(component string) {
	if len(component) == 0 {
		return
	}

	lock.Lock()
	defer lock.Unlock()

	if slices.Contains(components, component) {
		return
	}

	components = append(components, component)
}

func ListComponents() []string {
	lock.Lock()
	defer lock.Unlock()

	return slices.Clone(components)
}

const (
	Lsilent Level = 0
	Lerror  Level = 1
	Lwarn   Level = 2
	Linfo   Level = 3
	Ldebug  Level = 4
)

// String returns a string representing the log level.
func (level Level) String() string {
	names := []string{
		"SILENT",
		"ERROR",
		"WARN",
		"INFO",
		"DEBUG",
	}

	if level > Ldebug {
		return `¯\_(ツ)_/¯`
	}

	return names[level]
}

func (level *Level) MarshalJSON() ([]byte, error) {
	return json.Marshal(level.String())
}

type Fields map[string]interface{}

// Logger is an interface that provides means for writing log messages.
//
// There are 4 log levels available (debug, info, warn, error) with increasing
// severity. A message will be written to an output if the log level of the message
// has the same or a higher severity than the output. Otherwise it will be
// discarded.
//
// Different outputs should be added with the AddOutput function.
//
// The context is a string that represents who wrote the message.
type Logger interface {
	// WithOutput sets an output to the Logger. The messages are written to the
	// provided writer.
	WithOutput(w Writer) Logger

	// With returns a new Logger with the given context. The context may
	// printed along the message. This is up to the implementation.
	WithComponent(component string) Logger

	WithField(key string, value interface{}) Logger
	WithFields(fields Fields) Logger

	WithError(err error) Logger

	Log(format string, args ...interface{})

	// Debug writes a message with the debug log level to all registered outputs.
	// The message will be written according to fmt.Printf(). The detail field will
	// be reset to nil.
	Debug() Logger

	// Info writes a message with the info log level to all registered outputs.
	// The message will be written according to fmt.Printf(). The detail field will
	// be reset to nil.
	Info() Logger

	// Warn writes a message with the warn log level to all registered outputs.
	// The message will be written according to fmt.Printf(). The detail field will
	// be reset to nil.
	Warn() Logger

	// Error writes a message with the error log level to all registered outputs.
	// The message will be written according to fmt.Printf(). The detail field will
	// be reset to nil.
	Error() Logger

	// Write implements the io.Writer interface such that it can be used in e.g. the
	// the log/Logger facility. Messages will be printed with debug level.
	Write(p []byte) (int, error)

	Close()
}

// logger is an implementation of the Logger interface.
type logger struct {
	output     Writer
	component  string
	modulePath string
}

// New returns an implementation of the Logger interface.
func New(component string) Logger {
	registerComponent(component)

	l := &logger{
		component: component,
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		l.modulePath = info.Path
	}

	return l
}

func (l *logger) Close() {
	l.output.Close()
}

func (l *logger) clone() *logger {
	clone := &logger{
		output:     l.output,
		component:  l.component,
		modulePath: l.modulePath,
	}

	return clone
}

func (l *logger) WithOutput(w Writer) Logger {
	clone := l.clone()
	clone.output = w

	return clone
}

func (l *logger) WithField(key string, value interface{}) Logger {
	return newEvent(l).WithField(key, value)
}

func (l *logger) WithFields(f Fields) Logger {
	return newEvent(l).WithFields(f)
}

func (l *logger) WithError(err error) Logger {
	return newEvent(l).WithError(err)
}

func (l *logger) WithComponent(component string) Logger {
	clone := l.clone()
	clone.component = component

	registerComponent(component)

	return clone
}

func (l *logger) Log(format string, args ...interface{}) {
	e := newEvent(l)

	e.Log(format, args...)
}

func (l *logger) Debug() Logger {
	return newEvent(l).Debug()
}

func (l *logger) Info() Logger {
	return newEvent(l).Info()
}

func (l *logger) Warn() Logger {
	return newEvent(l).Warn()
}

func (l *logger) Error() Logger {
	return newEvent(l).Error()
}

func (l *logger) Write(p []byte) (int, error) {
	return newEvent(l).Write(p)
}

type Event struct {
	logger *logger

	Time      time.Time
	Level     Level
	Component string
	Caller    string
	Message   string

	err string

	Data Fields
}

func newEvent(l *logger) Logger {
	e := &Event{
		logger:    l,
		Component: l.component,
		Data:      map[string]interface{}{},
	}

	return e
}

func (e *Event) Close() {
	e.logger.Close()
}

func (e *Event) WithOutput(w Writer) Logger {
	return e.logger.WithOutput(w)
}

func (e *Event) WithComponent(component string) Logger {
	clone := e.clone()
	clone.Component = component

	registerComponent(component)

	return clone
}

func (e *Event) Log(format string, args ...interface{}) {
	_, file, line, _ := runtime.Caller(1)
	file = strings.TrimPrefix(file, e.logger.modulePath)

	n := e.clone()

	n.logger = nil
	n.Time = time.Now()
	n.Caller = fmt.Sprintf("%s:%d", file, line)

	if n.Level == Lsilent {
		n.Level = Ldebug
	}

	if len(format) != 0 {
		if len(args) == 0 {
			n.Message = format
		} else {
			n.Message = fmt.Sprintf(format, args...)
		}
	}

	if e.logger.output != nil {
		e.logger.output.Write(n)
	}
}

func (e *Event) clone() *Event {
	return &Event{
		Time:      e.Time,
		Caller:    e.Caller,
		logger:    e.logger,
		Level:     e.Level,
		Component: e.Component,
		Message:   e.Message,
		err:       e.err,
		Data:      maps.Clone(e.Data),
	}
}

func (e *Event) WithField(key string, value interface{}) Logger {
	return e.WithFields(Fields{
		key: value,
	})
}

const maxFields = 1024

func (e *Event) WithFields(f Fields) Logger {
	if maxFields-len(e.Data)-len(f) < 0 {
		return e
	}

	data := make(Fields, len(e.Data)+len(f))
	for k, v := range e.Data {
		data[k] = v
	}

	fieldErr := e.err
	for k, v := range f {
		isErrField := false
		if t := reflect.TypeOf(v); t != nil {
			switch {
			case t.Kind() == reflect.Func, t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Func:
				isErrField = true
			}
		}
		if isErrField {
			tmp := fmt.Sprintf("can not add field %q", k)
			if fieldErr != "" {
				fieldErr = e.err + ", " + tmp
			} else {
				fieldErr = tmp
			}
		} else {
			data[k] = v
		}
	}

	return &Event{
		logger:    e.logger,
		Component: e.Component,
		Level:     e.Level,
		err:       fieldErr,
		Data:      data,
	}
}

func (e *Event) WithError(err error) Logger {
	if err == nil {
		return e
	}

	return e.WithFields(Fields{
		"error": err,
	})
}

func (e *Event) Debug() Logger {
	clone := e.clone()
	clone.Level = Ldebug

	return clone
}

func (e *Event) Info() Logger {
	clone := e.clone()
	clone.Level = Linfo

	return clone
}

func (e *Event) Warn() Logger {
	clone := e.clone()
	clone.Level = Lwarn

	return clone
}

func (e *Event) Error() Logger {
	clone := e.clone()
	clone.Level = Lerror

	return clone
}

func (e *Event) Write(p []byte) (int, error) {
	e.Log("%s", strings.TrimSpace(string(p)))

	return len(p), nil
}
