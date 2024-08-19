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
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

type testDomainManager struct {
	domainRegistered    func(name string, id uuid.UUID, toDomain plugintk.DomainAPI) (fromDomain plugintk.DomainCallbacks)
	findAvailableStates func(context.Context, *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error)
}

func (tp *testDomainManager) FindAvailableStates(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
	return tp.findAvailableStates(ctx, req)
}

func (tdm *testDomainManager) DomainRegistered(name string, id uuid.UUID, toDomain plugintk.DomainAPI) (fromDomain plugintk.DomainCallbacks) {
	return tdm.domainRegistered(name, id, toDomain)
}

func TestDomainRequestsOK(t *testing.T) {

	waitForRegister := make(chan plugintk.DomainAPI, 1)

	tdm := &testDomainManager{}
	var domainID string
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain plugintk.DomainAPI) (fromDomain plugintk.DomainCallbacks) {
		assert.Equal(t, "domain1", name)
		domainID = id.String()
		waitForRegister <- toDomain
		return tdm
	}

	domainFunctions := &plugintk.DomainAPIFunctions{
		ConfigureDomain: func(ctx context.Context, cdr *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
			assert.Equal(t, int64(12345), cdr.ChainId)
			return &prototk.ConfigureDomainResponse{
				DomainConfig: &prototk.DomainConfig{
					ConstructorAbiJson: "ABI1",
				},
			}, nil
		},
		InitDomain: func(ctx context.Context, idr *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
			assert.Equal(t, domainID, idr.DomainUuid)
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
				SigningAddress: "signing1",
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
				Transaction: &prototk.BaseLedgerTransaction{
					FunctionName: "func1",
				},
			}, nil
		},
	}
	ctx, pc, done := newTestDomainPluginController(t, &testSetup{
		testDomainManager: tdm,
		testDomains: map[string]plugintk.Plugin{
			"domain1": plugintk.NewDomain(func(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {
				return plugintk.DomainImplementation(domainFunctions)
			}),
		},
	})
	defer done()

	domainAPI := <-waitForRegister

	cdr, err := domainAPI.ConfigureDomain(ctx, &prototk.ConfigureDomainRequest{
		ChainId: int64(12345),
	})
	assert.NoError(t, err)
	assert.Equal(t, "ABI1", cdr.DomainConfig.ConstructorAbiJson)

	_, err = domainAPI.InitDomain(ctx, &prototk.InitDomainRequest{
		DomainUuid: domainID,
	})
	assert.NoError(t, err)

	// Should not be initialized
	assert.NoError(t, pc.WaitForInit(ctx))

	idr, err := domainAPI.InitDeploy(ctx, &prototk.InitDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			TransactionId: "deploy_tx1_init",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "lookup1", idr.RequiredVerifiers[0].Lookup)

	pdr, err := domainAPI.PrepareDeploy(ctx, &prototk.PrepareDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			TransactionId: "deploy_tx1_prepare",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "signing1", pdr.SigningAddress)

	itr, err := domainAPI.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "tx2_init",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "lookup2", itr.RequiredVerifiers[0].Lookup)

	atr, err := domainAPI.AssembleTransaction(ctx, &prototk.AssembleTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "tx2_prepare",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_REVERT, atr.AssemblyResult)

	etr, err := domainAPI.EndorseTransaction(ctx, &prototk.EndorseTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "tx2_endorse",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, prototk.EndorseTransactionResponse_SIGN, etr.EndorsementResult)

	ptr, err := domainAPI.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		Transaction: &prototk.FinalizedTransaction{
			TransactionId: "tx2_prepare",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "func1", ptr.Transaction.FunctionName)
}

// func TestFromDomainRequestsOK(t *testing.T) {

// 	waitForResponse := make(chan struct{}, 1)

// 	tdm := &testDomainManager{}
// 	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain plugintk.DomainAPI) (fromDomain plugintk.DomainCallbacks) {
// 		return tdm
// 	}
// 	tdm.findAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
// 		return &prototk.FindAvailableStatesResponse{
// 			States: []*prototk.StoredState{
// 				{HashId: "12345"},
// 			},
// 		}, nil
// 	}

// 	msgID := uuid.NewString()
// 	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
// 		"domain1": {
// 			sendRequest: func(domainID string) *prototk.DomainMessage {
// 				return &prototk.DomainMessage{
// 					Header: &prototk.Header{
// 						PluginId:    domainID,
// 						MessageId:   msgID,
// 						MessageType: prototk.Header_REQUEST_FROM_PLUGIN,
// 					},
// 					RequestFromDomain: &prototk.DomainMessage_FindAvailableStates{
// 						FindAvailableStates: &prototk.FindAvailableStatesRequest{
// 							SchemaId: "schema1",
// 						},
// 					},
// 				}
// 			},
// 			handleResponse: func(dm *prototk.DomainMessage) {
// 				assert.Equal(t, msgID, *dm.Header.CorrelationId)
// 				res := dm.ResponseToDomain.(*prototk.DomainMessage_FindAvailableStatesRes).FindAvailableStatesRes
// 				assert.Equal(t, "12345", res.States[0].HashId)
// 				close(waitForResponse)
// 			},
// 		},
// 	})
// 	defer done()

// 	<-waitForResponse

// }

// func TestFromDomainRequestsError(t *testing.T) {

// 	waitForResponse := make(chan struct{}, 1)

// 	tdm := &testDomainManager{}
// 	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain plugintk.DomainAPI) (fromDomain plugintk.DomainCallbacks) {
// 		return tdm
// 	}
// 	tdm.findAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
// 		return nil, fmt.Errorf("pop")
// 	}

// 	msgID := uuid.NewString()
// 	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
// 		"domain1": {
// 			sendRequest: func(domainID string) *prototk.DomainMessage {
// 				return &prototk.DomainMessage{
// 					Header: &prototk.Header{
// 						PluginId:    domainID,
// 						MessageId:   msgID,
// 						MessageType: prototk.Header_REQUEST_FROM_PLUGIN,
// 					},
// 					RequestFromDomain: &prototk.DomainMessage_FindAvailableStates{
// 						FindAvailableStates: &prototk.FindAvailableStatesRequest{
// 							SchemaId: "schema1",
// 						},
// 					},
// 				}
// 			},
// 			handleResponse: func(dm *prototk.DomainMessage) {
// 				assert.Equal(t, msgID, *dm.Header.CorrelationId)
// 				assert.Regexp(t, "pop", *dm.Header.ErrorMessage)
// 				close(waitForResponse)
// 			},
// 		},
// 	})
// 	defer done()

// 	<-waitForResponse

// }

// func TestFromDomainRequestBadReq(t *testing.T) {

// 	waitForResponse := make(chan struct{}, 1)

// 	tdm := &testDomainManager{}
// 	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain plugintk.DomainAPI) (fromDomain plugintk.DomainCallbacks) {
// 		return tdm
// 	}

// 	msgID := uuid.NewString()
// 	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
// 		"domain1": {
// 			sendRequest: func(domainID string) *prototk.DomainMessage {
// 				return &prototk.DomainMessage{
// 					Header: &prototk.Header{
// 						PluginId:    domainID,
// 						MessageId:   msgID,
// 						MessageType: prototk.Header_REQUEST_FROM_PLUGIN,
// 						// Missing payload
// 					},
// 				}
// 			},
// 			handleResponse: func(dm *prototk.DomainMessage) {
// 				assert.Equal(t, msgID, *dm.Header.CorrelationId)
// 				assert.Regexp(t, "PD011203", *dm.Header.ErrorMessage)
// 				close(waitForResponse)
// 			},
// 		},
// 	})
// 	defer done()

// 	<-waitForResponse

// }

// func TestDomainRequestsFail(t *testing.T) {

// 	waitForRegister := make(chan plugintk.DomainAPI, 1)

// 	tdm := &testDomainManager{}
// 	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain plugintk.DomainAPI) (fromDomain plugintk.DomainCallbacks) {
// 		waitForRegister <- toDomain
// 		return tdm
// 	}

// 	ctx, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
// 		"domain1": {
// 			customResponses: func(req *prototk.DomainMessage) []*prototk.DomainMessage {
// 				return []*prototk.DomainMessage{
// 					{
// 						Header: &prototk.Header{
// 							PluginId:      req.Header.PluginId,
// 							MessageId:     uuid.NewString(),
// 							CorrelationId: &req.Header.MessageId,
// 							MessageType:   prototk.Header_ERROR_RESPONSE,
// 							ErrorMessage:  confutil.P("pop"),
// 						},
// 					},
// 				}
// 			},
// 		},
// 	})
// 	defer done()

// 	domainAPI := <-waitForRegister

// 	_, err := domainAPI.ConfigureDomain(ctx, &prototk.ConfigureDomainRequest{})
// 	assert.Regexp(t, "pop", err)

// }

// func TestDomainRequestsBadResponse(t *testing.T) {

// 	waitForRegister := make(chan plugintk.DomainAPI, 1)

// 	tdm := &testDomainManager{}
// 	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain plugintk.DomainAPI) (fromDomain plugintk.DomainCallbacks) {
// 		waitForRegister <- toDomain
// 		return tdm
// 	}

// 	ctx, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
// 		"domain1": {
// 			customResponses: func(req *prototk.DomainMessage) []*prototk.DomainMessage {
// 				return []*prototk.DomainMessage{
// 					{
// 						Header: &prototk.Header{
// 							PluginId:      req.Header.PluginId,
// 							CorrelationId: &req.Header.MessageId,
// 							MessageType:   prototk.Header_RESPONSE_FROM_PLUGIN,
// 						},
// 						ResponseFromDomain: &prototk.DomainMessage_AssembleTransactionRes{
// 							AssembleTransactionRes: &prototk.AssembleTransactionResponse{
// 								RevertReason: confutil.P("this is not a configure response"),
// 							},
// 						},
// 					},
// 				}
// 			},
// 		},
// 	})
// 	defer done()

// 	domainAPI := <-waitForRegister

// 	_, err := domainAPI.ConfigureDomain(ctx, &prototk.ConfigureDomainRequest{})
// 	assert.Regexp(t, "PD011204", err)

// }

// func TestDomainSendBeforeRegister(t *testing.T) {

// 	waitForRegister := make(chan uuid.UUID, 1)

// 	tdm := &testDomainManager{}
// 	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain plugintk.DomainAPI) (fromDomain plugintk.DomainCallbacks) {
// 		waitForRegister <- id
// 		return tdm
// 	}

// 	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
// 		"domain1": {
// 			preRegister: func(string) *prototk.DomainMessage {
// 				return &prototk.DomainMessage{
// 					Header: &prototk.Header{
// 						MessageType: prototk.Header_REQUEST_FROM_PLUGIN,
// 					},
// 				}
// 			},
// 		},
// 	})
// 	defer done()

// 	<-waitForRegister
// }

// func TestDomainSendDoubleRegister(t *testing.T) {

// 	waitForRegister := make(chan plugintk.DomainAPI, 1)

// 	tdm := &testDomainManager{}
// 	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain plugintk.DomainAPI) (fromDomain plugintk.DomainCallbacks) {
// 		waitForRegister <- toDomain
// 		return tdm
// 	}

// 	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
// 		"domain1": {
// 			preRegister: func(domainID string) *prototk.DomainMessage {
// 				return &prototk.DomainMessage{
// 					Header: &prototk.Header{
// 						MessageType: prototk.Header_REGISTER,
// 						PluginId:    domainID,
// 						MessageId:   uuid.NewString(),
// 					},
// 				}
// 			},
// 		},
// 	})
// 	defer done()

// 	<-waitForRegister

// }

// func TestDomainRegisterWrongID(t *testing.T) {

// 	waitForError := make(chan error, 1)

// 	tdm := &testDomainManager{}
// 	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain plugintk.DomainAPI) (fromDomain plugintk.DomainCallbacks) {
// 		return tdm
// 	}

// 	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
// 		"domain1": {
// 			preRegister: func(domainID string) *prototk.DomainMessage {
// 				return &prototk.DomainMessage{
// 					Header: &prototk.Header{
// 						MessageType: prototk.Header_REGISTER,
// 						PluginId:    uuid.NewString(), // unknown to registry
// 						MessageId:   uuid.NewString(),
// 					},
// 				}
// 			},
// 			expectClose: func(err error) {
// 				waitForError <- err
// 			},
// 		},
// 	})
// 	defer done()

// 	assert.Regexp(t, "UUID", <-waitForError)
// }

// func TestDomainSendResponseWrongID(t *testing.T) {

// 	waitForRegister := make(chan plugintk.DomainAPI, 1)

// 	tdm := &testDomainManager{}
// 	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain plugintk.DomainAPI) (fromDomain plugintk.DomainCallbacks) {
// 		waitForRegister <- toDomain
// 		return tdm
// 	}

// 	ctx, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
// 		"domain1": {
// 			customResponses: func(req *prototk.DomainMessage) []*prototk.DomainMessage {
// 				unknownRequest := uuid.NewString()
// 				return []*prototk.DomainMessage{
// 					// One response for an unknown request - should be ignored, as it could
// 					// simply be a context timeout on the requesting side
// 					{
// 						Header: &prototk.Header{
// 							PluginId:      req.Header.PluginId,
// 							MessageId:     uuid.NewString(),
// 							CorrelationId: &unknownRequest,
// 							MessageType:   prototk.Header_RESPONSE_FROM_PLUGIN,
// 						},
// 						ResponseFromDomain: &prototk.DomainMessage_AssembleTransactionRes{
// 							AssembleTransactionRes: &prototk.AssembleTransactionResponse{},
// 						},
// 					},
// 					// Response with the data we want to see on the right correlID after
// 					{
// 						Header: &prototk.Header{
// 							PluginId:      req.Header.PluginId,
// 							MessageId:     uuid.NewString(),
// 							CorrelationId: &req.Header.MessageId,
// 							MessageType:   prototk.Header_RESPONSE_FROM_PLUGIN,
// 						},
// 						ResponseFromDomain: &prototk.DomainMessage_AssembleTransactionRes{
// 							AssembleTransactionRes: &prototk.AssembleTransactionResponse{
// 								AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
// 							},
// 						},
// 					},
// 				}
// 			},
// 		},
// 	})
// 	defer done()

// 	domainAPI := <-waitForRegister
// 	atr, err := domainAPI.AssembleTransaction(ctx, &prototk.AssembleTransactionRequest{
// 		Transaction: &prototk.TransactionSpecification{
// 			TransactionId: "tx2_prepare",
// 		},
// 	})
// 	assert.NoError(t, err)
// 	assert.Equal(t, prototk.AssembleTransactionResponse_REVERT, atr.AssemblyResult)
// }
