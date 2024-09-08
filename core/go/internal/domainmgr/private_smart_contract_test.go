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
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/statestore"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPrivateSmartContractQueryFail(t *testing.T) {

	ctx, dm, _, done := newTestDomain(t, false, goodDomainConf(), func(mc *mockComponents) {
		mc.domainStateInterface.On("EnsureABISchemas", mock.Anything).Return(nil, nil)
		mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	_, err := dm.GetSmartContractByAddress(ctx, tktypes.EthAddress(tktypes.RandBytes(20)))
	assert.Regexp(t, "pop", err)

}

func TestPrivateSmartContractQueryNoResult(t *testing.T) {

	ctx, dm, _, done := newTestDomain(t, false, goodDomainConf(), func(mc *mockComponents) {
		mc.domainStateInterface.On("EnsureABISchemas", mock.Anything).Return(nil, nil)
		mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows([]string{}))
	})
	defer done()

	_, err := dm.GetSmartContractByAddress(ctx, tktypes.EthAddress(tktypes.RandBytes(20)))
	assert.Regexp(t, "PD011609", err)

}

func goodPSC(d *domain) *domainContract {
	return d.newSmartContract(&PrivateSmartContract{
		DeployTX:      uuid.New(),
		DomainAddress: *d.Address(),
		Address:       tktypes.EthAddress(tktypes.RandBytes(20)),
		ConfigBytes:   []byte{0xfe, 0xed, 0xbe, 0xef},
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

func doDomainInitTransactionOK(t *testing.T, ctx context.Context, tp *testPlugin, resFn ...func(*prototk.InitTransactionResponse)) (*domainContract, *components.PrivateTransaction) {
	psc := goodPSC(tp.d)
	tx := goodPrivateTXWithInputs(psc)
	tx.PreAssembly = &components.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{},
	}
	tp.Functions.InitTransaction = func(ctx context.Context, itr *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
		assert.Equal(t, tktypes.Bytes32UUIDFirst16(tx.ID).String(), itr.Transaction.TransactionId)
		assert.Equal(t, int64(12345), itr.Transaction.BaseBlock)
		res := &prototk.InitTransactionResponse{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{
					Lookup:    tx.Signer,
					Algorithm: algorithms.ECDSA_SECP256K1_PLAINBYTES,
				},
			},
		}
		for _, fn := range resFn {
			fn(res)
		}
		return res, nil
	}

	err := psc.InitTransaction(ctx, tx)
	require.NoError(t, err)
	assert.Len(t, tx.PreAssembly.RequiredVerifiers, 1)
	return psc, tx
}

func doDomainInitAssembleTransactionOK(t *testing.T, ctx context.Context, tp *testPlugin) (*domainContract, *components.PrivateTransaction) {
	psc, tx := doDomainInitTransactionOK(t, ctx, tp)
	tp.Functions.AssembleTransaction = func(ctx context.Context, atr *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
		return &prototk.AssembleTransactionResponse{
			AssemblyResult:       prototk.AssembleTransactionResponse_OK,
			AssembledTransaction: &prototk.AssembledTransaction{},
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "ensorsement1",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1_PLAINBYTES,
					Parties:         []string{"endorser1"},
				},
			},
		}, nil
	}
	err := psc.AssembleTransaction(ctx, tx)
	require.NoError(t, err)
	return psc, tx
}

func mockBlockHeight(mc *mockComponents) {
	mc.blockIndexer.On("GetConfirmedBlockHeight", mock.Anything).Return(uint64(12345), nil)
}

func TestDomainInitTransactionOK(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	_, _ = doDomainInitTransactionOK(t, ctx, tp)
}

func TestDomainInitTransactionOKWithEncodeEntry(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	_, tx := doDomainInitTransactionOK(t, ctx, tp, func(res *prototk.InitTransactionResponse) {
		res.AbiEncodingRequests = []*prototk.ABIEncodingRequest{
			{
				Name:            "fnEncode",
				AbiEncodingType: prototk.ABIEncodingRequest_FUNCTION_CALL_DATA,
				AbiEntry: `{
				  "type": "function",
				  "name": "doStuff",
				  "inputs": [
				     { "name": "intVal", "type": "uint256" }
				  ]
				}`,
				ParamsJson: `{ "intVal": 42 }`,
			},
			{
				Name:            "tupleEncode",
				AbiEncodingType: prototk.ABIEncodingRequest_TUPLE,
				AbiEntry: `{
				  "type": "tuple",
				  "components": [
				     { "name": "intVal", "type": "uint256" }
				  ]
				}`,
				ParamsJson: `{ "intVal": 42 }`,
			},
		}
	})
	assert.Len(t, tx.PreAssembly.ABIEncodedData, 2)

	fnEncode := tx.PreAssembly.ABIEncodedData[0]
	assert.Equal(t, "fnEncode", fnEncode.Name)
	assert.Equal(t, "0x23bad5cd000000000000000000000000000000000000000000000000000000000000002a", tktypes.HexBytes(fnEncode.Data).String())

	tupleEncode := tx.PreAssembly.ABIEncodedData[1]
	assert.Equal(t, "tupleEncode", tupleEncode.Name)
	assert.Equal(t, "0x000000000000000000000000000000000000000000000000000000000000002a", tktypes.HexBytes(tupleEncode.Data).String())
}

func TestDomainInitTransactionWithEncodeEntryFail(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	psc := goodPSC(tp.d)
	tx := goodPrivateTXWithInputs(psc)
	tp.Functions.InitTransaction = func(ctx context.Context, itr *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
		return &prototk.InitTransactionResponse{
			AbiEncodingRequests: []*prototk.ABIEncodingRequest{
				{Name: "wrong", AbiEncodingType: prototk.ABIEncodingRequest_ABIEncodingType(99)},
			},
		}, nil
	}
	err := psc.InitTransaction(ctx, tx)
	assert.Regexp(t, "PD011635", err)
	assert.Nil(t, tx.PreAssembly)

}

func TestDomainInitTransactionMissingInput(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	psc := goodPSC(tp.d)

	tx := &components.PrivateTransaction{}
	err := psc.InitTransaction(ctx, tx)
	assert.Regexp(t, "PD011626", err)
	assert.Nil(t, tx.PreAssembly)

}

func TestDomainInitTransactionConfirmedBlockFail(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.blockIndexer.On("GetConfirmedBlockHeight", mock.Anything).Return(uint64(0), fmt.Errorf("pop"))
	})
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	psc := goodPSC(tp.d)
	tx := goodPrivateTXWithInputs(psc)

	err := psc.InitTransaction(ctx, tx)
	assert.Regexp(t, "pop", err)
	assert.Nil(t, tx.PreAssembly)

}

func TestDomainInitTransactionError(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	psc := goodPSC(tp.d)
	tx := goodPrivateTXWithInputs(psc)

	tp.Functions.InitTransaction = func(ctx context.Context, itr *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	err := psc.InitTransaction(ctx, tx)
	assert.Regexp(t, "pop", err)
	assert.Nil(t, tx.PreAssembly)

}

func TestDomainInitTransactionBadInputs(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	psc := goodPSC(tp.d)
	tx := goodPrivateTXWithInputs(psc)
	tx.Inputs.Inputs = tktypes.RawJSON(`{"missing": "parameters}`)

	err := psc.InitTransaction(ctx, tx)
	assert.Regexp(t, "PD011612", err)
	assert.Nil(t, tx.PreAssembly)

}

func TestFullTransactionRealDBOK(t *testing.T) {
	ctx, dm, tp, done := newTestDomain(t, true /* real DB */, goodDomainConf(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitTransactionOK(t, ctx, tp)
	domain := tp.d

	state1 := storeState(t, dm, tp, tx.ID, ethtypes.NewHexInteger64(1111111))
	state2 := storeState(t, dm, tp, tx.ID, ethtypes.NewHexInteger64(2222222))
	state3 := storeState(t, dm, tp, tx.ID, ethtypes.NewHexInteger64(3333333))
	state4 := storeState(t, dm, tp, tx.ID, ethtypes.NewHexInteger64(4444444))

	state5 := &fakeState{
		Salt:   tktypes.Bytes32(tktypes.RandBytes(32)),
		Owner:  tktypes.EthAddress(tktypes.RandBytes(20)),
		Amount: ethtypes.NewHexInteger64(5555555),
	}
	tp.Functions.AssembleTransaction = func(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
		assert.Same(t, req.Transaction, tx.PreAssembly.TransactionSpecification)

		stateRes, err := domain.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
			SchemaId: tp.stateSchemas[0].Id,
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
					{SchemaId: tp.stateSchemas[0].Id, StateDataJson: string(newStateData)},
				},
			},
			AttestationPlan: []*prototk.AttestationRequest{
				{Name: "sign", AttestationType: prototk.AttestationType_SIGN, Algorithm: algorithms.ECDSA_SECP256K1_PLAINBYTES},
			},
		}, nil
	}
	err := psc.AssembleTransaction(ctx, tx)
	require.NoError(t, err)

	assert.Len(t, tx.PostAssembly.InputStates, 2)
	assert.Len(t, tx.PostAssembly.ReadStates, 1)
	assert.Len(t, tx.PostAssembly.OutputStatesPotential, 1)
	assert.Equal(t, prototk.AssembleTransactionResponse_OK, tx.PostAssembly.AssemblyResult)
	assert.Len(t, tx.PostAssembly.AttestationPlan, 1)

	// Write the output states
	err = psc.WritePotentialStates(ctx, tx)
	require.NoError(t, err)

	stateRes, err := domain.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
		SchemaId: tx.PostAssembly.OutputStatesPotential[0].SchemaId,
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
	err = psc.LockStates(ctx, tx)
	require.NoError(t, err)

	stillAvailable, err := domain.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
		SchemaId:  tx.PostAssembly.OutputStatesPotential[0].SchemaId,
		QueryJson: `{}`,
	})
	require.NoError(t, err)
	assert.Len(t, stillAvailable.States, 3)
	// state1 & state3 are now locked for spending (state4 was just read, and state2 untouched)
	// state2 & state4 still exist
	// state5 is new
	// The order should be deterministic based on crate time (even before written to DB)
	log.L(ctx).Infof("STATES %+v", stillAvailable.States)
	assert.Contains(t, stillAvailable.States[0].DataJson, state2.Salt.String())
	assert.Contains(t, stillAvailable.States[1].DataJson, state4.Salt.String())
	assert.Contains(t, stillAvailable.States[2].DataJson, state5.Salt.String())

	tp.Functions.EndorseTransaction = func(ctx context.Context, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
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
		Lookup:    "endorser1",
		Algorithm: algorithms.ECDSA_SECP256K1_PLAINBYTES,
		Verifier:  endorserAddr.String(),
	}
	endorsement, err := psc.EndorseTransaction(ctx, tx, endorsementRequest, endorser)
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
	err = psc.ResolveDispatch(ctx, tx)
	require.NoError(t, err)
	assert.Equal(t, "endorser1", tx.Signer)

	tp.Functions.PrepareTransaction = func(ctx context.Context, ptr *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
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
			Transaction: &prototk.BaseLedgerTransaction{
				FunctionName: "execute",
				ParamsJson:   string(params),
			},
		}, nil
	}

	// And now prepare
	err = psc.PrepareTransaction(ctx, tx)
	require.NoError(t, err)
	assert.NotNil(t, tx.PreparedTransaction.FunctionABI)
	assert.NotNil(t, tx.PreparedTransaction.Inputs)
}

func TestDomainAssembleTransactionError(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitTransactionOK(t, ctx, tp)
	tp.Functions.AssembleTransaction = func(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
		return nil, fmt.Errorf("pop")
	}
	err := psc.AssembleTransaction(ctx, tx)
	assert.Regexp(t, "pop", err)

	assert.Nil(t, tx.PostAssembly)
}

func TestDomainAssembleTransactionLoadInputError(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitTransactionOK(t, ctx, tp)
	tp.Functions.AssembleTransaction = func(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
		return &prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			AssembledTransaction: &prototk.AssembledTransaction{
				InputStates: []*prototk.StateRef{
					{Id: "badid", SchemaId: "schemaid"},
				},
			},
			AttestationPlan: []*prototk.AttestationRequest{
				{Name: "sign", AttestationType: prototk.AttestationType_SIGN, Algorithm: algorithms.ECDSA_SECP256K1_PLAINBYTES},
			},
		}, nil
	}
	err := psc.AssembleTransaction(ctx, tx)
	assert.Regexp(t, "PD011614.*badid", err)

	assert.Nil(t, tx.PostAssembly)
}

func TestDomainAssembleTransactionLoadReadError(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitTransactionOK(t, ctx, tp)
	tp.Functions.AssembleTransaction = func(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
		return &prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			AssembledTransaction: &prototk.AssembledTransaction{
				ReadStates: []*prototk.StateRef{
					{Id: "badid", SchemaId: "schemaid"},
				},
			},
			AttestationPlan: []*prototk.AttestationRequest{
				{Name: "sign", AttestationType: prototk.AttestationType_SIGN, Algorithm: algorithms.ECDSA_SECP256K1_PLAINBYTES},
			},
		}, nil
	}
	err := psc.AssembleTransaction(ctx, tx)
	assert.Regexp(t, "PD011614.*badid", err)

	assert.Nil(t, tx.PostAssembly)
}

func TestDomainWritePotentialStatesBadSchema(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, ctx, tp)
	tx.PostAssembly.OutputStatesPotential = []*prototk.NewState{
		{SchemaId: "unknown"},
	}
	err := psc.WritePotentialStates(ctx, tx)
	assert.Regexp(t, "PD011613", err)
}

func TestDomainWritePotentialStatesFail(t *testing.T) {
	schema := componentmocks.NewSchema(t)
	schema.On("IDString").Return("schema1")
	schema.On("Signature").Return("schema1_signature")
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(schema), mockBlockHeight, func(mc *mockComponents) {
		mc.domainStateInterface.On("UpsertStates", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	})
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, ctx, tp)
	tx.PostAssembly.OutputStatesPotential = []*prototk.NewState{
		{SchemaId: "schema1"},
	}
	err := psc.WritePotentialStates(ctx, tx)
	assert.Regexp(t, "pop", err)
}

func TestEndorseTransactionFail(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, ctx, tp)
	tx.PostAssembly.OutputStates = []*components.FullState{}

	tp.Functions.EndorseTransaction = func(ctx context.Context, etr *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	_, err := psc.EndorseTransaction(ctx, tx, &prototk.AttestationRequest{}, &prototk.ResolvedVerifier{})
	assert.Regexp(t, "pop", err)
}

func TestResolveDispatchDuplicateSigners(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, ctx, tp)
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

	err := psc.ResolveDispatch(ctx, tx)
	assert.Regexp(t, "PD011623", err)
}

func TestResolveDispatchSignerOneTimeUse(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, ctx, tp)
	tx.PostAssembly.Endorsements = []*prototk.AttestationResult{}

	err := psc.ResolveDispatch(ctx, tx)
	require.NoError(t, err)
	assert.Equal(t, "one/time/keys/"+tx.ID.String(), tx.Signer)
}

func TestResolveDispatchNoEndorser(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	tp.d.config.BaseLedgerSubmitConfig.SubmitMode = prototk.BaseLedgerSubmitConfig_ENDORSER_SUBMISSION

	psc, tx := doDomainInitAssembleTransactionOK(t, ctx, tp)
	tx.PostAssembly.Endorsements = []*prototk.AttestationResult{}

	err := psc.ResolveDispatch(ctx, tx)
	assert.Regexp(t, "PD011624", err)
}

func TestResolveDispatchWrongType(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	tp.d.config.BaseLedgerSubmitConfig.SubmitMode = prototk.BaseLedgerSubmitConfig_Mode(99)

	psc, tx := doDomainInitAssembleTransactionOK(t, ctx, tp)
	tx.PostAssembly.Endorsements = []*prototk.AttestationResult{}

	err := psc.ResolveDispatch(ctx, tx)
	assert.Regexp(t, "PD011625", err)
}

func TestPrepareTransactionFail(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, ctx, tp)
	tx.Signer = "signer1"

	tp.Functions.PrepareTransaction = func(ctx context.Context, ptr *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	err := psc.PrepareTransaction(ctx, tx)
	assert.Regexp(t, "pop", err)
}

func TestPrepareTransactionBadFunction(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, ctx, tp)
	tx.Signer = "signer1"

	tp.Functions.PrepareTransaction = func(ctx context.Context, ptr *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
		return &prototk.PrepareTransactionResponse{
			Transaction: &prototk.BaseLedgerTransaction{
				FunctionName: "wrong",
			},
		}, nil
	}

	err := psc.PrepareTransaction(ctx, tx)
	assert.Regexp(t, "PD011618", err)
}

func TestPrepareTransactionBadData(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, ctx, tp)
	tx.Signer = "signer1"

	tp.Functions.PrepareTransaction = func(ctx context.Context, ptr *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
		return &prototk.PrepareTransactionResponse{
			Transaction: &prototk.BaseLedgerTransaction{
				FunctionName: "execute",
				ParamsJson:   `{"missing": "expected"}`,
			},
		}, nil
	}

	err := psc.PrepareTransaction(ctx, tx)
	assert.Regexp(t, "FF22040", err)
}

func TestLoadStatesError(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight, func(mc *mockComponents) {
		mc.domainStateInterface.On("FindAvailableStates", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	})
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, ctx, tp)
	tx.Signer = "signer1"

	_, err := psc.loadStates(ctx, []*prototk.StateRef{
		{Id: tktypes.RandHex(32)},
	})
	assert.Regexp(t, "pop", err)
}

func TestLoadStatesNotFound(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight, func(mc *mockComponents) {
		mc.domainStateInterface.On("FindAvailableStates", mock.Anything, mock.Anything).Return([]*statestore.State{}, nil)
	})
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, ctx, tp)
	tx.Signer = "signer1"

	_, err := psc.loadStates(ctx, []*prototk.StateRef{
		{Id: tktypes.RandHex(32)},
	})
	assert.Regexp(t, "PD011615", err)
}

func TestEncodeABIDataFailCases(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	psc := goodPSC(tp.d)

	_, err := psc.encodeABIData(ctx, &prototk.ABIEncodingRequest{
		Name:            "badFuncDef",
		AbiEncodingType: prototk.ABIEncodingRequest_FUNCTION_CALL_DATA,
		AbiEntry:        `{!!!`,
	})
	assert.Regexp(t, "PD011633", err)
	_, err = psc.encodeABIData(ctx, &prototk.ABIEncodingRequest{
		Name:            "badTupleDef",
		AbiEncodingType: prototk.ABIEncodingRequest_TUPLE,
		AbiEntry:        `{!!!`,
	})
	assert.Regexp(t, "PD011633", err)
	_, err = psc.encodeABIData(ctx, &prototk.ABIEncodingRequest{
		Name:            "badFuncInput",
		AbiEncodingType: prototk.ABIEncodingRequest_FUNCTION_CALL_DATA,
		AbiEntry:        `{"inputs":[{"name":"int1","type":"uint256"}]}`,
		ParamsJson:      `{}`,
	})
	assert.Regexp(t, "PD011634.*int1", err)
	_, err = psc.encodeABIData(ctx, &prototk.ABIEncodingRequest{
		Name:            "badTupleInput",
		AbiEncodingType: prototk.ABIEncodingRequest_TUPLE,
		AbiEntry:        `{"components":[{"name":"int1","type":"uint256"}]}`,
		ParamsJson:      `{}`,
	})
	assert.Regexp(t, "PD011634.*int1", err)
}

func TestIncompleteStages(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	psc := goodPSC(tp.d)
	tx := &components.PrivateTransaction{}

	err := psc.InitTransaction(ctx, tx)
	assert.Regexp(t, "PD011626", err)

	err = psc.AssembleTransaction(ctx, tx)
	assert.Regexp(t, "PD011627", err)

	err = psc.WritePotentialStates(ctx, tx)
	assert.Regexp(t, "PD011628", err)

	err = psc.LockStates(ctx, tx)
	assert.Regexp(t, "PD011629", err)

	_, err = psc.EndorseTransaction(ctx, tx, nil, nil)
	assert.Regexp(t, "PD011630", err)

	err = psc.ResolveDispatch(ctx, tx)
	assert.Regexp(t, "PD011631", err)

	err = psc.PrepareTransaction(ctx, tx)
	assert.Regexp(t, "PD011632", err)
}
