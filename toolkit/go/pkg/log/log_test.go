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
	"os"
	"path"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogContext(t *testing.T) {
	ctx := WithLogField(context.Background(), "myfield", "myvalue")
	assert.Equal(t, "myvalue", L(ctx).Data["myfield"])
}

func TestLogContextLimited(t *testing.T) {
	ctx := WithLogField(context.Background(), "myfield", "0123456789012345678901234567890123456789012345678901234567890123456789")
	assert.Equal(t, "0123456789012345678901234567890123456789012345678901234567890...", L(ctx).Data["myfield"])
}

func TestSettingErrorLevel(t *testing.T) {
	SetLevel("eRrOr")
	assert.Equal(t, logrus.ErrorLevel, logrus.GetLevel())
	assert.Equal(t, "error", GetLevel())
}

func TestSettingWarnLevel(t *testing.T) {
	SetLevel("WARNING")
	assert.Equal(t, logrus.WarnLevel, logrus.GetLevel())
	assert.Equal(t, "warn", GetLevel())
}

func TestSettingDebugLevel(t *testing.T) {
	SetLevel("DEBUG")
	assert.True(t, IsDebugEnabled())
	assert.Equal(t, logrus.DebugLevel, logrus.GetLevel())
	assert.Equal(t, "debug", GetLevel())
}

func TestSettingTraceLevel(t *testing.T) {
	SetLevel("trace")
	assert.True(t, IsTraceEnabled())
	assert.Equal(t, logrus.TraceLevel, logrus.GetLevel())
	assert.Equal(t, "trace", GetLevel())
}

func TestSettingInfoLevel(t *testing.T) {
	SetLevel("info")
	assert.Equal(t, logrus.InfoLevel, logrus.GetLevel())
	assert.Equal(t, "info", GetLevel())
}

func TestSettingDefaultLevel(t *testing.T) {
	SetLevel("something else")
	assert.Equal(t, logrus.InfoLevel, logrus.GetLevel())
	assert.Equal(t, "info", GetLevel())
}

func TestSetFormatting(t *testing.T) {
	defer func() { InitConfig(&pldconf.LogConfig{}) /* reinstae defaults for other tests */ }()
	InitConfig(&pldconf.LogConfig{
		DisableColor: confutil.P(true),
		UTC:          confutil.P(true),
	})
	L(context.Background()).Infof("time in UTC")
}

func TestSetFormattingStderr(t *testing.T) {
	defer func() { InitConfig(&pldconf.LogConfig{}) /* reinstae defaults for other tests */ }()
	InitConfig(&pldconf.LogConfig{
		Output: confutil.P("stderr"),
	})
	L(context.Background()).Infof("code info included")
}

func TestSetFormattingStdout(t *testing.T) {
	defer func() { InitConfig(&pldconf.LogConfig{}) /* reinstae defaults for other tests */ }()
	InitConfig(&pldconf.LogConfig{
		Output: confutil.P("stdout"),
	})
	L(context.Background()).Infof("code info included")
}

func TestSetFormattingIncludeCodeInfo(t *testing.T) {
	defer func() { InitConfig(&pldconf.LogConfig{}) /* reinstae defaults for other tests */ }()
	InitConfig(&pldconf.LogConfig{
		Format: confutil.P("detailed"),
	})
	L(context.Background()).Infof("code info included")
}

func TestSetFormattingJSONEnabled(t *testing.T) {
	defer func() { InitConfig(&pldconf.LogConfig{}) /* reinstae defaults for other tests */ }()
	InitConfig(&pldconf.LogConfig{
		Format: confutil.P("json"),
	})
	L(context.Background()).Infof("JSON logs")
}

func TestSetFormattingFile(t *testing.T) {
	defer func() { InitConfig(&pldconf.LogConfig{}) /* reinstae defaults for other tests */ }()
	logFile := path.Join(t.TempDir(), "paladin.log")
	InitConfig(&pldconf.LogConfig{
		Output: confutil.P("file"),
		File: pldconf.LogFileConfig{
			Filename: confutil.P(logFile),
		},
	})
	L(context.Background()).Infof("File logs")

	fileExists, err := os.Stat(logFile)
	require.NoError(t, err)
	assert.False(t, fileExists.IsDir())
}
