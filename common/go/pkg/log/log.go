// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package log

import (
	"context"
	"math"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

var (
	rootLogger = logrus.NewEntry(logrus.StandardLogger())

	// L accesses the current logger from the context
	L = loggerFromContext

	initAtLeastOnce atomic.Bool
)

type (
	ctxLogKey struct{}
)

func InitConfig(conf *pldconf.LogConfig) {
	initAtLeastOnce.Store(true) // must store before SetLevel

	level := confutil.StringNotEmpty(conf.Level, *pldconf.LogDefaults.Level)
	SetLevel(level)

	output := confutil.StringNotEmpty(conf.Output, *pldconf.LogDefaults.Output)
	switch output {
	case "file":
		filename := confutil.StringNotEmpty(conf.File.Filename, *pldconf.LogDefaults.File.Filename)
		rootLogger.Infof("Logs diverted to %s", filename)
		maxSizeBytes := confutil.ByteSize(conf.File.MaxSize, 0, *pldconf.LogDefaults.File.MaxSize)
		maxAgeDuration := confutil.DurationMin(conf.File.MaxAge, 0, *pldconf.LogDefaults.File.MaxAge)
		lumberjack := &lumberjack.Logger{
			Filename:   filename,
			MaxSize:    int(math.Ceil(float64(maxSizeBytes) / 1024 / 1024)), /* round up in megabytes */
			MaxBackups: confutil.IntMin(conf.File.MaxBackups, 0, *pldconf.LogDefaults.File.MaxBackups),
			MaxAge:     int(math.Ceil(float64(maxAgeDuration) / float64(time.Hour) / 24)), /* round up in days */
			Compress:   confutil.Bool(conf.File.Compress, *pldconf.LogDefaults.File.Compress),
		}
		logrus.SetOutput(lumberjack)
	case "stderr":
		logrus.SetOutput(os.Stderr)
	case "stdout":
		logrus.SetOutput(os.Stdout)
		fallthrough
	default:
	}

	setFormatting(&Formatting{
		Format:             confutil.StringNotEmpty(conf.Format, *pldconf.LogDefaults.Format),
		DisableColor:       confutil.Bool(conf.DisableColor, *pldconf.LogDefaults.DisableColor),
		ForceColor:         confutil.Bool(conf.ForceColor, *pldconf.LogDefaults.ForceColor),
		TimestampFormat:    confutil.StringNotEmpty(conf.TimeFormat, *pldconf.LogDefaults.TimeFormat),
		UTC:                confutil.Bool(conf.UTC, *pldconf.LogDefaults.UTC),
		JSONTimestampField: confutil.StringNotEmpty(conf.JSON.TimestampField, *pldconf.LogDefaults.JSON.TimestampField),
		JSONLevelField:     confutil.StringNotEmpty(conf.JSON.LevelField, *pldconf.LogDefaults.JSON.LevelField),
		JSONMessageField:   confutil.StringNotEmpty(conf.JSON.MessageField, *pldconf.LogDefaults.JSON.MessageField),
		JSONFuncField:      confutil.StringNotEmpty(conf.JSON.FuncField, *pldconf.LogDefaults.JSON.FuncField),
		JSONFileField:      confutil.StringNotEmpty(conf.JSON.FileField, *pldconf.LogDefaults.JSON.FileField),
	})
}

func IsDebugEnabled() bool {
	return logrus.IsLevelEnabled(logrus.DebugLevel)
}

func IsTraceEnabled() bool {
	return logrus.IsLevelEnabled(logrus.TraceLevel)
}

func EnsureInit() {
	// Called at a couple of strategic points to check we get log initialize in things like unit tests
	// However NOT guaranteed to be called because we can't afford to do atomic load on every log line
	if !initAtLeastOnce.Load() {
		InitConfig(&pldconf.LogConfig{})
	}
}

// WithLogger adds the specified logger to the context
func WithLogger(ctx context.Context, logger *logrus.Entry) context.Context {
	EnsureInit()
	return context.WithValue(ctx, ctxLogKey{}, logger)
}

// WithLogField adds the specified field to the logger in the context
func WithLogField(ctx context.Context, key, value string) context.Context {
	EnsureInit()
	if len(value) > 61 {
		value = value[0:61] + "..."
	}
	return WithLogger(ctx, loggerFromContext(ctx).WithField(key, value))
}

// LoggerFromContext returns the logger for the current context, or no logger if there is no context
func loggerFromContext(ctx context.Context) *logrus.Entry {
	logger := ctx.Value(ctxLogKey{})
	if logger == nil {
		return rootLogger
	}
	return logger.(*logrus.Entry)
}

func GetLevel() string {
	switch logrus.GetLevel() {
	case logrus.ErrorLevel:
		return "error"
	case logrus.WarnLevel:
		return "warn"
	case logrus.DebugLevel:
		return "debug"
	case logrus.TraceLevel:
		return "trace"
	default:
		return "info"
	}
}

func SetLevel(level string) {
	var l logrus.Level
	switch strings.ToLower(level) {
	case "error":
		l = logrus.ErrorLevel
	case "warn", "warning":
		l = logrus.WarnLevel
	case "debug":
		l = logrus.DebugLevel
	case "trace":
		l = logrus.TraceLevel
	default:
		l = logrus.InfoLevel
	}
	logrus.SetLevel(l)
}

type Formatting struct {
	Format             string
	DisableColor       bool
	ForceColor         bool
	TimestampFormat    string
	UTC                bool
	JSONTimestampField string
	JSONLevelField     string
	JSONMessageField   string
	JSONFuncField      string
	JSONFileField      string
}

type utcFormat struct {
	f logrus.Formatter
}

func (utc *utcFormat) Format(e *logrus.Entry) ([]byte, error) {
	e.Time = e.Time.UTC()
	return utc.f.Format(e)
}

func setFormatting(format *Formatting) {
	var formatter logrus.Formatter
	switch format.Format {
	case "json":
		formatter = &logrus.JSONFormatter{
			TimestampFormat: format.TimestampFormat,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  format.JSONTimestampField,
				logrus.FieldKeyLevel: format.JSONLevelField,
				logrus.FieldKeyMsg:   format.JSONMessageField,
				logrus.FieldKeyFunc:  format.JSONFuncField,
				logrus.FieldKeyFile:  format.JSONFileField,
			},
		}
	case "detailed":
		formatter = &logrus.TextFormatter{
			DisableColors:   format.DisableColor,
			ForceColors:     format.ForceColor,
			TimestampFormat: format.TimestampFormat,
			DisableSorting:  false,
			FullTimestamp:   true,
		}
		logrus.SetReportCaller(true)
	case "simple":
		fallthrough
	default:
		formatter = &prefixed.TextFormatter{
			DisableColors:   format.DisableColor,
			ForceColors:     format.ForceColor,
			TimestampFormat: format.TimestampFormat,
			DisableSorting:  false,
			ForceFormatting: true,
			FullTimestamp:   true,
		}
	}
	if format.UTC {
		formatter = &utcFormat{f: formatter}
	}
	logrus.SetFormatter(formatter)
}
