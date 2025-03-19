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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type persistedEventListener struct {
	Name    string            `gorm:"column:name"`
	Created tktypes.Timestamp `gorm:"column:created"`
	Started *bool             `gorm:"column:started"`
	Sources tktypes.RawJSON   `gorm:"sources:filters"`
	Options tktypes.RawJSON   `gorm:"column:options"`
}

var eventListenerFilters = filters.FieldMap{
	"name":    filters.StringField("name"),
	"created": filters.TimestampField("created"),
	"started": filters.BooleanField("started"),
}

func (persistedEventListener) TableName() string {
	return "event_listeners"
}

type registeredEventReceiver struct {
	id uuid.UUID
	el *eventListener
	components.EventReceiver
}

// TODO AM: how much can this converge with the receiptListener struct?

type eventListener struct {
	tm *txManager

	ctx       context.Context
	cancelCtx context.CancelFunc

	spec            *pldapi.TransactionEventListener
	receiverLock    sync.Mutex
	receivers       []*registeredEventReceiver
	newReceivers    chan bool
	receiverCounter int
}

func (tm *txManager) CreateEventListener(ctx context.Context, spec *pldapi.TransactionEventListener) error {
	log.L(ctx).Infof("Creating event listener '%s'", spec.Name)
	if err := tm.validateEventListenerSpec(ctx, spec); err != nil {
		return err
	}
	started := (spec.Started == nil /* default is true */) || *spec.Started

	dbSpec := &persistedEventListener{
		Name:    spec.Name,
		Started: &started,
		Created: tktypes.TimestampNow(),
	}

	return tm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		err := dbTX.DB().WithContext(ctx).Create(dbSpec).Error
		if err != nil {
			log.L(ctx).Errorf("Failed to create event listener '%s': %s", spec.Name, err)
			// Check for a simple duplicate object
			if existing := tm.GetEventListener(ctx, spec.Name); existing != nil {
				return i18n.NewError(ctx, msgs.MsgTxMgrDuplicateEventListenerName, spec.Name)
			}
			// Otherwise return the error
			return err
		}
		_, err = tm.loadEventListener(ctx, dbSpec, dbTX)
		return err
	})

	// if err == nil && *l.spec.Started {
	// l.start()
	// }
}

func (tm *txManager) QueryEventListeners(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.TransactionEventListener, error) {
	qw := &filters.QueryWrapper[persistedEventListener, pldapi.TransactionEventListener]{
		P:           tm.p,
		Table:       "event_listeners",
		DefaultSort: "-created",
		Filters:     eventListenerFilters,
		Query:       jq,
		MapResult: func(pl *persistedEventListener) (*pldapi.TransactionEventListener, error) {
			return tm.mapEventListener(ctx, pl)
		},
	}
	return qw.Run(ctx, dbTX)
}

func (tm *txManager) GetEventListener(ctx context.Context, name string) *pldapi.TransactionEventListener {
	tm.eventListenerLock.Lock()
	defer tm.eventListenerLock.Unlock()

	l := tm.eventListeners[name]
	if l != nil {
		return l.spec
	}
	return nil
}

func (tm *txManager) StartEventListener(ctx context.Context, name string) error {
	return tm.setEventListenerStatus(ctx, name, true)
}

func (tm *txManager) StopEventListener(ctx context.Context, name string) error {
	return tm.setEventListenerStatus(ctx, name, false)
}

func (tm *txManager) DeleteEventListener(ctx context.Context, name string) error {
	tm.eventListenerLock.Lock()
	defer tm.eventListenerLock.Unlock()

	l := tm.eventListeners[name]
	if l == nil {
		return i18n.NewError(ctx, msgs.MsgTxMgrEventListenerNotLoaded, name)
	}

	// l.stop()

	err := tm.p.DB().
		WithContext(ctx).
		Where("name = ?", name).
		Delete(&persistedEventListener{}).
		Error
	if err != nil {
		return err
	}

	delete(tm.eventListeners, name)
	return nil
}

func (tm *txManager) validateEventListenerSpec(ctx context.Context, spec *pldapi.TransactionEventListener) error {
	// TODO AM: validate batch timeout opion parses to a time string
	return tktypes.ValidateSafeCharsStartEndAlphaNum(ctx, spec.Name, tktypes.DefaultNameMaxLen, "name")
}

// TODO AM: does this function need to create the internal event listener? - not sure that is the role of the load
// is it a one time start action because the block indexer manages the loading? How do we keep the link back?
// no it's fine we can
func (tm *txManager) loadEventListener(ctx context.Context, pl *persistedEventListener, dbTX persistence.DBTX) (*eventListener, error) {
	spec, err := tm.mapEventListener(ctx, pl)
	if err != nil {
		return nil, err
	}

	tm.eventListenerLock.Lock()
	defer tm.eventListenerLock.Unlock()

	if tm.eventListeners[pl.Name] != nil {
		return nil, i18n.NewError(ctx, msgs.MsgTxMgrEventListenerDupLoad, pl.Name)
	}
	el := &eventListener{
		tm:           tm,
		spec:         spec,
		newReceivers: make(chan bool, 1),
	}

	el.ctx, el.cancelCtx = context.WithCancel(log.WithLogField(el.tm.bgCtx, "event-listener", el.spec.Name))
	tm.eventListeners[pl.Name] = el

	eventStream := &blockindexer.EventStream{
		Name: spec.Name, // TODO AM: add a prefix
		Config: blockindexer.EventStreamConfig{
			BatchSize:    spec.Options.BatchSize,
			BatchTimeout: spec.Options.BatchTimeout,
		},
		Sources: blockindexer.EventSources{},
	}

	_, err = tm.blockIndexer.AddEventStream(ctx, dbTX, &blockindexer.InternalEventStream{
		Type:       blockindexer.IESTypeEventStream,
		Definition: eventStream,
		Handler:    el.handleEventBatch,
	})
	if err != nil {
		return nil, err
	}

	return el, nil
}

func (tm *txManager) mapEventListener(ctx context.Context, pl *persistedEventListener) (*pldapi.TransactionEventListener, error) {
	spec := &pldapi.TransactionEventListener{
		Name:    pl.Name,
		Started: pl.Started,
		Created: pl.Created,
		Options: pldapi.TransactionEventListenerOptions{
			BatchSize:    pl.BatchSize,
			BatchTimeout: pl.BatchTimeout,
		},
	}
	if err := tm.validateEventListenerSpec(ctx, spec); err != nil {
		return nil, err
	}
	return spec, nil
}

func (tm *txManager) setEventListenerStatus(ctx context.Context, name string, started bool) error {
	tm.eventListenerLock.Lock()
	defer tm.eventListenerLock.Unlock()

	log.L(ctx).Infof("Setting event listener '%s' status. Started=%t", name, started)

	l := tm.eventListeners[name]
	if l == nil {
		return i18n.NewError(ctx, msgs.MsgTxMgrEventListenerNotLoaded, name)
	}
	err := tm.p.DB().
		WithContext(ctx).
		Model(&persistedEventListener{}).
		Where("name = ?", name).
		Update("started", started).
		Error
	if err != nil {
		return err
	}
	l.spec.Started = &started
	if started {
		// l.start()
	} else {
		// l.stop()
	}
	return nil
}

// TODO AM: this is all duplicated below here

func (el *eventListener) addReceiver(r components.EventReceiver) *registeredEventReceiver {
	el.receiverLock.Lock()
	defer el.receiverLock.Unlock()

	registered := &registeredEventReceiver{
		id:            uuid.New(),
		el:            el,
		EventReceiver: r,
	}
	el.receivers = append(el.receivers, registered)

	select {
	case el.newReceivers <- true:
	default:
	}

	return registered
}

func (el *eventListener) removeReceiver(rid uuid.UUID) {
	el.receiverLock.Lock()
	defer el.receiverLock.Unlock()

	if len(el.receivers) > 0 {
		newReceivers := make([]*registeredEventReceiver, 0, len(el.receivers)-1)
		for _, existing := range el.receivers {
			if existing.id != rid {
				newReceivers = append(newReceivers, existing)
			}
		}
		el.receivers = newReceivers
	}
}

func (el *eventListener) nextReceiver() (r components.EventReceiver, err error) {
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

func (el *eventListener) handleEventBatch(_ context.Context, _ persistence.DBTX, batch *blockindexer.EventDeliveryBatch) error {
	r, err := el.nextReceiver()
	if err != nil {
		return err
	}
	log.L(el.ctx).Infof("Delivering event batch %s (receipts=%d)", batch.BatchID, len(batch.Events))
	err = r.DeliverEventBatch(el.ctx, batch.BatchID, batch.Events)
	log.L(el.ctx).Infof("Delivered receipt batch %s (err=%v)", batch.BatchID, err)
	return err
}
