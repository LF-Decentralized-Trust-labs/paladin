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
	"sort"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type peer struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	name      string
	tm        *transportManager
	transport *transport

	persistedMsgsAvailable chan struct{}
	sendQueue              chan *prototk.Message

	done chan struct{}
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
		log.L(ctx).Debugf("connection already active for peer '%s' (aft4er connection race)", nodeName)
		return p, nil
	}

	// We need to resolve the node transport, and build a new connection
	log.L(ctx).Debugf("attempting connection for peer '%s'", nodeName)
	p = &peer{
		tm:                     tm,
		name:                   nodeName,
		persistedMsgsAvailable: make(chan struct{}, 1),
		sendQueue:              make(chan *prototk.Message, tm.senderBufferLen),
		done:                   make(chan struct{}),
	}
	p.ctx, p.cancelCtx = context.WithCancel(
		log.WithLogField(tm.bgCtx /* go-routine need bg context*/, "peer", nodeName))

	if nodeName == "" || nodeName == tm.localNodeName {
		return nil, i18n.NewError(p.ctx, msgs.MsgTransportInvalidDestinationSend, tm.localNodeName, nodeName)
	}

	// Note the registry is responsible for caching to make this call as efficient as if
	// we maintained the transport details in-memory ourselves.
	registeredTransportDetails, err := tm.registryManager.GetNodeTransports(p.ctx, nodeName)
	if err != nil {
		return nil, err
	}

	// See if any of the transports registered by the node, are configured on this local node
	// Note: We just pick the first one if multiple are available, and there is no retry to
	//       fallback to a secondary one currently.
	for _, rtd := range registeredTransportDetails {
		p.transport = tm.transportsByName[rtd.Transport]
	}
	if p.transport == nil {
		// If we didn't find one, then feedback to the caller which transports were registered
		registeredTransportNames := []string{}
		for _, rtd := range registeredTransportDetails {
			registeredTransportNames = append(registeredTransportNames, rtd.Transport)
		}
		return nil, i18n.NewError(p.ctx, msgs.MsgTransportNoTransportsConfiguredForNode, nodeName, registeredTransportNames)
	}

	log.L(ctx).Debugf("connected to peer '%s'", nodeName)
	tm.peers[nodeName] = p
	return p, nil
}

func (p *peer) notifyPersistedMsgAvailable() {
	select {
	case p.persistedMsgsAvailable <- struct{}{}:
	default:
	}
}

func (p *peer) send(ctx context.Context, msg *components.TransportMessage) error {

	// Convert the message to the protobuf transport payload
	var correlID *string
	if msg.CorrelationID != nil {
		correlID = confutil.P(msg.CorrelationID.String())
	}
	pMsg := &prototk.Message{
		MessageType:   msg.MessageType,
		MessageId:     msg.MessageID.String(),
		CorrelationId: correlID,
		Component:     msg.Component,
		Node:          msg.Node,
		ReplyTo:       msg.ReplyTo,
		Payload:       msg.Payload,
	}

	// Push onto the sender channel as a fire-and-forget message, for the
	// goroutine to handle (alongside dispatching persisted messages)
	select {
	case p.sendQueue <- pMsg:
		log.L(ctx).Debugf("sending %s message %s (cid=%v)", msg.MessageType, msg.MessageID, msg.CorrelationID)
		return nil
	case <-ctx.Done():
		return i18n.NewError(ctx, msgs.MsgContextCanceled)
	}
}

func (p *peer) sender() {
	defer close(p.done)

	log.L(p.ctx).Infof("peer %s active", p.name)

	for {
		select {
		case <-p.ctx.Done():
			log.L(p.ctx).Infof("peer %s inactive", p.name)
			return
		}
	}
}

func (p *peer) close() {
	p.cancelCtx()
	<-p.done
}
