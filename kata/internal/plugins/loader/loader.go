// Copyright © 2024 Kaleido, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
// an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
// specific language governing permissions and limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package loader

import (
	"context"
)

type Binding string

const (
	GO_SHARED_LIBRARY Binding = "GO_SHARED_LIBRARY"
	JAVA              Binding = "JAVA"
	C_SHARED_LIBRARY  Binding = "C_SHARED_LIBRARY"
)

type Config struct {
	Binding Binding `yaml:"binding"`
	Path    string  `yaml:"path"`
}

type ProviderLoader interface {
	Load(ctx context.Context, providerConfig Config) (ProviderBinding, error)
}

// thin wrapper around the actual provider's language specific binding
type ProviderBinding interface {
	BuildInfo(ctx context.Context) (string, error)
	InitializeTransportProvider(ctx context.Context, socketAddress string, providerListenerDestination string) error
}

func NewPluginLoader(ctx context.Context, binding Binding) ProviderLoader {
	if binding == GO_SHARED_LIBRARY {
		return &goPluginLoader{}
	}
	return nil
}
