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

package sequencer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func NewEventLoopForUnitTesting(t *testing.T) *eventLoop {
	//TODO mocks for CSA and transport manager
	return NewEventLoop(nil, nil).(*eventLoop)
}

type fakeContractSequencerHeartbeatTicker struct {
	heartbeatFailureThreshold int
	csa                       ContractSequencerAgent
	channel                   chan struct{}
}

func (f *fakeContractSequencerHeartbeatTicker) C() <-chan struct{} {
	return f.channel
}
func TestEventLoop_StartFailNilTransportManager(t *testing.T) {
	el := NewEventLoopForUnitTesting(t)
	fakeContractSequencerHeartbeatTicker := &fakeContractSequencerHeartbeatTicker{
		//TODO csa:     csa,
		channel: make(chan struct{}),
	}
	err := el.Start(context.Background(), fakeContractSequencerHeartbeatTicker, nil, nil)
	assert.Error(t, err)
}

func TestEventLoop_StartFailNilHeartbeatTicker(t *testing.T) {
	csa := NewEventLoopForUnitTesting(t)
	outboundMessageMonitor := &fakeTransportManager{}

	err := csa.Start(context.Background(), nil, outboundMessageMonitor, nil)
	assert.Error(t, err)
}
