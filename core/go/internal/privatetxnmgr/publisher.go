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

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
)

func NewPublisher(p *privateTxManager, contractAddress string) *publisher {
	return &publisher{
		privateTxManager: p,
		contractAddress:  contractAddress,
	}
}

type publisher struct {
	privateTxManager *privateTxManager
	contractAddress  string
}

func (p *publisher) PublishTransactionDispatchedEvent(ctx context.Context, transactionId string, nonce uint64, signingAddress string) {

	p.privateTxManager.HandleNewEvent(ctx, &ptmgrtypes.TransactionDispatchedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			ContractAddress: p.contractAddress,
			TransactionID:   transactionId,
		},
		Nonce:          nonce,
		SigningAddress: signingAddress,
	})
	p.privateTxManager.publishToSubscribers(ctx, &components.TransactionDispatchedEvent{
		TransactionID:  transactionId,
		Nonce:          nonce,
		SigningAddress: signingAddress,
	})
}

func (p *publisher) PublishTransactionPreparedEvent(ctx context.Context, transactionId string) {
	p.privateTxManager.HandleNewEvent(ctx, &ptmgrtypes.TransactionPreparedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			ContractAddress: p.contractAddress,
			TransactionID:   transactionId,
		},
	})
}

func (p *publisher) PublishTransactionAssembledEvent(ctx context.Context, transactionId string, postAssembly *components.TransactionPostAssembly, requestID string) {
	event := &ptmgrtypes.TransactionAssembledEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			ContractAddress: p.contractAddress,
			TransactionID:   transactionId,
		},
		PostAssembly:      postAssembly,
		AssembleRequestID: requestID,
	}
	p.privateTxManager.HandleNewEvent(ctx, event)
}

func (p *publisher) PublishTransactionAssembleFailedEvent(ctx context.Context, transactionId string, errorMessage string, requestID string) {
	event := &ptmgrtypes.TransactionAssembleFailedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			ContractAddress: p.contractAddress,
			TransactionID:   transactionId,
		},
		Error:             errorMessage,
		AssembleRequestID: requestID,
	}
	p.privateTxManager.HandleNewEvent(ctx, event)
}

func (p *publisher) PublishTransactionSignedEvent(ctx context.Context, transactionId string, attestationResult *prototk.AttestationResult) {
	event := &ptmgrtypes.TransactionSignedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			ContractAddress: p.contractAddress,
			TransactionID:   transactionId,
		},
		AttestationResult: attestationResult,
	}
	p.privateTxManager.HandleNewEvent(ctx, event)
}

func (p *publisher) PublishTransactionEndorsedEvent(ctx context.Context, transactionId string, idempotencyKey string, party string, attestationRequestName string, endorsement *prototk.AttestationResult, revertReason *string) {
	event := &ptmgrtypes.TransactionEndorsedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			ContractAddress: p.contractAddress,
			TransactionID:   transactionId,
		},
		Endorsement:            endorsement,
		RevertReason:           revertReason,
		Party:                  party,
		AttestationRequestName: attestationRequestName,
		IdempotencyKey:         idempotencyKey,
	}
	p.privateTxManager.HandleNewEvent(ctx, event)
}

func (p *publisher) PublishResolveVerifierResponseEvent(ctx context.Context, transactionId string, lookup, algorithm, verifier, verifierType string) {
	event := &ptmgrtypes.ResolveVerifierResponseEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			ContractAddress: p.contractAddress,
			TransactionID:   transactionId,
		},
		Lookup:       &lookup,
		Algorithm:    &algorithm,
		Verifier:     &verifier,
		VerifierType: &verifierType,
	}

	p.privateTxManager.HandleNewEvent(ctx, event)

}

func (p *publisher) PublishResolveVerifierErrorEvent(ctx context.Context, transactionId string, lookup, algorithm, errorMessage string) {
	event := &ptmgrtypes.ResolveVerifierErrorEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			ContractAddress: p.contractAddress,
			TransactionID:   transactionId,
		},
		Lookup:       &lookup,
		Algorithm:    &algorithm,
		ErrorMessage: &errorMessage,
	}
	p.privateTxManager.HandleNewEvent(ctx, event)
}

func (p *publisher) PublishTransactionFinalizedEvent(ctx context.Context, transactionId string) {
	event := &ptmgrtypes.TransactionFinalizedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			ContractAddress: p.contractAddress,
			TransactionID:   transactionId,
		},
	}
	p.privateTxManager.HandleNewEvent(ctx, event)
}

func (p *publisher) PublishTransactionFinalizeError(ctx context.Context, transactionId string, revertReason string, err error) {
	event := &ptmgrtypes.TransactionFinalizeError{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			ContractAddress: p.contractAddress,
			TransactionID:   transactionId,
		},
		RevertReason: revertReason,
		ErrorMessage: err.Error(),
	}
	p.privateTxManager.HandleNewEvent(ctx, event)
}

func (p *publisher) PublishTransactionConfirmedEvent(ctx context.Context, transactionId string) {
	event := &ptmgrtypes.TransactionConfirmedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			ContractAddress: p.contractAddress,
			TransactionID:   transactionId,
		},
	}
	p.privateTxManager.HandleNewEvent(ctx, event)
}

func (p *publisher) PublishNudgeEvent(ctx context.Context, transactionId string) {
	event := &ptmgrtypes.TransactionNudgeEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			ContractAddress: p.contractAddress,
			TransactionID:   transactionId,
		},
	}
	p.privateTxManager.HandleNewEvent(ctx, event)
}
