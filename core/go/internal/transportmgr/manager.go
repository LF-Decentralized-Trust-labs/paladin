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

package transportmgr

import (
	"context"
	"sync"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/filters"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/flushwriter"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/retry"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
)

type transportManager struct {
	bgCtx     context.Context
	cancelCtx context.CancelFunc
	mux       sync.Mutex

	rpcModule        *rpcserver.RPCModule
	conf             *pldconf.TransportManagerConfig
	localNodeName    string
	registryManager  components.RegistryManager
	stateManager     components.StateManager
	domainManager    components.DomainManager
	keyManager       components.KeyManager
	txManager        components.TXManager
	privateTxManager components.PrivateTxManager
	identityResolver components.IdentityResolver
	groupManager     components.GroupManager
	persistence      persistence.Persistence

	transportsByID   map[uuid.UUID]*transport
	transportsByName map[string]*transport

	peersLock      sync.RWMutex
	peers          map[string]*peer
	peerReaperDone chan struct{}

	reliableMsgWriter flushwriter.Writer[*reliableMsgOp, *noResult]

	sendShortRetry        *retry.Retry
	reliableScanRetry     *retry.Retry
	peerInactivityTimeout time.Duration
	quiesceTimeout        time.Duration
	peerReaperInterval    time.Duration

	senderBufferLen         int
	reliableMessageResend   time.Duration
	reliableMessagePageSize int
}

var reliableMessageFilters = filters.FieldMap{
	"sequence":    filters.Int64Field("sequence"),
	"id":          filters.UUIDField("id"),
	"created":     filters.TimestampField("created"),
	"node":        filters.StringField("node"),
	"messageType": filters.StringField("msg_type"),
}

var reliableMessageAckFilters = filters.FieldMap{
	"messageId": filters.UUIDField("id"),
	"time":      filters.TimestampField("time"),
	"error":     filters.StringField("error"),
}

func NewTransportManager(bgCtx context.Context, conf *pldconf.TransportManagerConfig) components.TransportManager {
	tm := &transportManager{
		conf:                    conf,
		localNodeName:           conf.NodeName,
		transportsByID:          make(map[uuid.UUID]*transport),
		transportsByName:        make(map[string]*transport),
		peers:                   make(map[string]*peer),
		senderBufferLen:         confutil.IntMin(conf.SendQueueLen, 0, *pldconf.TransportManagerDefaults.SendQueueLen),
		reliableMessageResend:   confutil.DurationMin(conf.ReliableMessageResend, 100*time.Millisecond, *pldconf.TransportManagerDefaults.ReliableMessageResend),
		sendShortRetry:          retry.NewRetryLimited(&conf.SendRetry, &pldconf.TransportManagerDefaults.SendRetry),
		reliableScanRetry:       retry.NewRetryIndefinite(&conf.ReliableScanRetry, &pldconf.TransportManagerDefaults.ReliableScanRetry),
		peerInactivityTimeout:   confutil.DurationMin(conf.PeerInactivityTimeout, 0, *pldconf.TransportManagerDefaults.PeerInactivityTimeout),
		peerReaperInterval:      confutil.DurationMin(conf.PeerReaperInterval, 100*time.Millisecond, *pldconf.TransportManagerDefaults.PeerReaperInterval),
		quiesceTimeout:          1 * time.Second, // not currently tunable (considered very small edge case)
		reliableMessagePageSize: 100,             // not currently tunable
	}
	tm.bgCtx, tm.cancelCtx = context.WithCancel(bgCtx)
	return tm
}

func (tm *transportManager) PreInit(pic components.PreInitComponents) (*components.ManagerInitResult, error) {
	if tm.localNodeName == "" {
		return nil, i18n.NewError(tm.bgCtx, msgs.MsgTransportNodeNameNotConfigured)
	}
	tm.initRPC()
	return &components.ManagerInitResult{
		RPCModules: []*rpcserver.RPCModule{tm.rpcModule},
	}, nil
}

func (tm *transportManager) PostInit(c components.AllComponents) error {
	// Asserted to be thread safe to do initialization here without lock, as it's before the
	// plugin manager starts, and thus before any domain would have started any go-routine
	// that could have cached a nil value in memory.
	tm.registryManager = c.RegistryManager()
	tm.stateManager = c.StateManager()
	tm.domainManager = c.DomainManager()
	tm.keyManager = c.KeyManager()
	tm.txManager = c.TxManager()
	tm.privateTxManager = c.PrivateTxManager()
	tm.identityResolver = c.IdentityResolver()
	tm.groupManager = c.GroupManager()
	tm.persistence = c.Persistence()
	tm.reliableMsgWriter = flushwriter.NewWriter(tm.bgCtx, tm.handleReliableMsgBatch, tm.persistence,
		&tm.conf.ReliableMessageWriter, &pldconf.TransportManagerDefaults.ReliableMessageWriter)
	return nil
}

func (tm *transportManager) Start() error {
	tm.peerReaperDone = make(chan struct{})
	tm.reliableMsgWriter.Start()
	go tm.peerReaper()
	return nil
}

func (tm *transportManager) Stop() {

	peers := tm.listActivePeers()
	for _, p := range peers {
		tm.reapPeer(p)
	}

	tm.mux.Lock()
	var allTransports []*transport
	for _, t := range tm.transportsByID {
		allTransports = append(allTransports, t)
	}
	tm.mux.Unlock()
	for _, t := range allTransports {
		tm.cleanupTransport(t)
	}

	tm.cancelCtx()
	if tm.peerReaperDone != nil {
		<-tm.peerReaperDone
	}

	tm.reliableMsgWriter.Shutdown()

}

func (tm *transportManager) cleanupTransport(t *transport) {
	// must not hold the transport lock when running this
	t.close()
	delete(tm.transportsByID, t.id)
	delete(tm.transportsByName, t.name)
}

func (tm *transportManager) ConfiguredTransports() map[string]*pldconf.PluginConfig {
	pluginConf := make(map[string]*pldconf.PluginConfig)
	for name, conf := range tm.conf.Transports {
		pluginConf[name] = &conf.Plugin
	}
	return pluginConf
}

func (tm *transportManager) getTransportNames() []string {
	tm.mux.Lock()
	defer tm.mux.Unlock()

	transportNames := make([]string, 0, len(tm.transportsByName))
	for transportName := range tm.transportsByName {
		transportNames = append(transportNames, transportName)
	}
	return transportNames
}

func (tm *transportManager) getTransportByName(ctx context.Context, transportName string) (*transport, error) {
	tm.mux.Lock()
	defer tm.mux.Unlock()

	t := tm.transportsByName[transportName]
	if t == nil {
		return nil, i18n.NewError(ctx, msgs.MsgTransportNotFound, transportName)
	}
	return t, nil
}

func (tm *transportManager) getLocalTransportDetails(ctx context.Context, transportName string) (string, error) {
	t, err := tm.getTransportByName(ctx, transportName)
	if err != nil {
		return "", err
	}
	return t.getLocalDetails(ctx)
}

func (tm *transportManager) TransportRegistered(name string, id uuid.UUID, toTransport components.TransportManagerToTransport) (fromTransport plugintk.TransportCallbacks, err error) {
	tm.mux.Lock()
	defer tm.mux.Unlock()

	// Replaces any previously registered instance
	existing := tm.transportsByName[name]
	for existing != nil {
		// Can't hold the lock in cleanup, hence the loop
		tm.mux.Unlock()
		tm.cleanupTransport(existing)
		tm.mux.Lock()
		existing = tm.transportsByName[name]
	}

	// Get the config for this transport
	conf := tm.conf.Transports[name]
	if conf == nil {
		// Shouldn't be possible
		return nil, i18n.NewError(tm.bgCtx, msgs.MsgTransportNotFound, name)
	}

	// Initialize
	t := tm.newTransport(id, name, conf, toTransport)
	tm.transportsByID[id] = t
	tm.transportsByName[name] = t
	go t.init()
	return t, nil
}

func (tm *transportManager) LocalNodeName() string {
	return tm.localNodeName
}

// See docs in components package
func (tm *transportManager) Send(ctx context.Context, send *components.FireAndForgetMessageSend) error {

	// Check the message is valid
	if len(send.Payload) == 0 {
		log.L(ctx).Errorf("Invalid message send request %+v", send)
		return i18n.NewError(ctx, msgs.MsgTransportInvalidMessage)
	}

	if send.MessageID == nil {
		msgID := uuid.New()
		send.MessageID = &msgID
	}
	msg := &prototk.PaladinMsg{
		MessageId:   send.MessageID.String(),
		MessageType: send.MessageType,
		Component:   send.Component,
		Payload:     send.Payload,
	}
	if send.CorrelationID != nil {
		cidStr := send.CorrelationID.String()
		msg.CorrelationId = &cidStr
	}

	return tm.queueFireAndForget(ctx, send.Node, msg)
}

func (tm *transportManager) queueFireAndForget(ctx context.Context, nodeName string, msg *prototk.PaladinMsg) error {
	// Use or establish a p connection for the send
	p, err := tm.getPeer(ctx, nodeName, true)
	if err == nil {
		err = p.transport.checkInit(ctx)
	}
	if err != nil {
		return err
	}

	// Push the send to the peer - this is a best effort interaction.
	// There is some retry in the Paladin layer, and some transports provide resilience.
	// However, the send is at-most-once, and the higher level message protocols that
	// use this "send" must be fault tolerant to message loss.
	select {
	case p.sendQueue <- msg:
		log.L(ctx).Debugf("queued %s message %s (cid=%v) to %s", msg.MessageType, msg.MessageId, pldtypes.StrOrEmpty(msg.CorrelationId), p.Name)
		return nil
	case <-ctx.Done():
		return i18n.NewError(ctx, msgs.MsgContextCanceled)
	}

}

// See docs in components package
func (tm *transportManager) SendReliable(ctx context.Context, dbTX persistence.DBTX, msgs ...*pldapi.ReliableMessage) (err error) {

	peers := make(map[string]*peer)
	for _, msg := range msgs {
		var p *peer

		msg.ID = uuid.New()
		msg.Created = pldtypes.TimestampNow()
		_, err = msg.MessageType.Validate()

		if err == nil {
			p, err = tm.getPeer(ctx, msg.Node, true)
		}

		if err != nil {
			return err
		}

		peers[p.Name] = p
	}

	if err == nil {
		err = dbTX.DB().
			WithContext(ctx).
			Create(msgs).
			Error
	}

	if err != nil {
		return err
	}

	dbTX.AddPostCommit(func(ctx context.Context) {
		for _, p := range peers {
			p.notifyPersistedMsgAvailable()
		}
	})
	return nil

}

func (tm *transportManager) writeAcks(ctx context.Context, dbTX persistence.DBTX, acks ...*pldapi.ReliableMessageAck) error {
	for _, ack := range acks {
		log.L(ctx).Infof("ack received for message %s", ack.MessageID)
		ack.Time = pldtypes.TimestampNow()
	}
	return dbTX.DB().
		WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(acks).
		Error
}

//nolint:unused // May be used in future
func (tm *transportManager) getReliableMessageByID(ctx context.Context, dbTX persistence.DBTX, id uuid.UUID) (*pldapi.ReliableMessage, error) {
	var rms []*pldapi.ReliableMessage
	err := dbTX.DB().
		WithContext(ctx).
		Order("sequence ASC").
		Joins("Ack").
		Where(`"reliable_msgs"."id" = ?`, id).
		Limit(1).
		Find(&rms).
		Error
	if err != nil || len(rms) < 1 {
		return nil, err
	}
	return rms[0], nil
}

func (tm *transportManager) QueryReliableMessages(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.ReliableMessage, error) {
	qw := &filters.QueryWrapper[pldapi.ReliableMessage, pldapi.ReliableMessage]{
		P:           tm.persistence,
		DefaultSort: "-sequence",
		Filters:     reliableMessageFilters,
		Query:       jq,
		Finalize: func(db *gorm.DB) *gorm.DB {
			return db.Joins("Ack")
		},
		MapResult: func(msg *pldapi.ReliableMessage) (*pldapi.ReliableMessage, error) {
			return msg, nil
		},
	}
	return qw.Run(ctx, dbTX)
}

func (tm *transportManager) QueryReliableMessageAcks(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.ReliableMessageAck, error) {
	qw := &filters.QueryWrapper[pldapi.ReliableMessageAck, pldapi.ReliableMessageAck]{
		P:           tm.persistence,
		DefaultSort: "-time",
		Filters:     reliableMessageAckFilters,
		Query:       jq,
		MapResult: func(ack *pldapi.ReliableMessageAck) (*pldapi.ReliableMessageAck, error) {
			return ack, nil
		},
	}
	return qw.Run(ctx, dbTX)
}
