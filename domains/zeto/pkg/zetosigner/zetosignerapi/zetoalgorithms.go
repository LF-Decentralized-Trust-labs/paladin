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

package zetosignerapi

import (
	"fmt"
	"regexp"
)

// - SNARK proving engine
// - Baby Jub Jub key materials used in proofs
var ALGO_DOMAIN_ZETO_SNARK_BJJ_REGEXP = regexp.MustCompile(`^domain:([a-zA-Z0-9-._]+):snark:babyjubjub$`)

func AlgoDomainZetoSnarkBJJ(name string) string {
	return fmt.Sprintf("domain:%s:snark:babyjubjub", name)
}

const PAYLOAD_DOMAIN_ZETO_SNARK = "domain:zeto:snark"

const PAYLOAD_DOMAIN_ZETO_NULLIFIER = "domain:zeto:nullifier"

const IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X = "iden3_pubkey_babyjubjub_compressed_0x"
const IDEN3_PUBKEY_BABYJUBJUB_UNCOMPRESSED_0X = "iden3_pubkey_babyjubjub_uncompressed_0x"
