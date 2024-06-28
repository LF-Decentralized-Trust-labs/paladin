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

package manager

import (
	"context"
	"strings"
	"testing"

	smconfig "github.com/kaleido-io/paladin-state-manager/internal/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

var baseConfig = `
---
database:
  postgres:
    url: none
`

func setupConfig(t *testing.T, config string) {
	smconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(config))
	assert.NoError(t, err)
}

func TestNewStateManagerService(t *testing.T) {
	setupConfig(t, baseConfig)
	service, err := NewStateManagerService(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, service)
}

func TestNewStateManagerServiceBadConfig(t *testing.T) {
	setupConfig(t, "")
	_, err := NewStateManagerService(context.Background())
	assert.Regexp(t, "FF00183", err)
}
