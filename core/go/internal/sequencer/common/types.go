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
package common

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type FlushPoint struct {
	From          tktypes.EthAddress
	Nonce         uint64
	TransactionID uuid.UUID
	Hash          tktypes.Bytes32
	Confirmed     bool
}

func (f *FlushPoint) GetSignerNonce() string {
	return fmt.Sprintf("%s:%d", f.From.String(), f.Nonce)
}
