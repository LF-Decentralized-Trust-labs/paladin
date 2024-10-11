// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package identityresolver

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	pbIdentityResolver "github.com/kaleido-io/paladin/core/pkg/proto/identityresolver"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"google.golang.org/protobuf/proto"
)

type identityResolver struct {
	bgCtx                 context.Context
	nodeID                string
	keyManager            ethclient.KeyManager
	transportManager      components.TransportManager
	inflightRequests      map[string]*inflightRequest
	inflightRequestsMutex *sync.Mutex
}

type inflightRequest struct {
	resolved func(ctx context.Context, verifier string)
	failed   func(ctx context.Context, err error)
}

// As a LateBoundComponent, the identity resolver is created and initialised in a single function call
func NewIdentityResolver(ctx context.Context, nodeID string) components.IdentityResolver {
	return &identityResolver{
		bgCtx:                 ctx,
		nodeID:                nodeID,
		inflightRequests:      make(map[string]*inflightRequest),
		inflightRequestsMutex: &sync.Mutex{},
	}
}

func (ir *identityResolver) PreInit(c components.PreInitComponents) (*components.ManagerInitResult, error) {
	return &components.ManagerInitResult{}, nil
}

func (ir *identityResolver) PostInit(c components.AllComponents) error {
	ir.keyManager = c.KeyManager()
	ir.transportManager = c.TransportManager()
	return c.TransportManager().RegisterClient(ir.bgCtx, ir)
}

func (ir *identityResolver) Start() error {
	//TODO start a reaper thread to clean up inflight requests that have been hanging around too long

	return nil
}

func (ir *identityResolver) Stop() {
}

func (ir *identityResolver) ResolveVerifier(ctx context.Context, lookup string, algorithm string, verifierType string) (string, error) {
	//TODO should we have a timeout here? Shoudl be related to the async timeout and reaping of the inflight requests?
	replyChan := make(chan string)
	errChan := make(chan error)
	ir.ResolveVerifierAsync(ctx, lookup, algorithm, verifierType, func(ctx context.Context, verifier string) {
		replyChan <- verifier
	}, func(ctx context.Context, err error) {
		errChan <- err
	})
	select {
	case verifier := <-replyChan:
		return verifier, nil
	case err := <-errChan:
		return "", err
	}
}

func (ir *identityResolver) ResolveVerifierAsync(ctx context.Context, lookup string, algorithm string, verifierType string, resolved func(ctx context.Context, verifier string), failed func(ctx context.Context, err error)) {
	// if the verifier lookup is a local key, we can resolve it here
	// if it is a remote key, we need to delegate to the remote node

	atIndex := strings.Index(lookup, "@")

	if atIndex == -1 || lookup[atIndex+1:] == ir.nodeID {
		// this is an asyncronous call because the key manager may need to call out to a remote signer in order to
		// resovle the key (e.g. if this is the first time this key has been referenced)
		// its a one and done go routine so no need for additional concurency controls
		// we just need to be careful not to update the transaction object on this other thread
		go func() {
			_, verifier, err := ir.keyManager.ResolveKey(ctx, lookup, algorithm, verifierType)
			if err != nil {
				log.L(ctx).Errorf("Failed to resolve local signer for %s (algorithm=%s, verifierType=%s): %s", lookup, algorithm, verifierType, err)
				failed(ctx, err)
				return

			}
			resolved(ctx, verifier)
		}()

	} else {
		log.L(ctx).Debugf("resolving verifier via remote node %s", lookup)

		resolveVerifierRequest := &pbIdentityResolver.ResolveVerifierRequest{
			Lookup:       lookup,
			Algorithm:    algorithm,
			VerifierType: verifierType,
		}
		resolveVerifierRequestBytes, err := proto.Marshal(resolveVerifierRequest)
		if err != nil {
			log.L(ctx).Errorf("Failed to marshal ResolveVerifierRequest for lookup %s: %s", lookup, err)
			failed(ctx, err)
			return
		}

		requestID := uuid.New()

		remoteNodeId, err := tktypes.PrivateIdentityLocator(lookup).Node(ctx, false)
		if err != nil {
			failed(ctx, err)
			return
		}

		err = ir.transportManager.Send(ctx, &components.TransportMessage{
			MessageType: "ResolveVerifierRequest",
			MessageID:   requestID,
			Destination: tktypes.PrivateIdentityLocator(fmt.Sprintf("%s@%s", IDENTITY_RESOLVER_DESTINATION, remoteNodeId)),
			ReplyTo:     tktypes.PrivateIdentityLocator(fmt.Sprintf("%s@%s", IDENTITY_RESOLVER_DESTINATION, ir.nodeID)),
			Payload:     resolveVerifierRequestBytes,
		})
		if err != nil {
			failed(ctx, err)
			return
		}
		ir.addInflightRequest(requestID.String(), &inflightRequest{
			resolved: resolved,
			failed:   failed,
		})
	}
}

func (ir *identityResolver) addInflightRequest(requestID string, request *inflightRequest) {
	ir.inflightRequestsMutex.Lock()
	defer ir.inflightRequestsMutex.Unlock()
	ir.inflightRequests[requestID] = request
}

func (ir *identityResolver) resolveInflightRequest(ctx context.Context, requestID string, verifier string) {
	ir.inflightRequestsMutex.Lock()
	defer ir.inflightRequestsMutex.Unlock()
	request := ir.inflightRequests[requestID]
	if request == nil {
		log.L(ctx).Warnf("Failed to find inflight request %s", requestID)
		return
	}
	delete(ir.inflightRequests, requestID)

	// make sure we don't hold the lock while calling the callback
	go request.resolved(ctx, verifier)
}

func (ir *identityResolver) failInflightRequest(ctx context.Context, requestID string, err error) {
	ir.inflightRequestsMutex.Lock()
	defer ir.inflightRequestsMutex.Unlock()
	request := ir.inflightRequests[requestID]
	if request == nil {
		log.L(ctx).Warnf("Failed to find inflight request %s", requestID)
		return
	}
	delete(ir.inflightRequests, requestID)

	// make sure we don't hold the lock while calling the callback
	go request.failed(ctx, err)
}

func (ir *identityResolver) handleResolveVerifierReply(ctx context.Context, messagePayload []byte, correlationID string) {

	resolveVerifierResponse := &pbIdentityResolver.ResolveVerifierResponse{}
	err := proto.Unmarshal(messagePayload, resolveVerifierResponse)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal resolveVerifierResponse: %s", err)
		return
	}

	ir.resolveInflightRequest(ctx, correlationID, resolveVerifierResponse.Verifier)

}

func (ir *identityResolver) handleResolveVerifierError(ctx context.Context, messagePayload []byte, correlationID string) {

	resolveVerifierError := &pbIdentityResolver.ResolveVerifierError{}
	err := proto.Unmarshal(messagePayload, resolveVerifierError)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal resolveVerifierError: %s", err)
		return
	}

	ir.failInflightRequest(ctx, correlationID, i18n.NewError(ctx, msgs.MsgResolveVerifierRemoteFailed, resolveVerifierError.Lookup, resolveVerifierError.Algorithm, resolveVerifierError.ErrorMessage))

}

// TODO some common code with ResolveVerifierAsync. Refactor out to a re-usable function
func (ir *identityResolver) handleResolveVerifierRequest(ctx context.Context, messagePayload []byte, replyTo tktypes.PrivateIdentityLocator, requestID *uuid.UUID) {

	resolveVerifierRequest := &pbIdentityResolver.ResolveVerifierRequest{}
	err := proto.Unmarshal(messagePayload, resolveVerifierRequest)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal resolveVerifierRequest: %s", err)
		return
	}

	//We don't need to bring a transaction processor into memory.  We can just service this request
	// in isolation to any other processing for that transaction

	// contractAddress and transactionID in the request message are simply used to populate the response
	// so that the requesting node can correlate the response with the transaction that needs it
	_, verifier, err := ir.keyManager.ResolveKey(ctx, resolveVerifierRequest.Lookup, resolveVerifierRequest.Algorithm, resolveVerifierRequest.VerifierType)
	if err == nil {
		resolveVerifierResponse := &pbIdentityResolver.ResolveVerifierResponse{
			Lookup:       resolveVerifierRequest.Lookup,
			Algorithm:    resolveVerifierRequest.Algorithm,
			Verifier:     verifier,
			VerifierType: resolveVerifierRequest.VerifierType,
		}
		resolveVerifierResponseBytes, err := proto.Marshal(resolveVerifierResponse)
		if err == nil {
			err = ir.transportManager.Send(ctx, &components.TransportMessage{
				MessageType:   "ResolveVerifierResponse",
				CorrelationID: requestID,
				ReplyTo:       tktypes.PrivateIdentityLocator(fmt.Sprintf("%s@%s", IDENTITY_RESOLVER_DESTINATION, ir.nodeID)),
				Destination:   replyTo,
				Payload:       resolveVerifierResponseBytes,
			})
			if err != nil {
				log.L(ctx).Errorf("Failed to send resolve verifier response: %s", err)
				// assume the requestor will eventually retry
			}
			return
		} else {
			log.L(ctx).Errorf("Failed to marshal resolve verifier response: %s", err)
		}

	} else {
		log.L(ctx).Errorf("Failed to resolve verifier for %s (algorithm=%s): %s", resolveVerifierRequest.Lookup, resolveVerifierRequest.Algorithm, err)
	}

	if err != nil {
		resolveVerifierError := &pbIdentityResolver.ResolveVerifierError{
			Lookup:       resolveVerifierRequest.Lookup,
			Algorithm:    resolveVerifierRequest.Algorithm,
			ErrorMessage: err.Error(),
		}
		resolveVerifierErrorBytes, err := proto.Marshal(resolveVerifierError)
		if err == nil {
			err = ir.transportManager.Send(ctx, &components.TransportMessage{
				MessageType:   "ResolveVerifierError",
				CorrelationID: requestID,
				ReplyTo:       tktypes.PrivateIdentityLocator(fmt.Sprintf("%s@%s", IDENTITY_RESOLVER_DESTINATION, ir.nodeID)),
				Destination:   replyTo,
				Payload:       resolveVerifierErrorBytes,
			})
			if err != nil {
				log.L(ctx).Errorf("Failed to send resolve verifier error: %s", err)
				// assume the requestor will eventually retry
			}
			return
		} else {
			log.L(ctx).Errorf("Failed to marshal resolve verifier response: %s", err)
			//TODO what can we do here other than panic
		}
	}
}
