// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package talaria_test

import (
	"context"
	"testing"
	"time"

	frm "github.com/kaleido-io/talaria/mocks/registrymocks"
	"github.com/kaleido-io/talaria/pkg/talaria"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestTalariaNonBlocking(t *testing.T) {
	// Throw slightly random combinations of messages at Talaria to show that it's not
	// blocking when it's processing messages

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rp := frm.NewRegistryProvider(t)
	rp.On("LookupPaladinEntity", mock.Anything).Return(talaria.RegistryEntry{
		RoutingInformation: "{\"address\":\"localhost:8080\"}",
		TransactingEntity: "someone-on-this-machine",
	}, nil)

	tal := talaria.NewTalaria(rp, 8080)
	tal.Initialise(ctx)
	assert.NotNil(t, tal)

	collectedMessages := make(chan string, 100)
	go func(){
		recvMessages := tal.GetMessages()

	// Collect all the messages from Talaria as quickly as we can
		for {
			select {
			case <- ctx.Done():
				return
			case msg := <-recvMessages:
				collectedMessages <- msg
			}
		}
	}()

	// Publish batches of 10 messages and then wait before sending the next batch
	for i := 0; i < 10; i += 1 {
		for j := 0; j < 10; j += 1 {
			err := tal.SendMessage(ctx, "someone-on-this-machine", "Hello, World!")
			assert.Nil(t, err)
		}
		time.Sleep(300 * time.Millisecond)
	}

	assert.Equal(t, 100, len(collectedMessages))
}

func TestTalariaMessageFlow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	rp := frm.NewRegistryProvider(t)
	rp.On("LookupPaladinEntity", mock.Anything).Return(talaria.RegistryEntry{
		RoutingInformation: "{\"address\":\"localhost:8080\"}",
		TransactingEntity: "someone-on-this-machine",
	}, nil)

	tal := talaria.NewTalaria(rp, 8080)
	tal.Initialise(ctx)
	assert.NotNil(t, tal)

	err := tal.SendMessage(ctx, "someone-on-this-machine", "Hello, World! 1")
	assert.Nil(t, err)
	err = tal.SendMessage(ctx, "someone-on-this-machine", "Hello, World! 2")
	assert.Nil(t, err)
	err = tal.SendMessage(ctx, "someone-on-this-machine", "Hello, World! 3")
	assert.Nil(t, err)

	recvMessages := tal.GetMessages()
	for i := 0; i < 3; i++ {
		message := <- recvMessages
		assert.Contains(t, message, "Hello, World!")
	}
}

func TestInitNewTalaria(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	rp := frm.NewRegistryProvider(t)
	tal := talaria.NewTalaria(rp, 8080)
	tal.Initialise(ctx)
	assert.NotNil(t, tal)
}