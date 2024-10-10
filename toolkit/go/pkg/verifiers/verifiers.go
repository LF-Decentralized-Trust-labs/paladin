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

package verifiers

type VerifierType string

const (
	// An ethereum address - 20 byte compressed public key identifier with 0x prefix (no checksum)
	ETH_ADDRESS VerifierType = "eth_address"

	// An ethereum address - 20 byte compressed public key identifier with 0x prefix and ERC-55 mixed-case checksum address encoding
	ETH_ADDRESS_CHECKSUM VerifierType = "eth_address_checksum"

	// ECDSA public key in uncompressed form hex encoded (x and y [FIPS186] in uncompressed form [X9.62] without leading 0x04 "uncompressed" constant prefix)
	HEX_ECDSA_PUBKEY_UNCOMPRESSED VerifierType = "hex_ecdsa_pubkey_uncompressed"

	// ECDSA public key in uncompressed form hex encoded (x and y [FIPS186] in uncompressed form [X9.62] without leading 0x04 "uncompressed" constant prefix)
	HEX_ECDSA_PUBKEY_UNCOMPRESSED_0X VerifierType = "hex_ecdsa_pubkey_uncompressed_0x"
)
