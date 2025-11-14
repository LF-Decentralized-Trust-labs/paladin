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
import PaladinClient, {
  PaladinVerifier,
  PentePrivacyGroup,
  PentePrivateContract,
} from "@lfdecentralizedtrust/paladin-sdk";
import { DEFAULT_POLL_TIMEOUT } from "paladin-example-common";
import storage from "../abis/Storage.json";

export const newPrivateStorage = async (
  pente: PentePrivacyGroup,
  from: PaladinVerifier,
) => {
  const address = await pente.deploy({
    abi: storage.abi,
    bytecode: storage.bytecode,
    from: from.lookup,
  }).waitForDeploy(DEFAULT_POLL_TIMEOUT);
  return address ? new PrivateStorage(pente, address) : undefined;
};

export class PrivateStorage extends PentePrivateContract<{}> {
  constructor(
    protected evm: PentePrivacyGroup,
    public readonly address: string
  ) {
    super(evm, storage.abi, address);
  }

  using(paladin: PaladinClient) {
    return new PrivateStorage(this.evm.using(paladin), this.address);
  }
}
