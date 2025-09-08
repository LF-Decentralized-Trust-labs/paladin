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
package plugintk

import (
	"context"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func setupSigningModuleTests(t *testing.T) (context.Context, *pluginExerciser[prototk.SigningModuleMessage], *SigningModuleAPIFunctions, SigningModuleCallbacks, map[string]func(*prototk.SigningModuleMessage), func()) {
	ctx, tc, tcDone := newTestController(t)

	/***** THIS PART AN IMPLEMENTATION WOULD DO ******/
	funcs := &SigningModuleAPIFunctions{
		// Functions go here
	}
	waitForCallbacks := make(chan SigningModuleCallbacks, 1)
	signingModule := NewSigningModule(func(callbacks SigningModuleCallbacks) SigningModuleAPI {
		// Implementation would construct an instance here to start handling the API calls from Paladin,
		// (rather than passing the callbacks to the test as we do here)
		waitForCallbacks <- callbacks
		return &SigningModuleAPIBase{funcs}
	})
	/************************************************/

	// The rest is mocking the other side of the interface
	inOutMap := map[string]func(*prototk.SigningModuleMessage){}
	pluginID := uuid.NewString()
	exerciser := newPluginExerciser(t, pluginID, &SigningModuleMessageWrapper{}, inOutMap)
	tc.fakeSigningModuleController = exerciser.controller

	signingModuleDone := make(chan struct{})
	go func() {
		defer close(signingModuleDone)
		signingModule.Run("unix:"+tc.socketFile, pluginID)
	}()
	callbacks := <-waitForCallbacks

	return ctx, exerciser, funcs, callbacks, inOutMap, func() {
		checkPanic()
		signingModule.Stop()
		tcDone()
		<-signingModuleDone
	}
}

func TestSigningModuleFunction_ConfigureSigningModule(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupSigningModuleTests(t)
	defer done()

	// ConfigureSigningModule - paladin to signing module
	funcs.ConfigureSigningModule = func(ctx context.Context, cdr *prototk.ConfigureSigningModuleRequest) (*prototk.ConfigureSigningModuleResponse, error) {
		return &prototk.ConfigureSigningModuleResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.SigningModuleMessage) {
		req.RequestToSigningModule = &prototk.SigningModuleMessage_ConfigureSigningModule{
			ConfigureSigningModule: &prototk.ConfigureSigningModuleRequest{},
		}
	}, func(res *prototk.SigningModuleMessage) {
		assert.IsType(t, &prototk.SigningModuleMessage_ConfigureSigningModuleRes{}, res.ResponseFromSigningModule)
	})
}

func TestSigningModuleFunction_ResolveKey(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupSigningModuleTests(t)
	defer done()

	// ResolveKey - paladin to signing module
	funcs.ResolveKey = func(ctx context.Context, cdr *prototk.ResolveKeyRequest) (*prototk.ResolveKeyResponse, error) {
		return &prototk.ResolveKeyResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.SigningModuleMessage) {
		req.RequestToSigningModule = &prototk.SigningModuleMessage_ResolveKey{
			ResolveKey: &prototk.ResolveKeyRequest{},
		}
	}, func(res *prototk.SigningModuleMessage) {
		assert.IsType(t, &prototk.SigningModuleMessage_ResolveKeyRes{}, res.ResponseFromSigningModule)
	})
}

func TestSigningModuleFunction_Sign(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupSigningModuleTests(t)
	defer done()

	// Sign - paladin to signing module
	funcs.Sign = func(ctx context.Context, cdr *prototk.SignWithKeyRequest) (*prototk.SignWithKeyResponse, error) {
		return &prototk.SignWithKeyResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.SigningModuleMessage) {
		req.RequestToSigningModule = &prototk.SigningModuleMessage_Sign{
			Sign: &prototk.SignWithKeyRequest{},
		}
	}, func(res *prototk.SigningModuleMessage) {
		assert.IsType(t, &prototk.SigningModuleMessage_SignRes{}, res.ResponseFromSigningModule)
	})
}

func TestSigningModuleFunction_ListKeys(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupSigningModuleTests(t)
	defer done()

	// ListKeys - paladin to signing module
	funcs.ListKeys = func(ctx context.Context, cdr *prototk.ListKeysRequest) (*prototk.ListKeysResponse, error) {
		return &prototk.ListKeysResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.SigningModuleMessage) {
		req.RequestToSigningModule = &prototk.SigningModuleMessage_ListKeys{
			ListKeys: &prototk.ListKeysRequest{},
		}
	}, func(res *prototk.SigningModuleMessage) {
		assert.IsType(t, &prototk.SigningModuleMessage_ListKeysRes{}, res.ResponseFromSigningModule)
	})
}

func TestSigningModuleFunction_Close(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupSigningModuleTests(t)
	defer done()

	// Close - paladin to signing module
	funcs.Close = func(ctx context.Context, cdr *prototk.CloseRequest) (*prototk.CloseResponse, error) {
		return &prototk.CloseResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.SigningModuleMessage) {
		req.RequestToSigningModule = &prototk.SigningModuleMessage_Close{
			Close: &prototk.CloseRequest{},
		}
	}, func(res *prototk.SigningModuleMessage) {
		assert.IsType(t, &prototk.SigningModuleMessage_CloseRes{}, res.ResponseFromSigningModule)
	})
}

func TestSigningModuleRequestError(t *testing.T) {
	_, exerciser, _, _, _, done := setupSigningModuleTests(t)
	defer done()

	// Check responseToPluginAs handles nil
	exerciser.doExchangeToPlugin(func(req *prototk.SigningModuleMessage) {}, func(res *prototk.SigningModuleMessage) {
		assert.Regexp(t, "PD020300", *res.Header.ErrorMessage)
	})
}

func TestSigningModuleWrapperFields(t *testing.T) {
	m := &SigningModulePluginMessage{m: &prototk.SigningModuleMessage{}}
	assert.Nil(t, m.RequestFromPlugin())
	assert.Nil(t, m.ResponseToPlugin())
}
