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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

func setupDomainTests(t *testing.T) (context.Context, *pluginExerciser[prototk.DomainMessage], *DomainAPIFunctions, DomainCallbacks, map[string]func(*prototk.DomainMessage), func()) {
	ctx, tc, tcDone := newTestController(t)

	/***** THIS PART AN IMPLEMENTATION WOULD DO ******/
	funcs := &DomainAPIFunctions{
		// Functions go here
	}
	waitForCallbacks := make(chan DomainCallbacks, 1)
	domain := NewDomain(func(callbacks DomainCallbacks) DomainAPI {
		// Implementation would construct an instance here to start handling the API calls from Paladin,
		// (rather than passing the callbacks to the test as we do here)
		waitForCallbacks <- callbacks
		return DomainImplementation(funcs)
	})
	/************************************************/

	// The rest is mocking the other side of the interface
	inOutMap := map[string]func(*prototk.DomainMessage){}
	pluginID := uuid.NewString()
	exerciser := newPluginExerciser(t, pluginID, &domainPlugin{}, inOutMap)
	tc.fakeDomainController = exerciser.controller

	domainDone := make(chan struct{})
	go func() {
		defer close(domainDone)
		domain.Run(pluginID, "unix:"+tc.socketFile)
	}()
	callbacks := <-waitForCallbacks

	return ctx, exerciser, funcs, callbacks, inOutMap, func() {
		checkPanic()
		domain.Stop()
		tcDone()
		<-domainDone
	}
}

func TestDomainCallback_FindAvailableStates(t *testing.T) {
	ctx, _, _, callbacks, inOutMap, done := setupDomainTests(t)
	defer done()

	inOutMap[fmt.Sprintf("%T", &prototk.DomainMessage_FindAvailableStates{})] = func(dm *prototk.DomainMessage) {
		dm.ResponseToDomain = &prototk.DomainMessage_FindAvailableStatesRes{
			FindAvailableStatesRes: &prototk.FindAvailableStatesResponse{},
		}
	}
	_, err := callbacks.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{})
	assert.NoError(t, err)
}

func TestDomainFunction_ConfigureDomain(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// ConfigureDomain - paladin to domain
	funcs.ConfigureDomain = func(ctx context.Context, cdr *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
		return &prototk.ConfigureDomainResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_ConfigureDomain{
			ConfigureDomain: &prototk.ConfigureDomainRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_ConfigureDomainRes{}, res.ResponseFromDomain)
	})
}

func TestDomainFunction_InitDomain(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// InitDomain - paladin to domain
	funcs.InitDomain = func(ctx context.Context, cdr *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
		return &prototk.InitDomainResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_InitDomain{
			InitDomain: &prototk.InitDomainRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_InitDomainRes{}, res.ResponseFromDomain)
	})
}

func TestDomainFunction_InitDeploy(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// InitDeploy - paladin to domain
	funcs.InitDeploy = func(ctx context.Context, cdr *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error) {
		return &prototk.InitDeployResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_InitDeploy{
			InitDeploy: &prototk.InitDeployRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_InitDeployRes{}, res.ResponseFromDomain)
	})
}

func TestDomainFunction_PrepareDeploy(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// PrepareDeploy - paladin to domain
	funcs.PrepareDeploy = func(ctx context.Context, cdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		return &prototk.PrepareDeployResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_PrepareDeploy{
			PrepareDeploy: &prototk.PrepareDeployRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_PrepareDeployRes{}, res.ResponseFromDomain)
	})
}

func TestDomainFunction_InitTransaction(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// InitTransaction - paladin to domain
	funcs.InitTransaction = func(ctx context.Context, cdr *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
		return &prototk.InitTransactionResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_InitTransaction{
			InitTransaction: &prototk.InitTransactionRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_InitTransactionRes{}, res.ResponseFromDomain)
	})
}

func TestDomainFunction_AssembleTransaction(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// AssembleTransaction - paladin to domain
	funcs.AssembleTransaction = func(ctx context.Context, cdr *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
		return &prototk.AssembleTransactionResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_AssembleTransaction{
			AssembleTransaction: &prototk.AssembleTransactionRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_AssembleTransactionRes{}, res.ResponseFromDomain)
	})
}

func TestDomainFunction_EndorseTransaction(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// EndorseTransaction - paladin to domain
	funcs.EndorseTransaction = func(ctx context.Context, cdr *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
		return &prototk.EndorseTransactionResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_EndorseTransaction{
			EndorseTransaction: &prototk.EndorseTransactionRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_EndorseTransactionRes{}, res.ResponseFromDomain)
	})
}

func TestDomainFunction_PrepareTransaction(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// PrepareTransaction - paladin to domain
	funcs.PrepareTransaction = func(ctx context.Context, cdr *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
		return &prototk.PrepareTransactionResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_PrepareTransaction{
			PrepareTransaction: &prototk.PrepareTransactionRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_PrepareTransactionRes{}, res.ResponseFromDomain)
	})
}
