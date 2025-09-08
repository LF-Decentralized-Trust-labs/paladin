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

package pldapi

import (
	"encoding/json"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnums(t *testing.T) {
	assert.NotEmpty(t, EthTransactionResult("").Enum().Options())
	assert.NotEmpty(t, TransactionType("").Enum().Options())
	assert.NotEmpty(t, ActiveFilter("").Enum().Options())
	assert.NotEmpty(t, SchemaType("").Enum().Options())
	assert.NotEmpty(t, StateLockType("").Enum().Options())
	assert.NotEmpty(t, SubmitMode("").Enum().Options())
	assert.NotEmpty(t, SubmitMode("").Default())
	assert.NotEmpty(t, PTXEventType("").Enum().Options())
	assert.NotEmpty(t, PGroupEventType("").Enum().Options())
	assert.NotEmpty(t, ReliableMessageType("").Enum().Options())

	// TODO: separate out from pldapi
	assert.NotEmpty(t, (StateBase{}).TableName())
}

func TestStateStatusQualifierJSON(t *testing.T) {
	var q StateStatusQualifier
	err := json.Unmarshal(([]byte)(`"wrong"`), &q)
	assert.Regexp(t, "PD020016", err)

	err = json.Unmarshal(([]byte)(`"ALL"`), &q)
	require.NoError(t, err)
	require.Equal(t, StateStatusAll, q)

	u := uuid.New().String()
	err = json.Unmarshal(pldtypes.JSONString(u), &q)
	require.NoError(t, err)
	assert.Equal(t, u, (string)(q))
}
