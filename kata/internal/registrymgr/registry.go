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

package registrymgr

import (
	"context"
	"encoding/json"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/retry"
	"gopkg.in/yaml.v3"
)

type registry struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	conf *RegistryConfig
	tm   *registryManager
	id   uuid.UUID
	name string
	api  plugins.RegistryManagerToRegistry

	initialized atomic.Bool
	initRetry   *retry.Retry

	initError atomic.Pointer[error]
	initDone  chan struct{}
}

func (tm *registryManager) newRegistry(id uuid.UUID, name string, conf *RegistryConfig, toRegistry plugins.RegistryManagerToRegistry) *registry {
	t := &registry{
		tm:        tm,
		conf:      conf,
		initRetry: retry.NewRetryIndefinite(&conf.Init.Retry),
		name:      name,
		id:        id,
		api:       toRegistry,
		initDone:  make(chan struct{}),
	}
	t.ctx, t.cancelCtx = context.WithCancel(log.WithLogField(tm.bgCtx, "registry", t.name))
	return t
}

func (t *registry) init() {
	defer close(t.initDone)

	// We block retrying each part of init until we succeed, or are cancelled
	// (which the plugin manager will do if the registry disconnects)
	err := t.initRetry.Do(t.ctx, func(attempt int) (bool, error) {
		// Send the configuration to the registry for processing
		confJSON, _ := json.Marshal(&t.conf.Config)
		_, err := t.api.ConfigureRegistry(t.ctx, &prototk.ConfigureRegistryRequest{
			Name:       t.name,
			ConfigJson: string(confJSON),
		})
		return true, err
	})
	if err != nil {
		log.L(t.ctx).Debugf("registry initialization cancelled before completion: %s", err)
		t.initError.Store(&err)
	} else {
		log.L(t.ctx).Debugf("registry initialization complete")
		t.initialized.Store(true)
		// Inform the plugin manager callback
		t.api.Initialized()
	}
}

func (t *registry) checkInit(ctx context.Context) error {
	if !t.initialized.Load() {
		return i18n.NewError(ctx, msgs.MsgDomainNotInitialized)
	}
	return nil
}

func (t *registry) Send(ctx context.Context, message *components.RegistryMessage) error {
	if err := t.checkInit(ctx); err != nil {
		return err
	}

	_, err := t.api.SendMessage(ctx, &prototk.SendMessageRequest{
		Node:    message.Destination.Node,
		Payload: message.Payload,
	})
	if err != nil {
		return err
	}

	return nil
}

// Registry callback to the registry manager when a message is received
func (t *registry) Receive(ctx context.Context, req *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
	if err := t.checkInit(ctx); err != nil {
		return nil, err
	}

	registryMessage := &components.RegistryMessage{}
	err := yaml.Unmarshal([]byte(req.Body), registryMessage)
	if err != nil {
		return nil, err
	}

	t.tm.receiveExternalMessage(*registryMessage)
	return &prototk.ReceiveMessageResponse{}, nil
}

func (t *registry) close() {
	t.cancelCtx()
	<-t.initDone
}
