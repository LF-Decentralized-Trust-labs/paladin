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
)

type SentMessageRecorder struct {
	hasSentConfirmationResponse    bool
	hasSentAssembleSuccessResponse bool
	hasSentAssembleRevertResponse  bool
	hasSentAssembleParkResponse    bool
}

func NewSentMessageRecorder() *SentMessageRecorder {
	return &SentMessageRecorder{}
}

func (r *SentMessageRecorder) SendDispatchConfirmationResponse(ctx context.Context) {
	r.hasSentConfirmationResponse = true
}

func (r *SentMessageRecorder) HasSentDispatchConfirmationResponse() bool {
	return r.hasSentConfirmationResponse
}

func (r *SentMessageRecorder) HasSentAssembleSuccessResponse() bool {
	return r.hasSentAssembleSuccessResponse
}

func (r *SentMessageRecorder) HasSentAssembleRevertResponse() bool {
	return r.hasSentAssembleRevertResponse
}

func (r *SentMessageRecorder) HasSentAssembleParkResponse() bool {
	return r.hasSentAssembleParkResponse
}

func (r *SentMessageRecorder) SendAssembleResponse(ctx context.Context, requestID uuid.UUID, postAssembly *components.TransactionPostAssembly) {
	switch postAssembly.AssemblyResult {
	case prototk.AssembleTransactionResponse_OK:
		r.hasSentAssembleSuccessResponse = true
	case prototk.AssembleTransactionResponse_REVERT:
		r.hasSentAssembleRevertResponse = true
	case prototk.AssembleTransactionResponse_PARK:
		r.hasSentAssembleParkResponse = true
	}

}
