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
	"testing"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/domaintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

type testDomainManager struct {
	domainRegistered    func(name string, id uuid.UUID, toDomain domaintk.DomainAPI) (fromDomain domaintk.DomainCallbacks)
	findAvailableStates func(context.Context, *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error)
}

func (tp *testDomainManager) FindAvailableStates(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
	return tp.findAvailableStates(ctx, req)
}

func (tdm *testDomainManager) DomainRegistered(name string, id uuid.UUID, toDomain domaintk.DomainAPI) (fromDomain domaintk.DomainCallbacks) {
	return tdm.domainRegistered(name, id, toDomain)
}

type testDomain struct {
	configureDomain     func(*prototk.ConfigureDomainRequest) *prototk.ConfigureDomainResponse
	initDomain          func(*prototk.InitDomainRequest) *prototk.InitDomainResponse
	initDeploy          func(*prototk.InitDeployRequest) *prototk.InitDeployResponse
	prepareDeploy       func(*prototk.PrepareDeployRequest) *prototk.PrepareDeployResponse
	initTransaction     func(*prototk.InitTransactionRequest) *prototk.InitTransactionResponse
	assembleTransaction func(*prototk.AssembleTransactionRequest) *prototk.AssembleTransactionResponse
	endorseTransaction  func(*prototk.EndorseTransactionRequest) *prototk.EndorseTransactionResponse
	prepareTransaction  func(*prototk.PrepareTransactionRequest) *prototk.PrepareTransactionResponse

	preRegister     func(domainID string) *prototk.DomainMessage
	customResponses func(*prototk.DomainMessage) []*prototk.DomainMessage
	expectClose     func(err error)

	sendRequest    func(domainID string) *prototk.DomainMessage
	handleResponse func(*prototk.DomainMessage)
}

func (tp *testDomain) conf() *PluginConfig {
	return &PluginConfig{
		Type:     types.Enum[LibraryType](LibraryTypeCShared),
		Location: "/any/where",
	}
}

func (tp *testDomain) run(t *testing.T, connectCtx context.Context, id string, client prototk.PluginControllerClient) {
	stream, err := client.ConnectDomain(connectCtx)
	assert.NoError(t, err)

	if tp.preRegister != nil {
		err = stream.Send(tp.preRegister(id))
		assert.NoError(t, err)
	}
	err = stream.Send(&prototk.DomainMessage{
		DomainId:    id,
		MessageId:   uuid.New().String(),
		MessageType: prototk.DomainMessage_REGISTER,
	})
	assert.NoError(t, err)

	ctx := stream.Context()
	for {
		if tp.sendRequest != nil {
			req := tp.sendRequest(id)
			err := stream.Send(req)
			assert.NoError(t, err)
			tp.sendRequest = nil
		}

		msg, err := stream.Recv()
		if err != nil {
			log.L(ctx).Infof("exiting: %s", err)
			if tp.expectClose != nil {
				tp.expectClose(err)
			}
			return
		}
		switch msg.MessageType {
		case prototk.DomainMessage_REQUEST_TO_DOMAIN:
			if tp.customResponses != nil {
				responses := tp.customResponses(msg)
				for _, r := range responses {
					err := stream.Send(r)
					assert.NoError(t, err)
				}
				continue
			}
			assert.NotEmpty(t, msg.MessageId)
			assert.Nil(t, msg.CorrelationId)
			reply := &prototk.DomainMessage{
				DomainId:      id,
				MessageId:     uuid.New().String(),
				MessageType:   prototk.DomainMessage_RESPONSE_FROM_DOMAIN,
				CorrelationId: &msg.MessageId,
			}
			switch req := msg.RequestToDomain.(type) {
			case *prototk.DomainMessage_ConfigureDomain:
				reply.ResponseFromDomain = &prototk.DomainMessage_ConfigureDomainRes{
					ConfigureDomainRes: tp.configureDomain(req.ConfigureDomain),
				}
			case *prototk.DomainMessage_InitDomain:
				reply.ResponseFromDomain = &prototk.DomainMessage_InitDomainRes{
					InitDomainRes: tp.initDomain(req.InitDomain),
				}
			case *prototk.DomainMessage_InitDeploy:
				reply.ResponseFromDomain = &prototk.DomainMessage_InitDeployRes{
					InitDeployRes: tp.initDeploy(req.InitDeploy),
				}
			case *prototk.DomainMessage_PrepareDeploy:
				reply.ResponseFromDomain = &prototk.DomainMessage_PrepareDeployRes{
					PrepareDeployRes: tp.prepareDeploy(req.PrepareDeploy),
				}
			case *prototk.DomainMessage_InitTransaction:
				reply.ResponseFromDomain = &prototk.DomainMessage_InitTransactionRes{
					InitTransactionRes: tp.initTransaction(req.InitTransaction),
				}
			case *prototk.DomainMessage_AssembleTransaction:
				reply.ResponseFromDomain = &prototk.DomainMessage_AssembleTransactionRes{
					AssembleTransactionRes: tp.assembleTransaction(req.AssembleTransaction),
				}
			case *prototk.DomainMessage_EndorseTransaction:
				reply.ResponseFromDomain = &prototk.DomainMessage_EndorseTransactionRes{
					EndorseTransactionRes: tp.endorseTransaction(req.EndorseTransaction),
				}
			case *prototk.DomainMessage_PrepareTransaction:
				reply.ResponseFromDomain = &prototk.DomainMessage_PrepareTransactionRes{
					PrepareTransactionRes: tp.prepareTransaction(req.PrepareTransaction),
				}
			default:
				assert.Failf(t, "unexpected: %s", jsonProto(msg))
			}
			err := stream.Send(reply)
			assert.NoError(t, err)
		case prototk.DomainMessage_RESPONSE_TO_DOMAIN, prototk.DomainMessage_ERROR_RESPONSE:
			tp.handleResponse(msg)
		}
	}
}

func TestDomainRequestsOK(t *testing.T) {

	waitForRegister := make(chan domaintk.DomainAPI, 1)

	tdm := &testDomainManager{}
	var domainID string
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain domaintk.DomainAPI) (fromDomain domaintk.DomainCallbacks) {
		assert.Equal(t, "domain1", name)
		assert.NotEmpty(t, id)
		domainID = id.String()
		waitForRegister <- toDomain
		return tdm
	}

	ctx, pc, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			configureDomain: func(cdr *prototk.ConfigureDomainRequest) *prototk.ConfigureDomainResponse {
				assert.Equal(t, int64(12345), cdr.ChainId)
				return &prototk.ConfigureDomainResponse{
					DomainConfig: &prototk.DomainConfig{
						ConstructorAbiJson: "ABI1",
					},
				}
			},
			initDomain: func(idr *prototk.InitDomainRequest) *prototk.InitDomainResponse {
				assert.Equal(t, domainID, idr.DomainUuid)
				return &prototk.InitDomainResponse{}
			},
			initDeploy: func(idr *prototk.InitDeployRequest) *prototk.InitDeployResponse {
				assert.Equal(t, "deploy_tx1_init", idr.Transaction.TransactionId)
				return &prototk.InitDeployResponse{RequiredVerifiers: []*prototk.ResolveVerifierRequest{
					{Lookup: "lookup1"},
				}}
			},
			prepareDeploy: func(pdr *prototk.PrepareDeployRequest) *prototk.PrepareDeployResponse {
				assert.Equal(t, "deploy_tx1_prepare", pdr.Transaction.TransactionId)
				return &prototk.PrepareDeployResponse{
					SigningAddress: "signing1",
				}
			},
			initTransaction: func(itr *prototk.InitTransactionRequest) *prototk.InitTransactionResponse {
				assert.Equal(t, "tx2_init", itr.Transaction.TransactionId)
				return &prototk.InitTransactionResponse{
					RequiredVerifiers: []*prototk.ResolveVerifierRequest{
						{Lookup: "lookup2"},
					},
				}
			},
			assembleTransaction: func(atr *prototk.AssembleTransactionRequest) *prototk.AssembleTransactionResponse {
				assert.Equal(t, "tx2_prepare", atr.Transaction.TransactionId)
				return &prototk.AssembleTransactionResponse{
					AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
				}
			},
			endorseTransaction: func(etr *prototk.EndorseTransactionRequest) *prototk.EndorseTransactionResponse {
				assert.Equal(t, "tx2_endorse", etr.Transaction.TransactionId)
				return &prototk.EndorseTransactionResponse{
					EndorsementResult: prototk.EndorseTransactionResponse_SIGN,
				}
			},
			prepareTransaction: func(ptr *prototk.PrepareTransactionRequest) *prototk.PrepareTransactionResponse {
				assert.Equal(t, "tx2_prepare", ptr.Transaction.TransactionId)
				return &prototk.PrepareTransactionResponse{
					Transaction: &prototk.BaseLedgerTransaction{
						FunctionName: "func1",
					},
				}
			},
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

func TestFromDomainRequestsOK(t *testing.T) {

	waitForResponse := make(chan struct{}, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain domaintk.DomainAPI) (fromDomain domaintk.DomainCallbacks) {
		return tdm
	}
	tdm.findAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{
				{HashId: "12345"},
			},
		}, nil
	}

	msgID := uuid.NewString()
	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			sendRequest: func(domainID string) *prototk.DomainMessage {
				return &prototk.DomainMessage{
					DomainId:    domainID,
					MessageId:   msgID,
					MessageType: prototk.DomainMessage_REQUEST_FROM_DOMAIN,
					RequestFromDomain: &prototk.DomainMessage_FindAvailableStates{
						FindAvailableStates: &prototk.FindAvailableStatesRequest{
							SchemaId: "schema1",
						},
					},
				}
			},
			handleResponse: func(dm *prototk.DomainMessage) {
				assert.Equal(t, msgID, *dm.CorrelationId)
				res := dm.ResponseToDomain.(*prototk.DomainMessage_FindAvailableStatesRes).FindAvailableStatesRes
				assert.Equal(t, "12345", res.States[0].HashId)
				close(waitForResponse)
			},
		},
	})
	defer done()

	<-waitForResponse

}

func TestFromDomainRequestsError(t *testing.T) {

	waitForResponse := make(chan struct{}, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain domaintk.DomainAPI) (fromDomain domaintk.DomainCallbacks) {
		return tdm
	}
	tdm.findAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	msgID := uuid.NewString()
	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			sendRequest: func(domainID string) *prototk.DomainMessage {
				return &prototk.DomainMessage{
					DomainId:    domainID,
					MessageId:   msgID,
					MessageType: prototk.DomainMessage_REQUEST_FROM_DOMAIN,
					RequestFromDomain: &prototk.DomainMessage_FindAvailableStates{
						FindAvailableStates: &prototk.FindAvailableStatesRequest{
							SchemaId: "schema1",
						},
					},
				}
			},
			handleResponse: func(dm *prototk.DomainMessage) {
				assert.Equal(t, msgID, *dm.CorrelationId)
				assert.Regexp(t, "pop", *dm.ErrorMessage)
				close(waitForResponse)
			},
		},
	})
	defer done()

	<-waitForResponse

}

func TestFromDomainRequestPanic(t *testing.T) {

	waitForResponse := make(chan struct{}, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain domaintk.DomainAPI) (fromDomain domaintk.DomainCallbacks) {
		return tdm
	}
	tdm.findAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		panic("pop")
	}

	msgID := uuid.NewString()
	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			sendRequest: func(domainID string) *prototk.DomainMessage {
				return &prototk.DomainMessage{
					DomainId:    domainID,
					MessageId:   msgID,
					MessageType: prototk.DomainMessage_REQUEST_FROM_DOMAIN,
					RequestFromDomain: &prototk.DomainMessage_FindAvailableStates{
						FindAvailableStates: &prototk.FindAvailableStatesRequest{
							SchemaId: "schema1",
						},
					},
				}
			},
			handleResponse: func(dm *prototk.DomainMessage) {
				assert.Equal(t, msgID, *dm.CorrelationId)
				assert.Regexp(t, "pop", *dm.ErrorMessage)
				close(waitForResponse)
			},
		},
	})
	defer done()

	<-waitForResponse

}

func TestFromDomainRequestBadReq(t *testing.T) {

	waitForResponse := make(chan struct{}, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain domaintk.DomainAPI) (fromDomain domaintk.DomainCallbacks) {
		return tdm
	}

	msgID := uuid.NewString()
	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			sendRequest: func(domainID string) *prototk.DomainMessage {
				return &prototk.DomainMessage{
					DomainId:    domainID,
					MessageId:   msgID,
					MessageType: prototk.DomainMessage_REQUEST_FROM_DOMAIN,
					// Missing payload
				}
			},
			handleResponse: func(dm *prototk.DomainMessage) {
				assert.Equal(t, msgID, *dm.CorrelationId)
				assert.Regexp(t, "PD011205", *dm.ErrorMessage)
				close(waitForResponse)
			},
		},
	})
	defer done()

	<-waitForResponse

}

func TestDomainRequestsFail(t *testing.T) {

	waitForRegister := make(chan domaintk.DomainAPI, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain domaintk.DomainAPI) (fromDomain domaintk.DomainCallbacks) {
		waitForRegister <- toDomain
		return tdm
	}

	ctx, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			customResponses: func(req *prototk.DomainMessage) []*prototk.DomainMessage {
				return []*prototk.DomainMessage{
					{
						DomainId:      req.DomainId,
						MessageId:     uuid.NewString(),
						CorrelationId: &req.MessageId,
						MessageType:   prototk.DomainMessage_ERROR_RESPONSE,
						ErrorMessage:  confutil.P("pop"),
					},
				}
			},
		},
	})
	defer done()

	domainAPI := <-waitForRegister

	_, err := domainAPI.ConfigureDomain(ctx, &prototk.ConfigureDomainRequest{})
	assert.Regexp(t, "pop", err)

}

func TestDomainRequestsBadResponse(t *testing.T) {

	waitForRegister := make(chan domaintk.DomainAPI, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain domaintk.DomainAPI) (fromDomain domaintk.DomainCallbacks) {
		waitForRegister <- toDomain
		return tdm
	}

	ctx, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			customResponses: func(req *prototk.DomainMessage) []*prototk.DomainMessage {
				return []*prototk.DomainMessage{
					{
						DomainId:      req.DomainId,
						CorrelationId: &req.MessageId,
						MessageType:   prototk.DomainMessage_RESPONSE_FROM_DOMAIN,
					},
				}
			},
		},
	})
	defer done()

	domainAPI := <-waitForRegister

	_, err := domainAPI.ConfigureDomain(ctx, &prototk.ConfigureDomainRequest{})
	assert.Regexp(t, "PD011204.*RESPONSE_FROM_DOMAIN", err)

}

func TestDomainSendAfterClose(t *testing.T) {

	waitForRegister := make(chan uuid.UUID, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain domaintk.DomainAPI) (fromDomain domaintk.DomainCallbacks) {
		waitForRegister <- id
		return tdm
	}

	ctx, pc, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {},
	})

	// Get handler, and close it
	domainID := <-waitForRegister
	plugin := pc.domainPlugins[domainID]
	assert.NotNil(t, plugin.handler)
	done()
	dh := plugin.handler
	<-dh.senderDone

	// Check send doesn't block on closed context
	dh.send(ctx, &prototk.DomainMessage{})

	// Not restart trying to send on closed stream, and check it handles it
	dh.sendChl = make(chan *prototk.DomainMessage, 1)
	dh.senderDone = make(chan struct{})
	dh.ctx = context.Background()
	dh.sendChl <- &prototk.DomainMessage{}
	dh.sender()

}

func TestDomainRequestTimeoutSend(t *testing.T) {

	waitForRegister := make(chan uuid.UUID, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain domaintk.DomainAPI) (fromDomain domaintk.DomainCallbacks) {
		waitForRegister <- id
		return tdm
	}

	_, pc, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {},
	})

	// Get handler, and close it
	domainID := <-waitForRegister
	plugin := pc.domainPlugins[domainID]
	assert.NotNil(t, plugin.handler)
	done()
	dh := plugin.handler
	<-dh.senderDone
	dh.sendChl = make(chan *prototk.DomainMessage, 1)

	// Request with cancelled context
	cancelled, cancelFunc := context.WithCancel(context.Background())
	cancelFunc()
	err := dh.requestToDomain(cancelled,
		func(dm *prototk.DomainMessage) {
			dm.RequestToDomain = &prototk.DomainMessage_InitDomain{}
		},
		func(dm *prototk.DomainMessage) bool {
			return true
		},
	)
	assert.Error(t, err)

}

func TestDomainSendBeforeRegister(t *testing.T) {

	waitForRegister := make(chan uuid.UUID, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain domaintk.DomainAPI) (fromDomain domaintk.DomainCallbacks) {
		waitForRegister <- id
		return tdm
	}

	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			preRegister: func(string) *prototk.DomainMessage {
				return &prototk.DomainMessage{
					MessageType: prototk.DomainMessage_REQUEST_FROM_DOMAIN,
				}
			},
		},
	})
	defer done()

	<-waitForRegister
}

func TestDomainSendDoubleRegister(t *testing.T) {

	waitForRegister := make(chan domaintk.DomainAPI, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain domaintk.DomainAPI) (fromDomain domaintk.DomainCallbacks) {
		waitForRegister <- toDomain
		return tdm
	}

	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			preRegister: func(domainID string) *prototk.DomainMessage {
				return &prototk.DomainMessage{
					MessageType: prototk.DomainMessage_REGISTER,
					DomainId:    domainID,
					MessageId:   uuid.NewString(),
				}
			},
		},
	})
	defer done()

	<-waitForRegister

}

func TestDomainRegisterWrongID(t *testing.T) {

	waitForError := make(chan error, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain domaintk.DomainAPI) (fromDomain domaintk.DomainCallbacks) {
		return tdm
	}

	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			preRegister: func(domainID string) *prototk.DomainMessage {
				return &prototk.DomainMessage{
					MessageType: prototk.DomainMessage_REGISTER,
					DomainId:    uuid.NewString(), // unknown to registry
					MessageId:   uuid.NewString(),
				}
			},
			expectClose: func(err error) {
				waitForError <- err
			},
		},
	})
	defer done()

	assert.Regexp(t, "UUID", <-waitForError)
}

func TestDomainSendResponseWrongID(t *testing.T) {

	waitForRegister := make(chan domaintk.DomainAPI, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain domaintk.DomainAPI) (fromDomain domaintk.DomainCallbacks) {
		waitForRegister <- toDomain
		return tdm
	}

	ctx, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			customResponses: func(req *prototk.DomainMessage) []*prototk.DomainMessage {
				unknownRequest := uuid.NewString()
				return []*prototk.DomainMessage{
					// One response for an unknown request - should be ignored, as it could
					// simply be a context timeout on the requesting side
					{
						DomainId:      req.DomainId,
						MessageId:     uuid.NewString(),
						CorrelationId: &unknownRequest,
						MessageType:   prototk.DomainMessage_RESPONSE_FROM_DOMAIN,
						ResponseFromDomain: &prototk.DomainMessage_AssembleTransactionRes{
							AssembleTransactionRes: &prototk.AssembleTransactionResponse{},
						},
					},
					// Response with the data we want to see on the right correlID after
					{
						DomainId:      req.DomainId,
						MessageId:     uuid.NewString(),
						CorrelationId: &req.MessageId,
						MessageType:   prototk.DomainMessage_RESPONSE_FROM_DOMAIN,
						ResponseFromDomain: &prototk.DomainMessage_AssembleTransactionRes{
							AssembleTransactionRes: &prototk.AssembleTransactionResponse{
								AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
							},
						},
					},
				}
			},
		},
	})
	defer done()

	domainAPI := <-waitForRegister
	atr, err := domainAPI.AssembleTransaction(ctx, &prototk.AssembleTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "tx2_prepare",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_REVERT, atr.AssemblyResult)
}
