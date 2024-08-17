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
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	pbp "github.com/kaleido-io/paladin/kata/pkg/proto/plugins"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/stretchr/testify/assert"
)

type testDomainManager struct {
	domainRegistered    func(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks)
	findAvailableStates func(context.Context, *pbp.FindAvailableStatesRequest) (*pbp.FindAvailableStatesResponse, error)
}

func (tp *testDomainManager) FindAvailableStates(ctx context.Context, req *pbp.FindAvailableStatesRequest) (*pbp.FindAvailableStatesResponse, error) {
	return tp.findAvailableStates(ctx, req)
}

func (tdm *testDomainManager) DomainRegistered(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks) {
	return tdm.domainRegistered(name, id, toDomain)
}

type testDomain struct {
	configureDomain     func(*pbp.ConfigureDomainRequest) *pbp.ConfigureDomainResponse
	initDomain          func(*pbp.InitDomainRequest) *pbp.InitDomainResponse
	initDeploy          func(*pbp.InitDeployRequest) *pbp.InitDeployResponse
	prepareDeploy       func(*pbp.PrepareDeployRequest) *pbp.PrepareDeployResponse
	initTransaction     func(*pbp.InitTransactionRequest) *pbp.InitTransactionResponse
	assembleTransaction func(*pbp.AssembleTransactionRequest) *pbp.AssembleTransactionResponse
	endorseTransaction  func(*pbp.EndorseTransactionRequest) *pbp.EndorseTransactionResponse
	prepareTransaction  func(*pbp.PrepareTransactionRequest) *pbp.PrepareTransactionResponse

	preRegister     func(domainID string) *pbp.DomainMessage
	customResponses func(*pbp.DomainMessage) []*pbp.DomainMessage
	expectClose     func(err error)

	sendRequest    func(domainID string) *pbp.DomainMessage
	handleResponse func(*pbp.DomainMessage)
}

func (tp *testDomain) conf() *PluginConfig {
	return &PluginConfig{
		Type:     types.Enum[LibraryType](LibraryTypeCShared),
		Location: "/any/where",
	}
}

func (tp *testDomain) run(t *testing.T, connectCtx context.Context, id string, client pbp.PluginControllerClient) {
	stream, err := client.ConnectDomain(connectCtx)
	assert.NoError(t, err)

	if tp.preRegister != nil {
		err = stream.Send(tp.preRegister(id))
		assert.NoError(t, err)
	}
	err = stream.Send(&pbp.DomainMessage{
		DomainId:    id,
		MessageId:   uuid.New().String(),
		MessageType: pbp.DomainMessage_REGISTER,
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
		case pbp.DomainMessage_REQUEST_TO_DOMAIN:
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
			reply := &pbp.DomainMessage{
				DomainId:      id,
				MessageId:     uuid.New().String(),
				MessageType:   pbp.DomainMessage_RESPONSE_FROM_DOMAIN,
				CorrelationId: &msg.MessageId,
			}
			switch req := msg.RequestToDomain.(type) {
			case *pbp.DomainMessage_ConfigureDomain:
				reply.ResponseFromDomain = &pbp.DomainMessage_ConfigureDomainRes{
					ConfigureDomainRes: tp.configureDomain(req.ConfigureDomain),
				}
			case *pbp.DomainMessage_InitDomain:
				reply.ResponseFromDomain = &pbp.DomainMessage_InitDomainRes{
					InitDomainRes: tp.initDomain(req.InitDomain),
				}
			case *pbp.DomainMessage_InitDeploy:
				reply.ResponseFromDomain = &pbp.DomainMessage_InitDeployRes{
					InitDeployRes: tp.initDeploy(req.InitDeploy),
				}
			case *pbp.DomainMessage_PrepareDeploy:
				reply.ResponseFromDomain = &pbp.DomainMessage_PrepareDeployRes{
					PrepareDeployRes: tp.prepareDeploy(req.PrepareDeploy),
				}
			case *pbp.DomainMessage_InitTransaction:
				reply.ResponseFromDomain = &pbp.DomainMessage_InitTransactionRes{
					InitTransactionRes: tp.initTransaction(req.InitTransaction),
				}
			case *pbp.DomainMessage_AssembleTransaction:
				reply.ResponseFromDomain = &pbp.DomainMessage_AssembleTransactionRes{
					AssembleTransactionRes: tp.assembleTransaction(req.AssembleTransaction),
				}
			case *pbp.DomainMessage_EndorseTransaction:
				reply.ResponseFromDomain = &pbp.DomainMessage_EndorseTransactionRes{
					EndorseTransactionRes: tp.endorseTransaction(req.EndorseTransaction),
				}
			case *pbp.DomainMessage_PrepareTransaction:
				reply.ResponseFromDomain = &pbp.DomainMessage_PrepareTransactionRes{
					PrepareTransactionRes: tp.prepareTransaction(req.PrepareTransaction),
				}
			default:
				assert.Failf(t, "unexpected: %s", jsonProto(msg))
			}
			err := stream.Send(reply)
			assert.NoError(t, err)
		case pbp.DomainMessage_RESPONSE_TO_DOMAIN, pbp.DomainMessage_ERROR_RESPONSE:
			tp.handleResponse(msg)
		}
	}
}

func TestDomainRequestsOK(t *testing.T) {

	waitForRegister := make(chan DomainAPI, 1)

	tdm := &testDomainManager{}
	var domainID string
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks) {
		assert.Equal(t, "domain1", name)
		assert.NotEmpty(t, id)
		domainID = id.String()
		waitForRegister <- toDomain
		return tdm
	}

	ctx, pc, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			configureDomain: func(cdr *pbp.ConfigureDomainRequest) *pbp.ConfigureDomainResponse {
				assert.Equal(t, int64(12345), cdr.ChainId)
				return &pbp.ConfigureDomainResponse{
					DomainConfig: &pbp.DomainConfig{
						ConstructorAbiJson: "ABI1",
					},
				}
			},
			initDomain: func(idr *pbp.InitDomainRequest) *pbp.InitDomainResponse {
				assert.Equal(t, domainID, idr.DomainUuid)
				return &pbp.InitDomainResponse{}
			},
			initDeploy: func(idr *pbp.InitDeployRequest) *pbp.InitDeployResponse {
				assert.Equal(t, "deploy_tx1_init", idr.Transaction.TransactionId)
				return &pbp.InitDeployResponse{RequiredVerifiers: []*pbp.ResolveVerifierRequest{
					{Lookup: "lookup1"},
				}}
			},
			prepareDeploy: func(pdr *pbp.PrepareDeployRequest) *pbp.PrepareDeployResponse {
				assert.Equal(t, "deploy_tx1_prepare", pdr.Transaction.TransactionId)
				return &pbp.PrepareDeployResponse{
					SigningAddress: "signing1",
				}
			},
			initTransaction: func(itr *pbp.InitTransactionRequest) *pbp.InitTransactionResponse {
				assert.Equal(t, "tx2_init", itr.Transaction.TransactionId)
				return &pbp.InitTransactionResponse{
					RequiredVerifiers: []*pbp.ResolveVerifierRequest{
						{Lookup: "lookup2"},
					},
				}
			},
			assembleTransaction: func(atr *pbp.AssembleTransactionRequest) *pbp.AssembleTransactionResponse {
				assert.Equal(t, "tx2_prepare", atr.Transaction.TransactionId)
				return &pbp.AssembleTransactionResponse{
					AssemblyResult: pbp.AssembleTransactionResponse_REVERT,
				}
			},
			endorseTransaction: func(etr *pbp.EndorseTransactionRequest) *pbp.EndorseTransactionResponse {
				assert.Equal(t, "tx2_endorse", etr.Transaction.TransactionId)
				return &pbp.EndorseTransactionResponse{
					EndorsementResult: pbp.EndorseTransactionResponse_SIGN,
				}
			},
			prepareTransaction: func(ptr *pbp.PrepareTransactionRequest) *pbp.PrepareTransactionResponse {
				assert.Equal(t, "tx2_prepare", ptr.Transaction.TransactionId)
				return &pbp.PrepareTransactionResponse{
					Transaction: &pbp.BaseLedgerTransaction{
						FunctionName: "func1",
					},
				}
			},
		},
	})
	defer done()

	domainAPI := <-waitForRegister

	cdr, err := domainAPI.ConfigureDomain(ctx, &pbp.ConfigureDomainRequest{
		ChainId: int64(12345),
	})
	assert.NoError(t, err)
	assert.Equal(t, "ABI1", cdr.DomainConfig.ConstructorAbiJson)

	_, err = domainAPI.InitDomain(ctx, &pbp.InitDomainRequest{
		DomainUuid: domainID,
	})
	assert.NoError(t, err)

	// Should not be initialized
	assert.NoError(t, pc.WaitForInit(ctx))

	idr, err := domainAPI.InitDeploy(ctx, &pbp.InitDeployRequest{
		Transaction: &pbp.DeployTransactionSpecification{
			TransactionId: "deploy_tx1_init",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "lookup1", idr.RequiredVerifiers[0].Lookup)

	pdr, err := domainAPI.PrepareDeploy(ctx, &pbp.PrepareDeployRequest{
		Transaction: &pbp.DeployTransactionSpecification{
			TransactionId: "deploy_tx1_prepare",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "signing1", pdr.SigningAddress)

	itr, err := domainAPI.InitTransaction(ctx, &pbp.InitTransactionRequest{
		Transaction: &pbp.TransactionSpecification{
			TransactionId: "tx2_init",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "lookup2", itr.RequiredVerifiers[0].Lookup)

	atr, err := domainAPI.AssembleTransaction(ctx, &pbp.AssembleTransactionRequest{
		Transaction: &pbp.TransactionSpecification{
			TransactionId: "tx2_prepare",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, pbp.AssembleTransactionResponse_REVERT, atr.AssemblyResult)

	etr, err := domainAPI.EndorseTransaction(ctx, &pbp.EndorseTransactionRequest{
		Transaction: &pbp.TransactionSpecification{
			TransactionId: "tx2_endorse",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, pbp.EndorseTransactionResponse_SIGN, etr.EndorsementResult)

	ptr, err := domainAPI.PrepareTransaction(ctx, &pbp.PrepareTransactionRequest{
		Transaction: &pbp.FinalizedTransaction{
			TransactionId: "tx2_prepare",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "func1", ptr.Transaction.FunctionName)
}

func TestFromDomainRequestsOK(t *testing.T) {

	waitForResponse := make(chan struct{}, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks) { return tdm }
	tdm.findAvailableStates = func(ctx context.Context, req *pbp.FindAvailableStatesRequest) (*pbp.FindAvailableStatesResponse, error) {
		return &pbp.FindAvailableStatesResponse{
			States: []*pbp.StoredState{
				{HashId: "12345"},
			},
		}, nil
	}

	msgID := uuid.NewString()
	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			sendRequest: func(domainID string) *pbp.DomainMessage {
				return &pbp.DomainMessage{
					DomainId:    domainID,
					MessageId:   msgID,
					MessageType: pbp.DomainMessage_REQUEST_FROM_DOMAIN,
					RequestFromDomain: &pbp.DomainMessage_FindAvailableStates{
						FindAvailableStates: &pbp.FindAvailableStatesRequest{
							SchemaId: "schema1",
						},
					},
				}
			},
			handleResponse: func(dm *pbp.DomainMessage) {
				assert.Equal(t, msgID, *dm.CorrelationId)
				res := dm.ResponseToDomain.(*pbp.DomainMessage_FindAvailableStatesRes).FindAvailableStatesRes
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
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks) { return tdm }
	tdm.findAvailableStates = func(ctx context.Context, req *pbp.FindAvailableStatesRequest) (*pbp.FindAvailableStatesResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	msgID := uuid.NewString()
	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			sendRequest: func(domainID string) *pbp.DomainMessage {
				return &pbp.DomainMessage{
					DomainId:    domainID,
					MessageId:   msgID,
					MessageType: pbp.DomainMessage_REQUEST_FROM_DOMAIN,
					RequestFromDomain: &pbp.DomainMessage_FindAvailableStates{
						FindAvailableStates: &pbp.FindAvailableStatesRequest{
							SchemaId: "schema1",
						},
					},
				}
			},
			handleResponse: func(dm *pbp.DomainMessage) {
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
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks) { return tdm }
	tdm.findAvailableStates = func(ctx context.Context, req *pbp.FindAvailableStatesRequest) (*pbp.FindAvailableStatesResponse, error) {
		panic("pop")
	}

	msgID := uuid.NewString()
	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			sendRequest: func(domainID string) *pbp.DomainMessage {
				return &pbp.DomainMessage{
					DomainId:    domainID,
					MessageId:   msgID,
					MessageType: pbp.DomainMessage_REQUEST_FROM_DOMAIN,
					RequestFromDomain: &pbp.DomainMessage_FindAvailableStates{
						FindAvailableStates: &pbp.FindAvailableStatesRequest{
							SchemaId: "schema1",
						},
					},
				}
			},
			handleResponse: func(dm *pbp.DomainMessage) {
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
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks) { return tdm }

	msgID := uuid.NewString()
	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			sendRequest: func(domainID string) *pbp.DomainMessage {
				return &pbp.DomainMessage{
					DomainId:    domainID,
					MessageId:   msgID,
					MessageType: pbp.DomainMessage_REQUEST_FROM_DOMAIN,
					// Missing payload
				}
			},
			handleResponse: func(dm *pbp.DomainMessage) {
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

	waitForRegister := make(chan DomainAPI, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks) {
		waitForRegister <- toDomain
		return tdm
	}

	ctx, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			customResponses: func(req *pbp.DomainMessage) []*pbp.DomainMessage {
				return []*pbp.DomainMessage{
					{
						DomainId:      req.DomainId,
						MessageId:     uuid.NewString(),
						CorrelationId: &req.MessageId,
						MessageType:   pbp.DomainMessage_ERROR_RESPONSE,
						ErrorMessage:  confutil.P("pop"),
					},
				}
			},
		},
	})
	defer done()

	domainAPI := <-waitForRegister

	_, err := domainAPI.ConfigureDomain(ctx, &pbp.ConfigureDomainRequest{})
	assert.Regexp(t, "pop", err)

}

func TestDomainRequestsBadResponse(t *testing.T) {

	waitForRegister := make(chan DomainAPI, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks) {
		waitForRegister <- toDomain
		return tdm
	}

	ctx, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			customResponses: func(req *pbp.DomainMessage) []*pbp.DomainMessage {
				return []*pbp.DomainMessage{
					{
						DomainId:      req.DomainId,
						CorrelationId: &req.MessageId,
						MessageType:   pbp.DomainMessage_RESPONSE_FROM_DOMAIN,
					},
				}
			},
		},
	})
	defer done()

	domainAPI := <-waitForRegister

	_, err := domainAPI.ConfigureDomain(ctx, &pbp.ConfigureDomainRequest{})
	assert.Regexp(t, "PD011204.*RESPONSE_FROM_DOMAIN", err)

}

func TestDomainSendAfterClose(t *testing.T) {

	waitForRegister := make(chan uuid.UUID, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks) {
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
	dh.send(ctx, &pbp.DomainMessage{})

	// Not restart trying to send on closed stream, and check it handles it
	dh.sendChl = make(chan *pbp.DomainMessage, 1)
	dh.senderDone = make(chan struct{})
	dh.ctx = context.Background()
	dh.sendChl <- &pbp.DomainMessage{}
	dh.sender()

}

func TestDomainRequestTimeoutSend(t *testing.T) {

	waitForRegister := make(chan uuid.UUID, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks) {
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
	dh.sendChl = make(chan *pbp.DomainMessage, 1)

	// Request with cancelled context
	cancelled, cancelFunc := context.WithCancel(context.Background())
	cancelFunc()
	err := dh.requestToDomain(cancelled,
		func(dm *pbp.DomainMessage) {
			dm.RequestToDomain = &pbp.DomainMessage_InitDomain{}
		},
		func(dm *pbp.DomainMessage) bool {
			return true
		},
	)
	assert.Error(t, err)

}

func TestDomainSendBeforeRegister(t *testing.T) {

	waitForRegister := make(chan uuid.UUID, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks) {
		waitForRegister <- id
		return tdm
	}

	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			preRegister: func(string) *pbp.DomainMessage {
				return &pbp.DomainMessage{
					MessageType: pbp.DomainMessage_REQUEST_FROM_DOMAIN,
				}
			},
		},
	})
	defer done()

	<-waitForRegister
}

func TestDomainSendDoubleRegister(t *testing.T) {

	waitForRegister := make(chan DomainAPI, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks) {
		waitForRegister <- toDomain
		return tdm
	}

	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			preRegister: func(domainID string) *pbp.DomainMessage {
				return &pbp.DomainMessage{
					MessageType: pbp.DomainMessage_REGISTER,
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
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks) {
		return tdm
	}

	_, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			preRegister: func(domainID string) *pbp.DomainMessage {
				return &pbp.DomainMessage{
					MessageType: pbp.DomainMessage_REGISTER,
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

	waitForRegister := make(chan DomainAPI, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks) {
		waitForRegister <- toDomain
		return tdm
	}

	ctx, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			customResponses: func(req *pbp.DomainMessage) []*pbp.DomainMessage {
				unknownRequest := uuid.NewString()
				return []*pbp.DomainMessage{
					// One response for an unknown request - should be ignored, as it could
					// simply be a context timeout on the requesting side
					{
						DomainId:      req.DomainId,
						MessageId:     uuid.NewString(),
						CorrelationId: &unknownRequest,
						MessageType:   pbp.DomainMessage_RESPONSE_FROM_DOMAIN,
						ResponseFromDomain: &pbp.DomainMessage_AssembleTransactionRes{
							AssembleTransactionRes: &pbp.AssembleTransactionResponse{},
						},
					},
					// Response with the data we want to see on the right correlID after
					{
						DomainId:      req.DomainId,
						MessageId:     uuid.NewString(),
						CorrelationId: &req.MessageId,
						MessageType:   pbp.DomainMessage_RESPONSE_FROM_DOMAIN,
						ResponseFromDomain: &pbp.DomainMessage_AssembleTransactionRes{
							AssembleTransactionRes: &pbp.AssembleTransactionResponse{
								AssemblyResult: pbp.AssembleTransactionResponse_REVERT,
							},
						},
					},
				}
			},
		},
	})
	defer done()

	domainAPI := <-waitForRegister
	atr, err := domainAPI.AssembleTransaction(ctx, &pbp.AssembleTransactionRequest{
		Transaction: &pbp.TransactionSpecification{
			TransactionId: "tx2_prepare",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, pbp.AssembleTransactionResponse_REVERT, atr.AssemblyResult)
}
