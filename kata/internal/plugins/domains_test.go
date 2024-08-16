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
	configureDomain     func(pbp.PluginController_ConnectDomainClient, *pbp.ConfigureDomainRequest) *pbp.ConfigureDomainResponse
	initDomain          func(pbp.PluginController_ConnectDomainClient, *pbp.InitDomainRequest) *pbp.InitDomainResponse
	initDeploy          func(pbp.PluginController_ConnectDomainClient, *pbp.InitDeployRequest) *pbp.InitDeployResponse
	prepareDeploy       func(pbp.PluginController_ConnectDomainClient, *pbp.PrepareDeployRequest) *pbp.PrepareDeployResponse
	initTransaction     func(pbp.PluginController_ConnectDomainClient, *pbp.InitTransactionRequest) *pbp.InitTransactionResponse
	assembleTransaction func(pbp.PluginController_ConnectDomainClient, *pbp.AssembleTransactionRequest) *pbp.AssembleTransactionResponse
	endorseTransaction  func(pbp.PluginController_ConnectDomainClient, *pbp.EndorseTransactionRequest) *pbp.EndorseTransactionResponse
	prepareTransaction  func(pbp.PluginController_ConnectDomainClient, *pbp.PrepareTransactionRequest) *pbp.PrepareTransactionResponse
}

func (tp *testDomain) conf() *PluginConfig {
	return &PluginConfig{
		Type:     types.Enum[LibraryType](LibraryTypeCShared),
		Location: "/any/where",
	}
}

func (tp *testDomain) run(t *testing.T, ctx context.Context, id string, client pbp.PluginControllerClient) {
	stream, err := client.ConnectDomain(ctx)
	assert.NoError(t, err)

	err = stream.Send(&pbp.DomainMessage{
		DomainId:    id,
		MessageId:   uuid.New().String(),
		MessageType: pbp.DomainMessage_REGISTER,
	})
	assert.NoError(t, err)

	for {
		msg, err := stream.Recv()
		assert.NoError(t, err)
		if err != nil {
			return
		}
		if msg.MessageType == pbp.DomainMessage_REQUEST_TO_DOMAIN {
			assert.NotEmpty(t, msg.MessageId)
			assert.NotNil(t, msg.CorrelationId)
			reply := &pbp.DomainMessage{
				DomainId:      id,
				MessageId:     uuid.New().String(),
				MessageType:   pbp.DomainMessage_RESPONSE_FROM_DOMAIN,
				CorrelationId: &msg.MessageId,
			}
			switch req := msg.RequestToDomain.(type) {
			case *pbp.DomainMessage_ConfigureDomain:
				reply.ResponseFromDomain = &pbp.DomainMessage_ConfigureDomainRes{
					ConfigureDomainRes: tp.configureDomain(stream, req.ConfigureDomain),
				}
			case *pbp.DomainMessage_InitDomain:
				reply.ResponseFromDomain = &pbp.DomainMessage_InitDomainRes{
					InitDomainRes: tp.initDomain(stream, req.InitDomain),
				}
			case *pbp.DomainMessage_InitDeploy:
				reply.ResponseFromDomain = &pbp.DomainMessage_InitDeployRes{
					InitDeployRes: tp.initDeploy(stream, req.InitDeploy),
				}
			case *pbp.DomainMessage_PrepareDeploy:
				reply.ResponseFromDomain = &pbp.DomainMessage_PrepareDeployRes{
					PrepareDeployRes: tp.prepareDeploy(stream, req.PrepareDeploy),
				}
			case *pbp.DomainMessage_InitTransaction:
				reply.ResponseFromDomain = &pbp.DomainMessage_InitTransactionRes{
					InitTransactionRes: tp.initTransaction(stream, req.InitTransaction),
				}
			case *pbp.DomainMessage_AssembleTransaction:
				reply.ResponseFromDomain = &pbp.DomainMessage_AssembleTransactionRes{
					AssembleTransactionRes: tp.assembleTransaction(stream, req.AssembleTransaction),
				}
			case *pbp.DomainMessage_EndorseTransaction:
				reply.ResponseFromDomain = &pbp.DomainMessage_EndorseTransactionRes{
					EndorseTransactionRes: tp.endorseTransaction(stream, req.EndorseTransaction),
				}
			case *pbp.DomainMessage_PrepareTransaction:
				reply.ResponseFromDomain = &pbp.DomainMessage_PrepareTransactionRes{
					PrepareTransactionRes: tp.prepareTransaction(stream, req.PrepareTransaction),
				}
			default:
				assert.Failf(t, "unexpected: %s", jsonProto(msg))
			}
			err := stream.Send(reply)
			assert.NoError(t, err)
		}
	}
}

func TestDomain1(t *testing.T) {

	waitForRegister := make(chan DomainAPI, 1)

	tdm := &testDomainManager{}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks) {
		assert.Equal(t, "domain1", name)
		assert.NotEmpty(t, id)
		waitForRegister <- toDomain
		return tdm
	}

	ctx, _, done := newTestDomainPluginController(t, tdm, map[string]*testDomain{
		"domain1": {
			configureDomain: func(pc pbp.PluginController_ConnectDomainClient, cdr *pbp.ConfigureDomainRequest) *pbp.ConfigureDomainResponse {
				assert.Equal(t, int64(12345), cdr.ChainId)
				return &pbp.ConfigureDomainResponse{
					DomainConfig: &pbp.DomainConfig{
						ConstructorAbiJson: "ABI1",
					},
				}
			},
		},
	})
	defer done()

	domainAPI := <-waitForRegister
	res, err := domainAPI.ConfigureDomain(ctx, &pbp.ConfigureDomainRequest{
		ChainId: int64(12345),
	})
	assert.NoError(t, err)
	assert.Equal(t, "ABI1", res.DomainConfig.AbiStateSchemasJson)

}
