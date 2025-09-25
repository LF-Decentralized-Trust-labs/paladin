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

/*
Test Kata component with no mocking of any internal units.
Starts the GRPC server and drives the internal functions via GRPC messages
*/
package coordinationtest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	testutils "github.com/LF-Decentralized-Trust-labs/paladin/core/noderuntests/pkg"
	"github.com/hyperledger/firefly-signer/pkg/abi"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/solutils"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var CONFIG_PATH = "../../test/config/postgres.config.yaml"

func deployDomainRegistry(t *testing.T) *pldtypes.EthAddress {
	return testutils.DeployDomainRegistry(t, CONFIG_PATH)
}

func newInstanceForComponentTesting(t *testing.T, deployDomainRegistry func(*testing.T) *pldtypes.EthAddress, binding interface{}, peerNodes []interface{}, domainConfig interface{}, enableWS bool) testutils.ComponentTestInstance {
	return testutils.NewInstanceForComponentTesting(t, deployDomainRegistry(t), binding, peerNodes, domainConfig, enableWS, CONFIG_PATH)
}

func subscribeAndSendDataToChannel(ctx context.Context, t *testing.T, wsClient pldclient.PaladinWSClient, listenerName string, data chan string) {
	sub, err := wsClient.PTX().SubscribeBlockchainEvents(ctx, listenerName)
	require.NoError(t, err)
	go func() {
		for {
			select {
			case subNotification, ok := <-sub.Notifications():
				if ok {
					eventData := make([]string, 0)
					var batch pldapi.TransactionEventBatch
					_ = json.Unmarshal(subNotification.GetResult(), &batch)
					for _, e := range batch.Events {
						t.Logf("Received event on %s from %d/%d/%d : %s", listenerName, e.BlockNumber, e.TransactionIndex, e.LogIndex, e.Data.String())
						eventData = append(eventData, e.Data.String())
					}
					require.NoError(t, subNotification.Ack(ctx))
					// send after the ack otherwise the main test can complete when it receives the last values and the websocket is closed before the ack
					// can be sent
					for _, d := range eventData {
						data <- d
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func TestRunSimpleStorageEthTransaction2(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	logrus.SetLevel(logrus.DebugLevel)

	instance := newInstanceForComponentTesting(t, deployDomainRegistry, nil, nil, nil, true)
	c := pldclient.Wrap(instance.GetClient()).ReceiptPollingInterval(250 * time.Millisecond)

	build, err := solutils.LoadBuild(ctx, simpleStorageBuildJSON)
	require.NoError(t, err)

	simpleStorage := c.ForABI(ctx, build.ABI).Public().From("key1")

	res := simpleStorage.Clone().
		Constructor().
		Bytecode(build.Bytecode).
		Inputs(`{"x":11223344}`).
		Send().Wait(5 * time.Second)
	require.NoError(t, res.Error())
	contractAddr := res.Receipt().ContractAddress

	// set up the event listener
	success, err := c.PTX().CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name: "listener1",
		Sources: []pldapi.BlockchainEventListenerSource{{
			ABI:     abi.ABI{build.ABI.Events()["Changed"]},
			Address: contractAddr,
		}},
	})
	require.NoError(t, err)
	require.True(t, success)

	wsClient, err := c.WebSocket(ctx, instance.GetWSConfig())
	require.NoError(t, err)

	eventData := make(chan string)
	subscribeAndSendDataToChannel(ctx, t, wsClient, "listener1", eventData)

	success, err = c.PTX().StartBlockchainEventListener(ctx, "listener1")
	require.NoError(t, err)
	require.True(t, success)

	data := <-eventData
	assert.JSONEq(t, `{"x":"11223344"}`, data)

	var getX pldtypes.RawJSON
	err = simpleStorage.Clone().
		Function("get").
		To(contractAddr).
		Outputs(&getX).
		Call()
	require.NoError(t, err)
	assert.JSONEq(t, `{"x":"11223344"}`, getX.Pretty())

	res = simpleStorage.Clone().
		Function("set").
		To(contractAddr).
		Inputs(`{"_x":99887766}`).
		Send().Wait(5 * time.Second)
	require.NoError(t, res.Error())

	data = <-eventData
	assert.JSONEq(t, `{"x":"99887766"}`, data)

	res = simpleStorage.Clone().
		Function("set").
		To(contractAddr).
		Inputs(`{"_x":1234}`).
		Send().Wait(5 * time.Second)
	require.NoError(t, res.Error())

	data = <-eventData
	assert.JSONEq(t, `{"x":"1234"}`, data)
}
