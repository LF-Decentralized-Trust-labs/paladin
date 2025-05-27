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

package txmgr

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var mockABI = abi.ABI{{
	Name: "event1",
}}

var mockAddress = pldtypes.RandAddress()

func TestLoadBlockchainEventListeners(t *testing.T) {
	var blockIndexer *componentmocks.BlockIndexer
	_, txm, done := newTestTransactionManager(t, true, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.blockIndexer.On("StopEventStream", mock.Anything, mock.Anything).Return(nil)
		blockIndexer = mc.blockIndexer
	})
	defer done()

	txm.blockchainEventsInit()

	// no listeners to load
	blockIndexer.On("QueryEventStreamDefinitions", mock.Anything, mock.Anything, blockindexer.EventStreamTypePTXBlockchainEventListener.Enum(), mock.Anything).
		Return([]*blockindexer.EventStream{}, nil).Once()
	err := txm.LoadBlockchainEventListeners()
	assert.NoError(t, err)

	// error querying definitions
	blockIndexer.On("QueryEventStreamDefinitions", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("pop")).Once()
	err = txm.LoadBlockchainEventListeners()
	assert.ErrorContains(t, err, "pop")

	// error loading listener
	txm.blockchainEventListeners["dupName"] = &blockchainEventListener{
		definition: &blockindexer.EventStream{
			ID: uuid.New(),
		},
	}
	blockIndexer.On("QueryEventStreamDefinitions", mock.Anything, mock.Anything,
		blockindexer.EventStreamTypePTXBlockchainEventListener.Enum(), mock.Anything).
		Return([]*blockindexer.EventStream{{
			Name: "dupName",
		}}, nil).Once()
	err = txm.LoadBlockchainEventListeners()
	assert.ErrorContains(t, err, "PD012247")

	// success- multiple pages
	txm.blockchainEventListenersLoadPageSize = 2 // lower so testing pagination is more feasible
	mockQuery1 := blockIndexer.On("QueryEventStreamDefinitions", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*blockindexer.EventStream{{
		Name: "bel1",
	}, {
		Name: "bel2",
	}}, nil).Once()
	mockQuery2 := blockIndexer.On("QueryEventStreamDefinitions", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*blockindexer.EventStream{{
		Name: "bel3",
	}}, nil).Once()
	blockIndexer.On("AddEventStream", mock.Anything, mock.Anything, mock.Anything).Return(&blockindexer.EventStream{
		ID: uuid.New(),
	}, nil).Times(3)

	mockQuery1.Run(func(args mock.Arguments) {
		eventStreamType := args.Get(2).(pldtypes.Enum[blockindexer.EventStreamType])
		assert.Equal(t, blockindexer.EventStreamTypePTXBlockchainEventListener.Enum(), eventStreamType)
		q := args.Get(3).(*query.QueryJSON)
		assert.Equal(t, 2, *q.Limit)
		assert.Equal(t, "name", q.Sort[0])
		assert.Len(t, q.GreaterThan, 0)
	})
	mockQuery2.Run(func(args mock.Arguments) {
		eventStreamType := args.Get(2).(pldtypes.Enum[blockindexer.EventStreamType])
		assert.Equal(t, blockindexer.EventStreamTypePTXBlockchainEventListener.Enum(), eventStreamType)
		q := args.Get(3).(*query.QueryJSON)
		assert.Equal(t, 2, *q.Limit)
		assert.Equal(t, "name", q.Sort[0])
		if assert.Len(t, q.GT, 1) {
			assert.Equal(t, "name", q.GT[0].Field)
			assert.Equal(t, pldtypes.JSONString("bel2"), q.GT[0].Value)
		}
	})

	err = txm.LoadBlockchainEventListeners()
	assert.NoError(t, err)
}

func TestStopBlockchainEventListeners(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()

	_, txm, done := newTestTransactionManager(t, true, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.blockIndexer.On("StopEventStream", mock.Anything, id1).Return(errors.New("pop"))
		mc.blockIndexer.On("StopEventStream", mock.Anything, id2).Return(nil)
	})
	defer done()

	txm.blockchainEventListeners["bel1"] = &blockchainEventListener{
		definition: &blockindexer.EventStream{
			ID: id1,
		},
	}

	txm.blockchainEventListeners["bel2"] = &blockchainEventListener{
		definition: &blockindexer.EventStream{
			ID: id2,
		},
	}

	txm.stopBlockchainEventListeners()
}

func TestCreateBlockchainEventListener(t *testing.T) {
	var blockIndexer *componentmocks.BlockIndexer
	ctx, txm, done := newTestTransactionManager(t, true, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		blockIndexer = mc.blockIndexer
		blockIndexer.On("StopEventStream", mock.Anything, mock.Anything).Return(nil)
	})
	defer done()

	// duplicate name
	txm.blockchainEventListeners["dupName"] = &blockchainEventListener{
		definition: &blockindexer.EventStream{
			ID: uuid.New(),
		},
	}

	err := txm.CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name: "dupName",
	})
	assert.ErrorContains(t, err, "PD012246")

	// invalid name
	err = txm.CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name: "--name",
	})
	assert.ErrorContains(t, err, "PD020005")

	// no sources configured
	err = txm.CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name: "bel1",
	})
	assert.ErrorContains(t, err, "PD012251")

	err = txm.CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name:    "bel1",
		Sources: []pldapi.BlockchainEventListenerSource{},
	})
	assert.ErrorContains(t, err, "PD012251")

	err = txm.CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name: "bel1",
		Sources: []pldapi.BlockchainEventListenerSource{{
			Address: mockAddress,
		}},
	})
	assert.ErrorContains(t, err, "PD012252")

	// invalid timeout
	err = txm.CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name: "bel1",
		Sources: []pldapi.BlockchainEventListenerSource{{
			ABI: mockABI,
		}},
		Options: pldapi.BlockchainEventListenerOptions{
			BatchTimeout: confutil.P("1x"),
		},
	})
	assert.ErrorContains(t, err, "PD012250")

	// error creating event stream
	blockIndexer.On("AddEventStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("pop")).Once()
	err = txm.CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name: "bel1",
		Sources: []pldapi.BlockchainEventListenerSource{{
			ABI: mockABI,
		}},
		Options: pldapi.BlockchainEventListenerOptions{
			BatchTimeout: confutil.P("1m"),
		},
	})
	assert.ErrorContains(t, err, "pop")
	assert.NotContains(t, txm.blockchainEventListeners, "bel1")

	// success
	mockAdd := blockIndexer.On("AddEventStream", mock.Anything, mock.Anything, mock.Anything).Return(&blockindexer.EventStream{}, nil).Once()
	mockAdd.Run(func(args mock.Arguments) {
		def := args.Get(2).(*blockindexer.InternalEventStream).Definition
		assert.Equal(t, "bel1", def.Name)
		assert.Equal(t, blockindexer.EventStreamTypePTXBlockchainEventListener.Enum(), def.Type)
		assert.Equal(t, "1m", *def.Config.BatchTimeout)
		assert.Equal(t, json.RawMessage(`4`), def.Config.FromBlock)
		assert.Equal(t, mockABI, def.Sources[0].ABI)
		assert.Equal(t, mockAddress, def.Sources[0].Address)
	})
	err = txm.CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name: "bel1",
		Options: pldapi.BlockchainEventListenerOptions{
			BatchTimeout: confutil.P("1m"),
			FromBlock:    json.RawMessage(`4`),
		},
		Sources: []pldapi.BlockchainEventListenerSource{{
			ABI:     mockABI,
			Address: mockAddress,
		}},
	})
	assert.NoError(t, err)
	assert.Contains(t, txm.blockchainEventListeners, "bel1")
}

func TestQueryBlockchainEventListeners(t *testing.T) {
	var blockIndexer *componentmocks.BlockIndexer
	ctx, txm, done := newTestTransactionManager(t, true, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		blockIndexer = mc.blockIndexer
	})
	defer done()

	q := query.NewQueryBuilder().Equal("name", "bel1").Query()

	// error querying definitions
	blockIndexer.On("QueryEventStreamDefinitions", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("pop")).Once()
	_, err := txm.QueryBlockchainEventListeners(ctx, txm.p.NOTX(), q)
	assert.ErrorContains(t, err, "pop")

	// success
	mockQuery := blockIndexer.On("QueryEventStreamDefinitions", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]*blockindexer.EventStream{{
			Name:    "bel1",
			Started: confutil.P(true),
			Config: blockindexer.EventStreamConfig{
				BatchTimeout: confutil.P("1m"),
				BatchSize:    confutil.P(100),
				FromBlock:    json.RawMessage(`"latest"`),
			},
			Sources: blockindexer.EventSources{{
				ABI:     mockABI,
				Address: mockAddress,
			}},
		}}, nil).Once()
	mockQuery.Run(func(args mock.Arguments) {
		q := args.Get(3).(*query.QueryJSON)
		assert.Equal(t, "name", q.Eq[0].Field)
		assert.Equal(t, pldtypes.JSONString("bel1"), q.Eq[0].Value)
	})
	listeners, err := txm.QueryBlockchainEventListeners(ctx, txm.p.NOTX(), q)
	require.NoError(t, err)
	require.Len(t, listeners, 1)
	assert.Equal(t, "bel1", listeners[0].Name)
	assert.True(t, *listeners[0].Started)
	assert.Equal(t, "1m", *listeners[0].Options.BatchTimeout)
	assert.Equal(t, 100, *listeners[0].Options.BatchSize)
	assert.Equal(t, "\"latest\"", string(listeners[0].Options.FromBlock))
	assert.Equal(t, mockABI, listeners[0].Sources[0].ABI)
	assert.Equal(t, mockAddress, listeners[0].Sources[0].Address)

}

func TestGetBlockchainEventListener(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, true, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.blockIndexer.On("StopEventStream", mock.Anything, mock.Anything).Return(nil)
	})
	defer done()

	// returns nil if not found
	listener := txm.GetBlockchainEventListener(ctx, "bel1")
	assert.Nil(t, listener)

	// returns the listener
	txm.blockchainEventListeners["bel1"] = &blockchainEventListener{
		definition: &blockindexer.EventStream{
			Name: "bel1",
			ID:   uuid.New(),
		},
	}
	listener = txm.GetBlockchainEventListener(ctx, "bel1")
	require.NotNil(t, listener)
	assert.Equal(t, "bel1", listener.Name)
}

func TestStartBlockchainEventListener(t *testing.T) {
	id := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, true, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.blockIndexer.On("StartEventStream", mock.Anything, id).Return(nil)
		mc.blockIndexer.On("StopEventStream", mock.Anything, mock.Anything).Return(nil)
	})
	defer done()

	err := txm.StartBlockchainEventListener(ctx, "bel1")
	assert.ErrorContains(t, err, "PD012248")

	txm.blockchainEventListeners["bel1"] = &blockchainEventListener{
		definition: &blockindexer.EventStream{
			Name: "bel1",
			ID:   id,
		},
	}

	err = txm.StartBlockchainEventListener(ctx, "bel1")
	assert.NoError(t, err)
}

func TestStopBlockchainEventListener(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, true, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.blockIndexer.On("StopEventStream", mock.Anything, mock.Anything).Return(nil)
	})
	defer done()

	err := txm.StopBlockchainEventListener(ctx, "bel1")
	assert.ErrorContains(t, err, "PD012248")

	txm.blockchainEventListeners["bel1"] = &blockchainEventListener{
		definition: &blockindexer.EventStream{
			Name: "bel1",
			ID:   uuid.New(),
		},
	}

	err = txm.StopBlockchainEventListener(ctx, "bel1")
	assert.NoError(t, err)
}

func TestDeleteBlockchainEventListener(t *testing.T) {
	id := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, true, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.blockIndexer.On("RemoveEventStream", mock.Anything, id).Return(nil)
		mc.blockIndexer.On("StopEventStream", mock.Anything, mock.Anything).Return(nil).Maybe()
	})
	defer done()

	err := txm.DeleteBlockchainEventListener(ctx, "bel1")
	assert.ErrorContains(t, err, "PD012248")

	txm.blockchainEventListeners["bel1"] = &blockchainEventListener{
		definition: &blockindexer.EventStream{
			Name: "bel1",
			ID:   id,
		},
	}
	err = txm.DeleteBlockchainEventListener(ctx, "bel1")
	assert.NoError(t, err)
	assert.NotContains(t, txm.blockchainEventListeners, "bel1")
}

func TestGetBlockchainEventListenerStatus(t *testing.T) {
	id := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, true, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.blockIndexer.On("GetEventStreamStatus", mock.Anything, id).Return(nil, errors.New("pop")).Once()
		mc.blockIndexer.On("GetEventStreamStatus", mock.Anything, id).Return(&blockindexer.EventStreamStatus{
			Catchup:         false,
			CheckpointBlock: 25,
		}, nil).Once()
		mc.blockIndexer.On("StopEventStream", mock.Anything, mock.Anything).Return(nil)
	})
	defer done()

	_, err := txm.GetBlockchainEventListenerStatus(ctx, "bel1")
	assert.ErrorContains(t, err, "PD012248")

	txm.blockchainEventListeners["bel1"] = &blockchainEventListener{
		definition: &blockindexer.EventStream{
			Name: "bel1",
			ID:   id,
		},
	}

	_, err = txm.GetBlockchainEventListenerStatus(ctx, "bel1")
	require.ErrorContains(t, err, "pop")

	status, err := txm.GetBlockchainEventListenerStatus(ctx, "bel1")
	require.NoError(t, err)
	assert.Equal(t, int64(25), status.Checkpoint.BlockNumber)
	assert.False(t, status.Catchup)
}

type testBlockchainEventReceiver struct {
	index          int
	checkArguments func(batchID uuid.UUID, receipts []*pldapi.EventWithData)
}

func (r *testBlockchainEventReceiver) DeliverBlockchainEventBatch(ctx context.Context, batchID uuid.UUID, events []*pldapi.EventWithData) error {
	return nil
}

func TestAddRemoveBlockchainEventReceiver(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, true, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.blockIndexer.On("StopEventStream", mock.Anything, mock.Anything).Return(nil)
	})
	defer done()

	// listener not found
	_, err := txm.AddBlockchainEventReceiver(ctx, "bel1", &testBlockchainEventReceiver{})
	assert.ErrorContains(t, err, "PD012248")

	// success
	txm.blockchainEventListeners["bel1"] = &blockchainEventListener{
		definition: &blockindexer.EventStream{
			Name: "bel1",
		},
		newReceivers: make(chan bool, 1),
	}
	_, err = txm.AddBlockchainEventReceiver(ctx, "bel1", &testBlockchainEventReceiver{})
	assert.NoError(t, err)
	assert.Len(t, txm.blockchainEventListeners["bel1"].receivers, 1)

	receiver, err := txm.AddBlockchainEventReceiver(ctx, "bel1", &testBlockchainEventReceiver{})
	assert.NoError(t, err)
	assert.Len(t, txm.blockchainEventListeners["bel1"].receivers, 2)

	receiver.Close()
	assert.Len(t, txm.blockchainEventListeners["bel1"].receivers, 1)
}

func TestNextReceiver(t *testing.T) {
	// waiting for a receiver to be added
	nextReceiver := make(chan components.BlockchainEventReceiver, 1)
	el := &blockchainEventListener{
		newReceivers: make(chan bool, 1),
	}
	el.ctx, el.cancelCtx = context.WithCancel(context.Background())

	go func() {
		r, err := el.nextReceiver()
		require.NoError(t, err)
		nextReceiver <- r
	}()

	time.Sleep(100 * time.Millisecond)

	el.addReceiver(&testBlockchainEventReceiver{
		index: 0,
	})

	r1 := <-nextReceiver
	require.NotNil(t, r1)
	assert.Equal(t, 0, r1.(*registeredBlockchainEventReceiver).BlockchainEventReceiver.(*testBlockchainEventReceiver).index)

	// add another receiver
	el.addReceiver(&testBlockchainEventReceiver{
		index: 1,
	})
	r2, err := el.nextReceiver()
	require.NoError(t, err)
	assert.Equal(t, 1, r2.(*registeredBlockchainEventReceiver).BlockchainEventReceiver.(*testBlockchainEventReceiver).index)

	// getting the next receiver should go back to the first receiver
	r1, err = el.nextReceiver()
	require.NoError(t, err)
	assert.Equal(t, 0, r1.(*registeredBlockchainEventReceiver).BlockchainEventReceiver.(*testBlockchainEventReceiver).index)

	r1.(*registeredBlockchainEventReceiver).Close()
	r2.(*registeredBlockchainEventReceiver).Close()

	// closing the context should make nextReceiver return an error
	gotError := make(chan bool, 1)
	go func() {
		_, err := el.nextReceiver()
		require.Error(t, err)
		gotError <- true
	}()

	el.cancelCtx()
	<-gotError
}

func TestHandleEventBatch(t *testing.T) {
	el := &blockchainEventListener{
		newReceivers: make(chan bool, 1),
	}
	el.ctx, el.cancelCtx = context.WithCancel(context.Background())

	testBatchID := uuid.New()
	testEvents := []*pldapi.EventWithData{{
		IndexedEvent: &pldapi.IndexedEvent{
			BlockNumber:      1,
			TransactionIndex: 2,
			LogIndex:         3,
		},
	}}

	r := el.addReceiver(&testBlockchainEventReceiver{
		checkArguments: func(batchID uuid.UUID, receipts []*pldapi.EventWithData) {
			assert.Equal(t, testBatchID, batchID)
			assert.Equal(t, testEvents, receipts)
		},
	})

	err := el.handleEventBatch(context.Background(), &blockindexer.EventDeliveryBatch{
		BatchID: testBatchID,
		Events:  testEvents,
	})
	require.NoError(t, err)

	r.Close()

	// call again in a goroutine with no listeners and cancel
	gotError := make(chan bool, 1)
	go func() {
		err := el.handleEventBatch(context.Background(), nil)
		require.Error(t, err)
		gotError <- true
	}()
	el.cancelCtx()
	<-gotError

}
