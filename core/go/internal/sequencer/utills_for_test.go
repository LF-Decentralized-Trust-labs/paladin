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

/*
This file contains utility functions that are used by the sequencer spec tests.
*/

package sequencer

import (
	"context"
	"testing"

	"github.com/kaleido-io/paladin/core/internal/components"
	"gorm.io/gorm"
)

// TODO: TBD would it be better to replace this with mockery generated code?
type fakeTransportManager struct {
	t                         *testing.T
	sentFireAndForgetMessages []*components.FireAndForgetMessageSend
	sentReliableMessages      []*components.ReliableMessage
}

func newFakeTransportManager(t *testing.T) *fakeTransportManager {
	return &fakeTransportManager{
		t: t,
	}
}

func (f *fakeTransportManager) Send(ctx context.Context, send *components.FireAndForgetMessageSend) error {
	f.sentFireAndForgetMessages = append(f.sentFireAndForgetMessages, send)
	return nil
}

func (f *fakeTransportManager) SendReliable(ctx context.Context, dbTX *gorm.DB, msg ...*components.ReliableMessage) (preCommit func(), err error) {
	f.sentReliableMessages = append(f.sentReliableMessages, msg...)
	return func() {}, nil
}

func (f *fakeTransportManager) Sends(outboundMessageMatcher FireAndForgetMessageMatcher) bool {
	for _, message := range f.sentFireAndForgetMessages {
		if outboundMessageMatcher(message) {
			return true
		}
	}
	logString := "Sent messages: "
	for _, message := range f.sentFireAndForgetMessages {
		logString += message.MessageType + ", "
	}
	f.t.Log(logString)

	return false
}

func (f *fakeTransportManager) SendsReliable(outboundMessageMatcher ReliableMessageMatcher) bool {
	for _, message := range f.sentReliableMessages {
		if outboundMessageMatcher(message) {
			return true
		}
	}
	return false
}

func ptrTo[T any](v T) *T {
	return &v
}
