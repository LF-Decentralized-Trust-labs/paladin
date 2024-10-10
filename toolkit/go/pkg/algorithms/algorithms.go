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

package algorithms

type Algorithm string

const (
	// Primary constant used throughout Paladin codebase - ECDSA algorithm with SECP256K1 curve
	ECDSA_SECP256K1 Algorithm = Prefix_ECDSA + ":" + Curve_SECP256K1
	Prefix_ECDSA    Algorithm = "ecdsa"
	Curve_SECP256K1 Algorithm = "secp256k1"
)
