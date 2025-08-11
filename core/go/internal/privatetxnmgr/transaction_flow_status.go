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
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
)

func (tf *transactionFlow) GetTxStatus(ctx context.Context) (components.PrivateTxStatus, error) {
	tf.statusLock.RLock()
	defer tf.statusLock.RUnlock()
	endorsementRequirements := tf.endorsementRequirements(ctx)
	endorsementStatus := make([]components.PrivateTxEndorsementStatus, len(endorsementRequirements))
	for i, requirement := range endorsementRequirements {
		endorsementStatus[i] = components.PrivateTxEndorsementStatus{
			Party:               requirement.party,
			EndorsementReceived: true,
		}
		if parties, found := tf.pendingEndorsementRequests[requirement.attRequest.Name]; found {
			if request, found := parties[requirement.party]; found {
				endorsementStatus[i].EndorsementReceived = false
				endorsementStatus[i].RequestTime = request.requestTime.Format(time.RFC3339Nano)
			}
		}
	}

	return components.PrivateTxStatus{
		TxID:         tf.transaction.ID.String(),
		Status:       tf.status,
		LatestEvent:  tf.latestEvent,
		LatestError:  tf.latestError,
		Endorsements: endorsementStatus,
		Transaction:  tf.transaction,
	}, nil
}

func (tf *transactionFlow) hasOutstandingVerifierRequests(ctx context.Context) bool {
	log.L(ctx).Debug("transactionFlow:hasOutstandingVerifierRequests")

	// assume they are all resolved until we find one in RequiredVerifiers that is not in Verifiers
	verifiersResolved := true
	for _, v := range tf.transaction.PreAssembly.RequiredVerifiers {
		thisVerifierIsResolved := false
		for _, rv := range tf.transaction.PreAssembly.Verifiers {
			if rv.Lookup == v.Lookup {
				thisVerifierIsResolved = true
				break
			}
		}
		if !thisVerifierIsResolved {
			verifiersResolved = false
		}
	}
	if verifiersResolved {
		return false
	} else {
		log.L(ctx).Infof("Waiting for verifiers to be resolved for transaction %s", tf.transaction.ID.String())
		return true
	}

}

func (tf *transactionFlow) hasOutstandingSignatureRequests() bool {
	outstandingSignatureRequests := false
out:
	for _, attRequest := range tf.transaction.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_SIGN {
			found := false
			for _, signatures := range tf.transaction.PostAssembly.Signatures {
				if signatures.Name == attRequest.Name {
					found = true
					break
				}
			}
			if !found {
				outstandingSignatureRequests = true
				// no point checking any further, we have at least one outstanding signature request
				break out
			}
		}
	}
	return outstandingSignatureRequests
}

func (tf *transactionFlow) hasOutstandingEndorsementRequests(ctx context.Context) bool {
	return len(tf.outstandingEndorsementRequests(ctx)) > 0
}

type endorsementRequirement struct {
	attRequest *prototk.AttestationRequest
	party      string
}

func (tf *transactionFlow) outstandingEndorsementRequests(ctx context.Context) []*endorsementRequirement {
	outstandingEndorsementRequests := make([]*endorsementRequirement, 0)
	if tf.transaction.PostAssembly == nil {
		log.L(ctx).Debugf("PostAssembly is nil so there are no outstanding endorsement requests")
		return outstandingEndorsementRequests
	}
	for _, attRequest := range tf.transaction.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_ENDORSE {
			for _, party := range attRequest.Parties {
				found := false
				for _, endorsement := range tf.transaction.PostAssembly.Endorsements {
					found = endorsement.Name == attRequest.Name &&
						party == endorsement.Verifier.Lookup &&
						attRequest.VerifierType == endorsement.Verifier.VerifierType
					log.L(ctx).Infof("endorsement matched=%t: request[name=%s,party=%s,verifierType=%s] endorsement[name=%s,party=%s,verifierType=%s] verifier=%s",
						found,
						attRequest.Name, party, attRequest.VerifierType,
						endorsement.Name, endorsement.Verifier.Lookup, endorsement.Verifier.VerifierType,
						endorsement.Verifier.Verifier,
					)
					if found {
						break
					}
				}
				if !found {
					log.L(ctx).Debugf("endorsement request for %s outstanding for transaction %s", party, tf.transaction.ID)
					outstandingEndorsementRequests = append(outstandingEndorsementRequests, &endorsementRequirement{party: party, attRequest: attRequest})
				}
			}
		}
	}
	return outstandingEndorsementRequests
}

func (tf *transactionFlow) endorsementRequirements(ctx context.Context) []*endorsementRequirement {
	//utility function to fold all the attestation plan into a single list, filtered by type - Endorse
	endorsementRequests := make([]*endorsementRequirement, 0)
	if tf.transaction.PostAssembly == nil {
		log.L(ctx).Debugf("PostAssembly is nil so there are no endorsement requests")
		return endorsementRequests
	}
	for _, attRequest := range tf.transaction.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_ENDORSE {
			for _, party := range attRequest.Parties {
				endorsementRequests = append(endorsementRequests, &endorsementRequirement{party: party, attRequest: attRequest})
			}
		}
	}
	return endorsementRequests
}
