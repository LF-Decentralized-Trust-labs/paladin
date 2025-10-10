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
	"sync"
	"sync/atomic"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"gorm.io/gorm/clause"
)

type peer struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	tm        *transportManager
	transport *transport // the transport mutually supported by us and the remote node

	pldapi.PeerInfo
	statsLock sync.Mutex

	persistedMsgsAvailable chan struct{}
	sendQueue              chan *prototk.PaladinMsg

	// Send loop state (no lock as only used on the loop)
	lastFullScan          time.Time
	lastDrainHWM          *uint64
	persistentMsgsDrained bool

	senderStarted atomic.Bool
	senderDone    chan struct{}
}

type nameSortedPeers []*peer

func (p nameSortedPeers) Len() int           { return len(p) }
func (p nameSortedPeers) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p nameSortedPeers) Less(i, j int) bool { return cmp.Less(p[i].Name, p[j].Name) }

func (tm *transportManager) getPeer(ctx context.Context, nodeName string, sending bool) (*peer, error) {

	if err := pldtypes.ValidateSafeCharsStartEndAlphaNum(ctx, nodeName, pldtypes.DefaultNameMaxLen, "node"); err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTransportInvalidTargetNode, nodeName)
	}
	if nodeName == tm.localNodeName {
		return nil, i18n.NewError(ctx, msgs.MsgTransportSendLocalNode, tm.localNodeName)
	}

	// Hopefully this is an already active connection
	p := tm.getActivePeer(nodeName)
	if p != nil && (p.senderStarted.Load() || !sending) {
		// Already active and obtained via read-lock
		log.L(ctx).Debugf("connection already active for peer '%s'", nodeName)
		return p, nil
	}

	return tm.connectPeer(ctx, nodeName, sending)
}

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

func (tm *transportManager) listActivePeerInfo() []*pldapi.PeerInfo {
	peers := tm.listActivePeers()
	peerInfo := make([]*pldapi.PeerInfo, len(peers))
	for i, p := range peers {
		peerInfo[i] = &p.PeerInfo
	}
	return peerInfo
}

func (tm *transportManager) getPeerInfo(nodeName string) *pldapi.PeerInfo {
	peer := tm.getActivePeer(nodeName)
	if peer == nil {
		return nil
	}
	return &peer.PeerInfo
}

// efficient read-locked call to get an active peer connection
func (tm *transportManager) getActivePeer(nodeName string) *peer {
	tm.peersLock.RLock()
	defer tm.peersLock.RUnlock()
	return tm.peers[nodeName]
}

func (tm *transportManager) reapPeer(p *peer) {
	p.tm.peersLock.Lock()
	defer p.tm.peersLock.Unlock()
	delete(p.tm.peers, p.Name)

	// Close down the peer
	log.L(p.ctx).Infof("peer %s deactivating", p.Name)
	p.close()

	if p.senderStarted.Load() {
		// Holding the lock while activating/deactivating ensures we never dual-activate in the transport
		if _, err := p.transport.api.DeactivatePeer(p.ctx, &prototk.DeactivatePeerRequest{
			NodeName: p.Name,
		}); err != nil {
			log.L(p.ctx).Warnf("peer %s returned deactivation error: %s", p.Name, err)
		}
	}

}

func (tm *transportManager) peerReaper() {
	defer close(tm.peerReaperDone)

	for {
		select {
		case <-tm.bgCtx.Done():
			log.L(tm.bgCtx).Debugf("peer reaper exiting")
			return
		case <-time.After(tm.peerReaperInterval):
		}

		candidates := tm.listActivePeers()
		var reaped []*peer
		for _, p := range candidates {
			if p.isInactive() {
				tm.reapPeer(p)
				reaped = append(reaped, p)
			}
		}
		log.L(tm.bgCtx).Debugf("peer reaper before=%d reaped=%d", len(candidates), len(reaped))
	}
}

func (tm *transportManager) connectPeer(ctx context.Context, nodeName string, sending bool) (*peer, error) {
	// Race to grab the write-lock and race to connect
	tm.peersLock.Lock()
	defer tm.peersLock.Unlock()
	p := tm.peers[nodeName]
	if p != nil && (p.senderStarted.Load() || !sending) {
		// There was a race to connect to this peer, and the other routine won
		log.L(ctx).Debugf("connection already active for peer '%s' (after connection race)", nodeName)
		return p, nil
	}

	if p == nil {
		// We need to resolve the node transport, and build a new connection
		log.L(ctx).Debugf("activating new peer '%s'", nodeName)
		p = &peer{
			tm: tm,
			PeerInfo: pldapi.PeerInfo{
				Name:      nodeName,
				Activated: pldtypes.TimestampNow(),
			},
			persistedMsgsAvailable: make(chan struct{}, 1),
			sendQueue:              make(chan *prototk.PaladinMsg, tm.senderBufferLen),
			senderDone:             make(chan struct{}),
		}
		p.ctx, p.cancelCtx = context.WithCancel(
			log.WithLogField(tm.bgCtx /* go-routine need bg context*/, "peer", nodeName))
	}
	tm.peers[nodeName] = p

	if sending {
		p.OutboundTransport, p.OutboundError = p.startSender()
		if p.OutboundError != nil {
			// Note the peer is still in our list, but not connected for send.
			// This means status can be reported for it.
			return nil, p.OutboundError
		}
	}

	return p, nil
}

func (p *peer) startSender() (string, error) {
	// Note the registry is responsible for caching to make this call as efficient as if
	// we maintained the transport details in-memory ourselves.
	registeredTransportDetails, err := p.tm.registryManager.GetNodeTransports(p.ctx, p.Name)
	if err != nil {
		return "", err
	}

	// See if any of the transports registered by the node, are configured on this local node
	// Note: We just pick the first one if multiple are available, and there is no retry to
	//       fallback to a secondary one currently.
	var remoteTransportDetails string
	for _, rtd := range registeredTransportDetails {
		p.transport = p.tm.transportsByName[rtd.Transport]
		remoteTransportDetails = rtd.Details
	}
	if p.transport == nil {
		// If we didn't find one, then feedback to the caller which transports were registered
		registeredTransportNames := []string{}
		for _, rtd := range registeredTransportDetails {
			registeredTransportNames = append(registeredTransportNames, rtd.Transport)
		}
		return "", i18n.NewError(p.ctx, msgs.MsgTransportNoTransportsConfiguredForNode, p.Name, registeredTransportNames)
	}

	// Activate the connection (the deactivate is deferred to the send loop)
	res, err := p.transport.api.ActivatePeer(p.ctx, &prototk.ActivatePeerRequest{
		NodeName:         p.Name,
		TransportDetails: remoteTransportDetails,
	})
	if err != nil {
		return p.transport.name, err
	}
	if err = json.Unmarshal([]byte(res.PeerInfoJson), &p.Outbound); err != nil {
		// We've already activated at this point, so we need to keep going - but this
		// will mean there's no peer info, so we put it in as a string
		log.L(p.ctx).Warnf("Invalid peerInfo: %s", res.PeerInfoJson)
		p.Outbound = map[string]any{"info": string(res.PeerInfoJson)}
	}

	log.L(p.ctx).Debugf("connected to peer '%s'", p.Name)
	p.senderStarted.Store(true)
	go p.sender()
	return p.transport.name, nil
}

func (p *peer) notifyPersistedMsgAvailable() {
	select {
	case p.persistedMsgsAvailable <- struct{}{}:
	default:
	}
}

func (p *peer) send(msg *prototk.PaladinMsg, reliableSeq *uint64) error {
	err := p.tm.sendShortRetry.Do(p.ctx, func(attempt int) (retryable bool, err error) {
		return true, p.transport.send(p.ctx, p.Name, msg)
	})
	log.L(p.ctx).Infof("Sent %s/%s message %s to %s (cid=%s)", msg.Component.String(), msg.MessageType, msg.MessageId, p.Name, pldtypes.StrOrEmpty(msg.CorrelationId))
	if err == nil {
		now := pldtypes.TimestampNow()
		p.statsLock.Lock()
		defer p.statsLock.Unlock()
		p.Stats.LastSend = &now
		p.Stats.SentMsgs++
		p.Stats.SentBytes += uint64(len(msg.Payload))
		if reliableSeq != nil && *reliableSeq > p.Stats.ReliableHighestSent {
			p.Stats.ReliableHighestSent = *reliableSeq
		}
		if p.lastDrainHWM != nil {
			p.Stats.ReliableAckBase = *p.lastDrainHWM
		}
	}
	return err
}

func (p *peer) updateReceivedStats(msg *prototk.PaladinMsg) {
	log.L(p.ctx).Infof("Received %s/%s message %s from %s (cid=%s)", msg.Component.String(), msg.MessageType, msg.MessageId, p.Name, pldtypes.StrOrEmpty(msg.CorrelationId))

	now := pldtypes.TimestampNow()
	p.statsLock.Lock()
	defer p.statsLock.Unlock()
	p.Stats.LastReceive = &now
	p.Stats.ReceivedMsgs++
	p.Stats.ReceivedBytes += uint64(len(msg.Payload))
}

func (p *peer) reliableMessageScan(checkNew bool) error {

	fullScan := p.lastDrainHWM == nil || time.Since(p.lastFullScan) >= p.tm.reliableMessageResend
	if !fullScan && !checkNew {
		return nil // Nothing to do
	}

	pageSize := p.tm.reliableMessagePageSize
	var total = 0
	var lastPageEnd *uint64
	for {
		query := p.tm.persistence.DB().
			WithContext(p.ctx).
			Order("sequence ASC").
			Joins("Ack").
			Where(`"Ack"."time" IS NULL`).
			Where("node", p.Name).
			Limit(pageSize)
		if lastPageEnd != nil {
			query = query.Where("sequence > ?", *lastPageEnd)
		} else if !fullScan {
			query = query.Where("sequence > ?", *p.lastDrainHWM)
		}

		var page []*pldapi.ReliableMessage
		err := query.Find(&page).Error
		if err != nil {
			return err
		}

		// Process the page - building and sending the proto messages
		if err = p.processReliableMsgPage(p.tm.persistence.NOTX(), page); err != nil {
			// Errors returned are retryable - for data errors the function
			// must record those as acks with an error.
			return err
		}

		if len(page) > 0 {
			p.persistentMsgsDrained = false // we know there's some messages
			total += len(page)
			lastPageEnd = &page[len(page)-1].Sequence
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

func (p *peer) processReliableMsgPage(dbTX persistence.DBTX, page []*pldapi.ReliableMessage) (err error) {

	type paladinMsgWithSeq struct {
		*prototk.PaladinMsg
		seq uint64
	}

	// Build the messages
	msgsToSend := make([]paladinMsgWithSeq, 0, len(page))
	var errorAcks []*pldapi.ReliableMessageAck
	for _, rm := range page {

		// Check it's either after our HWM, or eligible for re-send
		afterHWM := p.lastDrainHWM == nil || *p.lastDrainHWM < rm.Sequence
		if !afterHWM && time.Since(rm.Created.Time()) < p.tm.reliableMessageResend {
			log.L(p.ctx).Infof("Unacknowledged message %s not yet eligible for re-send", rm.ID)
			continue
		}

		// Process it
		var msg *prototk.PaladinMsg
		var errorAck error
		switch rm.MessageType.V() {
		case pldapi.RMTState:
			msg, errorAck, err = p.tm.buildStateDistributionMsg(p.ctx, dbTX, rm)
		case pldapi.RMTPrivacyGroup:
			msg, errorAck, err = p.tm.buildPrivacyGroupDistributionMsg(p.ctx, dbTX, rm)
		case pldapi.RMTPrivacyGroupMessage:
			msg, errorAck, err = p.tm.buildPrivacyGroupMessageMsg(p.ctx, dbTX, rm)
		case pldapi.RMTReceipt:
			msg, errorAck, err = p.tm.buildReceiptDistributionMsg(p.ctx, dbTX, rm)
		default:
			errorAck = i18n.NewError(p.ctx, msgs.MsgTransportUnsupportedReliableMsg, rm.MessageType)
		}
		switch {
		case err != nil:
			return err
		case errorAck != nil:
			log.L(p.ctx).Errorf("Unable to send reliable message %s - writing persistent error: %s", rm.ID, errorAck)
			errorAcks = append(errorAcks, &pldapi.ReliableMessageAck{
				MessageID: rm.ID,
				Time:      pldtypes.TimestampNow(),
				Error:     errorAck.Error(),
			})
		case msg != nil:
			msgsToSend = append(msgsToSend, paladinMsgWithSeq{
				seq:        rm.Sequence,
				PaladinMsg: msg,
			})
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
		if err := p.send(msg.PaladinMsg, &msg.seq); err != nil {
			return err
		}
	}

	return nil

}

func (p *peer) sender() {
	defer close(p.senderDone)

	log.L(p.ctx).Infof("peer %s active", p.Name)

	checkNew := false
	for {

		// We send/resend any reliable messages queued up first
		err := p.tm.reliableScanRetry.Do(p.ctx, func(attempt int) (retryable bool, err error) {
			return true, p.reliableMessageScan(checkNew)
		})
		if err != nil {
			return // context closed
		}
		checkNew = false

		// Our wait timeout needs to be the shortest of:
		// - The full re-scan timeout for reliable messages
		// - The inactivity timeout
		resendTimer := time.NewTimer(p.tm.reliableMessageResend)
		processingMsgs := true
		for processingMsgs {
			select {
			case <-resendTimer.C:
				processingMsgs = false // spin round and check if we have persisted messages to (re)process
			case <-p.persistedMsgsAvailable:
				resendTimer.Stop()
				checkNew = true
				processingMsgs = false // spin round and get the messages
			case <-p.ctx.Done():
				resendTimer.Stop()
				return // we're done
			case msg := <-p.sendQueue:
				resendTimer.Stop()
				// send and spin straight round
				if err := p.send(msg, nil); err != nil {
					log.L(p.ctx).Errorf("failed to send message '%s' after short retry (discarding): %s", msg.MessageId, err)
				}
			}
		}
	}
}

func (p *peer) isInactive() bool {
	p.statsLock.Lock()
	defer p.statsLock.Unlock()

	now := time.Now()
	return (p.Stats.LastSend == nil || now.Sub(p.Stats.LastSend.Time()) > p.tm.peerInactivityTimeout) &&
		(p.Stats.LastReceive == nil || now.Sub(p.Stats.LastReceive.Time()) > p.tm.peerInactivityTimeout)
}

func (p *peer) close() {
	p.cancelCtx()
	if p.senderStarted.Load() {
		<-p.senderDone
	}
}
