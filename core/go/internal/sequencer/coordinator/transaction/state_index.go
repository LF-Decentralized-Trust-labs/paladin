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

	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

// TODO is this helpful as an interface?
//Interface StateIndex defines a dependency of transaction that provides methods to look up transactions for a given state ID
/*type StateIndex interface {
	LookupMinter(ctx context.Context, stateID tktypes.HexBytes) (*Transaction, error)
}*/

type StateIndex = *stateIndex

// TODO: decide whether to keep the implementation in here or move it to an upstream package e.g. as a shim around domain context
// in which case, this simple implementation could be moved to test_utils
type stateIndex struct {
	transactionByOutputState map[string]*Transaction
}

func NewStateIndex(ctx context.Context) StateIndex {
	return &stateIndex{
		transactionByOutputState: make(map[string]*Transaction),
	}
}

func (s *stateIndex) LookupMinter(ctx context.Context, stateID tktypes.HexBytes) (*Transaction, error) {
	return s.transactionByOutputState[stateID.String()], nil
}

func (s *stateIndex) AddMinter(ctx context.Context, stateID tktypes.HexBytes, transaction *Transaction) error {
	//TODO return error if duplicate
	s.transactionByOutputState[stateID.String()] = transaction
	return nil
}

func (s *stateIndex) ForgetMinter(ctx context.Context, stateID tktypes.HexBytes, transaction *Transaction) error {
	//TODO return error if does not match
	delete(s.transactionByOutputState, stateID.String())
	return nil
}
