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

package ethclient

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMappings(t *testing.T) {
	assert.Equal(t, ErrorReasonNonceTooLow, MapError(fmt.Errorf("nonce too low")))
	assert.Equal(t, ErrorReasonInsufficientFunds, MapError(fmt.Errorf("insufficient funds")))
	assert.Equal(t, ErrorReasonTransactionUnderpriced, MapError(fmt.Errorf("transaction underpriced")))
	assert.Equal(t, ErrorKnownTransaction, MapError(fmt.Errorf("known transaction")))
	assert.Equal(t, ErrorKnownTransaction, MapError(fmt.Errorf("already known")))
	assert.Equal(t, ErrorReasonTransactionReverted, MapError(fmt.Errorf("execution reverted")))
	assert.Equal(t, ErrorReasonNotFound, MapError(fmt.Errorf("filter not found")))
	assert.Equal(t, ErrorReasonNotFound, MapError(fmt.Errorf("cannot query unfinalized data")))
	assert.Equal(t, ErrorReasonNotFound, MapError(fmt.Errorf("the method net_version does not exist/is not available")))
	assert.Equal(t, ErrorReason(""), MapError(fmt.Errorf("unknown")))

	assert.True(t, MapSubmissionRejected(fmt.Errorf("execution reverted")))
	assert.False(t, MapSubmissionRejected(fmt.Errorf("known transaction")))
}
