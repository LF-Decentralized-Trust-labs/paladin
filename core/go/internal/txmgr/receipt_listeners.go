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

package txmgr

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/filters"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/retry"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type persistedReceiptListener struct {
	Name    string             `gorm:"column:name"`
	Created pldtypes.Timestamp `gorm:"column:created"`
	Started *bool              `gorm:"column:started"`
	Filters pldtypes.RawJSON   `gorm:"column:filters"`
	Options pldtypes.RawJSON   `gorm:"column:options"`
}

var receiptListenerFilters = filters.FieldMap{
	"name":    filters.StringField("name"),
	"created": filters.TimestampField("created"),
	"started": filters.BooleanField("started"),
}

func (persistedReceiptListener) TableName() string {
	return "receipt_listeners"
}

type persistedReceiptCheckpoint struct {
	Listener string             `gorm:"column:listener"`
	Sequence uint64             `gorm:"column:sequence"`
	Time     pldtypes.Timestamp `gorm:"column:time"`
}

func (persistedReceiptCheckpoint) TableName() string {
	return "receipt_listener_checkpoints"
}

type stateRef struct {
	DomainName string            `gorm:"column:domain_name;primaryKey"`
	ID         pldtypes.HexBytes `gorm:"column:id;primaryKey"`
}

func (sr stateRef) TableName() string {
	return "states"
}

type persistedReceiptGap struct {
	Listener    string               `gorm:"column:listener;primaryKey"`
	Source      *pldtypes.EthAddress `gorm:"column:source;primaryKey"`
	Transaction uuid.UUID            `gorm:"column:transaction"`
	Sequence    uint64               `gorm:"column:sequence"`
	DomainName  string               `gorm:"column:domain_name"`
	StateID     pldtypes.HexBytes    `gorm:"column:state"`
	State       *stateRef            `gorm:"foreignKey:DomainName,ID;references:DomainName,StateID"`
}

func (persistedReceiptGap) TableName() string {
	return "receipt_listener_gap"
}

type receiptListener struct {
	tm *txManager

	ctx       context.Context
	cancelCtx context.CancelFunc

	spec       *pldapi.TransactionReceiptListener
	checkpoint *uint64

	newReceipts chan bool

	nextBatchID  uint64
	newReceivers chan bool
	receiverLock sync.Mutex
	receivers    []*registeredReceiptReceiver
	done         chan struct{}
}

type registeredReceiptReceiver struct {
	id uuid.UUID
	l  *receiptListener
	components.ReceiptReceiver
}

type receiptDeliveryBatch struct {
	ID       uint64
	Receipts []*pldapi.TransactionReceiptFull
	Gaps     []*persistedReceiptGap
}

func (tm *txManager) receiptsInit() {
	tm.receiptsRetry = retry.NewRetryIndefinite(&tm.conf.ReceiptListeners.Retry, &pldconf.TxManagerDefaults.ReceiptListeners.Retry)
	tm.receiptsReadPageSize = confutil.IntMin(tm.conf.ReceiptListeners.ReadPageSize, 1, *pldconf.TxManagerDefaults.ReceiptListeners.ReadPageSize)
	tm.receiptListeners = make(map[string]*receiptListener)
	tm.receiptListenersLoadPageSize = 100 /* not currently tunable */
	tm.receiptsStateGapCheckTime = confutil.DurationMin(tm.conf.ReceiptListeners.StateGapCheckInterval, 100*time.Millisecond, *pldconf.TxManagerDefaults.ReceiptListeners.StateGapCheckInterval)
	tm.lastStateUpdateTime.Store(int64(pldtypes.TimestampNow()))
}

func (tm *txManager) CreateReceiptListener(ctx context.Context, spec *pldapi.TransactionReceiptListener) error {

	log.L(ctx).Infof("Creating receipt listener '%s'", spec.Name)
	if err := tm.validateReceiptListenerSpec(ctx, spec); err != nil {
		return err
	}

	started := (spec.Started == nil /* default is true */) || *spec.Started
	dbSpec := &persistedReceiptListener{
		Name:    spec.Name,
		Started: &started,
		Created: pldtypes.TimestampNow(),
		Filters: pldtypes.JSONString(&spec.Filters),
		Options: pldtypes.JSONString(&spec.Options),
	}
	if insertErr := tm.p.DB().
		WithContext(ctx).
		Create(dbSpec).
		Error; insertErr != nil {

		log.L(ctx).Errorf("Failed to create receipt listener '%s': %s", spec.Name, insertErr)

		// Check for a simple duplicate object
		if existing := tm.GetReceiptListener(ctx, spec.Name); existing != nil {
			return i18n.NewError(ctx, msgs.MsgTxMgrDuplicateReceiptListenerName, spec.Name)
		}

		// Otherwise return the error
		return insertErr
	}

	// Load the created listener now - we do not expect (or attempt to reconcile) a post-validation failure to load
	l, err := tm.loadReceiptListener(ctx, dbSpec)
	if err == nil && *l.spec.Started {
		l.start()
	}
	return err
}

func (rr *registeredReceiptReceiver) Close() {
	rr.l.removeReceiver(rr.id)
}

func (tm *txManager) AddReceiptReceiver(ctx context.Context, name string, r components.ReceiptReceiver) (components.ReceiverCloser, error) {
	tm.receiptListenerLock.Lock()
	defer tm.receiptListenerLock.Unlock()

	l := tm.receiptListeners[name]
	if l == nil {
		return nil, i18n.NewError(ctx, msgs.MsgTxMgrReceiptListenerNotLoaded, name)
	}

	return l.addReceiver(r), nil
}

func (tm *txManager) GetReceiptListener(ctx context.Context, name string) *pldapi.TransactionReceiptListener {

	tm.receiptListenerLock.Lock()
	defer tm.receiptListenerLock.Unlock()

	l := tm.receiptListeners[name]
	if l != nil {
		return l.spec
	}
	return nil

}

func (tm *txManager) StartReceiptListener(ctx context.Context, name string) error {
	return tm.setReceiptListenerStatus(ctx, name, true)
}

func (tm *txManager) StopReceiptListener(ctx context.Context, name string) error {
	return tm.setReceiptListenerStatus(ctx, name, false)
}

func (tm *txManager) NotifyStatesDBChanged(ctx context.Context) {
	tm.lastStateUpdateTime.Store(int64(pldtypes.TimestampNow()))
}

func (tm *txManager) setReceiptListenerStatus(ctx context.Context, name string, started bool) error {
	tm.receiptListenerLock.Lock()
	defer tm.receiptListenerLock.Unlock()

	log.L(ctx).Infof("Setting receipt listener '%s' status. Started=%t", name, started)

	l := tm.receiptListeners[name]
	if l == nil {
		return i18n.NewError(ctx, msgs.MsgTxMgrReceiptListenerNotLoaded, name)
	}
	err := tm.p.DB().
		WithContext(ctx).
		Model(&persistedReceiptListener{}).
		Where("name = ?", name).
		Update("started", started).
		Error
	if err != nil {
		return err
	}
	l.spec.Started = &started
	if started {
		l.start()
	} else {
		l.stop()
	}
	return nil
}

func (tm *txManager) DeleteReceiptListener(ctx context.Context, name string) error {
	tm.receiptListenerLock.Lock()
	defer tm.receiptListenerLock.Unlock()

	l := tm.receiptListeners[name]
	if l == nil {
		return i18n.NewError(ctx, msgs.MsgTxMgrReceiptListenerNotLoaded, name)
	}

	l.stop()

	err := tm.p.DB().
		WithContext(ctx).
		Where("name = ?", name).
		Delete(&persistedReceiptListener{}).
		Error
	if err != nil {
		return err
	}

	delete(tm.receiptListeners, name)
	return nil
}

func (tm *txManager) QueryReceiptListeners(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.TransactionReceiptListener, error) {
	qw := &filters.QueryWrapper[persistedReceiptListener, pldapi.TransactionReceiptListener]{
		P:           tm.p,
		Table:       "receipt_listeners",
		DefaultSort: "-created",
		Filters:     receiptListenerFilters,
		Query:       jq,
		MapResult: func(pl *persistedReceiptListener) (*pldapi.TransactionReceiptListener, error) {
			return tm.mapReceiptListener(ctx, pl)
		},
	}
	return qw.Run(ctx, dbTX)
}

func (tm *txManager) notifyNewReceipts(receipts []*transactionReceipt) {
	log := log.L(tm.bgCtx)
	for _, l := range tm.getReceiptListenerList() {
		hasMatch := false
		for _, r := range receipts {
			if l.checkMatch(r) {
				log.Debugf("Receipt %s (domain='%s') triggering re-poll of listener '%s'", r.TransactionID, r.Domain, l.spec.Name)
				hasMatch = true
				break
			}
		}
		if hasMatch {
			l.notifyNewReceipts()
		}
	}
}

func (tm *txManager) loadReceiptListeners() error {

	var lastPageEnd *string
	ctx := tm.bgCtx
	for {

		var page []*persistedReceiptListener
		q := tm.p.DB().
			WithContext(ctx).
			Order("name").
			Limit(tm.receiptListenersLoadPageSize)
		if lastPageEnd != nil {
			q = q.Where("name > ?", *lastPageEnd)
		}
		if err := q.Find(&page).Error; err != nil {
			return err
		}

		for _, pl := range page {
			if _, err := tm.loadReceiptListener(ctx, pl); err != nil {
				return err
			}
		}

		if len(page) < tm.receiptListenersLoadPageSize {
			log.L(ctx).Infof("loaded %d receipt listeners", len(tm.receiptListeners))
			return nil
		}

		lastPageEnd = &page[len(page)-1].Name
	}

}

func (tm *txManager) getReceiptListenerList() []*receiptListener {

	tm.receiptListenerLock.Lock()
	defer tm.receiptListenerLock.Unlock()

	listeners := make([]*receiptListener, 0, len(tm.receiptListeners))
	for _, l := range tm.receiptListeners {
		listeners = append(listeners, l)
	}
	return listeners
}

func (tm *txManager) startReceiptListeners() {

	tm.receiptListenerLock.Lock()
	defer tm.receiptListenerLock.Unlock()

	for _, l := range tm.receiptListeners {
		if *l.spec.Started {
			l.start()
		}
	}
}

func (tm *txManager) stopReceiptListeners() {

	tm.receiptListenerLock.Lock()
	defer tm.receiptListenerLock.Unlock()

	for _, l := range tm.receiptListeners {
		l.stop()
	}
}

func (tm *txManager) validateReceiptListenerSpec(ctx context.Context, spec *pldapi.TransactionReceiptListener) error {
	if err := pldtypes.ValidateSafeCharsStartEndAlphaNum(ctx, spec.Name, pldtypes.DefaultNameMaxLen, "name"); err != nil {
		return err
	}
	icrb, err := spec.Options.IncompleteStateReceiptBehavior.Validate()
	if err != nil {
		return err
	}
	spec.Options.IncompleteStateReceiptBehavior = icrb.Enum()
	_, err = tm.buildListenerDBQuery(ctx, spec, tm.p.DB())
	return err
}

// Build parts of the matching that can be pre-filtered efficiently in the DB.
//
// IMPORTANT: Make sure to also update checkMatch() when adding filter dimensions
func (tm *txManager) buildListenerDBQuery(ctx context.Context, spec *pldapi.TransactionReceiptListener, q *gorm.DB) (*gorm.DB, error) {
	// Filter based on the type and/or domain
	if spec.Filters.Type == nil {
		if spec.Filters.Domain != "" {
			return nil, i18n.NewError(ctx, msgs.MsgTxMgrBadReceiptListenerTypeDomain, spec.Name, "", spec.Filters.Domain)
		}
	} else {
		switch spec.Filters.Type.V() {
		case pldapi.TransactionTypePrivate:
			if spec.Filters.Domain != "" {
				q = q.Where("domain = ?", spec.Filters.Domain) // specific private domain
			} else {
				q = q.Where("domain <> ''") // private
			}
		case pldapi.TransactionTypePublic:
			if spec.Filters.Domain != "" {
				return nil, i18n.NewError(ctx, msgs.MsgTxMgrBadReceiptListenerTypeDomain, spec.Name, spec.Filters.Type, spec.Filters.Domain)
			}
			q = q.Where("domain = ''") // public
		default:
			return nil, i18n.NewError(ctx, msgs.MsgTxMgrBadReceiptListenerTypeDomain, spec.Name, spec.Filters.Type, spec.Filters.Domain)
		}
	}

	// Standard parts
	q = q.Order(`"transaction_receipts"."sequence"`).Limit(tm.receiptsReadPageSize)
	return q, nil
}

// Applies all the rules in-memory to a receipt, including:
// - Those which we pre-filter in the DB
// - Those too complex to efficiently pre-filter in the DB
// We do both, so that we can use this as a trigger-guard for re-polling the
// DB, as well as a post-filter on results from the DB.
//
// Note that blocked sequences are ONLY maintained in the DB, so we can never bypass the DB polling.
//
// IMPORTANT: Make sure to also consider adding pre-filters to buildListenerDBQuery() when adding filter dimensions
func (l *receiptListener) checkMatch(r *transactionReceipt) bool {
	matches := true
	spec := l.spec

	// Filter based on the type and/or domain
	if spec.Filters.Type != nil {
		switch spec.Filters.Type.V() {
		case pldapi.TransactionTypePrivate:
			if spec.Filters.Domain != "" {
				matches = matches && (r.Domain == spec.Filters.Domain)
			} else {
				matches = matches && (r.Domain != "")
			}
		case pldapi.TransactionTypePublic:
			matches = matches && (r.Domain == "")
		default:
			matches = false
		}
	}

	// Note we don't factor sequence into the tap - as the notification does not contain the DB-generated sequence

	return matches
}

func (tm *txManager) mapReceiptListener(ctx context.Context, pl *persistedReceiptListener) (*pldapi.TransactionReceiptListener, error) {
	spec := &pldapi.TransactionReceiptListener{
		Name:    pl.Name,
		Started: pl.Started,
		Created: pl.Created,
	}
	if err := json.Unmarshal(pl.Filters, &spec.Filters); err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTxMgrBadReceiptListenerFilter, pl.Name)
	}
	if err := json.Unmarshal(pl.Options, &spec.Options); err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTxMgrBadReceiptListenerOptions, pl.Name)
	}
	if err := tm.validateReceiptListenerSpec(ctx, spec); err != nil {
		return nil, err
	}
	return spec, nil
}

func (tm *txManager) loadReceiptListener(ctx context.Context, pl *persistedReceiptListener) (*receiptListener, error) {

	spec, err := tm.mapReceiptListener(ctx, pl)
	if err != nil {
		return nil, err
	}

	l := &receiptListener{
		tm:           tm,
		spec:         spec,
		newReceivers: make(chan bool, 1),
		newReceipts:  make(chan bool, 1),
	}

	tm.receiptListenerLock.Lock()
	defer tm.receiptListenerLock.Unlock()
	if tm.receiptListeners[pl.Name] != nil {
		return nil, i18n.NewError(ctx, msgs.MsgTxMgrReceiptListenerDupLoad, pl.Name)
	}
	tm.receiptListeners[pl.Name] = l
	return l, nil
}

func (l *receiptListener) initStart() {
	l.ctx, l.cancelCtx = context.WithCancel(log.WithLogField(l.tm.bgCtx, "receipt-listener", l.spec.Name))
	l.done = make(chan struct{})
}

func (l *receiptListener) start() {
	if l.done == nil {
		l.initStart()
		go l.runListener()
	}
}

func (l *receiptListener) stop() {
	if l.done != nil {
		l.cancelCtx()
		<-l.done
		l.done = nil
	}
}

func (l *receiptListener) notifyNewReceipts() {
	select {
	case l.newReceipts <- true:
	default:
	}
}

func (l *receiptListener) addReceiver(r components.ReceiptReceiver) *registeredReceiptReceiver {
	l.receiverLock.Lock()
	defer l.receiverLock.Unlock()

	registered := &registeredReceiptReceiver{
		id:              uuid.New(),
		l:               l,
		ReceiptReceiver: r,
	}
	l.receivers = append(l.receivers, registered)

	select {
	case l.newReceivers <- true:
	default:
	}

	return registered
}

func (l *receiptListener) removeReceiver(rid uuid.UUID) {
	l.receiverLock.Lock()
	defer l.receiverLock.Unlock()

	if len(l.receivers) > 0 {
		newReceivers := make([]*registeredReceiptReceiver, 0, len(l.receivers)-1)
		for _, existing := range l.receivers {
			if existing.id != rid {
				newReceivers = append(newReceivers, existing)
			}
		}
		l.receivers = newReceivers
	}
}

func (l *receiptListener) loadCheckpoint() error {
	var checkpoints []*persistedReceiptCheckpoint
	err := l.tm.p.DB().
		WithContext(l.ctx).
		Where("listener = ?", l.spec.Name).
		Limit(1).
		Find(&checkpoints).
		Error
	if err != nil {
		return err
	}
	if len(checkpoints) == 0 {
		if l.spec.Filters.SequenceAbove != nil {
			l.checkpoint = l.spec.Filters.SequenceAbove
			log.L(l.ctx).Infof("Started receipt listener with minSequence=%d", *l.checkpoint)
		} else {
			log.L(l.ctx).Infof("Started receipt listener from start of chain")
		}
	} else {
		cpSequence := checkpoints[0].Sequence
		l.checkpoint = &cpSequence
		log.L(l.ctx).Infof("Started receipt listener with checkpoint=%d", cpSequence)
	}
	return nil
}

func (l *receiptListener) readHeadPage() ([]*transactionReceipt, error) {
	var receipts []*transactionReceipt
	err := l.tm.receiptsRetry.Do(l.ctx, func(attempt int) (retryable bool, err error) {
		db := l.tm.p.DB()
		q, err := l.tm.buildListenerDBQuery(l.ctx, l.spec, db)
		if err == nil {
			// Only return non-blocked sequences
			q = q.Joins(`Gap`, db.Where(&persistedReceiptGap{Listener: l.spec.Name})).
				Where(`"Gap"."transaction" IS NULL`)
			if l.checkpoint != nil {
				q = q.Where(`"transaction_receipts"."sequence" > ?`, *l.checkpoint)
			}
		}
		if err == nil {
			err = q.Find(&receipts).Error
		}
		return true, err
	})
	return receipts, err
}

func (l *receiptListener) readGapPage(gap *persistedReceiptGap) ([]*transactionReceipt, error) {
	var receipts []*transactionReceipt
	err := l.tm.receiptsRetry.Do(l.ctx, func(attempt int) (retryable bool, err error) {
		q, err := l.tm.buildListenerDBQuery(l.ctx, l.spec, l.tm.p.DB())
		if err == nil {
			q = q.
				Where(`"transaction_receipts"."source" = ?`, gap.Source).
				Where(`"transaction_receipts"."sequence" >= ?`, gap.Sequence)
			if l.checkpoint != nil {
				q = q.Where(`"transaction_receipts"."sequence" <= ?`, *l.checkpoint)
			}
		}
		if err == nil {
			err = q.Find(&receipts).Error
		}
		return true, err
	})
	return receipts, err
}

func (l *receiptListener) processPersistedReceipt(b *receiptDeliveryBatch, pr *transactionReceipt) error {
	if !l.checkMatch(pr) {
		return nil
	}

	// If we already have a block for this contract earlier in the same batch, we need to skip
	for _, block := range b.Gaps {
		if pr.Domain != "" && pr.Source.Equals(block.Source) {
			log.L(l.ctx).Infof("TXID %s is blocked by a gap created in the same batch by TXID %s", pr.TransactionID, block.Transaction)
			return nil
		}
	}

	// Build the full receipt
	fr, err := l.tm.buildFullReceipt(l.ctx, &pldapi.TransactionReceipt{
		ID:                     pr.TransactionID,
		TransactionReceiptData: *mapPersistedReceipt(pr),
	}, l.spec.Options.DomainReceipts)
	if err != nil {
		return err
	}
	// If we don't have the state receipt, and we're told to block, then block
	if fr.TransactionReceiptDataOnchainEvent != nil && fr.Domain != "" &&
		(fr.States == nil || fr.States.FirstUnavailable() != nil) &&
		l.spec.Options.IncompleteStateReceiptBehavior.V() == pldapi.IncompleteStateReceiptBehaviorBlockContract {
		log.L(l.ctx).Infof("States currently unavailable for TXID %s in blockchain TX %s blocking contract %s", fr.ID, fr.TransactionHash, fr.Source)
		b.Gaps = append(b.Gaps, &persistedReceiptGap{
			Listener:    l.spec.Name,
			Source:      &fr.Source,
			Sequence:    pr.Sequence,
			DomainName:  fr.Domain,
			StateID:     fr.States.FirstUnavailable(),
			Transaction: fr.ID,
		})
		return nil
	}
	// Otherwise we can process the receipt
	log.L(l.ctx).Infof("Added receipt %d/%s (domain='%s') to batch %d", pr.Sequence, fr.ID, fr.Domain, b.ID)
	b.Receipts = append(b.Receipts, fr)
	return nil
}

func (l *receiptListener) nextReceiver(b *receiptDeliveryBatch) (r components.ReceiptReceiver, err error) {

	for {
		l.receiverLock.Lock()
		if len(l.receivers) > 0 {
			r = l.receivers[int(b.ID)%len(l.receivers)]
		}
		l.receiverLock.Unlock()

		if r != nil {
			return r, nil
		}

		select {
		case <-l.newReceivers:
		case <-l.ctx.Done():
			return nil, i18n.NewError(l.ctx, msgs.MsgContextCanceled)
		}
	}

}

func (l *receiptListener) deliverBatch(b *receiptDeliveryBatch) error {
	r, err := l.nextReceiver(b)
	if err != nil {
		return err
	}

	log.L(l.ctx).Infof("Delivering receipt batch %d (receipts=%d)", b.ID, len(b.Receipts))
	err = r.DeliverReceiptBatch(l.ctx, b.ID, b.Receipts)
	log.L(l.ctx).Infof("Delivered receipt batch %d (err=%v)", b.ID, err)
	return err
}

func (l *receiptListener) updateCheckpoint(batch *receiptDeliveryBatch, newSequence uint64) error {
	return l.tm.p.Transaction(l.ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		err := dbTX.DB().
			WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "listener"},
				},
				DoUpdates: clause.AssignmentColumns([]string{
					"sequence",
					"time",
				}),
			}).
			Create(&persistedReceiptCheckpoint{
				Listener: l.spec.Name,
				Sequence: newSequence,
				Time:     pldtypes.TimestampNow(),
			}).
			Error
		if err == nil && len(batch.Gaps) > 0 {
			err = dbTX.DB().
				WithContext(ctx).
				Clauses(clause.OnConflict{
					DoNothing: true,
				}).
				Create(batch.Gaps).
				Error
		}
		if err != nil {
			return err
		}
		l.checkpoint = &newSequence
		return nil
	})

}

func (l *receiptListener) processPage(page []*transactionReceipt) (*receiptDeliveryBatch, error) {
	// Process each one building up a batch to process
	var batch receiptDeliveryBatch
	batch.ID = l.nextBatchID
	l.nextBatchID++
	for _, r := range page {
		err := l.tm.receiptsRetry.Do(l.ctx, func(attempt int) (retryable bool, err error) {
			return true, l.processPersistedReceipt(&batch, r)
		})
		if err != nil {
			return nil, err
		}
	}

	// If our batch contains some work, we need to wait for someone to process that work
	// (note we're not holding any resource open at this point - no DB TX or anything).
	if len(batch.Receipts) > 0 {
		err := l.tm.receiptsRetry.Do(l.ctx, func(attempt int) (retryable bool, err error) {
			return true, l.deliverBatch(&batch)
		})
		if err != nil {
			return nil, err
		}
	}

	return &batch, nil

}

func (l *receiptListener) processStaleGaps() error {

	// We process stale gaps one at a time, as the outcome is to:
	// 1) if still a problem, remove the stale flag from the gap
	// 2) move the gap onwards to the new gap
	// 3) remove the gap completely so the contract is in the head group again
	for {
		var gaps []*persistedReceiptGap
		err := l.tm.receiptsRetry.Do(l.ctx, func(attempt int) (retryable bool, err error) {
			return true, l.tm.p.DB().
				WithContext(l.ctx).
				Joins("State").
				Where(`"State"."id" IS NOT NULL`). // the state exists
				Limit(l.tm.receiptListenersLoadPageSize).
				Find(&gaps).
				Error
		})
		if err != nil {
			return err
		}
		if len(gaps) == 0 {
			// no gaps to process
			return nil
		}

		for _, gap := range gaps {
			if err := l.processStaleGap(gap); err != nil {
				return err
			}
		}
	}

}

func (l *receiptListener) processStaleGap(gap *persistedReceiptGap) error {

	for {
		// Read a page of events from the gap
		page, err := l.readGapPage(gap)
		if err != nil {
			return err
		}

		// Deliver the events
		batch, err := l.processPage(page)
		if err != nil {
			return err
		}

		// We find a gap still, then we update the gap to this new (non-stale) position
		if len(batch.Gaps) > 0 {
			log.L(l.ctx).Infof("Gap for contract %s remains old=%d/%s new=%d/%s", gap.Source, gap.Sequence, gap.Transaction, batch.Gaps[0].Sequence, batch.Gaps[0].Transaction)
			return l.tm.receiptsRetry.Do(l.ctx, func(attempt int) (retryable bool, err error) {
				return true, l.tm.p.DB().
					WithContext(l.ctx).
					Clauses(clause.OnConflict{
						Columns: []clause.Column{
							{Name: "listener"},
							{Name: "source"},
						},
						DoUpdates: clause.AssignmentColumns([]string{
							"sequence", // might move forwards
							"state",    // set to new state
						}),
					}).
					Create(batch.Gaps).
					Error
			})
		}

		if len(page) < l.tm.receiptsReadPageSize {
			// We are done at this point - delete the gap
			return l.tm.receiptsRetry.Do(l.ctx, func(attempt int) (retryable bool, err error) {
				return true, l.tm.p.DB().
					WithContext(l.ctx).
					Where("listener = ?", gap.Listener).
					Where("source = ?", gap.Source).
					Delete(&persistedReceiptGap{}).
					Error
			})
		}

		// We need to move the stale gap forwards, to the next record after this page.
		// Then we continue to query, until we run out of messages
		gap.Sequence = page[len(page)-1].Sequence + 1
		gap.StateID = nil
		gap.State = nil
		if err := l.tm.receiptsRetry.Do(l.ctx, func(attempt int) (retryable bool, err error) {
			return true, l.tm.p.DB().
				WithContext(l.ctx).
				Model(&persistedReceiptGap{}).
				Where("listener = ?", gap.Listener).
				Where("source = ?", gap.Source).
				Updates(map[string]any{
					"sequence": gap.Sequence,
					"state":    nil, // clear the state ref, so this will be stale immediately
				}).
				Error
		}); err != nil {
			return err
		}
	}

}

func (l *receiptListener) runListener() {
	defer close(l.done)

	err := l.tm.receiptsRetry.Do(l.ctx, func(attempt int) (retryable bool, err error) {
		return true, l.loadCheckpoint()
	})
	if err != nil {
		log.L(l.ctx).Warnf("listener stopping before reading checkpoint: %s", err)
		return
	}

	newReceipts := true
	newStates := true
	lastStateCheck := time.Now()
	stateGapCheckTicker := time.NewTicker(l.tm.receiptsStateGapCheckTime)
	defer stateGapCheckTicker.Stop()
	for {

		if newStates {
			lastStateCheck = time.Now()

			// Process up all stale gaps before we process the head
			if err := l.processStaleGaps(); err != nil {
				log.L(l.ctx).Warnf("listener stopping (processing stale gaps): %s", err) // cancelled context
				return
			}

			newStates = false
		}

		if newReceipts {
			// Read the next page of receipts from non-gapped sources - the head
			page, err := l.readHeadPage()
			if err != nil {
				log.L(l.ctx).Warnf("listener stopping: %s", err) // cancelled context
				return
			}

			// Deliver those events
			batch, err := l.processPage(page)
			if err != nil {
				log.L(l.ctx).Warnf("listener stopping (processing page of %d receipts): %s", len(page), err) // cancelled context
				return
			}

			// Whether we processed any receipts or not, we can move our checkpoint forwards
			if len(page) > 0 {
				err := l.tm.receiptsRetry.Do(l.ctx, func(attempt int) (retryable bool, err error) {
					return true, l.updateCheckpoint(batch, page[len(page)-1].Sequence)
				})
				if err != nil {
					log.L(l.ctx).Warnf("listener stopping (before updating checkpoint for batch %d): %s", batch.ID, err) // cancelled context
					return
				}
			}

			// We need to poll immediately if we have a full page
			newReceipts = len(page) == l.tm.receiptsReadPageSize
		}

		// If our page was not full, wait for notification of new receipts before we look again
		for !newReceipts && !newStates {
			select {
			case <-l.newReceipts:
				newReceipts = true
			case <-stateGapCheckTicker.C:
				// Only do the DB check if we've had the tap that new states have been received
				newStates = pldtypes.Timestamp(l.tm.lastStateUpdateTime.Load()).Time().After(lastStateCheck)
			case <-l.ctx.Done():
				log.L(l.ctx).Warnf("listener stopping (waiting for new receipts/states)") // cancelled context
				return
			}
		}

	}
}
