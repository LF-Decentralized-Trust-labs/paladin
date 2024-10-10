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

package signpayloads

type SignPayloadType string

const (
	// Input:
	// An opaque payload goes into the signing module. No validation, or other processing
	// of the payload is performed before signing.
	// Output:
	// A compact 65 byte encoded R,S,V byte string (R=32b, S=32b, V=1b) with the V value
	// according to the Bitcoin/Eth standard of 27+recid (27 or 28)
	// denoting an uncompressed public key.
	OPAQUE_TO_RSV SignPayloadType = "opaque:rsv"
)
