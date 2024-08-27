/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */
package plugins

import (
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type PluginControllerConfig struct {
	GRPC GRPCConfig `yaml:"grpc"`
}

type GRPCConfig struct {
	ShutdownTimeout *string `yaml:"shutdownTimeout"`
}

var DefaultGRPCConfig = &GRPCConfig{
	ShutdownTimeout: confutil.P("10s"),
}

type LibraryType string

const (
	LibraryTypeCShared LibraryType = "c-shared"
	LibraryTypeJar     LibraryType = "jar"
)

func (lt LibraryType) Enum() types.Enum[LibraryType] {
	return types.Enum[LibraryType](lt)
}

func (pl LibraryType) Options() []string {
	return []string{
		string(LibraryTypeCShared),
		string(LibraryTypeJar),
	}
}

var golangToProtoLibTypeMap = map[LibraryType]prototk.PluginLoad_LibType{
	LibraryTypeCShared: prototk.PluginLoad_C_SHARED,
	LibraryTypeJar:     prototk.PluginLoad_JAR,
}

type PluginConfig struct {
	Type     types.Enum[LibraryType]
	Location string
}
