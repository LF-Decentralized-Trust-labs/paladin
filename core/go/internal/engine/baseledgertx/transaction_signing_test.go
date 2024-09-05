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

package baseledgertx

import (
	"context"
	"fmt"
	"testing"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/mocks/componentmocks"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestInFlightTxSign(t *testing.T) {
	ctx := context.Background()
	config.RootConfigReset()

	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it

	mEC := componentmocks.NewEthClient(t)
	it.ethClient = mEC

	// signing error
	mtx := it.stateManager.GetTx()

	buildRawTransactionMock := mEC.On("BuildRawTransaction", ctx, ethclient.EIP1559, string(mtx.From), mtx.Transaction)
	buildRawTransactionMock.Run(func(args mock.Arguments) {
		from := args[2].(string)
		txObj := args[3].(*ethsigner.Transaction)

		assert.Equal(t, "0x4e598f6e918321dd47c86e7a077b4ab0e7414846", from)
		assert.Equal(t, ethtypes.MustNewHexBytes0xPrefix(testTransactionData), txObj.Data)
		buildRawTransactionMock.Return(nil, fmt.Errorf("pop"))
	}).Once()
	_, txHash, err := it.signTx(ctx, mtx)
	assert.Error(t, err)
	assert.Equal(t, "", txHash)

	// signing succeeded
	buildRawTransactionMock.Run(func(args mock.Arguments) {
		from := args[2].(string)
		txObj := args[3].(*ethsigner.Transaction)

		assert.Equal(t, "0x4e598f6e918321dd47c86e7a077b4ab0e7414846", from)
		assert.Equal(t, ethtypes.MustNewHexBytes0xPrefix(testTransactionData), txObj.Data)
	}).Return(types.MustParseHexBytes(testHashedSignedMessage), nil).Once()
	_, txHash, err = it.signTx(ctx, mtx)
	assert.NoError(t, err)
	assert.Equal(t, testTxHash, txHash)
}
