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

package groupmgr

import (
	"context"
	"encoding/json"
	"regexp"
	"sync"

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

type persistedMessageListener struct {
	Name    string             `gorm:"column:name"`
	Created pldtypes.Timestamp `gorm:"column:created"`
	Started *bool              `gorm:"column:started"`
	Filters pldtypes.RawJSON   `gorm:"column:filters"`
	Options pldtypes.RawJSON   `gorm:"column:options"`
}

var messageListenerFilters = filters.FieldMap{
	"name":    filters.StringField("name"),
	"created": filters.TimestampField("created"),
	"started": filters.BooleanField("started"),
}

func (persistedMessageListener) TableName() string {
	return "message_listeners"
}

type persistedMessageCheckpoint struct {
	Listener string             `gorm:"column:listener"`
	Sequence uint64             `gorm:"column:sequence"`
	Time     pldtypes.Timestamp `gorm:"column:time"`
}

func (persistedMessageCheckpoint) TableName() string {
	return "message_listener_checkpoints"
}

type messageListener struct {
	gm *groupManager

	ctx       context.Context
	cancelCtx context.CancelFunc

	spec       *pldapi.PrivacyGroupMessageListener
	topicMatch *regexp.Regexp
	checkpoint *uint64

	newMessages chan bool

	nextBatchID  uint64
	newReceivers chan bool
	receiverLock sync.Mutex
	receivers    []*registeredMessageReceiver
	done         chan struct{}
}

type registeredMessageReceiver struct {
	id uuid.UUID
	l  *messageListener
	components.PrivacyGroupMessageReceiver
}

type messageDeliveryBatch struct {
	ID       uint64
	Messages []*pldapi.PrivacyGroupMessage
}

func (gm *groupManager) messagesInit() {
	gm.messagesRetry = retry.NewRetryIndefinite(&gm.conf.MessageListeners.Retry, &pldconf.GroupManagerDefaults.MessageListeners.Retry)
	gm.messagesReadPageSize = confutil.IntMin(gm.conf.MessageListeners.ReadPageSize, 1, *pldconf.GroupManagerDefaults.MessageListeners.ReadPageSize)
	gm.messageListeners = make(map[string]*messageListener)
	gm.messageListenersLoadPageSize = 100 /* not currently tunable */
}

func (pm *persistedMessage) mapToAPI() *pldapi.PrivacyGroupMessage {
	return &pldapi.PrivacyGroupMessage{
		LocalSequence: pm.LocalSeq,
		Node:          pm.Node,
		Sent:          pm.Sent,
		Received:      pm.Received,
		ID:            pm.ID,
		PrivacyGroupMessageInput: pldapi.PrivacyGroupMessageInput{
			Domain:        pm.Domain,
			Group:         pm.Group,
			CorrelationID: pm.CID,
			Topic:         pm.Topic,
			Data:          pm.Data,
		},
	}
}

func (gm *groupManager) CreateMessageListener(ctx context.Context, spec *pldapi.PrivacyGroupMessageListener) error {

	log.L(ctx).Infof("Creating message listener '%s'", spec.Name)
	if _, err := gm.validateListenerSpec(ctx, spec); err != nil {
		return err
	}

	started := (spec.Started == nil /* default is true */) || *spec.Started
	dbSpec := &persistedMessageListener{
		Name:    spec.Name,
		Started: &started,
		Created: pldtypes.TimestampNow(),
		Filters: pldtypes.JSONString(&spec.Filters),
		Options: pldtypes.JSONString(&spec.Options),
	}
	if insertErr := gm.p.DB().
		WithContext(ctx).
		Create(dbSpec).
		Error; insertErr != nil {

		log.L(ctx).Errorf("Failed to create message listener '%s': %s", spec.Name, insertErr)

		// Check for a simple duplicate object
		if existing := gm.GetMessageListener(ctx, spec.Name); existing != nil {
			return i18n.NewError(ctx, msgs.MsgPGroupsDuplicateMessageListenerName, spec.Name)
		}

		// Otherwise return the error
		return insertErr
	}

	// Load the created listener now - we do not expect (or attempt to reconcile) a post-validation failure to load
	l, err := gm.loadListener(ctx, dbSpec)
	if err == nil && *l.spec.Started {
		l.start()
	}
	return err
}

func (rr *registeredMessageReceiver) Close() {
	rr.l.removeReceiver(rr.id)
}

func (gm *groupManager) AddMessageReceiver(ctx context.Context, name string, r components.PrivacyGroupMessageReceiver) (components.PrivacyGroupMessageReceiverCloser, error) {
	gm.messageListenerLock.Lock()
	defer gm.messageListenerLock.Unlock()

	l := gm.messageListeners[name]
	if l == nil {
		return nil, i18n.NewError(ctx, msgs.MsgPGroupsMessageListenerNotLoaded, name)
	}

	return l.addReceiver(r), nil
}

func (gm *groupManager) GetMessageListener(ctx context.Context, name string) *pldapi.PrivacyGroupMessageListener {

	gm.messageListenerLock.Lock()
	defer gm.messageListenerLock.Unlock()

	l := gm.messageListeners[name]
	if l != nil {
		return l.spec
	}
	return nil

}

func (gm *groupManager) StartMessageListener(ctx context.Context, name string) error {
	return gm.setMessageListenerStatus(ctx, name, true)
}

func (gm *groupManager) StopMessageListener(ctx context.Context, name string) error {
	return gm.setMessageListenerStatus(ctx, name, false)
}

func (gm *groupManager) setMessageListenerStatus(ctx context.Context, name string, started bool) error {
	gm.messageListenerLock.Lock()
	defer gm.messageListenerLock.Unlock()

	log.L(ctx).Infof("Setting message listener '%s' status. Started=%t", name, started)

	l := gm.messageListeners[name]
	if l == nil {
		return i18n.NewError(ctx, msgs.MsgPGroupsMessageListenerNotLoaded, name)
	}
	err := gm.p.DB().
		WithContext(ctx).
		Model(&persistedMessageListener{}).
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

func (gm *groupManager) DeleteMessageListener(ctx context.Context, name string) error {
	gm.messageListenerLock.Lock()
	defer gm.messageListenerLock.Unlock()

	l := gm.messageListeners[name]
	if l == nil {
		return i18n.NewError(ctx, msgs.MsgPGroupsMessageListenerNotLoaded, name)
	}

	l.stop()

	err := gm.p.DB().
		WithContext(ctx).
		Where("name = ?", name).
		Delete(&persistedMessageListener{}).
		Error
	if err != nil {
		return err
	}

	delete(gm.messageListeners, name)
	return nil
}

func (gm *groupManager) QueryMessageListeners(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.PrivacyGroupMessageListener, error) {
	qw := &filters.QueryWrapper[persistedMessageListener, pldapi.PrivacyGroupMessageListener]{
		P:           gm.p,
		Table:       "message_listeners",
		DefaultSort: "-created",
		Filters:     messageListenerFilters,
		Query:       jq,
		MapResult: func(pl *persistedMessageListener) (*pldapi.PrivacyGroupMessageListener, error) {
			_, l, err := gm.mapListener(ctx, pl)
			return l, err
		},
	}
	return qw.Run(ctx, dbTX)
}

func (gm *groupManager) notifyNewMessages(messages []*persistedMessage) {
	log := log.L(gm.bgCtx)
	for _, l := range gm.getMessageListenerList() {
		hasMatch := false
		for _, r := range messages {
			if l.checkMatch(r) {
				log.Debugf("Message %s (domain='%s') triggering re-poll of listener '%s'", r.ID, r.Domain, l.spec.Name)
				hasMatch = true
				break
			}
		}
		if hasMatch {
			l.notifyNewMessages()
		}
	}
}

func (gm *groupManager) loadMessageListeners() error {

	var lastPageEnd *string
	ctx := gm.bgCtx
	for {

		var page []*persistedMessageListener
		q := gm.p.DB().
			WithContext(ctx).
			Order("name").
			Limit(gm.messageListenersLoadPageSize)
		if lastPageEnd != nil {
			q = q.Where("name > ?", *lastPageEnd)
		}
		if err := q.Find(&page).Error; err != nil {
			return err
		}

		for _, pl := range page {
			if _, err := gm.loadListener(ctx, pl); err != nil {
				return err
			}
		}

		if len(page) < gm.messageListenersLoadPageSize {
			log.L(ctx).Infof("loaded %d message listeners", len(gm.messageListeners))
			return nil
		}

		lastPageEnd = &page[len(page)-1].Name
	}

}

func (gm *groupManager) getMessageListenerList() []*messageListener {

	gm.messageListenerLock.Lock()
	defer gm.messageListenerLock.Unlock()

	listeners := make([]*messageListener, 0, len(gm.messageListeners))
	for _, l := range gm.messageListeners {
		listeners = append(listeners, l)
	}
	return listeners
}

func (gm *groupManager) startMessageListeners() {

	gm.messageListenerLock.Lock()
	defer gm.messageListenerLock.Unlock()

	for _, l := range gm.messageListeners {
		if *l.spec.Started {
			l.start()
		}
	}
}

func (gm *groupManager) stopMessageListeners() {

	gm.messageListenerLock.Lock()
	defer gm.messageListenerLock.Unlock()

	for _, l := range gm.messageListeners {
		l.stop()
	}
}

func (gm *groupManager) validateListenerSpec(ctx context.Context, spec *pldapi.PrivacyGroupMessageListener) (topicMatch *regexp.Regexp, err error) {
	if err := pldtypes.ValidateSafeCharsStartEndAlphaNum(ctx, spec.Name, pldtypes.DefaultNameMaxLen, "name"); err != nil {
		return nil, err
	}

	if spec.Filters.Topic != "" {
		topicMatch, err = regexp.Compile(spec.Filters.Topic)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgPGroupsMessageListenerBadTopicFilter, spec.Filters.Topic)
		}
	}

	return topicMatch, nil
}

// Build parts of the matching that can be pre-filtered efficiently in the DB.
//
// IMPORTANT: Make sure to also update checkMatch() when adding filter dimensions
func (gm *groupManager) buildListenerDBQuery(spec *pldapi.PrivacyGroupMessageListener, q *gorm.DB) *gorm.DB {
	// Filter based on the domain
	if spec.Filters.Domain != "" {
		q = q.Where("domain = ?", spec.Filters.Domain)
	}
	if spec.Filters.Group != nil {
		q = q.Where(`"group" = ?`, spec.Filters.Group)
	}

	if spec.Options.ExcludeLocal {
		q = q.Where("node <> ?", gm.transportManager.LocalNodeName())
	}

	// Note we do post-filter on topic (no DB filter) as it's a regular expression

	// Standard parts
	q = q.Order(`"pgroup_msgs"."local_seq"`).Limit(gm.messagesReadPageSize)
	return q
}

// Applies all the rules in-memory to a message, including:
// - Those which we pre-filter in the DB
// - Those too complex to efficiently pre-filter in the DB
// We do both, so that we can use this as a trigger-guard for re-polling the
// DB, as well as a post-filter on results from the DB.
//
// Note that blocked sequences are ONLY maintained in the DB, so we can never bypass the DB polling.
//
// IMPORTANT: Make sure to also consider adding pre-filters to buildListenerDBQuery() when adding filter dimensions
func (l *messageListener) checkMatch(r *persistedMessage) bool {
	matches := true
	spec := l.spec

	if spec.Filters.Domain != "" {
		matches = matches && (r.Domain == spec.Filters.Domain)
	}
	if spec.Filters.Group != nil {
		matches = matches && (r.Group.Equals(spec.Filters.Group))
	}
	if l.topicMatch != nil {
		matches = matches && (l.topicMatch.MatchString(r.Topic))
	}
	if spec.Options.ExcludeLocal {
		matches = matches && (l.gm.transportManager.LocalNodeName() != r.Node)
	}

	// Note we don't factor sequence into the tap - as the notification does not contain the DB-generated sequence

	return matches
}

func (gm *groupManager) mapListener(ctx context.Context, pl *persistedMessageListener) (*regexp.Regexp, *pldapi.PrivacyGroupMessageListener, error) {
	spec := &pldapi.PrivacyGroupMessageListener{
		Name:    pl.Name,
		Started: pl.Started,
		Created: pl.Created,
	}
	if err := json.Unmarshal(pl.Filters, &spec.Filters); err != nil {
		return nil, nil, i18n.WrapError(ctx, err, msgs.MsgPGroupsBadMessageListenerFilter, pl.Name)
	}
	if err := json.Unmarshal(pl.Options, &spec.Options); err != nil {
		return nil, nil, i18n.WrapError(ctx, err, msgs.MsgPGroupsBadMessageListenerOptions, pl.Name)
	}
	topicMatch, err := gm.validateListenerSpec(ctx, spec)
	if err != nil {
		return nil, nil, err
	}
	return topicMatch, spec, nil
}

func (gm *groupManager) loadListener(ctx context.Context, pl *persistedMessageListener) (l *messageListener, err error) {

	l = &messageListener{
		gm:           gm,
		newReceivers: make(chan bool, 1),
		newMessages:  make(chan bool, 1),
	}

	l.topicMatch, l.spec, err = gm.mapListener(ctx, pl)
	if err != nil {
		return nil, err
	}

	gm.messageListenerLock.Lock()
	defer gm.messageListenerLock.Unlock()
	if gm.messageListeners[pl.Name] != nil {
		return nil, i18n.NewError(ctx, msgs.MsgPGroupsMessageListenerDupLoad, pl.Name)
	}
	gm.messageListeners[pl.Name] = l
	return l, nil
}

func (l *messageListener) initStart() {
	l.ctx, l.cancelCtx = context.WithCancel(log.WithLogField(l.gm.bgCtx, "message-listener", l.spec.Name))
	l.done = make(chan struct{})
}

func (l *messageListener) start() {
	if l.done == nil {
		l.initStart()
		go l.runListener()
	}
}

func (l *messageListener) stop() {
	if l.done != nil {
		l.cancelCtx()
		<-l.done
		l.done = nil
	}
}

func (l *messageListener) notifyNewMessages() {
	select {
	case l.newMessages <- true:
	default:
	}
}

func (l *messageListener) addReceiver(r components.PrivacyGroupMessageReceiver) *registeredMessageReceiver {
	l.receiverLock.Lock()
	defer l.receiverLock.Unlock()

	registered := &registeredMessageReceiver{
		id:                          uuid.New(),
		l:                           l,
		PrivacyGroupMessageReceiver: r,
	}
	l.receivers = append(l.receivers, registered)

	select {
	case l.newReceivers <- true:
	default:
	}

	return registered
}

func (l *messageListener) removeReceiver(rid uuid.UUID) {
	l.receiverLock.Lock()
	defer l.receiverLock.Unlock()

	if len(l.receivers) > 0 {
		newReceivers := make([]*registeredMessageReceiver, 0, len(l.receivers)-1)
		for _, existing := range l.receivers {
			if existing.id != rid {
				newReceivers = append(newReceivers, existing)
			}
		}
		l.receivers = newReceivers
	}
}

func (l *messageListener) loadCheckpoint() error {
	var checkpoints []*persistedMessageCheckpoint
	err := l.gm.p.DB().
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
			log.L(l.ctx).Infof("Started message listener with minSequence=%d", *l.checkpoint)
		} else {
			log.L(l.ctx).Infof("Started message listener from sequence 0")
		}
	} else {
		cpSequence := checkpoints[0].Sequence
		l.checkpoint = &cpSequence
		log.L(l.ctx).Infof("Started message listener with checkpoint=%d", cpSequence)
	}
	return nil
}

func (l *messageListener) readPage() ([]*persistedMessage, error) {
	var messages []*persistedMessage
	err := l.gm.messagesRetry.Do(l.ctx, func(attempt int) (retryable bool, err error) {
		db := l.gm.p.DB()
		q := l.gm.buildListenerDBQuery(l.spec, db)
		if l.checkpoint != nil {
			q = q.Where(`"pgroup_msgs"."local_seq" > ?`, *l.checkpoint)
		}
		return true, q.Find(&messages).Error
	})
	return messages, err
}

func (l *messageListener) processPersistedMessage(b *messageDeliveryBatch, pm *persistedMessage) {
	if !l.checkMatch(pm) {
		return
	}
	// Otherwise we can process the message
	log.L(l.ctx).Infof("Added message %d/%s (domain='%s') to batch %d", pm.LocalSeq, pm.ID, pm.Domain, b.ID)
	b.Messages = append(b.Messages, pm.mapToAPI())
}

func (l *messageListener) nextReceiver(b *messageDeliveryBatch) (r components.PrivacyGroupMessageReceiver, err error) {

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

func (l *messageListener) deliverBatch(b *messageDeliveryBatch) error {
	r, err := l.nextReceiver(b)
	if err != nil {
		return err
	}

	log.L(l.ctx).Infof("Delivering message batch %d (messages=%d)", b.ID, len(b.Messages))
	err = r.DeliverMessageBatch(l.ctx, b.ID, b.Messages)
	log.L(l.ctx).Infof("Delivered message batch %d (err=%v)", b.ID, err)
	return err
}

func (l *messageListener) updateCheckpoint(newSequence uint64) error {
	return l.gm.p.Transaction(l.ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
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
			Create(&persistedMessageCheckpoint{
				Listener: l.spec.Name,
				Sequence: newSequence,
				Time:     pldtypes.TimestampNow(),
			}).
			Error
		if err != nil {
			return err
		}
		l.checkpoint = &newSequence
		return nil
	})

}

func (l *messageListener) processPage(page []*persistedMessage) (*messageDeliveryBatch, error) {
	// Process each one building up a batch to process
	var batch messageDeliveryBatch
	batch.ID = l.nextBatchID
	l.nextBatchID++
	for _, r := range page {
		l.processPersistedMessage(&batch, r)
	}

	// If our batch contains some work, we need to wait for someone to process that work
	// (note we're not holding any resource open at this point - no DB TX or anything).
	if len(batch.Messages) > 0 {
		err := l.gm.messagesRetry.Do(l.ctx, func(attempt int) (retryable bool, err error) {
			return true, l.deliverBatch(&batch)
		})
		if err != nil {
			return nil, err
		}
	}

	return &batch, nil

}

func (l *messageListener) runListener() {
	defer close(l.done)

	err := l.gm.messagesRetry.Do(l.ctx, func(attempt int) (retryable bool, err error) {
		return true, l.loadCheckpoint()
	})
	if err != nil {
		log.L(l.ctx).Warnf("listener stopping before reading checkpoint: %s", err)
		return
	}

	for {

		// Read the next page of messages
		page, err := l.readPage()
		if err != nil {
			log.L(l.ctx).Warnf("listener stopping: %s", err) // cancelled context
			return
		}

		// Deliver those events
		batch, err := l.processPage(page)
		if err != nil {
			log.L(l.ctx).Warnf("listener stopping (processing page of %d messages): %s", len(page), err) // cancelled context
			return
		}

		// Whether we processed any messages or not, we can move our checkpoint forwards
		if len(page) > 0 {
			err := l.gm.messagesRetry.Do(l.ctx, func(attempt int) (retryable bool, err error) {
				return true, l.updateCheckpoint(page[len(page)-1].LocalSeq)
			})
			if err != nil {
				log.L(l.ctx).Warnf("listener stopping (before updating checkpoint for batch %d): %s", batch.ID, err) // cancelled context
				return
			}
		}

		// If our page was not full, wait for notification of new messages before we look again
		if len(page) < l.gm.messagesReadPageSize {
			select {
			case <-l.newMessages:
			case <-l.ctx.Done():
				log.L(l.ctx).Warnf("listener stopping (waiting for new messages/states)") // cancelled context
				return
			}
		}

	}
}
