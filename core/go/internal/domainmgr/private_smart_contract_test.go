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

package domainmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/signpayloads"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPrivateSmartContractQueryFail(t *testing.T) {

	td, done := newTestDomain(t, false, goodDomainConf(), func(mc *mockComponents) {
		mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, nil)
		mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	_, err := td.dm.GetSmartContractByAddress(td.ctx, tktypes.EthAddress(tktypes.RandBytes(20)))
	assert.Regexp(t, "pop", err)

}

func TestPrivateSmartContractQueryNoResult(t *testing.T) {

	td, done := newTestDomain(t, false, goodDomainConf(), func(mc *mockComponents) {
		mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, nil)
		mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows([]string{}))
	})
	defer done()

	_, err := td.dm.GetSmartContractByAddress(td.ctx, tktypes.EthAddress(tktypes.RandBytes(20)))
	assert.Regexp(t, "PD011609", err)

}

func goodPSC(d *domain) *domainContract {
	return d.newSmartContract(&PrivateSmartContract{
		DeployTX:        uuid.New(),
		RegistryAddress: *d.RegistryAddress(),
		Address:         tktypes.EthAddress(tktypes.RandBytes(20)),
		ConfigBytes:     []byte{0xfe, 0xed, 0xbe, 0xef},
	})
}

func goodPrivateTXWithInputs(psc *domainContract) *components.PrivateTransaction {
	return &components.PrivateTransaction{
		ID: uuid.New(),
		Inputs: &components.TransactionInputs{
			From: "txSigner",
			To:   psc.info.Address,
			Function: &abi.Entry{
				Type: abi.Function,
				Inputs: abi.ParameterArray{
					{Name: "from", Type: "string"},
					{Name: "to", Type: "string"},
					{Name: "amount", Type: "uint256"},
				},
			},
			Inputs: tktypes.RawJSON(`{
			   "from": "sender",
			   "to": "receiver",
			   "amount": "123000000000000000000"
			}`),
		},
	}
}

func doDomainInitTransactionOK(t *testing.T, td *testDomainContext, resFn ...func(*prototk.InitTransactionResponse)) (*domainContract, *components.PrivateTransaction) {
	psc := goodPSC(td.d)
	tx := goodPrivateTXWithInputs(psc)
	tx.PreAssembly = &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{},
	}
	td.tp.Functions.InitTransaction = func(ctx context.Context, itr *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
		assert.Equal(t, tktypes.Bytes32UUIDFirst16(tx.ID).String(), itr.Transaction.TransactionId)
		assert.Equal(t, int64(12345), itr.Transaction.BaseBlock)
		res := &prototk.InitTransactionResponse{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{
					Lookup:       tx.Signer,
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
		}
		for _, fn := range resFn {
			fn(res)
		}
		return res, nil
	}

	err := psc.InitTransaction(td.ctx, tx)
	require.NoError(t, err)
	assert.Len(t, tx.PreAssembly.RequiredVerifiers, 1)
	return psc, tx
}

func doDomainInitAssembleTransactionOK(t *testing.T, td *testDomainContext) (*domainContract, *components.PrivateTransaction) {
	psc, tx := doDomainInitTransactionOK(t, td)
	td.tp.Functions.AssembleTransaction = func(ctx context.Context, atr *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
		return &prototk.AssembleTransactionResponse{
			AssemblyResult:       prototk.AssembleTransactionResponse_OK,
			AssembledTransaction: &prototk.AssembledTransaction{},
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "ensorsement1",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties:         []string{"endorser1"},
				},
			},
		}, nil
	}
	err := psc.AssembleTransaction(td.mdc, tx)
	require.NoError(t, err)
	tx.PreAssembly.Verifiers = []*prototk.ResolvedVerifier{}
	tx.PostAssembly.Signatures = []*prototk.AttestationResult{}
	return psc, tx
}

func mockBlockHeight(mc *mockComponents) {
	mc.blockIndexer.On("GetConfirmedBlockHeight", mock.Anything).Return(uint64(12345), nil)
}

func TestDomainInitTransactionOK(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, td.d.initError.Load())

	_, _ = doDomainInitTransactionOK(t, td)
}

func TestEncodeDecodeABIData(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	funcDef := `{
		"type": "function",
		"name": "doStuff",
		"inputs": [
			{ "name": "intVal", "type": "uint256" }
		]
	}`
	encResult, err := td.d.EncodeData(td.ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_FUNCTION_CALL_DATA,
		Definition:   funcDef,
		Body:         `{ "intVal": 42 }`,
	})
	assert.NoError(t, err)

	decResult, err := td.d.DecodeData(td.ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_FUNCTION_CALL_DATA,
		Definition:   funcDef,
		Data:         encResult.Data,
	})
	assert.NoError(t, err)
	assert.Equal(t, `{"intVal":"42"}`, decResult.Body)

	tupleDef := `{
		"type": "tuple",
		"components": [
			{ "name": "intVal", "type": "uint256" }
		]
	}`
	encResult, err = td.d.EncodeData(td.ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_TUPLE,
		Definition:   tupleDef,
		Body:         `{ "intVal": 42 }`,
	})
	assert.NoError(t, err)

	decResult, err = td.d.DecodeData(td.ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_TUPLE,
		Definition:   tupleDef,
		Data:         encResult.Data,
	})
	assert.NoError(t, err)
	assert.Equal(t, `{"intVal":"42"}`, decResult.Body)

	txEIP1559_a, err := td.d.EncodeData(td.ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION,
		Definition:   "",
		Body: `{
		  "to": "0x05d936207F04D81a85881b72A0D17854Ee8BE45A"
		}`,
	})
	assert.NoError(t, err)

	txEIP1559_b, err := td.d.EncodeData(td.ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION,
		Definition:   "eip1559",
		Body: `{
		  "to": "0x05d936207F04D81a85881b72A0D17854Ee8BE45A"
		}`,
	})
	assert.NoError(t, err)
	assert.Equal(t, txEIP1559_a, txEIP1559_b)

	txEIP155, err := td.d.EncodeData(td.ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION,
		Definition:   "eip155",
		Body: `{
		  "to": "0x05d936207F04D81a85881b72A0D17854Ee8BE45A"
		}`,
	})
	assert.NoError(t, err)
	assert.NotEqual(t, txEIP155, txEIP1559_a)

	eventDef := `{
		"type": "event",
		"name": "Transfer",
		"inputs": [
			{ "name": "from", "type": "address", "indexed": true },
			{ "name": "to", "type": "address", "indexed": true },
			{ "name": "value", "type": "uint256" }
		]
	}`
	decResult, err = td.d.DecodeData(td.ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_EVENT_DATA,
		Definition:   eventDef,
		Data:         ethtypes.MustNewHexBytes0xPrefix("0x000000000000000000000000000000000000000000000000000000000000002a"),
		Topics: [][]byte{
			ethtypes.MustNewHexBytes0xPrefix("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"),
			ethtypes.MustNewHexBytes0xPrefix("0x000000000000000000000000dafce4acc2703a24f29d1321adaadf5768f54642"),
			ethtypes.MustNewHexBytes0xPrefix("0x000000000000000000000000dbfd76af2157dc15ee4e57f3f942bb45ba84af24"),
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, `{"from":"dafce4acc2703a24f29d1321adaadf5768f54642","to":"dbfd76af2157dc15ee4e57f3f942bb45ba84af24","value":"42"}`, decResult.Body)

	eventDef = `{
      "inputs": [
        {
          "name": "contractAddress",
          "type": "address"
        },
        {
          "name": "encodedCall",
          "type": "bytes"
        }
      ],
      "name": "PenteExternalCall",
      "type": "event"
    }`
	decResult, err = td.d.DecodeData(td.ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_EVENT_DATA,
		Definition:   eventDef,
		Data:         ethtypes.MustNewHexBytes0xPrefix("0x0000000000000000000000003153e3e67d3d4be35aa5baff60b5a862f55a54310000000000000000000000000000000000000000000000000000000000000040000000000000000000000000000000000000000000000000000000000000002460fe47b1000000000000000000000000000000000000000000000000000000000000006400000000000000000000000000000000000000000000000000000000"),
		Topics: [][]byte{
			ethtypes.MustNewHexBytes0xPrefix("0xcac03685d5ba4ab3e1465a8ee1b2bb21094ddbd612a969fd34f93a5be7a0ac4f"),
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, `{"contractAddress":"3153e3e67d3d4be35aa5baff60b5a862f55a5431","encodedCall":"60fe47b10000000000000000000000000000000000000000000000000000000000000064"}`, decResult.Body)

}

func TestRecoverSignature(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	kp, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)
	s, err := kp.SignDirect(([]byte)("some data"))
	require.NoError(t, err)

	res, err := td.d.RecoverSigner(td.ctx, &prototk.RecoverSignerRequest{
		Algorithm:   algorithms.ECDSA_SECP256K1,
		PayloadType: signpayloads.OPAQUE_TO_RSV,
		Payload:     ([]byte)("some data"),
		Signature:   s.CompactRSV(),
	})
	require.NoError(t, err)
	assert.Equal(t, kp.Address.String(), res.Verifier)
}

func TestDomainInitTransactionMissingInput(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(td.d)

	tx := &components.PrivateTransaction{}
	err := psc.InitTransaction(td.ctx, tx)
	assert.Regexp(t, "PD011626", err)
	assert.Nil(t, tx.PreAssembly)

}

func TestDomainInitTransactionConfirmedBlockFail(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.blockIndexer.On("GetConfirmedBlockHeight", mock.Anything).Return(uint64(0), fmt.Errorf("pop"))
	})
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(td.d)
	tx := goodPrivateTXWithInputs(psc)

	err := psc.InitTransaction(td.ctx, tx)
	assert.Regexp(t, "pop", err)
	assert.Nil(t, tx.PreAssembly)

}

func TestDomainInitTransactionError(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(td.d)
	tx := goodPrivateTXWithInputs(psc)

	td.tp.Functions.InitTransaction = func(ctx context.Context, itr *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	err := psc.InitTransaction(td.ctx, tx)
	assert.Regexp(t, "pop", err)
	assert.Nil(t, tx.PreAssembly)

}

func TestDomainInitTransactionBadInputs(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(td.d)
	tx := goodPrivateTXWithInputs(psc)
	tx.Inputs.Inputs = tktypes.RawJSON(`{"missing": "parameters}`)

	err := psc.InitTransaction(td.ctx, tx)
	assert.Regexp(t, "PD011612", err)
	assert.Nil(t, tx.PreAssembly)

}

func TestFullTransactionRealDBOK(t *testing.T) {
	td, done := newTestDomain(t, true /* real DB */, goodDomainConf(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitTransactionOK(t, td)
	domain := td.d
	dCtx := td.c.dCtx

	state1 := storeTestState(t, td, tx.ID, ethtypes.NewHexInteger64(1111111))
	state2 := storeTestState(t, td, tx.ID, ethtypes.NewHexInteger64(2222222))
	state3 := storeTestState(t, td, tx.ID, ethtypes.NewHexInteger64(3333333))
	state4 := storeTestState(t, td, tx.ID, ethtypes.NewHexInteger64(4444444))

	state5 := &fakeState{
		Salt:   tktypes.Bytes32(tktypes.RandBytes(32)),
		Owner:  tktypes.EthAddress(tktypes.RandBytes(20)),
		Amount: ethtypes.NewHexInteger64(5555555),
	}

	td.tp.Functions.AssembleTransaction = func(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
		assert.Same(t, req.Transaction, tx.PreAssembly.TransactionSpecification)

		stateRes, err := domain.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
			StateQueryContext: req.StateQueryContext,
			SchemaId:          td.tp.stateSchemas[0].Id,
			QueryJson: `{
				"or": [
					{
						"eq": [{ "field": "owner", "value": "` + state1.Owner.String() + `" }]
					},
					{
						"eq": [{ "field": "owner", "value": "` + state3.Owner.String() + `" }]
					},
					{
						"eq": [{ "field": "owner", "value": "` + state4.Owner.String() + `" }]
					}
				]
			  }`,
		})
		require.NoError(t, err)
		assert.Len(t, stateRes.States, 3)

		newStateData, err := json.Marshal(state5)
		require.NoError(t, err)

		return &prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			AssembledTransaction: &prototk.AssembledTransaction{
				InputStates: []*prototk.StateRef{
					{Id: stateRes.States[0].Id, SchemaId: stateRes.States[0].SchemaId},
					{Id: stateRes.States[1].Id, SchemaId: stateRes.States[1].SchemaId},
				},
				ReadStates: []*prototk.StateRef{
					{Id: stateRes.States[2].Id, SchemaId: stateRes.States[2].SchemaId},
				},
				OutputStates: []*prototk.NewState{
					{SchemaId: td.tp.stateSchemas[0].Id, StateDataJson: string(newStateData)},
				},
			},
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "sign",
					AttestationType: prototk.AttestationType_SIGN,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
				},
			},
		}, nil
	}
	err := psc.AssembleTransaction(dCtx, tx)
	require.NoError(t, err)

	assert.Len(t, tx.PostAssembly.InputStates, 2)
	assert.Len(t, tx.PostAssembly.ReadStates, 1)
	assert.Len(t, tx.PostAssembly.OutputStatesPotential, 1)
	assert.Equal(t, prototk.AssembleTransactionResponse_OK, tx.PostAssembly.AssemblyResult)
	assert.Len(t, tx.PostAssembly.AttestationPlan, 1)

	// This would be the engine's job
	tx.PreAssembly.Verifiers = make([]*prototk.ResolvedVerifier, 0)
	tx.PostAssembly.Signatures = make([]*prototk.AttestationResult, 0)

	// Write the output states
	err = psc.WritePotentialStates(dCtx, tx)
	require.NoError(t, err)

	stateRes, err := domain.FindAvailableStates(td.ctx, &prototk.FindAvailableStatesRequest{
		StateQueryContext: td.c.id,
		SchemaId:          tx.PostAssembly.OutputStatesPotential[0].SchemaId,
		QueryJson: `{
			"or": [
				{
					"eq": [{ "field": "owner", "value": "` + state5.Owner.String() + `" }]
				}
			]
		  }`,
	})
	require.NoError(t, err)
	assert.Len(t, stateRes.States, 1)

	// Lock all the states
	err = psc.LockStates(dCtx, tx)
	require.NoError(t, err)

	stillAvailable, err := domain.FindAvailableStates(td.ctx, &prototk.FindAvailableStatesRequest{
		StateQueryContext: td.c.id,
		SchemaId:          tx.PostAssembly.OutputStatesPotential[0].SchemaId,
		QueryJson:         `{}`,
	})
	require.NoError(t, err)
	assert.Len(t, stillAvailable.States, 3)
	// state1 & state3 are now locked for spending (state4 was just read, and state2 untouched)
	// state2 & state4 still exist
	// state5 is new
	// The order should be deterministic based on crate time (even before written to DB)
	log.L(td.ctx).Infof("STATES %+v", stillAvailable.States)
	assert.Contains(t, stillAvailable.States[0].DataJson, state2.Salt.String())
	assert.Contains(t, stillAvailable.States[1].DataJson, state4.Salt.String())
	assert.Contains(t, stillAvailable.States[2].DataJson, state5.Salt.String())

	td.tp.Functions.EndorseTransaction = func(ctx context.Context, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
		assert.Same(t, tx.PreAssembly.TransactionSpecification, req.Transaction)
		assert.Same(t, tx.PostAssembly.AttestationPlan[0], req.EndorsementRequest)
		assert.Equal(t, "endorser1", req.EndorsementVerifier.Lookup)
		assert.Same(t, tx.PreAssembly.TransactionSpecification, req.Transaction)
		assert.Len(t, tx.PostAssembly.InputStates, 2)
		assert.Contains(t, string(tx.PostAssembly.InputStates[0].Data), state1.Salt.String())
		assert.Contains(t, string(tx.PostAssembly.InputStates[1].Data), state3.Salt.String())
		assert.Len(t, tx.PostAssembly.OutputStates, 1)
		assert.Contains(t, string(tx.PostAssembly.OutputStates[0].Data), state5.Salt.String())
		return &prototk.EndorseTransactionResponse{
			EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
			Payload:           []byte(`some result`),
		}, nil
	}

	// Run an endorsement
	endorsementRequest := tx.PostAssembly.AttestationPlan[0]
	endorserAddr := tktypes.EthAddress(tktypes.RandBytes(20))
	endorser := &prototk.ResolvedVerifier{
		Lookup:       "endorser1",
		Algorithm:    algorithms.ECDSA_SECP256K1,
		VerifierType: verifiers.ETH_ADDRESS,
		Verifier:     endorserAddr.String(),
	}
	endorsement, err := psc.EndorseTransaction(dCtx, &components.PrivateTransactionEndorseRequest{
		TransactionSpecification: tx.PreAssembly.TransactionSpecification,
		Verifiers:                tx.PreAssembly.Verifiers,
		Signatures:               tx.PostAssembly.Signatures,
		InputStates:              psc.d.toEndorsableList(tx.PostAssembly.InputStates),
		ReadStates:               psc.d.toEndorsableList(tx.PostAssembly.ReadStates),
		OutputStates:             psc.d.toEndorsableList(tx.PostAssembly.OutputStates),
		Endorsement:              endorsementRequest,
		Endorser:                 endorser,
	})
	require.NoError(t, err)
	assert.Equal(t, prototk.EndorseTransactionResponse_ENDORSER_SUBMIT, endorsement.Result)

	// Processing of endorsement faked up here
	tx.PostAssembly.Endorsements = append(tx.PostAssembly.Endorsements, &prototk.AttestationResult{
		Name:            endorsementRequest.Name,
		AttestationType: endorsementRequest.AttestationType,
		Verifier:        endorser,
		Payload:         endorsement.Payload, // just copy over in this test
		Constraints:     []prototk.AttestationResult_AttestationConstraint{prototk.AttestationResult_ENDORSER_MUST_SUBMIT},
	})

	// Resolve who should sign it - we should find it's the endorser due to the endorser submit above
	err = psc.ResolveDispatch(td.ctx, tx)
	require.NoError(t, err)
	assert.Equal(t, "endorser1", tx.Signer)

	td.tp.Functions.PrepareTransaction = func(ctx context.Context, ptr *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
		assert.Same(t, tx.PreAssembly.TransactionSpecification, ptr.Transaction)
		assert.Len(t, tx.PostAssembly.InputStates, 2)
		assert.Contains(t, string(tx.PostAssembly.InputStates[0].Data), state1.Salt.String())
		assert.Contains(t, string(tx.PostAssembly.InputStates[1].Data), state3.Salt.String())
		assert.Len(t, tx.PostAssembly.OutputStates, 1)
		assert.Contains(t, string(tx.PostAssembly.OutputStates[0].Data), state5.Salt.String())
		// Check endorsement
		assert.Len(t, tx.PostAssembly.Endorsements, 1)
		endorsement := tx.PostAssembly.Endorsements[0]
		assert.Equal(t, prototk.AttestationResult_ENDORSER_MUST_SUBMIT, endorsement.Constraints[0])
		assert.Equal(t, "endorser1", endorsement.Verifier.Lookup)
		assert.Equal(t, endorserAddr.String(), endorsement.Verifier.Verifier)
		onChain := &fakeExecute{
			Data: endorsement.Payload,
		}
		for _, inState := range tx.PostAssembly.InputStates {
			onChain.Inputs = append(onChain.Inputs, inState.ID)
		}
		for _, inState := range tx.PostAssembly.InputStates {
			onChain.Outputs = append(onChain.Outputs, inState.ID)
		}
		params, err := json.Marshal(onChain)
		require.NoError(t, err)
		return &prototk.PrepareTransactionResponse{
			Transaction: &prototk.PreparedTransaction{
				FunctionAbiJson: fakeCoinExecuteABI,
				ParamsJson:      string(params),
			},
		}, nil
	}

	// And now prepare
	err = psc.PrepareTransaction(dCtx, tx)
	require.NoError(t, err)
	assert.Len(t, tx.PreparedPublicTransaction.ABI, 1)
	assert.NotNil(t, tx.PreparedPublicTransaction.Data)

	// Confirm the remaining unspent states
	stillAvailable, err = domain.FindAvailableStates(td.ctx, &prototk.FindAvailableStatesRequest{
		StateQueryContext: td.c.id,
		SchemaId:          tx.PostAssembly.OutputStatesPotential[0].SchemaId,
		QueryJson:         `{}`,
	})
	require.NoError(t, err)
	assert.Len(t, stillAvailable.States, 3)
	assert.Contains(t, stillAvailable.States[0].DataJson, state2.Salt.String())
	assert.Contains(t, stillAvailable.States[1].DataJson, state4.Salt.String())
	assert.Contains(t, stillAvailable.States[2].DataJson, state5.Salt.String())
}

func TestDomainAssembleTransactionError(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitTransactionOK(t, td)
	td.tp.Functions.AssembleTransaction = func(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
		return nil, fmt.Errorf("pop")
	}
	err := psc.AssembleTransaction(td.mdc, tx)
	assert.Regexp(t, "pop", err)

	assert.Nil(t, tx.PostAssembly)
}

func TestDomainAssembleTransactionLoadInputError(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitTransactionOK(t, td)
	td.tp.Functions.AssembleTransaction = func(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
		return &prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			AssembledTransaction: &prototk.AssembledTransaction{
				InputStates: []*prototk.StateRef{
					{Id: "badid", SchemaId: "schemaid"},
				},
			},
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "sign",
					AttestationType: prototk.AttestationType_SIGN,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
				},
			},
		}, nil
	}
	err := psc.AssembleTransaction(td.mdc, tx)
	assert.Regexp(t, "PD011614.*badid", err)

	assert.Nil(t, tx.PostAssembly)
}

func TestDomainAssembleTransactionLoadReadError(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitTransactionOK(t, td)
	td.tp.Functions.AssembleTransaction = func(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
		return &prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			AssembledTransaction: &prototk.AssembledTransaction{
				ReadStates: []*prototk.StateRef{
					{Id: "badid", SchemaId: "schemaid"},
				},
			},
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "sign",
					AttestationType: prototk.AttestationType_SIGN,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
				},
			},
		}, nil
	}
	err := psc.AssembleTransaction(td.mdc, tx)
	assert.Regexp(t, "PD011614.*badid", err)

	assert.Nil(t, tx.PostAssembly)
}

func TestDomainWritePotentialStatesBadSchema(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.PostAssembly.OutputStatesPotential = []*prototk.NewState{
		{SchemaId: "unknown"},
	}
	err := psc.WritePotentialStates(td.mdc, tx)
	assert.Regexp(t, "PD011613", err)
}

func TestDomainWritePotentialStatesFail(t *testing.T) {
	schema := componentmocks.NewSchema(t)
	schemaID := tktypes.Bytes32(tktypes.RandBytes(32))
	schema.On("ID").Return(schemaID)
	schema.On("Signature").Return("schema1_signature")
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(schema), mockBlockHeight)
	defer done()

	td.mdc.On("UpsertStates", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.PostAssembly.OutputStatesPotential = []*prototk.NewState{
		{SchemaId: schemaID.String()},
	}
	err := psc.WritePotentialStates(td.mdc, tx)
	assert.Regexp(t, "pop", err)
}

func TestDomainWritePotentialStatesBadID(t *testing.T) {
	schema := componentmocks.NewSchema(t)
	schemaID := tktypes.Bytes32(tktypes.RandBytes(32))
	schema.On("ID").Return(schemaID)
	schema.On("Signature").Return("schema1_signature")
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(schema), mockBlockHeight)
	defer done()
	badBytes := "0xnothex"

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.PostAssembly.OutputStatesPotential = []*prototk.NewState{
		{SchemaId: schemaID.String(), Id: &badBytes},
	}
	err := psc.WritePotentialStates(td.mdc, tx)
	assert.Regexp(t, "PD020007", err)
}

func TestEndorseTransactionFail(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.PostAssembly.OutputStates = []*components.FullState{}

	td.tp.Functions.EndorseTransaction = func(ctx context.Context, etr *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	_, err := psc.EndorseTransaction(td.mdc, &components.PrivateTransactionEndorseRequest{
		TransactionSpecification: tx.PreAssembly.TransactionSpecification,
		Verifiers:                tx.PreAssembly.Verifiers,
		Signatures:               tx.PostAssembly.Signatures,
		InputStates:              psc.d.toEndorsableList(tx.PostAssembly.InputStates),
		ReadStates:               psc.d.toEndorsableList(tx.PostAssembly.ReadStates),
		OutputStates:             psc.d.toEndorsableList(tx.PostAssembly.OutputStates),
		Endorsement:              &prototk.AttestationRequest{},
		Endorser:                 &prototk.ResolvedVerifier{},
	})
	assert.Regexp(t, "pop", err)
}

func TestResolveDispatchDuplicateSigners(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.PostAssembly.Endorsements = []*prototk.AttestationResult{
		{
			Name: "endorse", Verifier: &prototk.ResolvedVerifier{Lookup: "verifier1"},
			Constraints: []prototk.AttestationResult_AttestationConstraint{prototk.AttestationResult_ENDORSER_MUST_SUBMIT},
		},
		{
			Name: "endorse", Verifier: &prototk.ResolvedVerifier{Lookup: "verifier2"},
			Constraints: []prototk.AttestationResult_AttestationConstraint{prototk.AttestationResult_ENDORSER_MUST_SUBMIT},
		},
	}

	err := psc.ResolveDispatch(td.ctx, tx)
	assert.Regexp(t, "PD011623", err)
}

func TestResolveDispatchSignerOneTimeUse(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.PostAssembly.Endorsements = []*prototk.AttestationResult{}

	err := psc.ResolveDispatch(td.ctx, tx)
	require.NoError(t, err)
	assert.Equal(t, "one/time/keys/"+tx.ID.String(), tx.Signer)
}

func TestResolveDispatchNoEndorser(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	td.d.config.BaseLedgerSubmitConfig.SubmitMode = prototk.BaseLedgerSubmitConfig_ENDORSER_SUBMISSION

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.PostAssembly.Endorsements = []*prototk.AttestationResult{}

	err := psc.ResolveDispatch(td.ctx, tx)
	assert.Regexp(t, "PD011624", err)
}

func TestResolveDispatchWrongType(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	td.d.config.BaseLedgerSubmitConfig.SubmitMode = prototk.BaseLedgerSubmitConfig_Mode(99)

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.PostAssembly.Endorsements = []*prototk.AttestationResult{}

	err := psc.ResolveDispatch(td.ctx, tx)
	assert.Regexp(t, "PD011625", err)
}

func TestPrepareTransactionFail(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.Signer = "signer1"

	td.tp.Functions.PrepareTransaction = func(ctx context.Context, ptr *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	err := psc.PrepareTransaction(td.mdc, tx)
	assert.Regexp(t, "pop", err)
}

func TestPrepareTransactionABIInvalid(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.Signer = "signer1"

	td.tp.Functions.PrepareTransaction = func(ctx context.Context, ptr *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
		return &prototk.PrepareTransactionResponse{
			Transaction: &prototk.PreparedTransaction{
				FunctionAbiJson: `!!!wrong`,
			},
		}, nil
	}

	err := psc.PrepareTransaction(td.mdc, tx)
	assert.Regexp(t, "PD011607", err)
}

func TestPrepareTransactionPrivateResult(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.Signer = "signer1"

	contractAddr := tktypes.RandAddress()
	td.dm.contractCache.Set(*contractAddr, psc)

	td.tp.Functions.PrepareTransaction = func(ctx context.Context, ptr *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
		return &prototk.PrepareTransactionResponse{
			Transaction: &prototk.PreparedTransaction{
				Type:            prototk.PreparedTransaction_PRIVATE,
				FunctionAbiJson: fakeDownstreamPrivateABI,
				ParamsJson:      `{"thing": "something else"}`,
				ContractAddress: confutil.P(contractAddr.String()),
			},
		}, nil
	}

	err := psc.PrepareTransaction(td.mdc, tx)
	require.NoError(t, err)
	assert.Equal(t, pldapi.Transaction{
		IdempotencyKey: fmt.Sprintf("%s_doTheNextThing", tx.ID),
		Type:           pldapi.TransactionTypePrivate.Enum(),
		Function:       "doTheNextThing(string)",
		From:           tx.Signer,
		To:             contractAddr,
		Data:           tktypes.RawJSON(`{"thing": "something else"}`),
		Domain:         psc.Domain().Name(),
	}, tx.PreparedPrivateTransaction.Transaction)
}

func TestPrepareTransactionPrivateBadAddr(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.Signer = "signer1"

	td.tp.Functions.PrepareTransaction = func(ctx context.Context, ptr *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
		return &prototk.PrepareTransactionResponse{
			Transaction: &prototk.PreparedTransaction{
				Type:            prototk.PreparedTransaction_PRIVATE,
				FunctionAbiJson: fakeDownstreamPrivateABI,
				ParamsJson:      `{"thing": "something else"}`,
				ContractAddress: confutil.P("wrong"),
			},
		}, nil
	}

	err := psc.PrepareTransaction(td.mdc, tx)
	require.Regexp(t, "bad address", err)
}

func TestPrepareTransactionUnknownContract(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight, func(mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows([]string{}))
	})
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.Signer = "signer1"

	contractAddr := tktypes.RandAddress()

	td.tp.Functions.PrepareTransaction = func(ctx context.Context, ptr *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
		return &prototk.PrepareTransactionResponse{
			Transaction: &prototk.PreparedTransaction{
				Type:            prototk.PreparedTransaction_PRIVATE,
				FunctionAbiJson: fakeDownstreamPrivateABI,
				ParamsJson:      `{"thing": "something else"}`,
				ContractAddress: confutil.P(contractAddr.String()),
			},
		}, nil
	}

	err := psc.PrepareTransaction(td.mdc, tx)
	require.Regexp(t, "PD011609", err)
}

func TestLoadStatesBadSchema(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.Signer = "signer1"

	_, err := psc.loadStates(td.mdc, []*prototk.StateRef{
		{
			Id:       tktypes.RandHex(32),
			SchemaId: "wrong",
		},
	})
	assert.Regexp(t, "PD011614", err)
}

func TestLoadStatesError(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	td.mdc.On("FindAvailableStates", mock.Anything, mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.Signer = "signer1"

	_, err := psc.loadStates(td.mdc, []*prototk.StateRef{
		{
			Id:       tktypes.RandHex(32),
			SchemaId: tktypes.RandHex(32),
		},
	})
	assert.Regexp(t, "pop", err)
}

func TestLoadStatesNotFound(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	td.mdc.On("FindAvailableStates", mock.Anything, mock.Anything).Return(nil, []*components.State{}, nil)

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.Signer = "signer1"

	_, err := psc.loadStates(td.mdc, []*prototk.StateRef{
		{
			Id:       tktypes.RandHex(32),
			SchemaId: tktypes.RandHex(32),
		},
	})
	assert.Regexp(t, "PD011615", err)
}

func TestIncompleteStages(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	psc := goodPSC(td.d)
	tx := &components.PrivateTransaction{}

	err := psc.InitTransaction(td.ctx, tx)
	assert.Regexp(t, "PD011626", err)

	err = psc.AssembleTransaction(td.mdc, tx)
	assert.Regexp(t, "PD011627", err)

	err = psc.WritePotentialStates(td.mdc, tx)
	assert.Regexp(t, "PD011628", err)

	err = psc.LockStates(td.mdc, tx)
	assert.Regexp(t, "PD011629", err)

	_, err = psc.EndorseTransaction(td.mdc, nil)
	assert.Regexp(t, "PD011630", err)

	err = psc.ResolveDispatch(td.ctx, tx)
	assert.Regexp(t, "PD011631", err)

	err = psc.PrepareTransaction(td.mdc, tx)
	assert.Regexp(t, "PD011632", err)
}
