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
package plugintk

import (
	"context"
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTransportTests(t *testing.T) (context.Context, *pluginExerciser[prototk.TransportMessage], *TransportAPIFunctions, TransportCallbacks, map[string]func(*prototk.TransportMessage), func()) {
	ctx, tc, tcDone := newTestController(t)

	/***** THIS PART AN IMPLEMENTATION WOULD DO ******/
	funcs := &TransportAPIFunctions{
		// Functions go here
	}
	waitForCallbacks := make(chan TransportCallbacks, 1)
	transport := NewTransport(func(callbacks TransportCallbacks) TransportAPI {
		// Implementation would construct an instance here to start handling the API calls from Paladin,
		// (rather than passing the callbacks to the test as we do here)
		waitForCallbacks <- callbacks
		return &TransportAPIBase{funcs}
	})
	/************************************************/

	// The rest is mocking the other side of the interface
	inOutMap := map[string]func(*prototk.TransportMessage){}
	pluginID := uuid.NewString()
	exerciser := newPluginExerciser(t, pluginID, &TransportMessageWrapper{}, inOutMap)
	tc.fakeTransportController = exerciser.controller

	transportDone := make(chan struct{})
	go func() {
		defer close(transportDone)
		transport.Run("unix:"+tc.socketFile, pluginID)
	}()
	callbacks := <-waitForCallbacks

	return ctx, exerciser, funcs, callbacks, inOutMap, func() {
		checkPanic()
		transport.Stop()
		tcDone()
		<-transportDone
	}
}

func TestTransportCallback_ReceiveMessage(t *testing.T) {
	ctx, _, _, callbacks, inOutMap, done := setupTransportTests(t)
	defer done()

	inOutMap[fmt.Sprintf("%T", &prototk.TransportMessage_ReceiveMessage{})] = func(dm *prototk.TransportMessage) {
		dm.ResponseToTransport = &prototk.TransportMessage_ReceiveMessageRes{
			ReceiveMessageRes: &prototk.ReceiveMessageResponse{},
		}
	}
	_, err := callbacks.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{})
	require.NoError(t, err)
}

func TestTransportCallback_GetTransportDetails(t *testing.T) {
	ctx, _, _, callbacks, inOutMap, done := setupTransportTests(t)
	defer done()

	inOutMap[fmt.Sprintf("%T", &prototk.TransportMessage_GetTransportDetails{})] = func(dm *prototk.TransportMessage) {
		dm.ResponseToTransport = &prototk.TransportMessage_GetTransportDetailsRes{
			GetTransportDetailsRes: &prototk.GetTransportDetailsResponse{},
		}
	}
	_, err := callbacks.GetTransportDetails(ctx, &prototk.GetTransportDetailsRequest{})
	require.NoError(t, err)
}

func TestTransportFunction_ConfigureTransport(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupTransportTests(t)
	defer done()

	// ConfigureTransport - paladin to transport
	funcs.ConfigureTransport = func(ctx context.Context, cdr *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error) {
		return &prototk.ConfigureTransportResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.TransportMessage) {
		req.RequestToTransport = &prototk.TransportMessage_ConfigureTransport{
			ConfigureTransport: &prototk.ConfigureTransportRequest{},
		}
	}, func(res *prototk.TransportMessage) {
		assert.IsType(t, &prototk.TransportMessage_ConfigureTransportRes{}, res.ResponseFromTransport)
	})
}

func TestTransportFunction_SendMessage(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupTransportTests(t)
	defer done()

	// InitTransport - paladin to transport
	funcs.SendMessage = func(ctx context.Context, cdr *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		return &prototk.SendMessageResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.TransportMessage) {
		req.RequestToTransport = &prototk.TransportMessage_SendMessage{
			SendMessage: &prototk.SendMessageRequest{},
		}
	}, func(res *prototk.TransportMessage) {
		assert.IsType(t, &prototk.TransportMessage_SendMessageRes{}, res.ResponseFromTransport)
	})
}

func TestTransportFunction_GetLocalDetails(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupTransportTests(t)
	defer done()

	// InitTransport - paladin to transport
	funcs.GetLocalDetails = func(ctx context.Context, cdr *prototk.GetLocalDetailsRequest) (*prototk.GetLocalDetailsResponse, error) {
		return &prototk.GetLocalDetailsResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.TransportMessage) {
		req.RequestToTransport = &prototk.TransportMessage_GetLocalDetails{
			GetLocalDetails: &prototk.GetLocalDetailsRequest{},
		}
	}, func(res *prototk.TransportMessage) {
		assert.IsType(t, &prototk.TransportMessage_GetLocalDetailsRes{}, res.ResponseFromTransport)
	})
}

func TestTransportFunction_ActivatePeer(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupTransportTests(t)
	defer done()

	// InitTransport - paladin to transport
	funcs.ActivatePeer = func(ctx context.Context, cdr *prototk.ActivatePeerRequest) (*prototk.ActivatePeerResponse, error) {
		return &prototk.ActivatePeerResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.TransportMessage) {
		req.RequestToTransport = &prototk.TransportMessage_ActivatePeer{
			ActivatePeer: &prototk.ActivatePeerRequest{},
		}
	}, func(res *prototk.TransportMessage) {
		assert.IsType(t, &prototk.TransportMessage_ActivatePeerRes{}, res.ResponseFromTransport)
	})
}

func TestTransportFunction_DeactivatePeer(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupTransportTests(t)
	defer done()

	// InitTransport - paladin to transport
	funcs.DeactivatePeer = func(ctx context.Context, cdr *prototk.DeactivatePeerRequest) (*prototk.DeactivatePeerResponse, error) {
		return &prototk.DeactivatePeerResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.TransportMessage) {
		req.RequestToTransport = &prototk.TransportMessage_DeactivatePeer{
			DeactivatePeer: &prototk.DeactivatePeerRequest{},
		}
	}, func(res *prototk.TransportMessage) {
		assert.IsType(t, &prototk.TransportMessage_DeactivatePeerRes{}, res.ResponseFromTransport)
	})
}

func TestTransportRequestError(t *testing.T) {
	_, exerciser, _, _, _, done := setupTransportTests(t)
	defer done()

	// Check responseToPluginAs handles nil
	exerciser.doExchangeToPlugin(func(req *prototk.TransportMessage) {}, func(res *prototk.TransportMessage) {
		assert.Regexp(t, "PD020300", *res.Header.ErrorMessage)
	})
}
