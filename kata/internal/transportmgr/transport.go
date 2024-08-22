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

package transportmgr

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/retry"
	"gopkg.in/yaml.v3"
)

type transport struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	conf          *TransportConfig
	tm            *transportManager
	id            uuid.UUID
	name          string
	api           plugins.TransportManagerToTransport

	stateLock              sync.Mutex
	initialized            atomic.Bool
	initRetry              *retry.Retry
	config                 *prototk.TransportConfig

	initError atomic.Pointer[error]
	initDone  chan struct{}
}

func (tm *transportManager) newTransport(id uuid.UUID, name string, conf *TransportConfig, toTransport plugins.TransportManagerToTransport) *transport {
	t := &transport{
		tm:            tm,
		conf:          conf,
		initRetry:     retry.NewRetryIndefinite(&conf.Init.Retry),
		name:          name,
		id:            id,
		api:           toTransport,
		initDone:      make(chan struct{}),
	}
	t.ctx, t.cancelCtx = context.WithCancel(log.WithLogField(tm.bgCtx, "domain", t.name))
	return t
}

func (t *transport) init() {
	defer close(t.initDone)

	// We block retrying each part of init until we succeed, or are cancelled
	// (which the plugin manager will do if the transport disconnects)
	err := t.initRetry.Do(t.ctx, func(attempt int) (bool, error) {
		// Send the configuration to the transport for processing
		confYAML, _ := yaml.Marshal(&t.conf.Config)
		_, err := t.api.ConfigureTransport(t.ctx, &prototk.ConfigureTransportRequest{
			Name:       t.name,
			ConfigYaml: string(confYAML),
		})
		if err != nil {
			return true, err
		}

		// Complete the initialization
		_, err = t.api.InitTransport(t.ctx, &prototk.InitTransportRequest{})
		return true, err
	})
	if err != nil {
		log.L(t.ctx).Debugf("transport initialization cancelled before completion: %s", err)
		t.initError.Store(&err)
	} else {
		log.L(t.ctx).Debugf("transport initialization complete")
		t.initialized.Store(true)
		// Inform the plugin manager callback
		t.api.Initialized()
	}
}

func (t *transport) checkInit(ctx context.Context) error {
	if !t.initialized.Load() {
		return i18n.NewError(ctx, msgs.MsgDomainNotInitialized)
	}
	return nil
}

func (t *transport) ReceiveMessage(ctx context.Context, req *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
	if err := t.checkInit(ctx); err != nil {
		return nil, err
	}

	return &prototk.ReceiveMessageResponse{}, nil
}

func (t *transport) close() {
	t.cancelCtx()
	<-t.initDone
}
