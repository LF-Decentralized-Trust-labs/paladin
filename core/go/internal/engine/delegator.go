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

package engine

import (
	"context"

	"github.com/kaleido-io/paladin/kata/internal/engine/enginespi"
)

func NewDelegator() enginespi.Delegator {
	return &delegator{}
}

type delegator struct {
}

// Delegate implements types.Delegator.
func (p *delegator) Delegate(ctx context.Context, transactionId string, delegateNodeId string) error {
	panic("unimplemented")
}
