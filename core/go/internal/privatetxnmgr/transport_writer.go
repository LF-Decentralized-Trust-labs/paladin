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

package privatetxnmgr

import (
	"context"
	"encoding/json"

	"github.com/kaleido-io/paladin/core/internal/components"
	pb "github.com/kaleido-io/paladin/core/pkg/proto/engine"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"google.golang.org/protobuf/proto"
)

func NewTransportWriter(domainName string, contractAddress *tktypes.EthAddress, nodeID string, transportManager components.TransportManager) *transportWriter {
	return &transportWriter{
		nodeID:           nodeID,
		transportManager: transportManager,
		domainName:       domainName,
		contractAddress:  contractAddress,
	}
}

type transportWriter struct {
	nodeID           string
	transportManager components.TransportManager
	domainName       string
	contractAddress  *tktypes.EthAddress
}

func (tw *transportWriter) SendDelegationRequest(
	ctx context.Context,
	delegationId string,
	delegateNodeId string,
	transaction *components.PrivateTransaction,

) error {

	transactionBytes, err := json.Marshal(transaction)

	if err != nil {
		log.L(ctx).Errorf("Error marshalling transaction message: %s", err)
		return err
	}
	delegationRequest := &pb.DelegationRequest{
		DelegationId:       delegationId,
		TransactionId:      transaction.ID.String(),
		DelegateNodeId:     delegateNodeId,
		PrivateTransaction: transactionBytes,
	}
	delegationRequestBytes, err := proto.Marshal(delegationRequest)
	if err != nil {
		log.L(ctx).Errorf("Error marshalling delegationRequest  message: %s", err)
		return err
	}

	if err = tw.transportManager.Send(ctx, &components.TransportMessage{
		MessageType: "DelegationRequest",
		Payload:     delegationRequestBytes,
		Component:   PRIVATE_TX_MANAGER_DESTINATION,
		Node:        delegateNodeId,
		ReplyTo:     tw.nodeID,
	}); err != nil {
		return err
	}
	return nil
}

func (tw *transportWriter) SendDelegateTransactionMessage(ctx context.Context, transactionId string, delegateNodeId string) error {
	//This is deprecated in favour of SendDelegationRequest.  all callers of this are due to be refactored away
	//leaving here as a no-op temporarily to avoid breaking the build

	return nil
}

func (tw *transportWriter) SendState(ctx context.Context, stateId string, schemaId string, stateDataJson string, party string) error {
	stateProducedEvent := &pb.StateProducedEvent{
		DomainName:      tw.domainName,
		ContractAddress: tw.contractAddress.String(),
		SchemaId:        schemaId,
		StateId:         stateId,
		StateDataJson:   stateDataJson,
		Party:           party,
	}
	stateProducedEventBytes, err := proto.Marshal(stateProducedEvent)
	if err != nil {
		log.L(ctx).Errorf("Error marshalling state distribution message: %s", err)
		return err
	}

	targetNode, err := tktypes.PrivateIdentityLocator(party).Node(ctx, false)
	if err != nil {
		log.L(ctx).Errorf("Error getting node for party %s", party)
		return err
	}

	err = tw.transportManager.Send(ctx, &components.TransportMessage{
		MessageType: "StateProducedEvent",
		Payload:     stateProducedEventBytes,
		Component:   PRIVATE_TX_MANAGER_DESTINATION,
		Node:        targetNode,
		ReplyTo:     tw.nodeID,
	})
	if err != nil {
		log.L(ctx).Errorf("Error sending state produced event: %s", err)
		return err
	}

	return nil
}
