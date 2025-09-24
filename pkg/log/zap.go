/*
Copyright 2025 The KubeFlag Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package log

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	logzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	// DebugLevel is the debug log level, i.e. the most verbose.
	DebugLevel LogLevel = "debug"
	// InfoLevel is the default log level.
	InfoLevel LogLevel = "info"
	// ErrorLevel is a log level where only errors are logged.
	ErrorLevel LogLevel = "error"
)

type LogLevel string
type Format string

const (
	FormatJSON    Format = "JSON"
	FormatConsole Format = "Console"
)

var (
	// AllLogLevels is a slice of all available log levels.
	AllLogLevels = []LogLevel{DebugLevel, InfoLevel, ErrorLevel}
	// AllLogFormats is a slice of all available log formats.
	AllLogFormats = []Format{FormatJSON, FormatConsole}
)

func setCommonEncoderConfigOptions(encoderConfig *zapcore.EncoderConfig) {
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeDuration = zapcore.StringDurationEncoder
}

// MustNewZapLogger is like NewZapLogger but panics on invalid input.
func MustNewZapLogger(level LogLevel, format Format, additionalOpts ...logzap.Opts) logr.Logger {
	logger, err := NewZapLogger(level, format, additionalOpts...)
	utilruntime.Must(err)

	return logger
}

// NewZapLogger creates a new logr.Logger backed by Zap.
func NewZapLogger(level LogLevel, format Format, additionalOpts ...logzap.Opts) (logr.Logger, error) {
	var opts []logzap.Opts

	// map our log levels to zap levels
	var zapLevel zapcore.LevelEnabler

	switch level {
	case DebugLevel:
		zapLevel = zap.DebugLevel
	case ErrorLevel:
		zapLevel = zap.ErrorLevel
	case "", InfoLevel:
		zapLevel = zap.InfoLevel
	default:
		return logr.Logger{}, fmt.Errorf("invalid log level %q", level)
	}

	opts = append(opts, logzap.Level(zapLevel))

	// map our log format to encoder
	switch format {
	case FormatJSON:
		opts = append(opts, logzap.JSONEncoder(setCommonEncoderConfigOptions))
	case "", FormatConsole:
		opts = append(opts, logzap.ConsoleEncoder(setCommonEncoderConfigOptions))
	default:
		return logr.Logger{}, fmt.Errorf("invalid log format %q", format)
	}

	return logzap.New(append(opts, additionalOpts...)...), nil
}

// NewDefault creates new default logger.
func NewDefault() logr.Logger {
	return MustNewZapLogger(InfoLevel, FormatJSON)
}

// Type returns the type name (optional for flag.Value).
func (f *Format) Type() string {
	return "logFormat"
}

// Set implements the cli.Value and flag.Value interfaces.
func (f *Format) Set(s string) error {
	switch strings.ToLower(s) {
	case "json":
		*f = FormatJSON
		return nil
	case "console":
		*f = FormatConsole
		return nil
	default:
		return fmt.Errorf("invalid format '%s'", s)
	}
}

// String implements the cli.Value and flag.Value interfaces.
func (f *Format) String() string {
	return string(*f)
}

// Type returns the type name (optional for flag.Value).
func (f *LogLevel) Type() string {
	return "logLevel"
}

// Set implements the cli.Value and flag.Value interfaces.
func (f *LogLevel) Set(s string) error {
	switch strings.ToLower(s) {
	case "info":
		*f = InfoLevel
		return nil
	case "debug":
		*f = DebugLevel
		return nil
	case "error":
		*f = ErrorLevel
		return nil
	default:
		return fmt.Errorf("invalid level '%s'", s)
	}
}

// String implements the cli.Value and flag.Value interfaces.
func (f *LogLevel) String() string {
	return string(*f)
}
