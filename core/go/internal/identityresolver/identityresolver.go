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
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	pbIdentityResolver "github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/proto/identityresolver"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/cache"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

type identityResolver struct {
	bgCtx                 context.Context
	nodeName              string
	keyManager            components.KeyManager
	transportManager      components.TransportManager
	inflightRequests      map[string]*inflightRequest
	inflightRequestsMutex *sync.Mutex
	verifierCache         cache.Cache[string, string]
}

type inflightRequest struct {
	resolved func(ctx context.Context, verifier string)
	failed   func(ctx context.Context, err error)
}

func NewIdentityResolver(ctx context.Context, conf *pldconf.IdentityResolverConfig) components.IdentityResolver {
	return &identityResolver{
		bgCtx:                 ctx,
		inflightRequests:      make(map[string]*inflightRequest),
		inflightRequestsMutex: &sync.Mutex{},
		verifierCache:         cache.NewCache[string, string](&conf.VerifierCache, &pldconf.IdentityResolverDefaults.VerifierCache),
	}
}

func cacheKey(identifier, node, algorithm, verifierType string) string {
	return fmt.Sprintf("%s@%s|%s|%s", identifier, node, algorithm, verifierType)
}

func (ir *identityResolver) PreInit(c components.PreInitComponents) (*components.ManagerInitResult, error) {
	return &components.ManagerInitResult{}, nil
}

func (ir *identityResolver) PostInit(c components.AllComponents) error {
	ir.nodeName = c.TransportManager().LocalNodeName()
	ir.keyManager = c.KeyManager()
	ir.transportManager = c.TransportManager()
	return nil
}

func (ir *identityResolver) Start() error {
	//TODO start a reaper thread to clean up inflight requests that have been hanging around too long

	return nil
}

func (ir *identityResolver) Stop() {
}

func (ir *identityResolver) ResolveVerifier(ctx context.Context, lookup string, algorithm string, verifierType string) (string, error) {
	replyChan := make(chan string, 1)
	errChan := make(chan error, 1)
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
	case <-ctx.Done():
		return "", i18n.NewError(ctx, msgs.MsgContextCanceled)
	}
}

func (ir *identityResolver) ResolveVerifierAsync(ctx context.Context, lookup string, algorithm string, verifierType string, resolved func(ctx context.Context, verifier string), failed func(ctx context.Context, err error)) {
	// if the verifier lookup is a local key, we can resolve it here
	// if it is a remote key, we need to delegate to the remote node

	identifier, node, err := pldtypes.PrivateIdentityLocator(lookup).Validate(ctx, ir.nodeName, true)
	if err != nil {
		log.L(ctx).Errorf("Invalid resolve verifier request: %s (algorithm=%s, verifierType=%s): %s", lookup, algorithm, verifierType, err)
		failed(ctx, err)
		return
	}

	// Ensure we log and cache if we resolve
	cacheKey := cacheKey(identifier, node, algorithm, verifierType)
	cachedVerifier, _ := ir.verifierCache.Get(cacheKey)
	isCached := cachedVerifier != ""
	isLocal := node == ir.nodeName
	cacheAndResolve := func(ctx context.Context, verifier string) {
		if !isCached {
			ir.verifierCache.Set(cacheKey, verifier)
		}
		log.L(ctx).Debugf("ResolvedVerifier(lookup='%s',identifier='%s',node='%s',isLocal=%t,algorithm='%s',verifierType='%s',cached=%t): %s",
			lookup, identifier, node, isLocal, algorithm, verifierType, isCached, verifier,
		)
		resolved(ctx, verifier)
	}

	if isCached {
		cacheAndResolve(ctx, cachedVerifier)
		return
	}

	if isLocal {
		// this is an asynchronous call because the key manager may need to call out to a remote signer in order to
		// resolve the key (e.g. if this is the first time this key has been referenced)
		// its a one and done go routine so no need for additional concurrency controls
		// we just need to be careful not to update the transaction object on this other thread
		go func() {
			var resolvedKey *pldapi.KeyMappingAndVerifier
			if err == nil {
				resolvedKey, err = ir.keyManager.ResolveKeyNewDatabaseTX(ctx, identifier, algorithm, verifierType)
			}
			if err != nil {
				log.L(ctx).Errorf("Failed to resolve local signer for %s (algorithm=%s, verifierType=%s): %s", lookup, algorithm, verifierType, err)
				failed(ctx, err)
				return
			}
			cacheAndResolve(ctx, resolvedKey.Verifier.Verifier)
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

		remoteNodeId, err := pldtypes.PrivateIdentityLocator(lookup).Node(ctx, false)
		if err != nil {
			failed(ctx, err)
			return
		}

		err = ir.transportManager.Send(ctx, &components.FireAndForgetMessageSend{
			MessageID:   &requestID,
			MessageType: "ResolveVerifierRequest",
			Component:   prototk.PaladinMsg_IDENTITY_RESOLVER,
			Node:        remoteNodeId,
			Payload:     resolveVerifierRequestBytes,
		})
		if err != nil {
			failed(ctx, err)
			return
		}
		ir.addInflightRequest(requestID.String(), &inflightRequest{
			resolved: cacheAndResolve,
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
func (ir *identityResolver) handleResolveVerifierRequest(ctx context.Context, messagePayload []byte, replyTo string, requestID *uuid.UUID) {

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
	var resolvedKey *pldapi.KeyMappingAndVerifier
	unqualifiedLookup, err := pldtypes.PrivateIdentityLocator(resolveVerifierRequest.Lookup).Identity(ctx)
	if err == nil {
		resolvedKey, err = ir.keyManager.ResolveKeyNewDatabaseTX(ctx, unqualifiedLookup, resolveVerifierRequest.Algorithm, resolveVerifierRequest.VerifierType)
	}
	if err == nil {
		resolveVerifierResponse := &pbIdentityResolver.ResolveVerifierResponse{
			Lookup:       resolveVerifierRequest.Lookup,
			Algorithm:    resolveVerifierRequest.Algorithm,
			Verifier:     resolvedKey.Verifier.Verifier,
			VerifierType: resolveVerifierRequest.VerifierType,
		}
		resolveVerifierResponseBytes, err := proto.Marshal(resolveVerifierResponse)
		if err == nil {
			err = ir.transportManager.Send(ctx, &components.FireAndForgetMessageSend{
				MessageType:   "ResolveVerifierResponse",
				CorrelationID: requestID,
				Component:     prototk.PaladinMsg_IDENTITY_RESOLVER,
				Node:          replyTo,
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
			err = ir.transportManager.Send(ctx, &components.FireAndForgetMessageSend{
				MessageType:   "ResolveVerifierError",
				CorrelationID: requestID,
				Component:     prototk.PaladinMsg_IDENTITY_RESOLVER,
				Node:          replyTo,
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
