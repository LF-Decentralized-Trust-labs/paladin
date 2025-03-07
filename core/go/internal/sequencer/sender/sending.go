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

package sender

import (
	"context"

	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

func action_SendDelegationRequest(ctx context.Context, s *sender) error {
	transactions, err := s.transactionsOrderedByCreatedTime(ctx)
	if err != nil {
		log.L(ctx).Errorf("Failed to get transactions ordered by created time: %v", err)
		return err
	}
	s.messageSender.SendDelegationRequest(ctx, transactions, s.currentBlockHeight)
	return nil

}

func guard_HasUnconfirmedTransactions(ctx context.Context, s *sender) bool {
	return false //TODO
}
