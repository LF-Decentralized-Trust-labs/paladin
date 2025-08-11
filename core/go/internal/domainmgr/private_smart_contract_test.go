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
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/keymanager"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signpayloads"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPrivateSmartContractQueryFail(t *testing.T) {

	td, done := newTestDomain(t, false, goodDomainConf(), func(mc *mockComponents) {
		mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, nil)
		mc.db.ExpectBegin()
		mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	_, err := td.dm.GetSmartContractByAddress(td.ctx, td.c.dbTX, pldtypes.EthAddress(pldtypes.RandBytes(20)))
	assert.Regexp(t, "pop", err)

}

func TestPrivateSmartContractQueryNoResult(t *testing.T) {

	td, done := newTestDomain(t, false, goodDomainConf(), func(mc *mockComponents) {
		mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, nil)
		mc.db.ExpectBegin()
		mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows([]string{}))
	})
	defer done()

	_, err := td.dm.GetSmartContractByAddress(td.ctx, td.c.dbTX, pldtypes.EthAddress(pldtypes.RandBytes(20)))
	assert.Regexp(t, "PD011609", err)

}

func goodPSC(t *testing.T, td *testDomainContext) *domainContract {
	d := td.d
	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{
			Valid: true,
			ContractConfig: &prototk.ContractConfig{
				ContractConfigJson:   `{}`,
				CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
				SubmitterSelection:   prototk.ContractConfig_SUBMITTER_SENDER,
			},
		}, nil
	}

	loadResult, psc, err := d.initSmartContract(d.ctx, &PrivateSmartContract{
		DeployTX:        uuid.New(),
		RegistryAddress: *d.RegistryAddress(),
		Address:         pldtypes.EthAddress(pldtypes.RandBytes(20)),
		ConfigBytes:     []byte{0xfe, 0xed, 0xbe, 0xef},
	})
	require.NoError(t, err)
	require.Equal(t, pscValid, loadResult)
	require.NotNil(t, psc.ContractConfig())
	return psc
}

func goodPrivateTXWithInputs(psc *domainContract) *components.ResolvedTransaction {
	return &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			ID: confutil.P(uuid.New()),
			TransactionBase: pldapi.TransactionBase{
				Domain: psc.d.name,
				From:   "txSigner",
				To:     &psc.info.Address,
				Data: pldtypes.RawJSON(`{
				   "from": "sender",
				   "to": "receiver",
				   "amount": "123000000000000000000"
				}`),
			},
		},
		Function: &components.ResolvedFunction{
			Definition: &abi.Entry{
				Type: abi.Function,
				Inputs: abi.ParameterArray{
					{Name: "from", Type: "string"},
					{Name: "to", Type: "string"},
					{Name: "amount", Type: "uint256"},
				},
			},
		},
	}
}

func doDomainInitTransactionOK(t *testing.T, td *testDomainContext, resFn ...func(*prototk.InitTransactionResponse)) (*domainContract, *components.PrivateTransaction, *components.ResolvedTransaction) {
	psc := goodPSC(t, td)
	localTx := goodPrivateTXWithInputs(psc)

	td.tp.Functions.InitTransaction = func(ctx context.Context, itr *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
		assert.Equal(t, pldtypes.Bytes32UUIDFirst16(*localTx.Transaction.ID).String(), itr.Transaction.TransactionId)
		assert.Equal(t, int64(12345), itr.Transaction.BaseBlock)
		res := &prototk.InitTransactionResponse{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{
					Lookup:       localTx.Transaction.From,
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

	ptx := &components.PrivateTransaction{
		ID:      *localTx.Transaction.ID,
		Domain:  localTx.Transaction.Domain,
		Address: *localTx.Transaction.To,
	}
	err := psc.InitTransaction(td.ctx, ptx, localTx)
	require.NoError(t, err)
	assert.Len(t, ptx.PreAssembly.RequiredVerifiers, 1)
	return psc, ptx, localTx
}

func doDomainInitAssembleTransactionOK(t *testing.T, td *testDomainContext) (*domainContract, *components.PrivateTransaction) {
	psc, tx, localTx := doDomainInitTransactionOK(t, td)
	td.tp.Functions.AssembleTransaction = func(ctx context.Context, atr *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
		return &prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			AssembledTransaction: &prototk.AssembledTransaction{
				OutputStates: []*prototk.NewState{
					{
						SchemaId:         "schema1",
						DistributionList: []string{"party1"},
						NullifierSpecs: []*prototk.NullifierSpec{
							{
								Party: "party1",
							},
						},
					},
				},
				InfoStates: []*prototk.NewState{
					{
						SchemaId:         "schema2",
						DistributionList: []string{"party2@node2"},
						NullifierSpecs: []*prototk.NullifierSpec{
							{
								Party: "party2@node2",
							},
						},
					},
				},
			},
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "endorsement1",
					AttestationType: prototk.AttestationType_ENDORSE,
					Algorithm:       algorithms.ECDSA_SECP256K1,
					PayloadType:     signpayloads.OPAQUE_TO_RSV,
					Parties:         []string{"endorser1"},
				},
			},
		}, nil
	}
	err := psc.AssembleTransaction(td.mdc, td.c.dbTX, tx, localTx)
	require.NoError(t, err)
	tx.PreAssembly.Verifiers = []*prototk.ResolvedVerifier{}
	tx.PostAssembly.Signatures = []*prototk.AttestationResult{}
	// Check we resolved the identities to local node
	require.Equal(t, "endorser1@node1", tx.PostAssembly.AttestationPlan[0].Parties[0])
	require.Equal(t, "party1@node1", tx.PostAssembly.OutputStatesPotential[0].DistributionList[0])
	require.Equal(t, "party1@node1", tx.PostAssembly.OutputStatesPotential[0].NullifierSpecs[0].Party)
	require.Equal(t, "party2@node2", tx.PostAssembly.InfoStatesPotential[0].DistributionList[0])
	require.Equal(t, "party2@node2", tx.PostAssembly.InfoStatesPotential[0].NullifierSpecs[0].Party)
	return psc, tx
}

func mockBlockHeight(mc *mockComponents) {
	mc.blockIndexer.On("GetConfirmedBlockHeight", mock.Anything).Return(pldtypes.HexUint64(12345), nil)
}

func TestDomainInitTransactionOK(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, td.d.initError.Load())

	_, _, _ = doDomainInitTransactionOK(t, td)
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
	require.NoError(t, err)

	decResult, err := td.d.DecodeData(td.ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_FUNCTION_CALL_DATA,
		Definition:   funcDef,
		Data:         encResult.Data,
	})
	require.NoError(t, err)
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
	require.NoError(t, err)

	decResult, err = td.d.DecodeData(td.ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_TUPLE,
		Definition:   tupleDef,
		Data:         encResult.Data,
	})
	require.NoError(t, err)
	assert.Equal(t, `{"intVal":"42"}`, decResult.Body)

	txEIP1559_a, err := td.d.EncodeData(td.ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION,
		Definition:   "",
		Body: `{
		  "to": "0x05d936207F04D81a85881b72A0D17854Ee8BE45A"
		}`,
	})
	require.NoError(t, err)

	txEIP1559_b, err := td.d.EncodeData(td.ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION,
		Definition:   "eip1559",
		Body: `{
		  "to": "0x05d936207F04D81a85881b72A0D17854Ee8BE45A"
		}`,
	})
	require.NoError(t, err)
	assert.Equal(t, txEIP1559_a, txEIP1559_b)

	txEIP155, err := td.d.EncodeData(td.ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION,
		Definition:   "eip155",
		Body: `{
		  "to": "0x05d936207F04D81a85881b72A0D17854Ee8BE45A"
		}`,
	})
	require.NoError(t, err)
	assert.NotEqual(t, txEIP155, txEIP1559_a)

	original, err := td.d.DecodeData(td.ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION,
		Definition:   "eip1559",
		Data:         txEIP1559_a.Data,
	})
	require.NoError(t, err)
	var recoveredTx *ethsigner.Transaction
	err = json.Unmarshal([]byte(original.Body), &recoveredTx)
	require.NoError(t, err)
	assert.Equal(t, "0x05d936207f04d81a85881b72a0d17854ee8be45a", recoveredTx.To.String())

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
	require.NoError(t, err)
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
	require.NoError(t, err)
	assert.Equal(t, `{"contractAddress":"3153e3e67d3d4be35aa5baff60b5a862f55a5431","encodedCall":"60fe47b10000000000000000000000000000000000000000000000000000000000000064"}`, decResult.Body)

}

func initRealKeyManagerForTest(t *testing.T) (components.KeyManager, func(mc *mockComponents)) {
	keymgr := keymanager.NewKeyManager(context.Background(), &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{
			{
				Name: "wallet1",
				Signer: &pldconf.SignerConfig{
					KeyDerivation: pldconf.KeyDerivationConfig{
						Type: pldconf.KeyDerivationTypeBIP32,
					},
					KeyStore: pldconf.KeyStoreConfig{
						Type: pldconf.KeyStoreTypeStatic,
						Static: pldconf.StaticKeyStoreConfig{
							Keys: map[string]pldconf.StaticKeyEntryConfig{
								"seed": {
									Encoding: "hex",
									Inline:   pldtypes.RandHex(32),
								},
							},
						},
					},
				},
			},
		},
	})
	return keymgr, func(mc *mockComponents) {
		_, err := keymgr.PreInit(mc.c)
		require.NoError(t, err)
		err = keymgr.PostInit(mc.c)
		require.NoError(t, err)
		err = keymgr.Start()
		require.NoError(t, err)
	}
}

func TestAttemptSignRemoteAddress(t *testing.T) {
	td, done := newTestDomain(t, true, goodDomainConf(), func(mc *mockComponents) {
		mc.transportMgr.On("LocalNodeName").Return("localnode")
	})
	defer done()
	assert.Nil(t, td.d.initError.Load())

	_, err := td.d.EncodeData(td.ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION_SIGNED,
		Definition:   "eip1559",
		Body: `{
		  "to": "0x05d936207F04D81a85881b72A0D17854Ee8BE45A"
		}`,
		KeyIdentifier: "key1@remote1",
	})
	assert.Regexp(t, "PD011656", err)
}

func TestEncodeDecodeABIDataWithSigningEIP1559(t *testing.T) {
	keymgr, setupKeyManager := initRealKeyManagerForTest(t)

	td, done := newTestDomain(t, true, goodDomainConf(), setupKeyManager)
	defer done()
	assert.Nil(t, td.d.initError.Load())

	td.d.dm.keyManager = keymgr

	expectedSigner, err := keymgr.ResolveEthAddressNewDatabaseTX(td.ctx, "key1")
	require.NoError(t, err)

	signedEIP1559Tx, err := td.d.EncodeData(td.ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION_SIGNED,
		Definition:   "eip1559",
		Body: `{
		  "to": "0x05d936207F04D81a85881b72A0D17854Ee8BE45A"
		}`,
		KeyIdentifier: "key1",
	})
	require.NoError(t, err)

	original, err := td.d.DecodeData(td.ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION_SIGNED,
		Definition:   "eip1559",
		Data:         signedEIP1559Tx.Data,
	})
	require.NoError(t, err)
	var recoveredTx *ethsigner.Transaction
	err = json.Unmarshal([]byte(original.Body), &recoveredTx)
	require.NoError(t, err)
	assert.Equal(t, "0x05d936207f04d81a85881b72a0d17854ee8be45a", recoveredTx.To.String())
	assert.Equal(t, fmt.Sprintf(`"%s"`, expectedSigner), string(recoveredTx.From))

}

func TestEncodeDecodeABIDataWithSigningEIP155(t *testing.T) {
	keymgr, setupKeyManager := initRealKeyManagerForTest(t)

	td, done := newTestDomain(t, true, goodDomainConf(), setupKeyManager)
	defer done()
	assert.Nil(t, td.d.initError.Load())

	td.d.dm.keyManager = keymgr

	expectedSigner, err := keymgr.ResolveEthAddressNewDatabaseTX(td.ctx, "key1")
	require.NoError(t, err)

	signedEIP1559Tx, err := td.d.EncodeData(td.ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION_SIGNED,
		Definition:   "eip155",
		Body: `{
		  "to": "0x05d936207F04D81a85881b72A0D17854Ee8BE45A"
		}`,
		KeyIdentifier: "key1",
	})
	require.NoError(t, err)

	original, err := td.d.DecodeData(td.ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION_SIGNED,
		Definition:   "eip155",
		Data:         signedEIP1559Tx.Data,
	})
	require.NoError(t, err)
	var recoveredTx *ethsigner.Transaction
	err = json.Unmarshal([]byte(original.Body), &recoveredTx)
	require.NoError(t, err)
	assert.Equal(t, "0x05d936207f04d81a85881b72a0d17854ee8be45a", recoveredTx.To.String())
	assert.Equal(t, fmt.Sprintf(`"%s"`, expectedSigner), string(recoveredTx.From))

}

func TestEncodeAndSignEIP1559Fail(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.keyManager.On("ResolveKeyNewDatabaseTX", mock.Anything, "key1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).
			Return(nil, fmt.Errorf("pop"))
	})
	defer done()
	assert.Nil(t, td.d.initError.Load())

	_, err := td.d.EncodeData(td.ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION_SIGNED,
		Definition:   "eip155",
		Body: `{
		  "to": "0x05d936207F04D81a85881b72A0D17854Ee8BE45A"
		}`,
		KeyIdentifier: "key1",
	})
	assert.Regexp(t, "PD011656.*pop", err)

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

func TestSendTransaction(t *testing.T) {
	txID := uuid.New()
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.txManager.On("SendTransactions", mock.Anything, mock.Anything, mock.Anything).Return([]uuid.UUID{txID}, nil)
	})
	defer done()
	assert.Nil(t, td.d.initError.Load())

	td.c.readOnly = false

	_, err := td.d.SendTransaction(td.ctx, &prototk.SendTransactionRequest{
		StateQueryContext: td.c.id,
		Transaction: &prototk.TransactionInput{
			ContractAddress: "0x05d936207F04D81a85881b72A0D17854Ee8BE45A",
			FunctionAbiJson: `{}`,
			ParamsJson:      `{}`,
		},
	})
	require.NoError(t, err)
}

func TestSendTransactionFail(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.txManager.On("SendTransactions", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	})
	defer done()
	assert.Nil(t, td.d.initError.Load())

	td.c.readOnly = false

	_, err := td.d.SendTransaction(td.ctx, &prototk.SendTransactionRequest{
		StateQueryContext: td.c.id,
		Transaction: &prototk.TransactionInput{
			ContractAddress: "0x05d936207F04D81a85881b72A0D17854Ee8BE45A",
			FunctionAbiJson: `{}`,
			ParamsJson:      `{}`,
		},
	})
	require.EqualError(t, err, "pop")
}

func TestLocalNodeName(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	res, err := td.d.LocalNodeName(td.ctx, &prototk.LocalNodeNameRequest{})
	require.NoError(t, err)
	assert.Equal(t, "node1", res.Name)
}

func TestDomainInitTransactionMissingInput(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)

	tx := &components.PrivateTransaction{}
	err := psc.InitTransaction(td.ctx, tx, &components.ResolvedTransaction{})
	assert.Regexp(t, "PD011626", err)
	assert.Nil(t, tx.PreAssembly)

}

func TestDomainInitTransactionConfirmedBlockFail(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.blockIndexer.On("GetConfirmedBlockHeight", mock.Anything).Return(pldtypes.HexUint64(0), fmt.Errorf("pop"))
	})
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)
	localTx := goodPrivateTXWithInputs(psc)

	ptx := &components.PrivateTransaction{}
	err := psc.InitTransaction(td.ctx, ptx, localTx)
	assert.Regexp(t, "pop", err)
	assert.Nil(t, ptx.PreAssembly)

}

func TestDomainInitTransactionError(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)
	localTx := goodPrivateTXWithInputs(psc)

	td.tp.Functions.InitTransaction = func(ctx context.Context, itr *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	ptx := &components.PrivateTransaction{
		ID: *localTx.Transaction.ID,
	}
	err := psc.InitTransaction(td.ctx, ptx, localTx)
	assert.Regexp(t, "pop", err)
	assert.Nil(t, ptx.PreAssembly)

}

func TestDomainInitTransactionBadInputs(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)
	localTx := goodPrivateTXWithInputs(psc)
	localTx.Transaction.Data = pldtypes.RawJSON(`{"missing": "parameters}`)

	ptx := &components.PrivateTransaction{
		ID: *localTx.Transaction.ID,
	}
	err := psc.InitTransaction(td.ctx, ptx, localTx)
	assert.Regexp(t, "PD011612", err)
	assert.Nil(t, ptx.PreAssembly)

}

func TestFullTransactionRealDBOK(t *testing.T) {
	td, done := newTestDomain(t, true /* real DB */, goodDomainConf(), mockBlockHeight)
	defer done()

	psc, ptx, localTx := doDomainInitTransactionOK(t, td)
	domain := td.d
	dCtx := td.c.dCtx

	state1 := storeTestState(t, td, ptx.ID, ethtypes.NewHexInteger64(1111111))
	state2 := storeTestState(t, td, ptx.ID, ethtypes.NewHexInteger64(2222222))
	state3 := storeTestState(t, td, ptx.ID, ethtypes.NewHexInteger64(3333333))
	state4 := storeTestState(t, td, ptx.ID, ethtypes.NewHexInteger64(4444444))

	state5 := &fakeState{
		Salt:   pldtypes.RandBytes32(),
		Owner:  pldtypes.EthAddress(pldtypes.RandBytes(20)),
		Amount: ethtypes.NewHexInteger64(5555555),
	}

	state6 := &fakeState{
		Salt:   pldtypes.RandBytes32(),
		Owner:  pldtypes.EthAddress(pldtypes.RandBytes(20)),
		Amount: ethtypes.NewHexInteger64(6666666),
	}

	td.tp.Functions.AssembleTransaction = func(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
		assert.Same(t, req.Transaction, ptx.PreAssembly.TransactionSpecification)

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

		infoStateData, err := json.Marshal(state6)
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
				InfoStates: []*prototk.NewState{
					{SchemaId: td.tp.stateSchemas[0].Id, StateDataJson: string(infoStateData)},
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
	err := psc.AssembleTransaction(dCtx, td.c.dbTX, ptx, localTx)
	require.NoError(t, err)

	assert.Len(t, ptx.PostAssembly.InputStates, 2)
	assert.Len(t, ptx.PostAssembly.ReadStates, 1)
	assert.Len(t, ptx.PostAssembly.OutputStatesPotential, 1)
	assert.Len(t, ptx.PostAssembly.InfoStatesPotential, 1)
	assert.Equal(t, prototk.AssembleTransactionResponse_OK, ptx.PostAssembly.AssemblyResult)
	assert.Len(t, ptx.PostAssembly.AttestationPlan, 1)

	// This would be the engine's job
	ptx.PreAssembly.Verifiers = make([]*prototk.ResolvedVerifier, 0)
	ptx.PostAssembly.Signatures = make([]*prototk.AttestationResult, 0)

	// Write the output states
	err = psc.WritePotentialStates(dCtx, td.c.dbTX, ptx)
	require.NoError(t, err)

	stateRes, err := domain.FindAvailableStates(td.ctx, &prototk.FindAvailableStatesRequest{
		StateQueryContext: td.c.id,
		SchemaId:          ptx.PostAssembly.OutputStatesPotential[0].SchemaId,
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
	err = psc.LockStates(dCtx, td.c.dbTX, ptx)
	require.NoError(t, err)

	stillAvailable, err := domain.FindAvailableStates(td.ctx, &prototk.FindAvailableStatesRequest{
		StateQueryContext: td.c.id,
		SchemaId:          ptx.PostAssembly.OutputStatesPotential[0].SchemaId,
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
		assert.Same(t, ptx.PreAssembly.TransactionSpecification, req.Transaction)
		assert.Same(t, ptx.PostAssembly.AttestationPlan[0], req.EndorsementRequest)
		assert.Equal(t, "endorser1", req.EndorsementVerifier.Lookup)
		assert.Same(t, ptx.PreAssembly.TransactionSpecification, req.Transaction)
		assert.Len(t, ptx.PostAssembly.InputStates, 2)
		assert.Contains(t, string(ptx.PostAssembly.InputStates[0].Data), state1.Salt.String())
		assert.Contains(t, string(ptx.PostAssembly.InputStates[1].Data), state3.Salt.String())
		assert.Len(t, ptx.PostAssembly.OutputStates, 1)
		assert.Contains(t, string(ptx.PostAssembly.OutputStates[0].Data), state5.Salt.String())
		assert.Len(t, ptx.PostAssembly.InfoStates, 1)
		assert.Contains(t, string(ptx.PostAssembly.InfoStates[0].Data), state6.Salt.String())
		return &prototk.EndorseTransactionResponse{
			EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
			Payload:           []byte(`some result`),
		}, nil
	}

	// Run an endorsement
	endorsementRequest := ptx.PostAssembly.AttestationPlan[0]
	endorserAddr := pldtypes.EthAddress(pldtypes.RandBytes(20))
	endorser := &prototk.ResolvedVerifier{
		Lookup:       "endorser1",
		Algorithm:    algorithms.ECDSA_SECP256K1,
		VerifierType: verifiers.ETH_ADDRESS,
		Verifier:     endorserAddr.String(),
	}
	endorsement, err := psc.EndorseTransaction(dCtx, td.c.dbTX, &components.PrivateTransactionEndorseRequest{
		TransactionSpecification: ptx.PreAssembly.TransactionSpecification,
		Verifiers:                ptx.PreAssembly.Verifiers,
		Signatures:               ptx.PostAssembly.Signatures,
		InputStates:              psc.d.toEndorsableList(ptx.PostAssembly.InputStates),
		ReadStates:               psc.d.toEndorsableList(ptx.PostAssembly.ReadStates),
		OutputStates:             psc.d.toEndorsableList(ptx.PostAssembly.OutputStates),
		InfoStates:               psc.d.toEndorsableList(ptx.PostAssembly.InfoStates),
		Endorsement:              endorsementRequest,
		Endorser:                 endorser,
	})
	require.NoError(t, err)
	assert.Equal(t, prototk.EndorseTransactionResponse_ENDORSER_SUBMIT, endorsement.Result)

	// Processing of endorsement faked up here
	ptx.PostAssembly.Endorsements = append(ptx.PostAssembly.Endorsements, &prototk.AttestationResult{
		Name:            endorsementRequest.Name,
		AttestationType: endorsementRequest.AttestationType,
		Verifier:        endorser,
		Payload:         endorsement.Payload, // just copy over in this test
		Constraints:     []prototk.AttestationResult_AttestationConstraint{prototk.AttestationResult_ENDORSER_MUST_SUBMIT},
	})

	// Prepare the transaction for submission to the blockchain
	td.tp.Functions.PrepareTransaction = func(ctx context.Context, ptr *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
		assert.Same(t, ptx.PreAssembly.TransactionSpecification, ptr.Transaction)
		assert.Len(t, ptx.PostAssembly.InputStates, 2)
		assert.Contains(t, string(ptx.PostAssembly.InputStates[0].Data), state1.Salt.String())
		assert.Contains(t, string(ptx.PostAssembly.InputStates[1].Data), state3.Salt.String())
		assert.Len(t, ptx.PostAssembly.OutputStates, 1)
		assert.Contains(t, string(ptx.PostAssembly.OutputStates[0].Data), state5.Salt.String())
		assert.Len(t, ptx.PostAssembly.InfoStates, 1)
		assert.Contains(t, string(ptx.PostAssembly.InfoStates[0].Data), state6.Salt.String())
		// Check endorsement
		assert.Len(t, ptx.PostAssembly.Endorsements, 1)
		endorsement := ptx.PostAssembly.Endorsements[0]
		assert.Equal(t, prototk.AttestationResult_ENDORSER_MUST_SUBMIT, endorsement.Constraints[0])
		assert.Equal(t, "endorser1", endorsement.Verifier.Lookup)
		assert.Equal(t, endorserAddr.String(), endorsement.Verifier.Verifier)
		onChain := &fakeExecute{
			Data: endorsement.Payload,
		}
		for _, inState := range ptx.PostAssembly.InputStates {
			onChain.Inputs = append(onChain.Inputs, inState.ID)
		}
		for _, inState := range ptx.PostAssembly.InputStates {
			onChain.Outputs = append(onChain.Outputs, inState.ID)
		}
		params, err := json.Marshal(onChain)
		require.NoError(t, err)
		return &prototk.PrepareTransactionResponse{
			Transaction: &prototk.PreparedTransaction{
				FunctionAbiJson: fakeCoinExecuteABI,
				ParamsJson:      string(params),
				RequiredSigner:  &localTx.Transaction.From,
			},
			Metadata: confutil.P(`{"some":"data"}`),
		}, nil
	}

	// Pass in a random signer - which will be overridden in this case
	ptx.Signer = pldtypes.RandAddress().String()

	// And now prepare
	err = psc.PrepareTransaction(dCtx, td.c.dbTX, ptx)
	require.NoError(t, err)
	assert.Len(t, ptx.PreparedPublicTransaction.ABI, 1)
	assert.NotNil(t, ptx.PreparedPublicTransaction.Data)
	assert.Equal(t, "txSigner", ptx.Signer)

	// Confirm the remaining unspent states
	stillAvailable, err = domain.FindAvailableStates(td.ctx, &prototk.FindAvailableStatesRequest{
		StateQueryContext: td.c.id,
		SchemaId:          ptx.PostAssembly.OutputStatesPotential[0].SchemaId,
		QueryJson:         `{}`,
	})
	require.NoError(t, err)
	assert.Len(t, stillAvailable.States, 3)
	assert.Contains(t, stillAvailable.States[0].DataJson, state2.Salt.String())
	assert.Contains(t, stillAvailable.States[1].DataJson, state4.Salt.String())
	assert.Contains(t, stillAvailable.States[2].DataJson, state5.Salt.String())

	assert.JSONEq(t, `{"some":"data"}`, string(ptx.PreparedMetadata))
}

func TestDomainAssembleTransactionInvalidTxn(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, ptx, localTx := doDomainInitTransactionOK(t, td)
	err := psc.AssembleTransaction(td.mdc, td.c.dbTX, ptx, &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			ID: localTx.Transaction.ID,
		},
	})
	assert.Regexp(t, "PD011626", err)

	assert.Nil(t, ptx.PostAssembly)
}

func TestDomainAssembleTransactionError(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, ptx, localTx := doDomainInitTransactionOK(t, td)
	td.tp.Functions.AssembleTransaction = func(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
		return nil, fmt.Errorf("pop")
	}
	err := psc.AssembleTransaction(td.mdc, td.c.dbTX, ptx, localTx)
	assert.Regexp(t, "pop", err)

	assert.Nil(t, ptx.PostAssembly)
}

func TestDomainAssembleTransactionLoadInputError(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, ptx, localTx := doDomainInitTransactionOK(t, td)
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
	err := psc.AssembleTransaction(td.mdc, td.c.dbTX, ptx, localTx)
	assert.Regexp(t, "PD011614.*badid", err)

	assert.Nil(t, ptx.PostAssembly)
}

func TestDomainAssembleTransactionRevert(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, ptx, localTx := doDomainInitTransactionOK(t, td)
	td.tp.Functions.AssembleTransaction = func(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
		return &prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			RevertReason:   confutil.P("failed with error"),
		}, nil
	}
	err := psc.AssembleTransaction(td.mdc, td.c.dbTX, ptx, localTx)
	require.NoError(t, err)

	assert.NotNil(t, ptx.PostAssembly)
	assert.Equal(t, prototk.AssembleTransactionResponse_REVERT, ptx.PostAssembly.AssemblyResult)
	assert.Equal(t, "failed with error", *ptx.PostAssembly.RevertReason)
}

func TestDomainAssembleTransactionLoadReadError(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, ptx, localTx := doDomainInitTransactionOK(t, td)
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
	err := psc.AssembleTransaction(td.mdc, td.c.dbTX, ptx, localTx)
	assert.Regexp(t, "PD011614.*badid", err)

	assert.Nil(t, ptx.PostAssembly)
}

func TestDomainWritePotentialStatesBadSchema(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.PostAssembly.OutputStatesPotential = []*prototk.NewState{
		{SchemaId: "unknown"},
	}
	err := psc.WritePotentialStates(td.mdc, td.c.dbTX, tx)
	assert.Regexp(t, "PD011613", err)
}

func TestDomainWritePotentialStatesFail(t *testing.T) {
	schema := componentsmocks.NewSchema(t)
	schemaID := pldtypes.RandBytes32()
	schema.On("ID").Return(schemaID)
	schema.On("Signature").Return("schema1_signature")
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(schema), mockBlockHeight)
	defer done()

	td.mdc.On("UpsertStates", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.PostAssembly.OutputStatesPotential = []*prototk.NewState{
		{SchemaId: schemaID.String()},
	}
	err := psc.WritePotentialStates(td.mdc, td.c.dbTX, tx)
	assert.Regexp(t, "pop", err)
}

func TestDomainWritePotentialStatesBadID(t *testing.T) {
	schema := componentsmocks.NewSchema(t)
	schemaID := pldtypes.RandBytes32()
	schema.On("ID").Return(schemaID)
	schema.On("Signature").Return("schema1_signature")
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(schema), mockBlockHeight)
	defer done()
	badBytes := "0xnothex"

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.PostAssembly.OutputStatesPotential = []*prototk.NewState{
		{SchemaId: schemaID.String(), Id: &badBytes},
	}
	err := psc.WritePotentialStates(td.mdc, td.c.dbTX, tx)
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

	_, err := psc.EndorseTransaction(td.mdc, td.c.dbTX, &components.PrivateTransactionEndorseRequest{
		TransactionSpecification: tx.PreAssembly.TransactionSpecification,
		Verifiers:                tx.PreAssembly.Verifiers,
		Signatures:               tx.PostAssembly.Signatures,
		InputStates:              psc.d.toEndorsableList(tx.PostAssembly.InputStates),
		ReadStates:               psc.d.toEndorsableList(tx.PostAssembly.ReadStates),
		OutputStates:             psc.d.toEndorsableList(tx.PostAssembly.OutputStates),
		InfoStates:               psc.d.toEndorsableList(tx.PostAssembly.InfoStates),
		Endorsement:              &prototk.AttestationRequest{},
		Endorser:                 &prototk.ResolvedVerifier{},
	})
	assert.Regexp(t, "pop", err)
}

func TestPrepareTransactionFail(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.Signer = "signer1"

	td.tp.Functions.PrepareTransaction = func(ctx context.Context, ptr *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	err := psc.PrepareTransaction(td.mdc, td.c.dbTX, tx)
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

	err := psc.PrepareTransaction(td.mdc, td.c.dbTX, tx)
	assert.Regexp(t, "PD011607", err)
}

func TestPrepareTransactionPrivateResult(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.Signer = "signer1"

	contractAddr := pldtypes.RandAddress()
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

	err := psc.PrepareTransaction(td.mdc, td.c.dbTX, tx)
	require.NoError(t, err)
	assert.Equal(t, pldapi.TransactionBase{
		IdempotencyKey: fmt.Sprintf("%s_doTheNextThing", tx.ID),
		Type:           pldapi.TransactionTypePrivate.Enum(),
		Function:       "doTheNextThing(string)",
		From:           tx.Signer,
		To:             contractAddr,
		Data:           pldtypes.RawJSON(`{"thing": "something else"}`),
		Domain:         psc.Domain().Name(),
	}, tx.PreparedPrivateTransaction.TransactionBase)
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

	err := psc.PrepareTransaction(td.mdc, td.c.dbTX, tx)
	require.Regexp(t, "bad address", err)
}

func TestPrepareTransactionUnknownContract(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight, func(mc *mockComponents) {
		mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows([]string{}))
	})
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.Signer = "signer1"

	contractAddr := pldtypes.RandAddress()

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

	err := psc.PrepareTransaction(td.mdc, td.c.dbTX, tx)
	require.Regexp(t, "PD011609", err)
}

func TestLoadStatesBadSchema(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.Signer = "signer1"

	_, err := psc.loadStatesFromContext(td.mdc, td.c.dbTX, []*prototk.StateRef{
		{
			Id:       pldtypes.RandHex(32),
			SchemaId: "wrong",
		},
	})
	assert.Regexp(t, "PD011614", err)
}

func TestLoadStatesError(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	td.mdc.On("FindAvailableStates", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.Signer = "signer1"

	_, err := psc.loadStatesFromContext(td.mdc, td.c.dbTX, []*prototk.StateRef{
		{
			Id:       pldtypes.RandHex(32),
			SchemaId: pldtypes.RandHex(32),
		},
	})
	assert.Regexp(t, "pop", err)
}

func TestLoadStatesNotFound(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()

	td.mdc.On("FindAvailableStates", mock.Anything, mock.Anything, mock.Anything).Return(nil, []*pldapi.State{}, nil)

	psc, tx := doDomainInitAssembleTransactionOK(t, td)
	tx.Signer = "signer1"

	_, err := psc.loadStatesFromContext(td.mdc, td.c.dbTX, []*prototk.StateRef{
		{
			Id:       pldtypes.RandHex(32),
			SchemaId: pldtypes.RandHex(32),
		},
	})
	assert.Regexp(t, "PD011615", err)
}

func TestIncompleteStages(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	psc := goodPSC(t, td)
	ptx := &components.PrivateTransaction{}
	localTx := &components.ResolvedTransaction{}

	err := psc.InitTransaction(td.ctx, ptx, localTx)
	assert.Regexp(t, "PD011626", err)

	err = psc.AssembleTransaction(td.mdc, td.c.dbTX, ptx, localTx)
	assert.Regexp(t, "PD011627", err)

	err = psc.WritePotentialStates(td.mdc, td.c.dbTX, ptx)
	assert.Regexp(t, "PD011628", err)

	err = psc.LockStates(td.mdc, td.c.dbTX, ptx)
	assert.Regexp(t, "PD011629", err)

	_, err = psc.EndorseTransaction(td.mdc, td.c.dbTX, nil)
	assert.Regexp(t, "PD011630", err)

	err = psc.PrepareTransaction(td.mdc, td.c.dbTX, ptx)
	assert.Regexp(t, "PD011632", err)
}

func goodPrivateCallWithInputsAndOutputs(psc *domainContract) *components.ResolvedTransaction {
	return &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				Domain: psc.d.name,
				From:   "me",
				To:     &psc.info.Address,
				Data: pldtypes.RawJSON(`{
					"address": "0xf2C41ae275A9acE65e1Fb78B97270a61D86Aa0Ed"
				}`),
			},
		},
		Function: &components.ResolvedFunction{
			Definition: &abi.Entry{
				Type: abi.Function,
				Name: "getBalance",
				Inputs: abi.ParameterArray{
					{Name: "address", Type: "address"},
				},
				Outputs: abi.ParameterArray{
					{Name: "amount", Type: "uint256"},
				},
			},
		},
	}
}

func TestInitCallOk(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)

	td.tp.Functions.InitCall = func(ctx context.Context, icr *prototk.InitCallRequest) (*prototk.InitCallResponse, error) {
		assert.JSONEq(t, `{"address":"0xf2c41ae275a9ace65e1fb78b97270a61d86aa0ed"}`, icr.Transaction.FunctionParamsJson)
		assert.Equal(t, "function getBalance(address address) external returns (uint256 amount) { }", icr.Transaction.FunctionSignature)
		return &prototk.InitCallResponse{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{
					Lookup:       "lookup1",
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
		}, nil
	}

	txi := goodPrivateCallWithInputsAndOutputs(psc)

	requiredVerifiers, err := psc.InitCall(td.ctx, txi)
	require.NoError(t, err)
	require.Len(t, requiredVerifiers, 1)
	assert.Equal(t, "lookup1", requiredVerifiers[0].Lookup)
	assert.Equal(t, algorithms.ECDSA_SECP256K1, requiredVerifiers[0].Algorithm)
	assert.Equal(t, verifiers.ETH_ADDRESS, requiredVerifiers[0].VerifierType)
}

func TestInitCallBadInput(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)

	_, err := psc.InitCall(td.ctx, &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				Domain: psc.d.name,
				To:     &psc.info.Address,
				Data: pldtypes.RawJSON(`{
					"wrong": "0xf2C41ae275A9acE65e1Fb78B97270a61D86Aa0Ed"
				}`),
			},
		},
		Function: &components.ResolvedFunction{
			Definition: &abi.Entry{
				Type: abi.Function,
				Name: "getBalance",
				Inputs: abi.ParameterArray{
					{Name: "address", Type: "address"},
				},
			},
		},
	})
	assert.Regexp(t, "PD011612", err)
}

func TestInitCallError(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)

	td.tp.Functions.InitCall = func(ctx context.Context, icr *prototk.InitCallRequest) (*prototk.InitCallResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	txi := goodPrivateCallWithInputsAndOutputs(psc)

	_, err := psc.InitCall(td.ctx, txi)
	assert.Regexp(t, "pop", err)
}

func TestExecCall(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)

	td.tp.Functions.ExecCall = func(ctx context.Context, cr *prototk.ExecCallRequest) (*prototk.ExecCallResponse, error) {
		require.Len(t, cr.ResolvedVerifiers, 1)
		assert.Equal(t, "lookup1", cr.ResolvedVerifiers[0].Lookup)
		assert.Equal(t, algorithms.ECDSA_SECP256K1, cr.ResolvedVerifiers[0].Algorithm)
		assert.Equal(t, verifiers.ETH_ADDRESS, cr.ResolvedVerifiers[0].VerifierType)
		assert.Equal(t, "0xf2c41ae275a9ace65e1fb78b97270a61d86aa0ed", cr.ResolvedVerifiers[0].Verifier)
		assert.JSONEq(t, `{"address":"0xf2c41ae275a9ace65e1fb78b97270a61d86aa0ed"}`, cr.Transaction.FunctionParamsJson)
		assert.Equal(t, "function getBalance(address address) external returns (uint256 amount) { }", cr.Transaction.FunctionSignature)
		return &prototk.ExecCallResponse{
			ResultJson: `{"amount": 11223344556677889900}`,
		}, nil
	}

	txi := goodPrivateCallWithInputsAndOutputs(psc)

	cv, err := psc.ExecCall(td.c.dCtx, td.c.dbTX, txi, []*prototk.ResolvedVerifier{
		{
			Lookup:       "lookup1",
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
			Verifier:     "0xf2c41ae275a9ace65e1fb78b97270a61d86aa0ed",
		},
	})
	require.NoError(t, err)
	jv, err := cv.JSON()
	require.NoError(t, err)
	assert.JSONEq(t, `{"amount":"11223344556677889900"}`, string(jv))
}

func TestExecCallBadInput(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)

	_, err := psc.ExecCall(td.c.dCtx, td.c.dbTX, &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				Domain: psc.d.name,
				To:     &psc.info.Address,
				Data: pldtypes.RawJSON(`{
					"wrong": "0xf2C41ae275A9acE65e1Fb78B97270a61D86Aa0Ed"
				}`)},
		},
		Function: &components.ResolvedFunction{
			Definition: &abi.Entry{
				Type: abi.Function,
				Name: "getBalance",
				Inputs: abi.ParameterArray{
					{Name: "address", Type: "address"},
				},
			},
		},
	}, []*prototk.ResolvedVerifier{})
	assert.Regexp(t, "PD011612", err)
}

func TestExecCallBadOutput(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)

	td.tp.Functions.ExecCall = func(ctx context.Context, cr *prototk.ExecCallRequest) (*prototk.ExecCallResponse, error) {
		assert.JSONEq(t, `{"address":"0xf2c41ae275a9ace65e1fb78b97270a61d86aa0ed"}`, cr.Transaction.FunctionParamsJson)
		assert.Equal(t, "function getBalance(address address) external returns (uint256 amount) { }", cr.Transaction.FunctionSignature)
		return &prototk.ExecCallResponse{
			ResultJson: `{"wrong": 11223344556677889900}`,
		}, nil
	}

	txi := goodPrivateCallWithInputsAndOutputs(psc)

	_, err := psc.ExecCall(td.c.dCtx, td.c.dbTX, txi, []*prototk.ResolvedVerifier{})
	assert.Regexp(t, "PD011653", err)
}

func TestExecCallNilOutputOk(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)

	td.tp.Functions.ExecCall = func(ctx context.Context, cr *prototk.ExecCallRequest) (*prototk.ExecCallResponse, error) {
		assert.JSONEq(t, `{"address":"0xf2c41ae275a9ace65e1fb78b97270a61d86aa0ed"}`, cr.Transaction.FunctionParamsJson)
		assert.Equal(t, "function getBalance(address address) external { }", cr.Transaction.FunctionSignature)
		return &prototk.ExecCallResponse{}, nil
	}

	localTx := goodPrivateCallWithInputsAndOutputs(psc)
	localTx.Function.Definition.Outputs = nil

	_, err := psc.ExecCall(td.c.dCtx, td.c.dbTX, localTx, []*prototk.ResolvedVerifier{})
	require.NoError(t, err)
}

func TestExecCallFail(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), mockBlockHeight)
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)

	td.tp.Functions.ExecCall = func(ctx context.Context, cr *prototk.ExecCallRequest) (*prototk.ExecCallResponse, error) {
		return &prototk.ExecCallResponse{}, fmt.Errorf("pop")
	}

	txi := goodPrivateCallWithInputsAndOutputs(psc)

	_, err := psc.ExecCall(td.c.dCtx, td.c.dbTX, txi, []*prototk.ResolvedVerifier{})
	assert.Regexp(t, "pop", err)
}

func TestGetPSCInvalidConfig(t *testing.T) {
	addr := pldtypes.RandAddress()
	var mc *mockComponents
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(_mc *mockComponents) {
		mc = _mc
	})
	defer done()
	assert.Nil(t, td.d.initError.Load())

	mc.db.ExpectQuery("SELECT.*private_smart_contracts").
		WillReturnRows(sqlmock.NewRows([]string{"address", "domain_address"}).
			AddRow(addr.String(), td.d.registryAddress.String()))

	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{
			Valid: false, // Not valid
			ContractConfig: &prototk.ContractConfig{
				ContractConfigJson:   `{}`,
				CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
				SubmitterSelection:   prototk.ContractConfig_SUBMITTER_SENDER,
			},
		}, nil
	}

	psc, err := td.dm.GetSmartContractByAddress(td.ctx, td.c.dbTX, *addr)
	require.Regexp(t, "PD011610", err) // invalid config
	assert.Nil(t, psc)
}

func TestGetPSCUnknownDomain(t *testing.T) {
	addr := pldtypes.RandAddress()
	var mc *mockComponents
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(_mc *mockComponents) {
		mc = _mc
	})
	defer done()
	assert.Nil(t, td.d.initError.Load())

	mc.db.ExpectQuery("SELECT.*private_smart_contracts").
		WillReturnRows(sqlmock.NewRows([]string{"address", "domain_address"}).
			AddRow(addr.String(), pldtypes.RandAddress().String()))

	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{
			Valid: true,
			ContractConfig: &prototk.ContractConfig{
				ContractConfigJson:   `{}`,
				CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
				SubmitterSelection:   prototk.ContractConfig_SUBMITTER_SENDER,
			},
		}, nil
	}

	psc, err := td.dm.GetSmartContractByAddress(td.ctx, td.c.dbTX, *addr)
	require.Regexp(t, "PD011654", err) // domain no longer configured
	assert.Nil(t, psc)
}

func TestGetPSCInitError(t *testing.T) {
	addr := pldtypes.RandAddress()
	var mc *mockComponents
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(_mc *mockComponents) {
		mc = _mc
	})
	defer done()
	assert.Nil(t, td.d.initError.Load())

	mc.db.ExpectQuery("SELECT.*private_smart_contracts").
		WillReturnRows(sqlmock.NewRows([]string{"address", "domain_address"}).
			AddRow(addr.String(), td.d.registryAddress.String()))

	td.tp.Functions.InitContract = func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	psc, err := td.dm.GetSmartContractByAddress(td.ctx, td.c.dbTX, *addr)
	require.Regexp(t, "pop", err) // domain no longer configured
	assert.Nil(t, psc)
}

func goodWrapPGTxCall(psc *domainContract, salt pldtypes.Bytes32) (*pldapi.TransactionInput, error) {
	tx := &pldapi.PrivacyGroupEVMTX{
		From:  "from.addr",
		To:    confutil.P(psc.Address()),
		Gas:   confutil.P(pldtypes.HexUint64(12345)),
		Value: pldtypes.Uint64ToUint256(10000000000),
		Input: pldtypes.JSONString(map[string]any{
			"who":   pldtypes.MustEthAddress("0x09ec006415815b28538d5b9b2d3c0b5d7f43e7f6"),
			"thing": "stuff",
		}),
		Function: &abi.Entry{
			Type: abi.Function,
			Name: "doAThing",
			Inputs: abi.ParameterArray{
				{Type: "address", Name: "who"},
				{Type: "string", Name: "thing"},
			},
		},
	}
	return psc.WrapPrivacyGroupEVMTX(context.Background(), &pldapi.PrivacyGroup{
		GenesisSalt: salt,
		Name:        "pg1",
	}, tx)
}

func TestWrapPGTxOk(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)

	td.tp.Functions.WrapPrivacyGroupEVMTX = func(ctx context.Context, wpgtr *prototk.WrapPrivacyGroupEVMTXRequest) (*prototk.WrapPrivacyGroupEVMTXResponse, error) {
		require.Equal(t, "pg1", wpgtr.PrivacyGroup.Name)
		var fnDef abi.Entry
		err := json.Unmarshal([]byte(*wpgtr.Transaction.FunctionAbiJson), &fnDef)
		require.NoError(t, err)
		paramsJson := pldtypes.JSONString(map[string]any{
			"pgName":            wpgtr.PrivacyGroup.Name,
			"gas":               wpgtr.Transaction.Gas,
			"value":             wpgtr.Transaction.Value,
			"wrappedParamsJSON": pldtypes.RawJSON(*wpgtr.Transaction.InputJson),
		}).Pretty()
		return &prototk.WrapPrivacyGroupEVMTXResponse{
			Transaction: &prototk.PreparedTransaction{
				ContractAddress: confutil.P(psc.Address().String()),
				Type:            prototk.PreparedTransaction_PRIVATE,
				RequiredSigner:  confutil.P("pgroup.signer"),
				FunctionAbiJson: pldtypes.JSONString(&abi.Entry{
					Type: abi.Function,
					Name: "wrappedDoThing",
					Inputs: abi.ParameterArray{
						{Name: "pgName", Type: "string"},
						{Name: "gas", Type: "uint64"},
						{Name: "value", Type: "uint256"},
						{Name: "wrappedParamsJSON", Type: "tuple", Components: fnDef.Inputs},
					},
				}).Pretty(),
				ParamsJson: paramsJson,
			},
		}, nil
	}

	salt := pldtypes.RandBytes32()
	tx, err := goodWrapPGTxCall(psc, salt)
	require.NoError(t, err)

	require.JSONEq(t, fmt.Sprintf(`{
		"type":           "private",
		"domain":         "test1",
		"function":       "wrappedDoThing",
		"from":           "pgroup.signer",
		"to":             "%s",
		"data": {
			"pgName": "pg1",
			"gas": "0x3039",
			"value": "0x02540be400",
			"wrappedParamsJSON": {
			   "who": "0x09ec006415815b28538d5b9b2d3c0b5d7f43e7f6",
			   "thing": "stuff"
			}
		},
		"abi": [{
			"type": "function",
			"name": "wrappedDoThing",
			"inputs": [
				{ "name": "pgName", "type": "string" },
				{ "name": "gas", "type": "uint64" },
				{ "name": "value", "type": "uint256" },
				{
					"components": [
						{
							"name": "who",
							"type": "address"
						},
						{
							"name": "thing",
							"type": "string"
						}
					],
					"name": "wrappedParamsJSON",
					"type": "tuple"
				}
			],
			"outputs": null
		}]
	}`, psc.Address()), pldtypes.JSONString(tx).Pretty())
}

func TestWrapPGFail(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)

	td.tp.Functions.WrapPrivacyGroupEVMTX = func(ctx context.Context, wpgtr *prototk.WrapPrivacyGroupEVMTXRequest) (*prototk.WrapPrivacyGroupEVMTXResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	_, err := goodWrapPGTxCall(psc, pldtypes.RandBytes32())
	require.Regexp(t, "pop", err)
}

func TestWrapPGBadTxType(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)

	td.tp.Functions.WrapPrivacyGroupEVMTX = func(ctx context.Context, wpgtr *prototk.WrapPrivacyGroupEVMTXRequest) (*prototk.WrapPrivacyGroupEVMTXResponse, error) {
		return &prototk.WrapPrivacyGroupEVMTXResponse{
			Transaction: &prototk.PreparedTransaction{
				Type: prototk.PreparedTransaction_PUBLIC,
			},
		}, nil
	}

	_, err := goodWrapPGTxCall(psc, pldtypes.RandBytes32())
	require.Regexp(t, "PD011665", err)
}

func TestWrapPGBadToAddr(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)

	td.tp.Functions.WrapPrivacyGroupEVMTX = func(ctx context.Context, wpgtr *prototk.WrapPrivacyGroupEVMTXRequest) (*prototk.WrapPrivacyGroupEVMTXResponse, error) {
		return &prototk.WrapPrivacyGroupEVMTXResponse{
			Transaction: &prototk.PreparedTransaction{
				Type:            prototk.PreparedTransaction_PRIVATE,
				ContractAddress: confutil.P("wrong"),
			},
		}, nil
	}

	_, err := goodWrapPGTxCall(psc, pldtypes.RandBytes32())
	require.Regexp(t, "bad address", err)
}

func TestWrapPGWrongToAddr(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)

	td.tp.Functions.WrapPrivacyGroupEVMTX = func(ctx context.Context, wpgtr *prototk.WrapPrivacyGroupEVMTXRequest) (*prototk.WrapPrivacyGroupEVMTXResponse, error) {
		return &prototk.WrapPrivacyGroupEVMTXResponse{
			Transaction: &prototk.PreparedTransaction{
				Type:            prototk.PreparedTransaction_PRIVATE,
				ContractAddress: confutil.P(pldtypes.RandAddress().String()),
			},
		}, nil
	}

	_, err := goodWrapPGTxCall(psc, pldtypes.RandBytes32())
	require.Regexp(t, "PD011666", err)
}

func TestWrapPGBadData(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	psc := goodPSC(t, td)

	td.tp.Functions.WrapPrivacyGroupEVMTX = func(ctx context.Context, wpgtr *prototk.WrapPrivacyGroupEVMTXRequest) (*prototk.WrapPrivacyGroupEVMTXResponse, error) {
		return &prototk.WrapPrivacyGroupEVMTXResponse{
			Transaction: &prototk.PreparedTransaction{
				Type: prototk.PreparedTransaction_PRIVATE,
			},
		}, nil
	}

	_, err := goodWrapPGTxCall(psc, pldtypes.RandBytes32())
	require.Regexp(t, "PD011612", err)
}
