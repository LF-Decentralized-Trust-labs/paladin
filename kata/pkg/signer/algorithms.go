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

package signer

// TODO: More work on algorithm taxonomy. These could become very dynamic strings,
// as we specify complex dynamic payloads for ZKP proof generation
// (with signing key access during the proof generation stage).

// For now however, it's extremely simple - we just need one:
// - ECDSA algorithm
// - SECP256K1 curve
// - Plain bytes-in, bytes-out (caller is responsible for generating/formatting/hashing the payload such as Eth TX at some version, or EIP-712 etc. prior to signing)
const Algorithm_ECDSA_SECP256K1_PLAINBYTES = "ecdsa_secp256k1_plainbytes"
