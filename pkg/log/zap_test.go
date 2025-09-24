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

package log_test

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"

	"github.com/kubeflag/kubeflag/pkg/log"
)

func TestFormat_SetAndString(t *testing.T) {
	var f log.Format

	// Valid inputs
	require.NoError(t, f.Set("json"))
	require.Equal(t, log.FormatJSON, f)
	require.Equal(t, "JSON", f.String())

	require.NoError(t, f.Set("console"))
	require.Equal(t, log.FormatConsole, f)
	require.Equal(t, "Console", f.String())

	// Invalid input
	err := f.Set("yaml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid format")
}

func TestLogLevel_SetAndString(t *testing.T) {
	var lvl log.LogLevel

	// Valid levels
	require.NoError(t, lvl.Set("info"))
	require.Equal(t, log.InfoLevel, lvl)
	require.Equal(t, "info", lvl.String())

	require.NoError(t, lvl.Set("debug"))
	require.Equal(t, log.DebugLevel, lvl)
	require.Equal(t, "debug", lvl.String())

	require.NoError(t, lvl.Set("error"))
	require.Equal(t, log.ErrorLevel, lvl)
	require.Equal(t, "error", lvl.String())

	// Invalid
	err := lvl.Set("warn")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid level")
}

func TestNewZapLogger_ValidCombinations(t *testing.T) {
	tests := []struct {
		name   string
		level  log.LogLevel
		format log.Format
	}{
		{"info-json", log.InfoLevel, log.FormatJSON},
		{"debug-console", log.DebugLevel, log.FormatConsole},
		{"error-json", log.ErrorLevel, log.FormatJSON},
		{"default-level-default-format", "", ""}, // empty should default to info + console
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := log.NewZapLogger(tt.level, tt.format)
			require.NoError(t, err)
			require.NotNil(t, logger)

			// Instead of Implements, just ensure it's not the zero struct
			require.NotEqual(t, logr.Logger{}, logger, "logger should not be the zero value")

			logger.Info("test log message")
		})
	}
}

func TestNewZapLogger_InvalidInputs(t *testing.T) {
	_, err := log.NewZapLogger("invalid-level", log.FormatJSON)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid log level")

	_, err = log.NewZapLogger(log.InfoLevel, "bad-format")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid log format")
}

func TestMustNewZapLogger_PanicsOnError(t *testing.T) {
	require.Panics(t, func() {
		_ = log.MustNewZapLogger("invalid-level", log.FormatJSON)
	})
}

func TestNewDefault(t *testing.T) {
	logger := log.NewDefault()
	require.NotNil(t, logger)
	logger.Info("Testing default logger")
	require.NotEqual(t, logr.Logger{}, logger, "logger should not be the zero value")
}
