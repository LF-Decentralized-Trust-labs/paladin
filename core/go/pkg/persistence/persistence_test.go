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

package persistence

import (
	"context"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistenceTypes(t *testing.T) {
	ctx := context.Background()

	_, err := NewPersistence(ctx, &pldconf.DBConfig{})
	assert.Regexp(t, "PD010201", err)

	_, err = NewPersistence(ctx, &pldconf.DBConfig{Type: "sqlite"})
	assert.Regexp(t, "PD010201", err)

	_, err = NewPersistence(ctx, &pldconf.DBConfig{Type: "postgres"})
	assert.Regexp(t, "PD010201", err)

	// Different error for wrong case
	_, err = NewPersistence(ctx, &pldconf.DBConfig{Type: "wrong"})
	assert.Regexp(t, "PD010200.*wrong", err)

}

func TestHashCodeAlwaysPositive(t *testing.T) {
	require.Equal(t, int64(1793351735952061022), hashCode("aaa"))
	require.Equal(t, int64(18883120392660901), hashCode("bbb"))
}
