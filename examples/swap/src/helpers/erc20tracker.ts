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
import erc20Tracker from "../abis/NotoTrackerERC20.json";
import { DEFAULT_POLL_TIMEOUT } from "paladin-example-common";

export interface ERC20TrackerConstructorParams {
  name: string;
  symbol: string;
}

export const newERC20Tracker = async (
  pente: PentePrivacyGroup,
  from: PaladinVerifier,
  params: ERC20TrackerConstructorParams
) => {
  const address = await pente.deploy({
    abi: erc20Tracker.abi,
    bytecode: erc20Tracker.bytecode,
    from: from.lookup,
    inputs: params,
  }).waitForDeploy(DEFAULT_POLL_TIMEOUT);
  return address ? new BondTracker(pente, address) : undefined;
};

export class BondTracker extends PentePrivateContract<ERC20TrackerConstructorParams> {
  constructor(
    protected evm: PentePrivacyGroup,
    public readonly address: string
  ) {
    super(evm, erc20Tracker.abi, address);
  }

  using(paladin: PaladinClient) {
    return new BondTracker(this.evm.using(paladin), this.address);
  }
}
