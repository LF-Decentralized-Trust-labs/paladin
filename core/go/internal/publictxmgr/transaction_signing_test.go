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

package publictxmgr

import (
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signpayloads"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestInFlightTxSignFail(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t)
	defer done()
	it, _ := newInflightTransaction(o, 1)

	fromAddr := *pldtypes.RandAddress()

	m.ethClient.On("ChainID").Return(int64(1122334455))
	keyMapping := &pldapi.KeyMappingAndVerifier{
		KeyMappingWithPath: &pldapi.KeyMappingWithPath{
			KeyMapping: &pldapi.KeyMapping{
				Identifier: "any.key",
			},
		},
		Verifier: &pldapi.KeyVerifier{
			Verifier: fromAddr.String(),
		},
	}

	mockKeyManager := m.keyManager.(*componentsmocks.KeyManager)
	mockKeyManager.On("ReverseKeyLookup", mock.Anything, mock.Anything, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, fromAddr.String()).
		Return(keyMapping, nil)
	mockKeyManager.On("Sign", mock.Anything, keyMapping, signpayloads.OPAQUE_TO_RSV, mock.Anything).
		Return(nil, fmt.Errorf("sign failed")).Once()

	ethTx := &ethsigner.Transaction{
		Nonce: ethtypes.NewHexInteger64(12345),
	}

	_, txHash, err := it.signTx(ctx, fromAddr, ethTx)
	assert.Regexp(t, "sign failed", err)
	assert.Nil(t, txHash)

}
