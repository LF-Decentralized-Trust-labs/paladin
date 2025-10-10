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
package plugins

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"testing"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type testDomainManager struct {
	domains             map[string]plugintk.Plugin
	domainRegistered    func(name string, toDomain components.DomainManagerToDomain) (fromDomain plugintk.DomainCallbacks, err error)
	findAvailableStates func(context.Context, *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error)
	encodeData          func(context.Context, *prototk.EncodeDataRequest) (*prototk.EncodeDataResponse, error)
	decodeData          func(context.Context, *prototk.DecodeDataRequest) (*prototk.DecodeDataResponse, error)
	recoverSigner       func(context.Context, *prototk.RecoverSignerRequest) (*prototk.RecoverSignerResponse, error)
	sendTransaction     func(context.Context, *prototk.SendTransactionRequest) (*prototk.SendTransactionResponse, error)
	localNodeName       func(context.Context, *prototk.LocalNodeNameRequest) (*prototk.LocalNodeNameResponse, error)
	getStates           func(context.Context, *prototk.GetStatesByIDRequest) (*prototk.GetStatesByIDResponse, error)
}

func (tp *testDomainManager) FindAvailableStates(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
	return tp.findAvailableStates(ctx, req)
}

func (tp *testDomainManager) EncodeData(ctx context.Context, req *prototk.EncodeDataRequest) (*prototk.EncodeDataResponse, error) {
	return tp.encodeData(ctx, req)
}

func (tp *testDomainManager) DecodeData(ctx context.Context, req *prototk.DecodeDataRequest) (*prototk.DecodeDataResponse, error) {
	return tp.decodeData(ctx, req)
}

func (tp *testDomainManager) RecoverSigner(ctx context.Context, req *prototk.RecoverSignerRequest) (*prototk.RecoverSignerResponse, error) {
	return tp.recoverSigner(ctx, req)
}

func (tp *testDomainManager) SendTransaction(ctx context.Context, req *prototk.SendTransactionRequest) (*prototk.SendTransactionResponse, error) {
	return tp.sendTransaction(ctx, req)
}

func (tp *testDomainManager) LocalNodeName(ctx context.Context, req *prototk.LocalNodeNameRequest) (*prototk.LocalNodeNameResponse, error) {
	return tp.localNodeName(ctx, req)
}

func (tp *testDomainManager) GetStatesByID(ctx context.Context, req *prototk.GetStatesByIDRequest) (*prototk.GetStatesByIDResponse, error) {
	return tp.getStates(ctx, req)
}

func domainConnectFactory(ctx context.Context, client prototk.PluginControllerClient) (grpc.BidiStreamingClient[prototk.DomainMessage, prototk.DomainMessage], error) {
	return client.ConnectDomain(context.Background())
}

func domainHeaderAccessor(msg *prototk.DomainMessage) *prototk.Header {
	if msg.Header == nil {
		msg.Header = &prototk.Header{}
	}
	return msg.Header
}

func (tp *testDomainManager) mock(t *testing.T) *componentsmocks.DomainManager {
	mdm := componentsmocks.NewDomainManager(t)
	pluginMap := make(map[string]*pldconf.PluginConfig)
	for name := range tp.domains {
		pluginMap[name] = &pldconf.PluginConfig{
			Type:    string(pldtypes.LibraryTypeCShared),
			Library: "/tmp/not/applicable",
		}
	}
	mdm.On("ConfiguredDomains").Return(pluginMap).Maybe()
	mdr := mdm.On("DomainRegistered", mock.Anything, mock.Anything).Maybe()
	mdr.Run(func(args mock.Arguments) {
		m2p, err := tp.domainRegistered(args[0].(string), args[1].(components.DomainManagerToDomain))
		mdr.Return(m2p, err)
	})
	return mdm
}

func newTestDomainPluginManager(t *testing.T, setup *testManagers) (context.Context, *pluginManager, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	pc := newTestPluginManager(t, setup)

	tpl, err := NewUnitTestPluginLoader(pc.GRPCTargetURL(), pc.loaderID.String(), setup.allPlugins())
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		tpl.Run()
	}()

	return ctx, pc, func() {
		recovered := recover()
		if recovered != nil {
			fmt.Fprintf(os.Stderr, "%v: %s", recovered, debug.Stack())
			panic(recovered)
		}
		cancelCtx()
		pc.Stop()
		tpl.Stop()
		<-done
	}

}

func TestDomainRequestsOK(t *testing.T) {

	log.InitConfig(&pldconf.LogConfig{Level: confutil.P("debug")}) // test debug specific logging
	waitForAPI := make(chan components.DomainManagerToDomain, 1)
	waitForCallbacks := make(chan plugintk.DomainCallbacks, 1)

	domainFunctions := &plugintk.DomainAPIFunctions{
		ConfigureDomain: func(ctx context.Context, cdr *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
			assert.Equal(t, int64(12345), cdr.ChainId)
			return &prototk.ConfigureDomainResponse{
				DomainConfig: &prototk.DomainConfig{
					CustomHashFunction: true,
				},
			}, nil
		},
		InitDomain: func(ctx context.Context, idr *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
			return &prototk.InitDomainResponse{}, nil
		},
		InitDeploy: func(ctx context.Context, idr *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error) {
			assert.Equal(t, "deploy_tx1_init", idr.Transaction.TransactionId)
			return &prototk.InitDeployResponse{RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{Lookup: "lookup1"},
			}}, nil
		},
		PrepareDeploy: func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
			assert.Equal(t, "deploy_tx1_prepare", pdr.Transaction.TransactionId)
			return &prototk.PrepareDeployResponse{
				Signer: confutil.P("signing1"),
			}, nil
		},
		InitContract: func(ctx context.Context, itr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
			assert.Equal(t, "0xaabbcc", itr.ContractAddress)
			return &prototk.InitContractResponse{
				ContractConfig: &prototk.ContractConfig{
					ContractConfigJson: `{"domain":"conf"}`,
				},
			}, nil
		},
		InitTransaction: func(ctx context.Context, itr *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
			assert.Equal(t, "tx2_init", itr.Transaction.TransactionId)
			return &prototk.InitTransactionResponse{
				RequiredVerifiers: []*prototk.ResolveVerifierRequest{
					{Lookup: "lookup2"},
				},
			}, nil
		},
		AssembleTransaction: func(ctx context.Context, atr *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
			assert.Equal(t, "tx2_prepare", atr.Transaction.TransactionId)
			return &prototk.AssembleTransactionResponse{
				AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			}, nil
		},
		EndorseTransaction: func(ctx context.Context, etr *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
			assert.Equal(t, "tx2_endorse", etr.Transaction.TransactionId)
			return &prototk.EndorseTransactionResponse{
				EndorsementResult: prototk.EndorseTransactionResponse_SIGN,
			}, nil
		},
		PrepareTransaction: func(ctx context.Context, ptr *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
			assert.Equal(t, "tx2_prepare", ptr.Transaction.TransactionId)
			return &prototk.PrepareTransactionResponse{
				Transaction: &prototk.PreparedTransaction{
					ParamsJson: `{"test": "value"}`,
				},
			}, nil
		},
		HandleEventBatch: func(ctx context.Context, hebr *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
			assert.Equal(t, "batch1", hebr.BatchId)
			return &prototk.HandleEventBatchResponse{
				TransactionsComplete: []*prototk.CompletedTransaction{{TransactionId: "tx1"}},
			}, nil
		},
		Sign: func(ctx context.Context, sr *prototk.SignRequest) (*prototk.SignResponse, error) {
			assert.Equal(t, "algo1", sr.Algorithm)
			return &prototk.SignResponse{
				Payload: []byte("signed"),
			}, nil
		},
		GetVerifier: func(ctx context.Context, gvr *prototk.GetVerifierRequest) (*prototk.GetVerifierResponse, error) {
			assert.Equal(t, "algo1", gvr.Algorithm)
			return &prototk.GetVerifierResponse{
				Verifier: "verifier1",
			}, nil
		},
		ValidateStateHashes: func(ctx context.Context, vshr *prototk.ValidateStateHashesRequest) (*prototk.ValidateStateHashesResponse, error) {
			assert.Equal(t, "state1_in", vshr.States[0].Id)
			return &prototk.ValidateStateHashesResponse{
				StateIds: []string{"state1_out"},
			}, nil
		},
		InitCall: func(ctx context.Context, cr *prototk.InitCallRequest) (*prototk.InitCallResponse, error) {
			assert.Equal(t, "tx1", cr.Transaction.TransactionId)
			return &prototk.InitCallResponse{
				RequiredVerifiers: []*prototk.ResolveVerifierRequest{
					{Lookup: "lookup3"},
				},
			}, nil
		},
		ExecCall: func(ctx context.Context, cr *prototk.ExecCallRequest) (*prototk.ExecCallResponse, error) {
			assert.Equal(t, "tx1", cr.Transaction.TransactionId)
			return &prototk.ExecCallResponse{
				ResultJson: `{"some":"data"}`,
			}, nil
		},
		BuildReceipt: func(ctx context.Context, brr *prototk.BuildReceiptRequest) (*prototk.BuildReceiptResponse, error) {
			assert.Equal(t, "tx1", brr.TransactionId)
			return &prototk.BuildReceiptResponse{
				ReceiptJson: `{"receipt":"data"}`,
			}, nil
		},
		ConfigurePrivacyGroup: func(ctx context.Context, cpgr *prototk.ConfigurePrivacyGroupRequest) (*prototk.ConfigurePrivacyGroupResponse, error) {
			assert.Equal(t, map[string]string{"input": "props"}, cpgr.InputConfiguration)
			return &prototk.ConfigurePrivacyGroupResponse{
				Configuration: map[string]string{"finalized": "props"},
			}, nil
		},
		InitPrivacyGroup: func(ctx context.Context, ipgr *prototk.InitPrivacyGroupRequest) (*prototk.InitPrivacyGroupResponse, error) {
			assert.Equal(t, `pg1`, ipgr.PrivacyGroup.Name)
			return &prototk.InitPrivacyGroupResponse{
				Transaction: &prototk.PreparedTransaction{
					ParamsJson: `{"some":"params"}`,
				},
			}, nil
		},
		WrapPrivacyGroupEVMTX: func(ctx context.Context, wpgtr *prototk.WrapPrivacyGroupEVMTXRequest) (*prototk.WrapPrivacyGroupEVMTXResponse, error) {
			assert.Equal(t, `{"orig":"params"}`, *wpgtr.Transaction.InputJson)
			return &prototk.WrapPrivacyGroupEVMTXResponse{
				Transaction: &prototk.PreparedTransaction{
					ParamsJson: `{"wrapped":"params"}`,
				},
			}, nil
		},
	}

	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": plugintk.NewDomain(func(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {
				waitForCallbacks <- callbacks
				return &plugintk.DomainAPIBase{Functions: domainFunctions}
			}),
		},
	}
	tdm.domainRegistered = func(name string, toDomain components.DomainManagerToDomain) (plugintk.DomainCallbacks, error) {
		assert.Equal(t, "domain1", name)
		waitForAPI <- toDomain
		return tdm, nil
	}

	tdm.findAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		assert.Equal(t, "schema1", req.SchemaId)
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{
				{Id: "12345"},
			},
		}, nil
	}

	tdm.encodeData = func(ctx context.Context, edr *prototk.EncodeDataRequest) (*prototk.EncodeDataResponse, error) {
		assert.Equal(t, edr.Body, "some input data")
		return &prototk.EncodeDataResponse{
			Data: []byte("some output data"),
		}, nil
	}

	tdm.decodeData = func(ctx context.Context, edr *prototk.DecodeDataRequest) (*prototk.DecodeDataResponse, error) {
		assert.Equal(t, edr.Data, []byte("some input data"))
		return &prototk.DecodeDataResponse{
			Body: "some output data",
		}, nil
	}

	tdm.recoverSigner = func(ctx context.Context, edr *prototk.RecoverSignerRequest) (*prototk.RecoverSignerResponse, error) {
		assert.Equal(t, edr.Algorithm, "some algo")
		return &prototk.RecoverSignerResponse{
			Verifier: "some verifier",
		}, nil
	}

	tdm.sendTransaction = func(ctx context.Context, str *prototk.SendTransactionRequest) (*prototk.SendTransactionResponse, error) {
		assert.Equal(t, str.Transaction.From, "user1")
		return &prototk.SendTransactionResponse{
			Id: "tx1",
		}, nil
	}

	tdm.localNodeName = func(ctx context.Context, lnr *prototk.LocalNodeNameRequest) (*prototk.LocalNodeNameResponse, error) {
		return &prototk.LocalNodeNameResponse{
			Name: "node1",
		}, nil
	}

	tdm.getStates = func(ctx context.Context, gsr *prototk.GetStatesByIDRequest) (*prototk.GetStatesByIDResponse, error) {
		assert.Equal(t, "schema1", gsr.SchemaId)
		return &prototk.GetStatesByIDResponse{
			States: []*prototk.StoredState{{}},
		}, nil
	}

	ctx, pc, done := newTestDomainPluginManager(t, &testManagers{
		testDomainManager: tdm,
	})
	defer done()

	var domainAPI components.DomainManagerToDomain
	select {
	case domainAPI = <-waitForAPI:
		// Received domain API
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for domain API - expected registration was not received")
	}

	cdr, err := domainAPI.ConfigureDomain(ctx, &prototk.ConfigureDomainRequest{
		ChainId: int64(12345),
	})
	require.NoError(t, err)
	assert.True(t, cdr.DomainConfig.CustomHashFunction)

	_, err = domainAPI.InitDomain(ctx, &prototk.InitDomainRequest{})
	require.NoError(t, err)

	// This is the point the domain manager would call us to say the domain is initialized
	// (once it's happy it's updated its internal state)
	domainAPI.Initialized()
	require.NoError(t, pc.WaitForInit(ctx, prototk.PluginInfo_DOMAIN))

	idr, err := domainAPI.InitDeploy(ctx, &prototk.InitDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			TransactionId: "deploy_tx1_init",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "lookup1", idr.RequiredVerifiers[0].Lookup)

	pdr, err := domainAPI.PrepareDeploy(ctx, &prototk.PrepareDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			TransactionId: "deploy_tx1_prepare",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "signing1", *pdr.Signer)

	iscr, err := domainAPI.InitContract(ctx, &prototk.InitContractRequest{
		ContractAddress: "0xaabbcc",
	})
	require.NoError(t, err)
	assert.Equal(t, `{"domain":"conf"}`, iscr.ContractConfig.ContractConfigJson)

	itr, err := domainAPI.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "tx2_init",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "lookup2", itr.RequiredVerifiers[0].Lookup)

	atr, err := domainAPI.AssembleTransaction(ctx, &prototk.AssembleTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "tx2_prepare",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_REVERT, atr.AssemblyResult)

	etr, err := domainAPI.EndorseTransaction(ctx, &prototk.EndorseTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "tx2_endorse",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, prototk.EndorseTransactionResponse_SIGN, etr.EndorsementResult)

	ptr, err := domainAPI.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "tx2_prepare",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, `{"test": "value"}`, ptr.Transaction.ParamsJson)

	heb, err := domainAPI.HandleEventBatch(ctx, &prototk.HandleEventBatchRequest{
		BatchId: "batch1",
	})
	require.NoError(t, err)
	assert.Equal(t, "tx1", heb.TransactionsComplete[0].TransactionId)

	sr, err := domainAPI.Sign(ctx, &prototk.SignRequest{
		Algorithm: "algo1",
	})
	require.NoError(t, err)
	assert.Equal(t, "signed", string(sr.Payload))

	gvr, err := domainAPI.GetVerifier(ctx, &prototk.GetVerifierRequest{
		Algorithm: "algo1",
	})
	require.NoError(t, err)
	assert.Equal(t, "verifier1", string(gvr.Verifier))

	vshr, err := domainAPI.ValidateStateHashes(ctx, &prototk.ValidateStateHashesRequest{
		States: []*prototk.EndorsableState{{Id: "state1_in"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "state1_out", vshr.StateIds[0])

	icr, err := domainAPI.InitCall(ctx, &prototk.InitCallRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "tx1"},
	})
	require.NoError(t, err)
	assert.Equal(t, `lookup3`, icr.RequiredVerifiers[0].Lookup)

	ecr, err := domainAPI.ExecCall(ctx, &prototk.ExecCallRequest{
		Transaction: &prototk.TransactionSpecification{TransactionId: "tx1"},
	})
	require.NoError(t, err)
	assert.Equal(t, `{"some":"data"}`, ecr.ResultJson)

	brr, err := domainAPI.BuildReceipt(ctx, &prototk.BuildReceiptRequest{
		TransactionId: "tx1",
	})
	require.NoError(t, err)
	assert.Equal(t, `{"receipt":"data"}`, brr.ReceiptJson)

	cpgr, err := domainAPI.ConfigurePrivacyGroup(ctx, &prototk.ConfigurePrivacyGroupRequest{
		InputConfiguration: map[string]string{"input": "props"},
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"finalized": "props"}, cpgr.Configuration)

	ipgr, err := domainAPI.InitPrivacyGroup(ctx, &prototk.InitPrivacyGroupRequest{
		PrivacyGroup: &prototk.PrivacyGroup{
			Name: "pg1",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, `{"some":"params"}`, ipgr.Transaction.ParamsJson)

	wpgtr, err := domainAPI.WrapPrivacyGroupEVMTX(ctx, &prototk.WrapPrivacyGroupEVMTXRequest{
		Transaction: &prototk.PrivacyGroupEVMTX{
			InputJson: confutil.P(`{"orig":"params"}`),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, `{"wrapped":"params"}`, wpgtr.Transaction.ParamsJson)

	// Add timeout for callbacks
	var callbacks plugintk.DomainCallbacks
	select {
	case callbacks = <-waitForCallbacks:
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for callbacks - expected callbacks were not received")
	}

	fas, err := callbacks.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
		SchemaId: "schema1",
	})
	require.NoError(t, err)
	assert.Equal(t, "12345", fas.States[0].Id)

	edr, err := callbacks.EncodeData(ctx, &prototk.EncodeDataRequest{
		Body: "some input data",
	})
	require.NoError(t, err)
	assert.Equal(t, "some output data", string(edr.Data))

	ddr, err := callbacks.DecodeData(ctx, &prototk.DecodeDataRequest{
		Data: []byte("some input data"),
	})
	require.NoError(t, err)
	assert.Equal(t, "some output data", string(ddr.Body))

	rsr, err := callbacks.RecoverSigner(ctx, &prototk.RecoverSignerRequest{
		Algorithm: "some algo",
	})
	require.NoError(t, err)
	assert.Equal(t, "some verifier", string(rsr.Verifier))

	str, err := callbacks.SendTransaction(ctx, &prototk.SendTransactionRequest{
		Transaction: &prototk.TransactionInput{
			From: "user1",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "tx1", str.Id)

	lnr, err := callbacks.LocalNodeName(ctx, &prototk.LocalNodeNameRequest{})
	require.NoError(t, err)
	assert.Equal(t, "node1", lnr.Name)

	gsr, err := callbacks.GetStatesByID(ctx, &prototk.GetStatesByIDRequest{
		SchemaId: "schema1",
	})
	require.NoError(t, err)
	assert.Len(t, gsr.States, 1)
}

func TestDomainRegisterFail(t *testing.T) {

	waitForError := make(chan struct{})

	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": &mockPlugin[prototk.DomainMessage]{
				t:              t,
				connectFactory: domainConnectFactory,
				headerAccessor: domainHeaderAccessor,
				preRegister: func(domainID string) *prototk.DomainMessage {
					return &prototk.DomainMessage{
						Header: &prototk.Header{
							MessageType: prototk.Header_REGISTER,
							PluginId:    domainID,
							MessageId:   uuid.NewString(),
						},
					}
				},
			},
		},
	}
	tdm.domainRegistered = func(name string, toDomain components.DomainManagerToDomain) (plugintk.DomainCallbacks, error) {
		close(waitForError)
		return nil, fmt.Errorf("pop")
	}

	_, _, done := newTestDomainPluginManager(t, &testManagers{
		testDomainManager: tdm,
	})
	defer done()

	// Add timeout to prevent test from hanging indefinitely
	select {
	case <-waitForError:
		// Error received successfully
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for error - expected error was not received")
	}
}

func TestFromDomainRequestBadReq(t *testing.T) {

	waitForResponse := make(chan struct{}, 1)

	msgID := uuid.NewString()
	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": &mockPlugin[prototk.DomainMessage]{
				t:              t,
				connectFactory: domainConnectFactory,
				headerAccessor: domainHeaderAccessor,
				sendRequest: func(domainID string) *prototk.DomainMessage {
					return &prototk.DomainMessage{
						Header: &prototk.Header{
							PluginId:    domainID,
							MessageId:   msgID,
							MessageType: prototk.Header_REQUEST_FROM_PLUGIN,
							// Missing payload
						},
					}
				},
				handleResponse: func(dm *prototk.DomainMessage) {
					assert.Equal(t, msgID, *dm.Header.CorrelationId)
					assert.Regexp(t, "PD011203", *dm.Header.ErrorMessage)
					close(waitForResponse)
				},
			},
		},
	}
	tdm.domainRegistered = func(name string, toDomain components.DomainManagerToDomain) (plugintk.DomainCallbacks, error) {
		return tdm, nil
	}

	_, _, done := newTestDomainPluginManager(t, &testManagers{
		testDomainManager: tdm,
	})
	defer done()

	// Add timeout to prevent test from hanging indefinitely
	select {
	case <-waitForResponse:
		// Response received successfully
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for response - expected response was not received")
	}
}
