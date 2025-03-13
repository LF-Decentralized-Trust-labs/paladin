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
		return &DomainAPIBase{funcs}
	})
	/************************************************/

	// The rest is mocking the other side of the interface
	inOutMap := map[string]func(*prototk.DomainMessage){}
	pluginID := uuid.NewString()
	exerciser := newPluginExerciser(t, pluginID, &DomainMessageWrapper{}, inOutMap)
	tc.fakeDomainController = exerciser.controller

	domainDone := make(chan struct{})
	go func() {
		defer close(domainDone)
		domain.Run("unix:"+tc.socketFile, pluginID)
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
	require.NoError(t, err)
}

func TestDomainCallback_EncodeData(t *testing.T) {
	ctx, _, _, callbacks, inOutMap, done := setupDomainTests(t)
	defer done()

	inOutMap[fmt.Sprintf("%T", &prototk.DomainMessage_EncodeData{})] = func(dm *prototk.DomainMessage) {
		dm.ResponseToDomain = &prototk.DomainMessage_EncodeDataRes{
			EncodeDataRes: &prototk.EncodeDataResponse{},
		}
	}
	_, err := callbacks.EncodeData(ctx, &prototk.EncodeDataRequest{})
	require.NoError(t, err)
}

func TestDomainCallback_DecodeData(t *testing.T) {
	ctx, _, _, callbacks, inOutMap, done := setupDomainTests(t)
	defer done()

	inOutMap[fmt.Sprintf("%T", &prototk.DomainMessage_DecodeData{})] = func(dm *prototk.DomainMessage) {
		dm.ResponseToDomain = &prototk.DomainMessage_DecodeDataRes{
			DecodeDataRes: &prototk.DecodeDataResponse{},
		}
	}
	_, err := callbacks.DecodeData(ctx, &prototk.DecodeDataRequest{})
	require.NoError(t, err)
}

func TestDomainCallback_RecoverSigner(t *testing.T) {
	ctx, _, _, callbacks, inOutMap, done := setupDomainTests(t)
	defer done()

	inOutMap[fmt.Sprintf("%T", &prototk.DomainMessage_RecoverSigner{})] = func(dm *prototk.DomainMessage) {
		dm.ResponseToDomain = &prototk.DomainMessage_RecoverSignerRes{
			RecoverSignerRes: &prototk.RecoverSignerResponse{},
		}
	}
	_, err := callbacks.RecoverSigner(ctx, &prototk.RecoverSignerRequest{})
	require.NoError(t, err)
}

func TestDomainCallback_SendTransaction(t *testing.T) {
	ctx, _, _, callbacks, inOutMap, done := setupDomainTests(t)
	defer done()

	inOutMap[fmt.Sprintf("%T", &prototk.DomainMessage_SendTransaction{})] = func(dm *prototk.DomainMessage) {
		dm.ResponseToDomain = &prototk.DomainMessage_SendTransactionRes{
			SendTransactionRes: &prototk.SendTransactionResponse{},
		}
	}
	_, err := callbacks.SendTransaction(ctx, &prototk.SendTransactionRequest{})
	require.NoError(t, err)
}

func TestDomainCallback_LocalNodeName(t *testing.T) {
	ctx, _, _, callbacks, inOutMap, done := setupDomainTests(t)
	defer done()

	inOutMap[fmt.Sprintf("%T", &prototk.DomainMessage_LocalNodeName{})] = func(dm *prototk.DomainMessage) {
		dm.ResponseToDomain = &prototk.DomainMessage_LocalNodeNameRes{
			LocalNodeNameRes: &prototk.LocalNodeNameResponse{},
		}
	}
	_, err := callbacks.LocalNodeName(ctx, &prototk.LocalNodeNameRequest{})
	require.NoError(t, err)
}

func TestDomainCallback_GetStates(t *testing.T) {
	ctx, _, _, callbacks, inOutMap, done := setupDomainTests(t)
	defer done()

	inOutMap[fmt.Sprintf("%T", &prototk.DomainMessage_GetStatesById{})] = func(dm *prototk.DomainMessage) {
		dm.ResponseToDomain = &prototk.DomainMessage_GetStatesByIdRes{
			GetStatesByIdRes: &prototk.GetStatesByIDResponse{},
		}
	}
	_, err := callbacks.GetStatesByID(ctx, &prototk.GetStatesByIDRequest{})
	require.NoError(t, err)
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

func TestDomainFunction_InitContract(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// InitContract - paladin to domain
	funcs.InitContract = func(ctx context.Context, cdr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
		return &prototk.InitContractResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_InitContract{
			InitContract: &prototk.InitContractRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_InitContractRes{}, res.ResponseFromDomain)
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

func TestDomainFunction_HandleEventBatch(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// HandleEventBatch - paladin to domain
	funcs.HandleEventBatch = func(ctx context.Context, cdr *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_HandleEventBatch{
			HandleEventBatch: &prototk.HandleEventBatchRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_HandleEventBatchRes{}, res.ResponseFromDomain)
	})
}

func TestDomainFunction_Sign(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// Sign - paladin to domain
	funcs.Sign = func(ctx context.Context, cdr *prototk.SignRequest) (*prototk.SignResponse, error) {
		return &prototk.SignResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_Sign{
			Sign: &prototk.SignRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_SignRes{}, res.ResponseFromDomain)
	})
}

func TestDomainFunction_GetVerifier(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// GetVerifier - paladin to domain
	funcs.GetVerifier = func(ctx context.Context, cdr *prototk.GetVerifierRequest) (*prototk.GetVerifierResponse, error) {
		return &prototk.GetVerifierResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_GetVerifier{
			GetVerifier: &prototk.GetVerifierRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_GetVerifierRes{}, res.ResponseFromDomain)
	})
}

func TestDomainFunction_ValidateStateHashes(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// ValidateStateHashes - paladin to domain
	funcs.ValidateStateHashes = func(ctx context.Context, cdr *prototk.ValidateStateHashesRequest) (*prototk.ValidateStateHashesResponse, error) {
		return &prototk.ValidateStateHashesResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_ValidateStateHashes{
			ValidateStateHashes: &prototk.ValidateStateHashesRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_ValidateStateHashesRes{}, res.ResponseFromDomain)
	})
}

func TestDomainFunction_InitCall(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// InitCall - paladin to domain
	funcs.InitCall = func(ctx context.Context, cdr *prototk.InitCallRequest) (*prototk.InitCallResponse, error) {
		return &prototk.InitCallResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_InitCall{
			InitCall: &prototk.InitCallRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_InitCallRes{}, res.ResponseFromDomain)
	})
}

func TestDomainFunction_ExecCall(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// ExecCall - paladin to domain
	funcs.ExecCall = func(ctx context.Context, cdr *prototk.ExecCallRequest) (*prototk.ExecCallResponse, error) {
		return &prototk.ExecCallResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_ExecCall{
			ExecCall: &prototk.ExecCallRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_ExecCallRes{}, res.ResponseFromDomain)
	})
}

func TestDomainFunction_BuildReceipt(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// BuildReceipt - paladin to domain
	funcs.BuildReceipt = func(ctx context.Context, cdr *prototk.BuildReceiptRequest) (*prototk.BuildReceiptResponse, error) {
		return &prototk.BuildReceiptResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_BuildReceipt{
			BuildReceipt: &prototk.BuildReceiptRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_BuildReceiptRes{}, res.ResponseFromDomain)
	})
}

func TestDomainFunction_ConfigurePrivacyGroup(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// ConfigurePrivacyGroup - paladin to domain
	funcs.ConfigurePrivacyGroup = func(ctx context.Context, cdr *prototk.ConfigurePrivacyGroupRequest) (*prototk.ConfigurePrivacyGroupResponse, error) {
		return &prototk.ConfigurePrivacyGroupResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_ConfigurePrivacyGroup{
			ConfigurePrivacyGroup: &prototk.ConfigurePrivacyGroupRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_ConfigurePrivacyGroupRes{}, res.ResponseFromDomain)
	})
}

func TestDomainFunction_InitPrivacyGroup(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// InitPrivacyGroup - paladin to domain
	funcs.InitPrivacyGroup = func(ctx context.Context, cdr *prototk.InitPrivacyGroupRequest) (*prototk.InitPrivacyGroupResponse, error) {
		return &prototk.InitPrivacyGroupResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_InitPrivacyGroup{
			InitPrivacyGroup: &prototk.InitPrivacyGroupRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_InitPrivacyGroupRes{}, res.ResponseFromDomain)
	})
}

func TestDomainFunction_WrapPrivacyGroupEVMTX(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// WrapPrivacyGroupEVMTX - paladin to domain
	funcs.WrapPrivacyGroupEVMTX = func(ctx context.Context, cdr *prototk.WrapPrivacyGroupEVMTXRequest) (*prototk.WrapPrivacyGroupEVMTXResponse, error) {
		return &prototk.WrapPrivacyGroupEVMTXResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_WrapPrivacyGroupEvmtx{
			WrapPrivacyGroupEvmtx: &prototk.WrapPrivacyGroupEVMTXRequest{},
		}
	}, func(res *prototk.DomainMessage) {
		assert.IsType(t, &prototk.DomainMessage_WrapPrivacyGroupEvmtxRes{}, res.ResponseFromDomain)
	})
}

func TestDomainRequestError(t *testing.T) {
	_, exerciser, _, _, _, done := setupDomainTests(t)
	defer done()

	// Check responseToPluginAs handles nil
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {}, func(res *prototk.DomainMessage) {
		assert.Regexp(t, "PD020300", *res.Header.ErrorMessage)
	})
}
