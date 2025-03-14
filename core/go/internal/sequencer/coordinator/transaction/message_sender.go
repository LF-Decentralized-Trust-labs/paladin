/*
 * Copyright © 2025 Kaleido, Inc.
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

package transaction

import (
	"context"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type MessageSender interface {
	SendAssembleRequest(
		ctx context.Context,
		assemblingNode string,
		transactionID uuid.UUID,
		idempotencyID uuid.UUID,
		transactionPreassembly *components.TransactionPreAssembly,
	) error

	//SendEndorsementRequest to either a local or remote endorser
	SendEndorsementRequest(
		ctx context.Context,
		idempotencyKey uuid.UUID,
		party string,
		attRequest *prototk.AttestationRequest,
		transactionSpecification *prototk.TransactionSpecification,
		verifiers []*prototk.ResolvedVerifier,
		signatures []*prototk.AttestationResult,
		inputStates []*prototk.EndorsableState,
		outputStates []*prototk.EndorsableState,
		infoStates []*prototk.EndorsableState,
	) error

	SendDispatchConfirmationRequest(
		ctx context.Context,
		transactionSender string,
		idempotencyKey uuid.UUID,
		transactionSpecification *prototk.TransactionSpecification,
		hash *tktypes.Bytes32,
	) error
}
