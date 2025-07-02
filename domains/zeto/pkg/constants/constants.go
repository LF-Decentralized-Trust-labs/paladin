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

package constants

const (
	// the circuit names supported by Zeto token implementations
	CIRCUIT_NAME_TRANSFER        = "transfer"
	CIRCUIT_NAME_TRANSFER_LOCKED = "transferLocked"
	CIRCUIT_NAME_DEPOSIT         = "deposit"
	CIRCUIT_NAME_WITHDRAW        = "withdraw"

	// the names of the Zeto token implementations
	TOKEN_ANON               = "Zeto_Anon"
	TOKEN_ANON_ENC           = "Zeto_AnonEnc"
	TOKEN_ANON_NULLIFIER     = "Zeto_AnonNullifier"
	TOKEN_ANON_NULLIFIER_KYC = "Zeto_AnonNullifierKyc"

	TOKEN_NF_ANON           = "Zeto_NfAnon"
	TOKEN_NF_ANON_NULLIFIER = "Zeto_NfAnonNullifier"
)
