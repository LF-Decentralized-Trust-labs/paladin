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
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type mockPlugin struct {
	t *testing.T

	conf            *PluginConfig
	preRegister     func(domainID string) *prototk.DomainMessage
	customResponses func(*prototk.DomainMessage) []*prototk.DomainMessage
	expectClose     func(err error)

	sendRequest    func(domainID string) *prototk.DomainMessage
	handleResponse func(*prototk.DomainMessage)
}

func (tp *mockPlugin) Conf() *PluginConfig {
	if tp.conf == nil {
		tp.conf = &PluginConfig{
			Type:    types.Enum[LibraryType](LibraryTypeCShared),
			Library: "/any/where",
		}
	}
	return tp.conf
}

// mockPlugin is used to test generic situations that apply across plugin types.
// Note in the tests in this file we use Domain plugins, but simply because we have to have a type.
func (tp *mockPlugin) Run(grpcTarget, pluginId string) {
	t := tp.t

	conn, err := grpc.NewClient(grpcTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NoError(t, err)
	defer conn.Close() // will close all the child conns too

	client := prototk.NewPluginControllerClient(conn)

	stream, err := client.ConnectDomain(context.Background())
	assert.NoError(t, err)

	if tp.preRegister != nil {
		err = stream.Send(tp.preRegister(pluginId))
		assert.NoError(t, err)
	}
	err = stream.Send(&prototk.DomainMessage{
		Header: &prototk.Header{
			PluginId:    pluginId,
			MessageId:   uuid.New().String(),
			MessageType: prototk.Header_REGISTER,
		},
	})
	assert.NoError(t, err)

	// Switch to stream conect
	ctx := stream.Context()
	for {
		if tp.sendRequest != nil {
			req := tp.sendRequest(pluginId)
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
		switch msg.Header.MessageType {
		case prototk.Header_REQUEST_TO_PLUGIN:
			if tp.customResponses != nil {
				responses := tp.customResponses(msg)
				for _, r := range responses {
					err := stream.Send(r)
					assert.NoError(t, err)
				}
				continue
			}
			assert.NotEmpty(t, msg.Header.MessageId)
			assert.Nil(t, msg.Header.CorrelationId)
			reply := &prototk.DomainMessage{
				Header: &prototk.Header{
					PluginId:      pluginId,
					MessageId:     uuid.New().String(),
					MessageType:   prototk.Header_RESPONSE_FROM_PLUGIN,
					CorrelationId: &msg.Header.MessageId,
				},
			}
			err := stream.Send(reply)
			assert.NoError(t, err)
		case prototk.Header_RESPONSE_TO_PLUGIN, prototk.Header_ERROR_RESPONSE:
			tp.handleResponse(msg)
		}
	}
}

func (tp *mockPlugin) Stop() {
	// the mock plugin stops when the conn is closed - that's not the right thing for a proper
	// plugin, but suits the mock one just fine
}

func TestPluginRequestsError(t *testing.T) {

	waitForResponse := make(chan struct{}, 1)

	msgID := uuid.NewString()
	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": &mockPlugin{
				sendRequest: func(domainID string) *prototk.DomainMessage {
					return &prototk.DomainMessage{
						Header: &prototk.Header{
							PluginId:    domainID,
							MessageId:   msgID,
							MessageType: prototk.Header_REQUEST_FROM_PLUGIN,
						},
						RequestFromDomain: &prototk.DomainMessage_FindAvailableStates{
							FindAvailableStates: &prototk.FindAvailableStatesRequest{
								SchemaId: "schema1",
							},
						},
					}
				},
				handleResponse: func(dm *prototk.DomainMessage) {
					assert.Equal(t, msgID, *dm.Header.CorrelationId)
					assert.Regexp(t, "pop", *dm.Header.ErrorMessage)
					close(waitForResponse)
				},
			},
		},
	}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainManagerToDomain) (plugintk.DomainCallbacks, error) {
		return tdm, nil
	}
	tdm.findAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	_, _, done := newTestDomainPluginController(t, &testManagers{
		testDomainManager: tdm,
	})
	defer done()

	<-waitForResponse

}

func TestFromDomainRequestBadReq(t *testing.T) {

	waitForResponse := make(chan struct{}, 1)

	msgID := uuid.NewString()
	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": &mockPlugin{
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
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainManagerToDomain) (plugintk.DomainCallbacks, error) {
		return tdm, nil
	}

	_, _, done := newTestDomainPluginController(t, &testManagers{
		testDomainManager: tdm,
	})
	defer done()

	<-waitForResponse

}

func TestSenderErrorHandling(t *testing.T) {

	waitForRegister := make(chan plugintk.DomainAPI, 1)

	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": &mockPlugin{},
		},
	}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainManagerToDomain) (plugintk.DomainCallbacks, error) {
		waitForRegister <- toDomain
		return tdm, nil
	}

	_, _, done := newTestDomainPluginController(t, &testManagers{
		testDomainManager: tdm,
	})

	domainAPI := <-waitForRegister

	// Stop
	done()

	// Check send loop sending on closed stream
	bridge := domainAPI.(*domainBridge)
	handler := bridge.toPlugin.(*pluginHandler[prototk.DomainMessage])
	handler.senderDone = make(chan struct{})
	handler.sendChl = make(chan plugintk.PluginMessage[prototk.DomainMessage])
	cancellable, cancel := context.WithCancel(context.Background())
	handler.ctx = cancellable
	go func() {
		handler.send(handler.wrapper.Wrap(&prototk.DomainMessage{}))
		cancel() // cancel after first message pushed to sender
	}()
	handler.sender()
	// Check does not block after context is closed
	handler.send(handler.wrapper.Wrap(&prototk.DomainMessage{}))

}

func TestDomainRequestsBadResponse(t *testing.T) {

	waitForRegister := make(chan plugintk.DomainAPI, 1)

	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": &mockPlugin{
				customResponses: func(req *prototk.DomainMessage) []*prototk.DomainMessage {
					return []*prototk.DomainMessage{
						{
							Header: &prototk.Header{
								PluginId:      req.Header.PluginId,
								CorrelationId: &req.Header.MessageId,
								MessageType:   prototk.Header_RESPONSE_FROM_PLUGIN,
							},
							ResponseFromDomain: &prototk.DomainMessage_AssembleTransactionRes{
								AssembleTransactionRes: &prototk.AssembleTransactionResponse{
									RevertReason: confutil.P("this is not a configure response"),
								},
							},
						},
					}
				},
			},
		},
	}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainManagerToDomain) (plugintk.DomainCallbacks, error) {
		waitForRegister <- toDomain
		return tdm, nil
	}

	ctx, _, done := newTestDomainPluginController(t, &testManagers{
		testDomainManager: tdm,
	})
	defer done()

	domainAPI := <-waitForRegister

	_, err := domainAPI.ConfigureDomain(ctx, &prototk.ConfigureDomainRequest{})
	assert.Regexp(t, "PD011205", err)

}

func TestDomainRequestsErrorWithMessage(t *testing.T) {

	waitForRegister := make(chan plugintk.DomainAPI, 1)

	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": &mockPlugin{
				customResponses: func(req *prototk.DomainMessage) []*prototk.DomainMessage {
					return []*prototk.DomainMessage{
						{
							Header: &prototk.Header{
								PluginId:      req.Header.PluginId,
								CorrelationId: &req.Header.MessageId,
								MessageType:   prototk.Header_ERROR_RESPONSE,
								ErrorMessage:  confutil.P("some error"),
							},
						},
					}
				},
			},
		},
	}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainManagerToDomain) (plugintk.DomainCallbacks, error) {
		waitForRegister <- toDomain
		return tdm, nil
	}

	ctx, _, done := newTestDomainPluginController(t, &testManagers{
		testDomainManager: tdm,
	})
	defer done()

	domainAPI := <-waitForRegister

	_, err := domainAPI.ConfigureDomain(ctx, &prototk.ConfigureDomainRequest{})
	assert.Regexp(t, "PD011206.*some error", err)

}

func TestDomainRequestsErrorNoMessage(t *testing.T) {

	waitForRegister := make(chan plugintk.DomainAPI, 1)

	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": &mockPlugin{
				customResponses: func(req *prototk.DomainMessage) []*prototk.DomainMessage {
					return []*prototk.DomainMessage{
						{
							Header: &prototk.Header{
								PluginId:      req.Header.PluginId,
								CorrelationId: &req.Header.MessageId,
								MessageType:   prototk.Header_ERROR_RESPONSE,
							},
						},
					}
				},
			},
		},
	}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainManagerToDomain) (plugintk.DomainCallbacks, error) {
		waitForRegister <- toDomain
		return tdm, nil
	}

	ctx, _, done := newTestDomainPluginController(t, &testManagers{
		testDomainManager: tdm,
	})
	defer done()

	domainAPI := <-waitForRegister

	_, err := domainAPI.ConfigureDomain(ctx, &prototk.ConfigureDomainRequest{})
	assert.Regexp(t, "PD011206.*ERROR_RESPONSE", err)

}

func TestReceiveAfterTimeout(t *testing.T) {

	waitForRegister := make(chan plugintk.DomainAPI, 1)

	readyToGo := make(chan struct{})
	gone := make(chan bool)
	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": &mockPlugin{
				customResponses: func(req *prototk.DomainMessage) []*prototk.DomainMessage {
					close(readyToGo)
					<-gone
					return nil
				},
			},
		},
	}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainManagerToDomain) (plugintk.DomainCallbacks, error) {
		waitForRegister <- toDomain
		return tdm, nil
	}

	ctx, _, done := newTestDomainPluginController(t, &testManagers{
		testDomainManager: tdm,
	})
	defer done()

	domainAPI := <-waitForRegister

	go func() {
		<-readyToGo
		// Force a timeout by closing the inflight handler while the request is mid-flight
		bridge := domainAPI.(*domainBridge)
		handler := bridge.toPlugin.(*pluginHandler[prototk.DomainMessage])
		handler.inflight.Close()
		close(gone)
	}()
	_, err := domainAPI.ConfigureDomain(ctx, &prototk.ConfigureDomainRequest{})
	assert.Regexp(t, "PD020100", err)

}

func TestDomainSendBeforeRegister(t *testing.T) {

	waitForRegister := make(chan uuid.UUID, 1)

	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": &mockPlugin{
				preRegister: func(string) *prototk.DomainMessage {
					return &prototk.DomainMessage{
						Header: &prototk.Header{
							MessageType: prototk.Header_REQUEST_FROM_PLUGIN,
						},
					}
				},
			},
		},
	}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainManagerToDomain) (plugintk.DomainCallbacks, error) {
		waitForRegister <- id
		return tdm, nil
	}

	_, _, done := newTestDomainPluginController(t, &testManagers{
		testDomainManager: tdm,
	})
	defer done()

	<-waitForRegister
}

func TestDomainSendDoubleRegister(t *testing.T) {

	waitForRegister := make(chan plugintk.DomainAPI, 1)

	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": &mockPlugin{
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
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainManagerToDomain) (plugintk.DomainCallbacks, error) {
		waitForRegister <- toDomain
		return tdm, nil
	}

	_, _, done := newTestDomainPluginController(t, &testManagers{
		testDomainManager: tdm,
	})
	defer done()

	<-waitForRegister

}

func TestDomainRegisterWrongID(t *testing.T) {

	waitForError := make(chan error, 1)

	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": &mockPlugin{
				preRegister: func(domainID string) *prototk.DomainMessage {
					return &prototk.DomainMessage{
						Header: &prototk.Header{
							MessageType: prototk.Header_REGISTER,
							PluginId:    uuid.NewString(), // unknown to registry
							MessageId:   uuid.NewString(),
						},
					}
				},
				expectClose: func(err error) {
					waitForError <- err
				},
			},
		},
	}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainManagerToDomain) (plugintk.DomainCallbacks, error) {
		return tdm, nil
	}

	_, _, done := newTestDomainPluginController(t, &testManagers{
		testDomainManager: tdm,
	})
	defer done()

	assert.Regexp(t, "UUID", <-waitForError)
}

func TestDomainRegisterFail(t *testing.T) {

	waitForError := make(chan error, 1)

	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": &mockPlugin{
				preRegister: func(domainID string) *prototk.DomainMessage {
					return &prototk.DomainMessage{
						Header: &prototk.Header{
							MessageType: prototk.Header_REGISTER,
							PluginId:    domainID,
							MessageId:   uuid.NewString(),
						},
					}
				},
				expectClose: func(err error) {
					waitForError <- err
				},
			},
		},
	}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainManagerToDomain) (plugintk.DomainCallbacks, error) {
		return nil, fmt.Errorf("pop")
	}

	_, _, done := newTestDomainPluginController(t, &testManagers{
		testDomainManager: tdm,
	})
	defer done()

	assert.Regexp(t, "pop", <-waitForError)
}

func TestDomainSendResponseWrongID(t *testing.T) {

	waitForRegister := make(chan plugintk.DomainAPI, 1)

	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": &mockPlugin{
				customResponses: func(req *prototk.DomainMessage) []*prototk.DomainMessage {
					unknownRequest := uuid.NewString()
					return []*prototk.DomainMessage{
						// One response for an unknown request - should be ignored, as it could
						// simply be a context timeout on the requesting side
						{
							Header: &prototk.Header{
								PluginId:      req.Header.PluginId,
								MessageId:     uuid.NewString(),
								CorrelationId: &unknownRequest,
								MessageType:   prototk.Header_RESPONSE_FROM_PLUGIN,
							},
							ResponseFromDomain: &prototk.DomainMessage_AssembleTransactionRes{
								AssembleTransactionRes: &prototk.AssembleTransactionResponse{},
							},
						},
						// One response with a nil correl ID
						{
							Header: &prototk.Header{
								PluginId:    req.Header.PluginId,
								MessageId:   uuid.NewString(),
								MessageType: prototk.Header_RESPONSE_FROM_PLUGIN,
							},
							ResponseFromDomain: &prototk.DomainMessage_AssembleTransactionRes{
								AssembleTransactionRes: &prototk.AssembleTransactionResponse{},
							},
						},
						// Response with the data we want to see on the right correlID after
						{
							Header: &prototk.Header{
								PluginId:      req.Header.PluginId,
								MessageId:     uuid.NewString(),
								CorrelationId: &req.Header.MessageId,
								MessageType:   prototk.Header_RESPONSE_FROM_PLUGIN,
							},
							ResponseFromDomain: &prototk.DomainMessage_AssembleTransactionRes{
								AssembleTransactionRes: &prototk.AssembleTransactionResponse{
									AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
								},
							},
						},
					}

				},
			},
		},
	}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainManagerToDomain) (plugintk.DomainCallbacks, error) {
		waitForRegister <- toDomain
		return tdm, nil
	}

	ctx, _, done := newTestDomainPluginController(t, &testManagers{
		testDomainManager: tdm,
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
