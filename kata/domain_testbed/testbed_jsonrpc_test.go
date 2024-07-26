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

package main

import (
	"fmt"
	"testing"

	"github.com/kaleido-io/paladin/kata/internal/types"
	"github.com/stretchr/testify/assert"
)

// Example of how someone might use this testbed externally
func TestE2E1(t *testing.T) {

	rpcCall, done := newDomainSimulator(t, map[string]domainSimulatorFn{
		CONFIGURE_REQUEST: func(reqJSON []byte) (string, []byte, error) {
			return CONFIGURE_RESPONSE, nil, fmt.Errorf("POP")
		},
	})
	defer done()

	err := rpcCall("testbed_configureInit", types.RawJSON(`{
	}`))
	assert.Regexp(t, "POP", err)
}
