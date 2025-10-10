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

package examplesigningmodule

import (
	"context"
	"encoding/json"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signer"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
	"github.com/kaleido-io/key-manager/signingmodules/example/internal/msgs"
)

type exampleSigningModule struct {
	bgCtx         context.Context
	callbacks     plugintk.SigningModuleCallbacks
	conf          *ExampleSigningModuleConfig
	name          string
	signingModule signer.SigningModule
}

func NewPlugin(ctx context.Context) plugintk.PluginBase {
	return plugintk.NewSigningModule(NewKeyManagerSigningModule)
}

func NewKeyManagerSigningModule(callbacks plugintk.SigningModuleCallbacks) plugintk.SigningModuleAPI {
	return &exampleSigningModule{
		bgCtx:     context.Background(),
		callbacks: callbacks,
	}
}

func (kmsm *exampleSigningModule) ConfigureSigningModule(ctx context.Context, req *prototk.ConfigureSigningModuleRequest) (*prototk.ConfigureSigningModuleResponse, error) {
	kmsm.name = req.Name

	// Extract the config
	err := json.Unmarshal([]byte(req.ConfigJson), &kmsm.conf)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgInvalidSigningModuleConfig)
	}

	if kmsm.conf.Signer == nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgInvalidSigningModuleConfig)
	}

	// Setup a new signing module using the provided configuration
	kmsm.signingModule, err = signer.NewSigningModule(ctx, (*signerapi.ConfigNoExt)(kmsm.conf.Signer))
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgInvalidSigningModuleConfig) // TODO better error
	}

	return &prototk.ConfigureSigningModuleResponse{}, nil
}
func (kmsm *exampleSigningModule) ResolveKey(ctx context.Context, req *prototk.ResolveKeyRequest) (*prototk.ResolveKeyResponse, error) {
	return kmsm.signingModule.Resolve(ctx, req)
}

func (kmsm *exampleSigningModule) Sign(ctx context.Context, req *prototk.SignWithKeyRequest) (*prototk.SignWithKeyResponse, error) {
	return kmsm.signingModule.Sign(ctx, req)
}

func (kmsm *exampleSigningModule) ListKeys(ctx context.Context, req *prototk.ListKeysRequest) (*prototk.ListKeysResponse, error) {
	return kmsm.signingModule.List(ctx, req)
}

func (kmsm *exampleSigningModule) Close(ctx context.Context, req *prototk.CloseRequest) (*prototk.CloseResponse, error) {
	kmsm.signingModule.Close()

	return &prototk.CloseResponse{}, nil
}
