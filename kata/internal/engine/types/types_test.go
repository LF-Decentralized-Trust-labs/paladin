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

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestForTypesAndMocks(t *testing.T) {

	pfs := NewPaladinStageFoundationService(nil, nil, nil, nil, nil)
	assert.Nil(t, pfs.DependencyChecker())
	assert.Nil(t, pfs.IdentityResolver())
	assert.Nil(t, pfs.StateStore())

	// mock object tests for coverage:
	mIR := &MockIdentityResolver{}

	assert.NoError(t, mIR.ConnectToBaseLeger())
	assert.True(t, mIR.IsCurrentNode("current-node"))
	assert.False(t, mIR.IsCurrentNode("not-current-node"))
	assert.Empty(t, mIR.GetDispatchAddress(nil))
	assert.Equal(t, "test", mIR.GetDispatchAddress([]string{"test"}))
}
