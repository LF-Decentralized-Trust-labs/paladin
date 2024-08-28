// Copyright © 2022 Kaleido, Inc.
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

import "github.com/kaleido-io/paladin/toolkit/pkg/confutil"

type Config struct {
	// the logging level
	Level *string `yaml:"level"`
	// the format ('simple', 'json')
	Format *string `yaml:"simple"`
	// the output location ('stdout','stderr','file')
	Output *string `yaml:"output"`
	// forces color to be enabled, even if we do not detect a TTY
	ForceColor *bool `yaml:"forceColor"`
	// forces color to be disabled, even if we detect a TTY
	DisableColor *bool `yaml:"disableColor"`
	// string format for timestamps
	TimeFormat *string `yaml:"timeFormat"`
	// sets log timestamps to the UTC timezone
	UTC *bool `yaml:"utc"`
	// configure file based logging
	File FileConfig `yaml:"file"`
	// configure json based logging
	JSON JSONConfig `yaml:"json"`
}

type FileConfig struct {
	// sets the log filename prefix
	Filename *string `yaml:"filename"`
	// sets the size to roll logs at a given size
	MaxSize *string `yaml:"maxSize"`
	// sets the maximum number of old files to keep
	MaxBackups *int `yaml:"maxBackups"`
	// sets the maximum age at which to roll
	MaxAge *string `yaml:"maxAge"`
	// Compress sets whether to compress backups
	Compress *bool `yaml:"compress"`
}

type JSONConfig struct {
	// configures the JSON key containing the timestamp of the log
	TimestampField *string `yaml:"timestampField"`
	// configures the JSON key containing the log level
	LevelField *string `yaml:"levelField"`
	// configures the JSON key containing the log message
	MessageField *string `yaml:"messageField"`
	// configures the JSON key containing the calling function
	FuncField *string `yaml:"funcField"`
	// configures the JSON key containing the calling file
	FileField *string `yaml:"fileField"`
}

var LogDefaults = &Config{
	Level:        confutil.P("info"),
	Format:       confutil.P("simple"),
	Output:       confutil.P("stderr"),
	ForceColor:   confutil.P(false),
	DisableColor: confutil.P(false),
	TimeFormat:   confutil.P("2006-01-02T15:04:05.000Z07:00"),
	UTC:          confutil.P(false),
	File: FileConfig{
		Filename:   confutil.P("paladin.log"),
		MaxSize:    confutil.P("100Mb"),
		MaxBackups: confutil.P(2),
		MaxAge:     confutil.P("24h"),
		Compress:   confutil.P(true),
	},
	JSON: JSONConfig{
		TimestampField: confutil.P("@timestamp"),
		LevelField:     confutil.P("level"),
		MessageField:   confutil.P("message"),
		FuncField:      confutil.P("func"),
		FileField:      confutil.P("file"),
	},
}
