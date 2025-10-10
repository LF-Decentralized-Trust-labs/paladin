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

package keymanager

import (
	"context"
	"encoding/json"
	"sync/atomic"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/retry"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signer"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
	"github.com/google/uuid"
)

// Plugin signing module
type signingModule struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	conf *pldconf.SigningModuleConfig
	km   *keyManager
	id   uuid.UUID
	name string
	api  components.KeyManagerToSigningModule

	initialized atomic.Bool
	initRetry   *retry.Retry

	initError atomic.Pointer[error]
	initDone  chan struct{}
}

func (km *keyManager) newSigningModule(id uuid.UUID, name string, conf *pldconf.SigningModuleConfig, toSigningModule components.KeyManagerToSigningModule) signer.SigningModule {
	sm := &signingModule{
		km:        km,
		conf:      conf,
		initRetry: retry.NewRetryIndefinite(&conf.Init.Retry),
		name:      name,
		id:        id,
		api:       toSigningModule,
		initDone:  make(chan struct{}),
	}
	sm.ctx, sm.cancelCtx = context.WithCancel(log.WithLogField(km.bgCtx, "signingModule", sm.name))
	return sm
}

func (sm *signingModule) init() {
	defer close(sm.initDone)

	// We block retrying each part of init until we succeed, or are cancelled
	// (which the plugin manager will do if the signing module disconnects)
	err := sm.initRetry.Do(sm.ctx, func(attempt int) (bool, error) {
		// Send the configuration to the signing module for processing
		confJSON, _ := json.Marshal(&sm.conf.Config)
		_, err := sm.api.ConfigureSigningModule(sm.ctx, &prototk.ConfigureSigningModuleRequest{
			Name:       sm.name,
			ConfigJson: string(confJSON),
		})
		return true, err
	})
	if err != nil {
		log.L(sm.ctx).Debugf("signing module initialization cancelled before completion: %s", err)
		sm.initError.Store(&err)
	} else {
		log.L(sm.ctx).Debugf("signing module initialization complete %s", sm.name)
		sm.initialized.Store(true)
		// Inform the plugin manager callback
		sm.api.Initialized()
	}
}

func (sm *signingModule) Resolve(ctx context.Context, req *prototk.ResolveKeyRequest) (res *prototk.ResolveKeyResponse, err error) {
	return sm.api.ResolveKey(ctx, req)
}

func (sm *signingModule) Sign(ctx context.Context, req *prototk.SignWithKeyRequest) (res *prototk.SignWithKeyResponse, err error) {
	return sm.api.Sign(ctx, req)
}

func (sm *signingModule) List(ctx context.Context, req *prototk.ListKeysRequest) (res *prototk.ListKeysResponse, err error) {
	return sm.api.ListKeys(ctx, req)
}

func (sm *signingModule) AddInMemorySigner(prefix string, signer signerapi.InMemorySigner) {
	log.L(sm.ctx).Warnf("Adding in-memory signer is not supported for signing module plugin: %s", sm.name)
}

func (sm *signingModule) Close() {
	_, _ = sm.api.Close(sm.ctx, &prototk.CloseRequest{})
}

func (sm *signingModule) close() {
	sm.cancelCtx()
	<-sm.initDone
}
