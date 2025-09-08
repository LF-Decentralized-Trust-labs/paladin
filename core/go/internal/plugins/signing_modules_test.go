/*
 * Copyright © 2025 Kaleido, Inc.
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

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signpayloads"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type testKeyManager struct {
	signingModules          map[string]plugintk.Plugin
	signingModuleRegistered func(name string, id uuid.UUID, toSigningModule components.KeyManagerToSigningModule) (fromSigningModule plugintk.SigningModuleCallbacks, err error)
}

func signingModuleConnectFactory(ctx context.Context, client prototk.PluginControllerClient) (grpc.BidiStreamingClient[prototk.SigningModuleMessage, prototk.SigningModuleMessage], error) {
	return client.ConnectSigningModule(context.Background())
}

func signingModuleHeaderAccessor(msg *prototk.SigningModuleMessage) *prototk.Header {
	if msg.Header == nil {
		msg.Header = &prototk.Header{}
	}
	return msg.Header
}

func (tkm *testKeyManager) mock(t *testing.T) *componentsmocks.KeyManager {
	mkm := componentsmocks.NewKeyManager(t)
	pluginMap := make(map[string]*pldconf.PluginConfig)
	for name := range tkm.signingModules {
		pluginMap[name] = &pldconf.PluginConfig{
			Type:    string(pldtypes.LibraryTypeCShared),
			Library: "/tmp/not/applicable",
		}
	}
	mkm.On("ConfiguredSigningModules").Return(pluginMap).Maybe()
	msmr := mkm.On("SigningModuleRegistered", mock.Anything, mock.Anything, mock.Anything).Maybe()
	msmr.Run(func(args mock.Arguments) {
		m2sm, err := tkm.signingModuleRegistered(args[0].(string), args[1].(uuid.UUID), args[2].(components.KeyManagerToSigningModule))
		msmr.Return(m2sm, err)
	})
	return mkm
}

func newTestSigningModulePluginManager(t *testing.T, setup *testManagers) (context.Context, *pluginManager, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	tpm := newTestPluginManager(t, setup)

	tpl, err := NewUnitTestPluginLoader(tpm.GRPCTargetURL(), tpm.loaderID.String(), setup.allPlugins())
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		tpl.Run()
	}()

	return ctx, tpm, func() {
		recovered := recover()
		if recovered != nil {
			fmt.Fprintf(os.Stderr, "%v: %s", recovered, debug.Stack())
			panic(recovered)
		}
		cancelCtx()
		tpm.Stop()
		tpl.Stop()
		<-done
	}
}

func TestSigningModuleRequestsOK(t *testing.T) {
	waitForAPI := make(chan components.KeyManagerToSigningModule, 1)
	waitForCallbacks := make(chan plugintk.SigningModuleCallbacks, 1)

	signingModuleFunctions := &plugintk.SigningModuleAPIFunctions{
		ConfigureSigningModule: func(ctx context.Context, csmr *prototk.ConfigureSigningModuleRequest) (*prototk.ConfigureSigningModuleResponse, error) {
			return &prototk.ConfigureSigningModuleResponse{}, nil
		},
		ResolveKey: func(ctx context.Context, rkr *prototk.ResolveKeyRequest) (*prototk.ResolveKeyResponse, error) {
			assert.Equal(t, "key1", rkr.Name)
			return &prototk.ResolveKeyResponse{
				KeyHandle: "key_handle_1",
				Identifiers: []*prototk.PublicKeyIdentifier{
					{
						Algorithm:    rkr.RequiredIdentifiers[0].Algorithm,
						VerifierType: rkr.RequiredIdentifiers[0].VerifierType,
						Verifier:     "0x98a356e0814382587d42b62bd97871ee59d10b69",
					},
				},
			}, nil
		},
		Sign: func(ctx context.Context, swkr *prototk.SignWithKeyRequest) (*prototk.SignWithKeyResponse, error) {
			assert.Equal(t, "key_handle_1", swkr.KeyHandle)
			return &prototk.SignWithKeyResponse{
				Payload: ([]byte)("signed data"),
			}, nil
		},
		ListKeys: func(ctx context.Context, lkr *prototk.ListKeysRequest) (*prototk.ListKeysResponse, error) {
			assert.Equal(t, int32(10), lkr.Limit)
			return &prototk.ListKeysResponse{
				Items: []*prototk.ListKeyEntry{
					{
						Name:      "key 23456",
						KeyHandle: "key23456",
						Identifiers: []*prototk.PublicKeyIdentifier{
							{Algorithm: algorithms.ECDSA_SECP256K1, Verifier: "0x93e5a15ce57564278575ff7182b5b3746251e781"},
						},
					},
				},
				Next: "key23456",
			}, nil
		},
		Close: func(ctx context.Context, cr *prototk.CloseRequest) (*prototk.CloseResponse, error) {
			return &prototk.CloseResponse{}, nil
		},
	}

	tkm := &testKeyManager{
		signingModules: map[string]plugintk.Plugin{
			"signingModule1": plugintk.NewSigningModule(func(callbacks plugintk.SigningModuleCallbacks) plugintk.SigningModuleAPI {
				waitForCallbacks <- callbacks
				return &plugintk.SigningModuleAPIBase{Functions: signingModuleFunctions}
			}),
		},
	}
	tkm.signingModuleRegistered = func(name string, id uuid.UUID, toSigningModule components.KeyManagerToSigningModule) (plugintk.SigningModuleCallbacks, error) {
		assert.Equal(t, "signingModule1", name)
		waitForAPI <- toSigningModule
		return tkm, nil
	}

	ctx, smpm, done := newTestSigningModulePluginManager(t, &testManagers{
		testKeyManager: tkm,
	})
	defer done()

	var signingModuleAPI components.KeyManagerToSigningModule
	select {
	case signingModuleAPI = <-waitForAPI:
		// Received signing module API
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for signing module API - expected registration was not received")
	}

	_, err := signingModuleAPI.ConfigureSigningModule(ctx, &prototk.ConfigureSigningModuleRequest{})
	require.NoError(t, err)

	resolveKeyResponse, err := signingModuleAPI.ResolveKey(ctx, &prototk.ResolveKeyRequest{
		RequiredIdentifiers: []*prototk.PublicKeyIdentifierType{{Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS}},
		Name:                "key1",
	})
	require.NoError(t, err)
	assert.Equal(t, "key_handle_1", resolveKeyResponse.KeyHandle)

	signWithKeyResponse, err := signingModuleAPI.Sign(ctx, &prototk.SignWithKeyRequest{
		KeyHandle:   "key_handle_1",
		Algorithm:   algorithms.ECDSA_SECP256K1,
		PayloadType: signpayloads.OPAQUE_TO_RSV,
		Payload:     ([]byte)("some data"),
	})
	require.NoError(t, err)
	assert.Equal(t, ([]byte)("signed data"), signWithKeyResponse.Payload)

	listKeysResponse, err := signingModuleAPI.ListKeys(ctx, &prototk.ListKeysRequest{
		Continue: "key12345",
		Limit:    int32(10),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, len(listKeysResponse.Items))

	closeResponse, err := signingModuleAPI.Close(ctx, &prototk.CloseRequest{})
	require.NoError(t, err)
	assert.NotNil(t, closeResponse)

	// This is the point the key manager would call us to say the signing module is initialized
	// (once it's happy it's updated its internal state)
	signingModuleAPI.Initialized()
	require.NoError(t, smpm.WaitForInit(ctx, prototk.PluginInfo_SIGNING_MODULE))

	// Add timeout for callbacks
	var callbacks plugintk.SigningModuleCallbacks
	select {
	case callbacks = <-waitForCallbacks:
		assert.NotNil(t, callbacks)
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for callbacks - expected callbacks were not received")
	}
}

func TestSigningModuleRegisterFail(t *testing.T) {
	waitForError := make(chan error, 1)

	tkm := &testKeyManager{
		signingModules: map[string]plugintk.Plugin{
			"signingModule1": &mockPlugin[prototk.SigningModuleMessage]{
				t:              t,
				connectFactory: signingModuleConnectFactory,
				headerAccessor: signingModuleHeaderAccessor,
				preRegister: func(signingModuleID string) *prototk.SigningModuleMessage {
					return &prototk.SigningModuleMessage{
						Header: &prototk.Header{
							MessageType: prototk.Header_REGISTER,
							PluginId:    signingModuleID,
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
	tkm.signingModuleRegistered = func(name string, id uuid.UUID, toSigningModule components.KeyManagerToSigningModule) (plugintk.SigningModuleCallbacks, error) {
		return nil, fmt.Errorf("pop")
	}

	_, _, done := newTestSigningModulePluginManager(t, &testManagers{
		testKeyManager: tkm,
	})
	defer done()

	// Add timeout to prevent test from hanging indefinitely
	select {
	case err := <-waitForError:
		assert.Regexp(t, "pop", err.Error())
	case <-time.After(20 * time.Second):
		t.Fatal("Test timed out waiting for error - expected error was not received")
	}
}

func TestFromSigningModuleRequestBadReq(t *testing.T) {
	waitForResponse := make(chan struct{}, 1)

	msgID := uuid.NewString()
	tkm := &testKeyManager{
		signingModules: map[string]plugintk.Plugin{
			"signingModule1": &mockPlugin[prototk.SigningModuleMessage]{
				t:              t,
				connectFactory: signingModuleConnectFactory,
				headerAccessor: signingModuleHeaderAccessor,
				sendRequest: func(pluginID string) *prototk.SigningModuleMessage {
					return &prototk.SigningModuleMessage{
						Header: &prototk.Header{
							PluginId:    pluginID,
							MessageId:   msgID,
							MessageType: prototk.Header_REQUEST_FROM_PLUGIN,
							// Missing payload
						},
					}
				},
				handleResponse: func(smm *prototk.SigningModuleMessage) {
					assert.Equal(t, msgID, *smm.Header.CorrelationId)
					assert.Regexp(t, "PD011203", *smm.Header.ErrorMessage)
					close(waitForResponse)
				},
			},
		},
	}
	tkm.signingModuleRegistered = func(name string, id uuid.UUID, toSigningModule components.KeyManagerToSigningModule) (plugintk.SigningModuleCallbacks, error) {
		return tkm, nil
	}

	_, _, done := newTestSigningModulePluginManager(t, &testManagers{
		testKeyManager: tkm,
	})
	defer done()

	// Add timeout to prevent test from hanging indefinitely
	select {
	case <-waitForResponse:
		// Response received successfully
	case <-time.After(10 * time.Second):
		t.Fatal("Test timed out waiting for response - expected response was not received")
	}
}
