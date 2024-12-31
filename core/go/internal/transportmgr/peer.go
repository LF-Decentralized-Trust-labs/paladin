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
	"cmp"
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm/clause"
)

type peer struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	name      string
	tm        *transportManager
	transport *transport     // the transport mutually supported by us and the remote node
	peerInfo  map[string]any // opaque JSON object from the transport

	persistedMsgsAvailable chan struct{}
	sendQueue              chan *prototk.PaladinMsg

	// Send loop state (no lock as only used on the loop)
	lastFullScan          time.Time
	lastDrainHWM          *tktypes.Timestamp
	persistentMsgsDrained bool

	quiescing bool
	done      chan struct{}
}

type nameSortedPeers []*peer

func (p nameSortedPeers) Len() int           { return len(p) }
func (p nameSortedPeers) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p nameSortedPeers) Less(i, j int) bool { return cmp.Less(p[i].name, p[j].name) }

// get a list of all active peers
func (tm *transportManager) listActivePeers() nameSortedPeers {
	tm.peersLock.RLock()
	defer tm.peersLock.RUnlock()
	peers := make(nameSortedPeers, 0, len(tm.peers))
	for _, p := range tm.peers {
		peers = append(peers, p)
	}
	sort.Sort(peers)
	return peers
}

// efficient read-locked call to get an active peer connection
func (tm *transportManager) getActivePeer(nodeName string) *peer {
	tm.peersLock.RLock()
	defer tm.peersLock.RUnlock()
	return tm.peers[nodeName]
}

func (tm *transportManager) getPeer(ctx context.Context, nodeName string) (*peer, error) {

	if err := tktypes.ValidateSafeCharsStartEndAlphaNum(ctx, nodeName, tktypes.DefaultNameMaxLen, "node"); err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTransportInvalidTargetNode, nodeName)
	}
	if nodeName == tm.localNodeName {
		return nil, i18n.NewError(ctx, msgs.MsgTransportSendLocalNode, tm.localNodeName)
	}

	// Hopefully this is an already active connection
	p := tm.getActivePeer(nodeName)
	if p != nil {
		// Already active and obtained via read-lock
		log.L(ctx).Debugf("connection already active for peer '%s'", nodeName)
		return p, nil
	}

	// Otherwise take the write-lock and race to connect
	tm.peersLock.Lock()
	defer tm.peersLock.Unlock()
	p = tm.peers[nodeName]
	if p != nil {
		// There was a race to connect to this peer, and the other routine won
		log.L(ctx).Debugf("connection already active for peer '%s' (after connection race)", nodeName)
		return p, nil
	}

	// We need to resolve the node transport, and build a new connection
	log.L(ctx).Debugf("attempting connection for peer '%s'", nodeName)
	p = &peer{
		tm:                     tm,
		name:                   nodeName,
		persistedMsgsAvailable: make(chan struct{}, 1),
		sendQueue:              make(chan *prototk.PaladinMsg, tm.senderBufferLen),
		done:                   make(chan struct{}),
	}
	p.ctx, p.cancelCtx = context.WithCancel(
		log.WithLogField(tm.bgCtx /* go-routine need bg context*/, "peer", nodeName))

	// Note the registry is responsible for caching to make this call as efficient as if
	// we maintained the transport details in-memory ourselves.
	registeredTransportDetails, err := tm.registryManager.GetNodeTransports(p.ctx, nodeName)
	if err != nil {
		return nil, err
	}

	// See if any of the transports registered by the node, are configured on this local node
	// Note: We just pick the first one if multiple are available, and there is no retry to
	//       fallback to a secondary one currently.
	var remoteTransportDetails string
	for _, rtd := range registeredTransportDetails {
		p.transport = tm.transportsByName[rtd.Transport]
		remoteTransportDetails = rtd.Details
	}
	if p.transport == nil {
		// If we didn't find one, then feedback to the caller which transports were registered
		registeredTransportNames := []string{}
		for _, rtd := range registeredTransportDetails {
			registeredTransportNames = append(registeredTransportNames, rtd.Transport)
		}
		return nil, i18n.NewError(p.ctx, msgs.MsgTransportNoTransportsConfiguredForNode, nodeName, registeredTransportNames)
	}

	// Activate the connection (the deactivate is deferred to the send loop)
	res, err := p.transport.api.ActivateNode(ctx, &prototk.ActivateNodeRequest{
		NodeName:         nodeName,
		TransportDetails: remoteTransportDetails,
	})
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(res.PeerInfoJson), &p.peerInfo); err != nil {
		log.L(ctx).Errorf("Invalid peerInfo: %s", p.peerInfo)
		return nil, i18n.NewError(ctx, msgs.MsgTransportInvalidPeerInfo)
	}

	log.L(ctx).Debugf("connected to peer '%s'", nodeName)
	tm.peers[nodeName] = p
	go p.sender()
	return p, nil
}

func (p *peer) notifyPersistedMsgAvailable() {
	select {
	case p.persistedMsgsAvailable <- struct{}{}:
	default:
	}
}

func (p *peer) stateDistributionMsg(rm *components.ReliableMessage, sd *components.StateDistributionWithData) *prototk.PaladinMsg {
	payload, _ := json.Marshal(sd)
	return &prototk.PaladinMsg{
		MessageId:   rm.ID.String(),
		MessageType: "StateProducedEvent",
		Payload:     payload,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
	}
}

func (p *peer) send(msg *prototk.PaladinMsg) error {
	return p.tm.sendShortRetry.Do(p.ctx, func(attempt int) (retryable bool, err error) {
		return true, p.transport.send(p.ctx, p.name, msg)
	})
}

func (p *peer) senderDone() {
	p.deactivate()

	// There's a very small window where we might have got delivered a message by a routine
	// that got us out of the map before we deactivated.
	// In this edge case, we need to spin off the new peer connection to replace us.
	for p.quiescing {
		select {
		case msg := <-p.sendQueue:
			log.L(p.ctx).Infof("message delivered in inactivity quiesce window. Re-connecting")
			_ = p.tm.queueFireAndForget(p.ctx, p.name, msg)
		case <-p.persistedMsgsAvailable:
			log.L(p.ctx).Infof("reliable message delivered in inactivity quiesce window. Re-connecting")
			_, _ = p.tm.getPeer(p.ctx, p.name)
		case <-time.After(p.tm.quiesceTimeout):
			p.quiescing = false
		}
	}

	close(p.done)
}

func (p *peer) deactivate() {
	// Hold the peers write lock to do this
	p.tm.peersLock.Lock()
	defer p.tm.peersLock.Unlock()
	delete(p.tm.peers, p.name)

	// Holding the lock while activating/deactivating ensures we never dual-activate in the transport
	log.L(p.ctx).Infof("peer %s deactivating", p.name)
	if _, err := p.transport.api.DeactivateNode(p.ctx, &prototk.DeactivateNodeRequest{
		NodeName: p.name,
	}); err != nil {
		log.L(p.ctx).Warnf("peer %s returned deactivation error: %s", p.name, err)
	}
}

func (p *peer) reliableMessageScan() error {

	checkNew := true
	fullScan := p.lastDrainHWM == nil || time.Since(p.lastFullScan) >= p.tm.reliableMessageResend
	select {
	case <-p.persistedMsgsAvailable:
		checkNew = true
	default:
	}

	if !fullScan && !checkNew {
		return nil // Nothing to do
	}

	const pageSize = 100

	var total = 0
	var lastPageEnd *tktypes.Timestamp
	for {
		query := p.tm.persistence.DB().
			WithContext(p.ctx).
			Order("created ASC").
			Joins("Ack").
			Where(`"Ack"."time" IS NULL`).
			Limit(pageSize)
		if lastPageEnd != nil {
			query = query.Where("created > ?", *lastPageEnd)
		} else if !fullScan {
			query = query.Where("created > ?", *p.lastDrainHWM)
		}

		var page []*components.ReliableMessage
		err := query.Find(&page).Error
		if err != nil {
			return err
		}

		// Process the page - building and sending the proto messages
		if err = p.processReliableMsgPage(page); err != nil {
			// Errors returned are retryable - for data errors the function
			// must record those as acks with an error.
			return err
		}

		if len(page) > 0 {
			p.persistentMsgsDrained = false // we know there's some messages
			total += len(page)
			lastPageEnd = &page[len(page)-1].Created
		}

		// If we didn't have a full page, then we're done
		if len(page) < pageSize {
			break
		}

	}

	log.L(p.ctx).Debugf("reliableMessageScan fullScan=%t total=%d lastPageEnd=%v", fullScan, total, lastPageEnd)

	// If we found anything, then mark that as our high water mark for
	// future scans. If an empty full scan - then we store nil
	if lastPageEnd != nil || fullScan {
		p.lastDrainHWM = lastPageEnd
	}

	// Record the last full scan
	if fullScan {
		// We only know we're empty when we do a full re-scan, and that comes back empty
		p.persistentMsgsDrained = (total == 0)

		p.lastFullScan = time.Now()
	}

	return nil
}

func (p *peer) buildStateDistributionMsg(rm *components.ReliableMessage) (*prototk.PaladinMsg, error, error) {

	// Validate the message first (not retryable)
	var sd components.StateDistributionWithData
	var stateID tktypes.HexBytes
	var contractAddr *tktypes.EthAddress
	parseErr := json.Unmarshal(rm.Metadata, &sd)
	if parseErr == nil {
		stateID, parseErr = tktypes.ParseHexBytes(p.ctx, sd.StateID)
	}
	if parseErr == nil {
		contractAddr, parseErr = tktypes.ParseEthAddress(sd.ContractAddress)
	}
	if parseErr != nil {
		return nil, parseErr, nil
	}

	// Get the state - distinguishing between not found, vs. a retryable error
	state, err := p.tm.stateManager.GetState(p.ctx, p.tm.persistence.DB(), sd.Domain, *contractAddr, stateID, false, false)
	if err != nil {
		return nil, nil, err
	}
	if state == nil {
		return nil,
			i18n.NewError(p.ctx, msgs.MsgTransportStateNotAvailableLocally, sd.Domain, *contractAddr, stateID),
			nil
	}

	return nil, nil, nil
}

func (p *peer) processReliableMsgPage(page []*components.ReliableMessage) (err error) {

	// Build the messages
	msgsToSend := make([]*prototk.PaladinMsg, 0, len(page))
	var errorAcks []*components.ReliableMessageAck
	for _, rm := range page {

		// Check it's either after our HWM, or eligible for re-send
		afterHWM := p.lastDrainHWM == nil || *p.lastDrainHWM < rm.Created
		if !afterHWM && time.Since(rm.Created.Time()) < p.tm.reliableMessageResend {
			log.L(p.ctx).Infof("Unacknowledged message %s not yet eligible for re-send", rm.ID)
			continue
		}

		// Process it
		var msg *prototk.PaladinMsg
		var errorAck error
		switch rm.MessageType.V() {
		case components.RMTState:
			msg, errorAck, err = p.buildStateDistributionMsg(rm)
		case components.RMTReceipt:
			// TODO: Implement for receipt distribution
			fallthrough
		default:
			errorAck = i18n.NewError(p.ctx, msgs.MsgTransportUnsupportedReliableMsg, rm.MessageType)
		}
		switch {
		case err != nil:
			return err
		case errorAck != nil:
			errorAcks = append(errorAcks, &components.ReliableMessageAck{
				MessageID: rm.ID,
				Time:      tktypes.TimestampNow(),
				Error:     errorAck.Error(),
			})
		case msg != nil:
			msgsToSend = append(msgsToSend, msg)
		}
	}

	// Persist any bad message failures
	if len(errorAcks) > 0 {
		err := p.tm.persistence.DB().
			WithContext(p.ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(errorAcks).
			Error
		if err != nil {
			return err
		}
	}

	// Send the messages, with short retry.
	// We fail the whole page on error, so we don't thrash (the outer infinite retry
	// gives a much longer maximum back-off).
	for _, msg := range msgsToSend {
		if err := p.send(msg); err != nil {
			return err
		}
	}

	return nil

}

func (p *peer) sender() {
	defer p.senderDone()

	log.L(p.ctx).Infof("peer %s active", p.name)

	hitInactivityTimeout := false
	for {

		// We send/resend any reliable messages queued up first
		err := p.tm.reliableScanRetry.Do(p.ctx, func(attempt int) (retryable bool, err error) {
			return true, p.reliableMessageScan()
		})
		if err != nil {
			return // context closed
		}

		// Depending on our persistent message status, check if we're able to quiesce
		if hitInactivityTimeout && p.persistentMsgsDrained {
			p.quiescing = true
			return // quiesce handling is in senderDone() deferred function
		}
		hitInactivityTimeout = false

		// Our wait timeout needs to be the shortest of:
		// - The full re-scan timeout for reliable messages
		// - The inactivity timeout
		inactivityTimeout := p.tm.reliableMessageResend
		if inactivityTimeout > p.tm.peerInactivityTimeout {
			inactivityTimeout = p.tm.peerInactivityTimeout
		}
		inactivityTimer := time.NewTimer(inactivityTimeout)
		processingMsgs := true
		for processingMsgs {
			select {
			case <-inactivityTimer.C:
				hitInactivityTimeout = true
				processingMsgs = false // spin round and check if we have persisted messages to (re)process
			case <-p.persistedMsgsAvailable:
				processingMsgs = false // spin round and get the messages
			case <-p.ctx.Done():
				return // we're done
			case msg := <-p.sendQueue:
				// send and spin straight round
				if err := p.send(msg); err != nil {
					log.L(p.ctx).Errorf("failed to send message '%s' after short retry (discarding): %s", msg.MessageId, err)
				}
			}
		}
	}
}

func (p *peer) close() {
	p.cancelCtx()
	<-p.done
}
