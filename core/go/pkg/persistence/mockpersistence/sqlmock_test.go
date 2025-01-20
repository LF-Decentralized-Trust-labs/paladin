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

package mockpersistence

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLMockProvider(t *testing.T) {
	m, err := NewSQLMockProvider()
	require.NoError(t, err)
	assert.NotNil(t, m.P.DB())
	assert.Equal(t, "sqlmock", (&SQLMockProvider{}).DBName())
	_, err = (&SQLMockProvider{}).GetMigrationDriver(nil)
	assert.Regexp(t, "not supported", err)
	require.NoError(t, m.TakeNamedLock(context.Background(), nil, ""))
}
