// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { BigNumberish, Signer } from "ethers";
import { readFileSync } from "fs";
import { formatPrivKeyForBabyJub, genKeypair } from "maci-crypto";
import * as path from "path";
import { Poseidon, newSalt } from "zeto-js";

const poseidonHash4 = Poseidon.poseidon4;

export interface UTXO {
  value?: number;
  tokenId?: number;
  uri?: string;
  hash: BigNumberish;
  salt?: BigInt;
}

export interface User {
  signer: Signer;
  ethAddress: string;
  babyJubPrivateKey: any;
  babyJubPublicKey: BigInt[];
  formattedPrivateKey: BigInt;
}

export async function newUser(signer: Signer) {
  const { privKey, pubKey } = genKeypair();
  const formattedPrivateKey = formatPrivKeyForBabyJub(privKey);

  return {
    signer,
    ethAddress: await signer.getAddress(),
    babyJubPrivateKey: privKey,
    babyJubPublicKey: pubKey,
    formattedPrivateKey,
  };
}

export function newUTXO(value: number, owner: User, salt?: BigInt): UTXO {
  if (!salt) salt = newSalt();
  const hash = poseidonHash4([
    BigInt(value),
    salt,
    owner.babyJubPublicKey[0],
    owner.babyJubPublicKey[1],
  ]);
  return { value, hash, salt };
}

function provingKeysRoot() {
  const PROVING_KEYS_ROOT = process.env.PROVING_KEYS_ROOT;
  if (!PROVING_KEYS_ROOT) {
    throw new Error("PROVING_KEYS_ROOT env var is not set");
  }
  return PROVING_KEYS_ROOT;
}

export function loadProvingKeys(type: string) {
  const provingKeyFile = path.join(provingKeysRoot(), `${type}.zkey`);
  const verificationKey = JSON.parse(
    new TextDecoder().decode(
      readFileSync(path.join(provingKeysRoot(), `${type}-vkey.json`))
    )
  );
  return {
    provingKeyFile,
    verificationKey,
  };
}
