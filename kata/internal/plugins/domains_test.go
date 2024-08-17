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

	errorResponse func(*pbp.DomainMessage) *pbp.DomainMessage
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

	err = stream.Send(&pbp.DomainMessage{
		DomainId:    id,
		MessageId:   uuid.New().String(),
		MessageType: pbp.DomainMessage_REGISTER,
	})
	assert.NoError(t, err)

	ctx := stream.Context()
	for {
		msg, err := stream.Recv()
		if err != nil {
			log.L(ctx).Infof("exiting: %s", err)
			return
		}
		if msg.MessageType == pbp.DomainMessage_REQUEST_TO_DOMAIN {
			if tp.errorResponse != nil {
				err := stream.Send(tp.errorResponse(msg))
				assert.NoError(t, err)
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

	ctx, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
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

func TestDomainRequestsFail(t *testing.T) {

	waitForRegister := make(chan DomainAPI, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks) {
		waitForRegister <- toDomain
		return tdm
	}

	ctx, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			errorResponse: func(req *pbp.DomainMessage) *pbp.DomainMessage {
				reply := &pbp.DomainMessage{
					DomainId:      req.DomainId,
					MessageId:     uuid.NewString(),
					CorrelationId: &req.MessageId,
					MessageType:   pbp.DomainMessage_ERROR_RESPONSE,
					ErrorMessage:  confutil.P("pop"),
				}
				return reply
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
			errorResponse: func(req *pbp.DomainMessage) *pbp.DomainMessage {
				reply := &pbp.DomainMessage{
					DomainId:      req.DomainId,
					CorrelationId: &req.MessageId,
					MessageType:   pbp.DomainMessage_RESPONSE_FROM_DOMAIN,
				}
				return reply
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

	// Not restart trying to send on closed channel, and check it handles it
	dh.send = make(chan *pbp.DomainMessage, 1)
	dh.senderDone = make(chan struct{})
	dh.send <- &pbp.DomainMessage{}
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
	dh.send = make(chan *pbp.DomainMessage, 1)

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
