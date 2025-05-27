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
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/query"
)

var ES_TYPE = blockindexer.EventStreamTypePTXBlockchainEventListener.Enum()

type registeredBlockchainEventReceiver struct {
	id uuid.UUID
	el *blockchainEventListener
	components.BlockchainEventReceiver
}

// A blockchain event listener is a wrapper which externalises a blockchain internal event stream.
// All persistence belongs to the internal event stream.
// This layer manages
//   - mapping between "ptx_<verb>BlockchainEventListener" RPC calls and internal event stream
//     lifecycle functions
//   - dispatching event batches to registered receivers

type blockchainEventListener struct {
	tm *txManager

	ctx       context.Context
	cancelCtx context.CancelFunc

	definition *blockindexer.EventStream

	receiverLock    sync.Mutex
	receivers       []*registeredBlockchainEventReceiver
	newReceivers    chan bool
	receiverCounter int
}

func (tm *txManager) blockchainEventsInit() {
	tm.blockchainEventListeners = make(map[string]*blockchainEventListener)
	tm.blockchainEventListenersLoadPageSize = 100 /* not currently tunable */
}

func (tm *txManager) LoadBlockchainEventListeners() error {
	var lastPageEnd *string
	ctx := tm.bgCtx

	for {
		q := query.NewQueryBuilder().
			Limit(tm.blockchainEventListenersLoadPageSize).
			Sort("name")
		if lastPageEnd != nil {
			q = q.GreaterThan("name", *lastPageEnd)
		}
		page, err := tm.blockIndexer.QueryEventStreamDefinitions(ctx, tm.p.NOTX(), ES_TYPE, q.Query())
		if err != nil {
			return err
		}
		for _, listener := range page {
			if _, err := tm.loadBlockchainEventListener(ctx, listener, tm.p.NOTX()); err != nil {
				return err
			}
		}

		if len(page) < tm.blockchainEventListenersLoadPageSize {
			log.L(ctx).Infof("loaded %d event listeners", len(tm.blockchainEventListeners))
			return nil
		}

		lastPageEnd = &page[len(page)-1].Name
	}
}

func (tm *txManager) loadBlockchainEventListener(ctx context.Context, es *blockindexer.EventStream, dbTX persistence.DBTX) (*blockchainEventListener, error) {
	tm.blockchainEventListenerLock.Lock()
	defer tm.blockchainEventListenerLock.Unlock()

	if tm.blockchainEventListeners[es.Name] != nil {
		return nil, i18n.NewError(ctx, msgs.MsgTxMgrBlockchainEventListenerDupLoad, es.Name)
	}
	el := &blockchainEventListener{
		tm:           tm,
		newReceivers: make(chan bool, 1),
	}

	el.ctx, el.cancelCtx = context.WithCancel(log.WithLogField(el.tm.bgCtx, "blockchain-event-listener", es.Name))
	var err error
	el.definition, err = tm.blockIndexer.AddEventStream(ctx, dbTX, &blockindexer.InternalEventStream{
		Type:        blockindexer.IESTypeEventStreamNOTX,
		Definition:  es,
		HandlerNOTX: el.handleEventBatch,
	})
	if err != nil {
		return nil, err
	}
	tm.blockchainEventListeners[es.Name] = el
	return el, nil
}

func (tm *txManager) stopBlockchainEventListeners() {
	tm.blockchainEventListenerLock.Lock()
	defer tm.blockchainEventListenerLock.Unlock()

	for _, el := range tm.blockchainEventListeners {
		if err := tm.blockIndexer.StopEventStream(tm.bgCtx, el.definition.ID); err != nil {
			log.L(tm.bgCtx).Errorf("Error stopping event listener '%s': %s", el.definition.Name, err)
		}
	}
}

func (tm *txManager) CreateBlockchainEventListener(ctx context.Context, spec *pldapi.BlockchainEventListener) error {
	log.L(ctx).Infof("Creating blockchain event listener '%s'", spec.Name)

	existing := tm.blockchainEventListeners[spec.Name]
	if existing != nil {
		return i18n.NewError(ctx, msgs.MsgTxMgrDuplicateBlockchainEventListenerName, spec.Name)
	}

	err := tm.validateBlockchainEventListenerSpec(ctx, spec)
	if err == nil {
		_, err = tm.loadBlockchainEventListener(ctx, tm.mapEventStream(spec), tm.p.NOTX())
	}
	return err

}

func (tm *txManager) QueryBlockchainEventListeners(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.BlockchainEventListener, error) {
	eventStreams, err := tm.blockIndexer.QueryEventStreamDefinitions(ctx, dbTX, ES_TYPE, jq)
	if err != nil {
		return nil, err
	}
	eventListeners := make([]*pldapi.BlockchainEventListener, len(eventStreams))
	for i, es := range eventStreams {
		eventListeners[i] = tm.mapBlockchainEventListener(es)
	}
	return eventListeners, nil
}

func (tm *txManager) GetBlockchainEventListener(ctx context.Context, name string) *pldapi.BlockchainEventListener {
	tm.blockchainEventListenerLock.Lock()
	defer tm.blockchainEventListenerLock.Unlock()

	l := tm.blockchainEventListeners[name]
	if l != nil {
		return tm.mapBlockchainEventListener(l.definition)
	}
	return nil
}

func (tm *txManager) StartBlockchainEventListener(ctx context.Context, name string) error {
	tm.blockchainEventListenerLock.Lock()
	defer tm.blockchainEventListenerLock.Unlock()
	el := tm.blockchainEventListeners[name]
	if el == nil {
		return i18n.NewError(ctx, msgs.MsgTxMgrBlockchainEventListenerNotLoaded, name)
	}
	return tm.blockIndexer.StartEventStream(ctx, el.definition.ID)
}

func (tm *txManager) StopBlockchainEventListener(ctx context.Context, name string) error {
	tm.blockchainEventListenerLock.Lock()
	defer tm.blockchainEventListenerLock.Unlock()
	el := tm.blockchainEventListeners[name]
	if el == nil {
		return i18n.NewError(ctx, msgs.MsgTxMgrBlockchainEventListenerNotLoaded, name)
	}
	return tm.blockIndexer.StopEventStream(ctx, el.definition.ID)
}

func (tm *txManager) DeleteBlockchainEventListener(ctx context.Context, name string) error {
	tm.blockchainEventListenerLock.Lock()
	defer tm.blockchainEventListenerLock.Unlock()
	el := tm.blockchainEventListeners[name]
	if el == nil {
		return i18n.NewError(ctx, msgs.MsgTxMgrBlockchainEventListenerNotLoaded, name)
	}
	err := tm.blockIndexer.RemoveEventStream(ctx, el.definition.ID)
	if err == nil {
		delete(tm.blockchainEventListeners, name)
	}
	return err
}

func (tm *txManager) GetBlockchainEventListenerStatus(ctx context.Context, name string) (*pldapi.BlockchainEventListenerStatus, error) {
	tm.blockchainEventListenerLock.Lock()
	defer tm.blockchainEventListenerLock.Unlock()
	el := tm.blockchainEventListeners[name]
	if el == nil {
		return nil, i18n.NewError(ctx, msgs.MsgTxMgrBlockchainEventListenerNotLoaded, name)
	}

	l := tm.blockchainEventListeners[name]
	status, err := tm.blockIndexer.GetEventStreamStatus(ctx, l.definition.ID)
	if err != nil {
		return nil, err
	}
	return &pldapi.BlockchainEventListenerStatus{
		Catchup: status.Catchup,
		Checkpoint: pldapi.BlockchainEventListenerCheckpoint{
			BlockNumber: status.CheckpointBlock,
		},
	}, nil
}

func (tm *txManager) validateBlockchainEventListenerSpec(ctx context.Context, spec *pldapi.BlockchainEventListener) error {
	if err := pldtypes.ValidateSafeCharsStartEndAlphaNum(ctx, spec.Name, pldtypes.DefaultNameMaxLen, "name"); err != nil {
		return err
	}

	if len(spec.Sources) == 0 {
		return i18n.NewError(ctx, msgs.MsgTxMgrBlockchainEventListenerNoSources, spec.Name)
	}

	noABI := false
	for _, source := range spec.Sources {
		if source.ABI == nil {
			noABI = true
		}
	}
	if noABI {
		return i18n.NewError(ctx, msgs.MsgTxMgrBlockchainEventListenerNoABIs, spec.Name)
	}

	if spec.Options.BatchTimeout != nil && *spec.Options.BatchTimeout != "" {
		if _, err := time.ParseDuration(*spec.Options.BatchTimeout); err != nil {
			return i18n.NewError(ctx, msgs.MsgTxMgrBlockchainEventListenerInvalidTimeout, *spec.Options.BatchTimeout, err.Error())
		}
	}
	return nil
}

func (tm *txManager) mapEventStream(el *pldapi.BlockchainEventListener) *blockindexer.EventStream {
	es := &blockindexer.EventStream{
		Name:    el.Name,
		Type:    ES_TYPE,
		Started: el.Started,
		Config: blockindexer.EventStreamConfig{
			BatchSize:    el.Options.BatchSize,
			BatchTimeout: el.Options.BatchTimeout,
			FromBlock:    el.Options.FromBlock,
		},
	}

	for _, source := range el.Sources {
		es.Sources = append(es.Sources, blockindexer.EventStreamSource{
			ABI:     source.ABI,
			Address: source.Address,
		})
	}

	return es
}

func (tm *txManager) mapBlockchainEventListener(es *blockindexer.EventStream) *pldapi.BlockchainEventListener {
	el := &pldapi.BlockchainEventListener{
		Name:    es.Name,
		Started: es.Started,
		Created: es.Created,
		Options: pldapi.BlockchainEventListenerOptions{
			BatchSize:    es.Config.BatchSize,
			BatchTimeout: es.Config.BatchTimeout,
			FromBlock:    es.Config.FromBlock,
		},
	}
	for _, source := range es.Sources {
		el.Sources = append(el.Sources, pldapi.BlockchainEventListenerSource{
			ABI:     source.ABI,
			Address: source.Address,
		})
	}

	return el
}

func (tm *txManager) AddBlockchainEventReceiver(ctx context.Context, name string, r components.BlockchainEventReceiver) (components.ReceiverCloser, error) {
	tm.blockchainEventListenerLock.Lock()
	defer tm.blockchainEventListenerLock.Unlock()

	l := tm.blockchainEventListeners[name]
	if l == nil {
		return nil, i18n.NewError(ctx, msgs.MsgTxMgrBlockchainEventListenerNotLoaded, name)
	}

	return l.addReceiver(r), nil
}

func (rr *registeredBlockchainEventReceiver) Close() {
	rr.el.removeReceiver(rr.id)
}

func (el *blockchainEventListener) addReceiver(r components.BlockchainEventReceiver) *registeredBlockchainEventReceiver {
	el.receiverLock.Lock()
	defer el.receiverLock.Unlock()

	registered := &registeredBlockchainEventReceiver{
		id:                      uuid.New(),
		el:                      el,
		BlockchainEventReceiver: r,
	}
	el.receivers = append(el.receivers, registered)

	select {
	case el.newReceivers <- true:
	default:
	}

	return registered
}

func (el *blockchainEventListener) removeReceiver(rid uuid.UUID) {
	el.receiverLock.Lock()
	defer el.receiverLock.Unlock()

	if len(el.receivers) > 0 {
		newReceivers := make([]*registeredBlockchainEventReceiver, 0, len(el.receivers)-1)
		for _, existing := range el.receivers {
			if existing.id != rid {
				newReceivers = append(newReceivers, existing)
			}
		}
		el.receivers = newReceivers
	}
}

func (el *blockchainEventListener) nextReceiver() (r components.BlockchainEventReceiver, err error) {
	for {
		el.receiverLock.Lock()
		if len(el.receivers) > 0 {
			r = el.receivers[el.receiverCounter%len(el.receivers)]
		}
		el.receiverLock.Unlock()

		if r != nil {
			el.receiverCounter++
			return r, nil
		}

		select {
		case <-el.newReceivers:
		case <-el.ctx.Done():
			return nil, i18n.NewError(el.ctx, msgs.MsgContextCanceled)
		}
	}
}

func (el *blockchainEventListener) handleEventBatch(_ context.Context, batch *blockindexer.EventDeliveryBatch) error {
	r, err := el.nextReceiver()
	if err != nil {
		return err
	}
	log.L(el.ctx).Infof("Delivering blockchain event batch %s (receipts=%d)", batch.BatchID, len(batch.Events))
	err = r.DeliverBlockchainEventBatch(el.ctx, batch.BatchID, batch.Events)
	log.L(el.ctx).Infof("Delivered blockchain event batch %s (err=%v)", batch.BatchID, err)
	return err
}
