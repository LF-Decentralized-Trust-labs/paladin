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
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestDomainCallbacks(t *testing.T) {
	ctx, tc, tcDone := newTestController(t)

	inOutMap := map[string]func(*prototk.DomainMessage){
		fmt.Sprintf("%T", &prototk.DomainMessage_FindAvailableStates{}): func(dm *prototk.DomainMessage) {
			dm.ResponseToDomain = &prototk.DomainMessage_FindAvailableStatesRes{
				FindAvailableStatesRes: &prototk.FindAvailableStatesResponse{},
			}
		},
	}
	pluginID := uuid.NewString()
	tc.fakeDomainController = func(stream grpc.BidiStreamingServer[prototk.DomainMessage, prototk.DomainMessage]) error {
		for {
			msg, err := stream.Recv()
			if err != nil {
				return err
			}
			assert.Equal(t, prototk.Header_REQUEST_FROM_PLUGIN, msg.Header.MessageType)
			assert.Equal(t, pluginID, msg.Header.PluginId)
			reply := &prototk.DomainMessage{
				Header: &prototk.Header{
					PluginId:      msg.Header.PluginId,
					MessageId:     uuid.NewString(),
					CorrelationId: &msg.Header.MessageId,
					MessageType:   prototk.Header_RESPONSE_TO_PLUGIN,
				},
			}
			reqType := fmt.Sprintf("%T", msg.RequestFromDomain)
			mapper := inOutMap[reqType]
			assert.NotNil(t, mapper, fmt.Sprintf("MISSING: %s", reqType))
			err = stream.Send(reply)
			assert.NoError(t, err)
		}
	}

	waitForCallbacks := make(chan DomainCallbacks, 1)
	domain := NewDomain(func(callbacks DomainCallbacks) DomainAPI {
		waitForCallbacks <- callbacks
		return DomainImplementation(DomainAPIFunctions{
			// No actual implementations - which is fine
		})
	})

	domainDone := make(chan struct{})
	go func() {
		defer close(domainDone)
		domain.Run(pluginID, "unix:"+tc.socketFile)
	}()
	defer func() {
		checkPanic()
		domain.Stop()
		tcDone()
		<-domainDone
	}()

	// By calling all the functions on the unimplemented domain and checking we get the correct
	// MsgPluginUnimplementedRequest response, we know all the type mapping works
	// (as those would cause other errors)
	callbacks := <-waitForCallbacks

	_, err := callbacks.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{})
	assert.Regexp(t, "erm", err)

}
