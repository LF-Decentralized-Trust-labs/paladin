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
  INotoDomainReceipt,
  IPreparedTransaction,
  NotoBalanceOfResult,
  NotoFactory,
  PenteFactory,
  ZetoFactory,
} from "@lfdecentralizedtrust/paladin-sdk";
import { ethers } from "ethers";
import {
  checkDeploy,
  checkReceipt,
  DEFAULT_POLL_TIMEOUT,
  LONG_POLL_TIMEOUT,
  POLL_INTERVAL,
} from "paladin-example-common";
import { newAtomFactory } from "./helpers/atom";
import { newERC20Tracker } from "./helpers/erc20tracker";
import * as fs from 'fs';
import * as path from 'path';
import { ContractData } from "./tests/data-persistence";
import { nodeConnections, getCachePath } from "paladin-example-common";

const logger = console;

async function main(): Promise<boolean> {
  // --- Initialization from Imported Config ---
  if (nodeConnections.length < 3) {
    logger.error("The environment config must provide at least 3 nodes for this scenario.");
    return false;
  }
  
  logger.log("Initializing Paladin clients from the environment configuration...");
  const clients = nodeConnections.map(node => new PaladinClient(node.clientOptions));
  const [paladin1, paladin2, paladin3] = clients;

  const [cashIssuer, assetIssuer] = paladin1.getVerifiers(
    `cashIssuer@${nodeConnections[0].id}`,
    `assetIssuer@${nodeConnections[0].id}`
  );
  const [investor1] = paladin2.getVerifiers(`investor1@${nodeConnections[1].id}`);
  const [investor2] = paladin3.getVerifiers(`investor2@${nodeConnections[2].id}`);

  // TODO: eliminate the need for this call
  async function encodeZetoTransfer(preparedCashTransfer: IPreparedTransaction) {
    try {
      const zetoTransferAbi = await paladin3.ptx.getStoredABI(
        preparedCashTransfer.transaction.abiReference ?? ""
      );
      
      if (!zetoTransferAbi) {
        throw new Error("Failed to get stored ABI for prepared transaction");
      }
      
      const encodedData = new ethers.Interface(zetoTransferAbi.abi).encodeFunctionData(
        "transferLocked",
        [
          preparedCashTransfer.transaction.data.inputs,
          preparedCashTransfer.transaction.data.outputs,
          preparedCashTransfer.transaction.data.proof,
          preparedCashTransfer.transaction.data.data,
        ]
      );
      
      logger.log("Successfully encoded Zeto transfer data");
      return encodedData;
    } catch (error) {
      logger.error("Failed to encode Zeto transfer:");
      logger.error(`Error: ${error}`);
      throw error;
    }
  }

  const issuedAssetAmount = 1000;
  const issuedCashAmount = 10000;
  const assetAmount = 100;
  const cashAmount = 10;

  // Deploy the atom factory on the base ledger
  logger.log("Creating atom factory...");
  const atomFactory = await newAtomFactory(paladin1, cashIssuer);
  if (!checkDeploy(atomFactory)) return false;

  // Deploy a Zeto token to represent cash
  logger.log("Deploying Zeto cash token...");
  const zetoFactory = new ZetoFactory(paladin1, "zeto");
  const zetoCash = await zetoFactory
    .newZeto(cashIssuer, {
      tokenName: "Zeto_Anon",
    })
    .waitForDeploy(DEFAULT_POLL_TIMEOUT);
  if (!checkDeploy(zetoCash)) return false;

  // Create a Pente privacy group for the asset issuer only
  logger.log("Creating asset issuer privacy group...");
  const penteFactory = new PenteFactory(paladin1, "pente");
  const issuerGroup = await penteFactory
    .newPrivacyGroup({
      members: [assetIssuer],
      evmVersion: "shanghai",
      externalCallsEnabled: true,
    })
    .waitForDeploy(DEFAULT_POLL_TIMEOUT);
  if (!checkDeploy(issuerGroup)) return false;

  // Deploy private tracker to the issuer privacy group
  logger.log("Creating private asset tracker...");
  const tracker = await newERC20Tracker(issuerGroup, assetIssuer, {
    name: "ASSET",
    symbol: "ASSET",
  });
  if (!checkDeploy(tracker)) return false;

  // Create a Noto token to represent an asset
  logger.log("Deploying Noto asset token...");
  const notoFactory = new NotoFactory(paladin1, "noto");
  const notoAsset = await notoFactory
    .newNoto(assetIssuer, {
      name: "NOTO",
      symbol: "NOTO",
      notary: assetIssuer,
      notaryMode: "hooks",
      options: {
        hooks: {
          privateGroup: issuerGroup,
          publicAddress: issuerGroup.address,
          privateAddress: tracker.address,
        },
      },
    })
    .waitForDeploy(DEFAULT_POLL_TIMEOUT);
  if (!checkDeploy(notoAsset)) return false;

  // Issue asset
  logger.log("Issuing asset to investor1...");
  let receipt = await notoAsset
    .mint(assetIssuer, {
      to: investor1,
      amount: issuedAssetAmount,
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;

  // Issue cash
  logger.log("Issuing cash to investor2...");
  receipt = await zetoCash
    .mint(cashIssuer, {
      mints: [
        {
          to: investor2,
          amount: issuedCashAmount,
          data: "0x",
        },
      ],
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;

  // Lock the asset for the swap
  logger.log("Locking asset from investor1...");
  receipt = await notoAsset
    .using(paladin2)
    .lock(investor1, {
      amount: assetAmount,
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;
  receipt = await paladin2.ptx.getTransactionReceiptFull(receipt.id);

  let domainReceipt = receipt?.domainReceipt as INotoDomainReceipt | undefined;
  const lockId = domainReceipt?.lockInfo?.lockId;
  if (lockId === undefined) {
    logger.error("No lock ID found in domain receipt");
    return false;
  }

  // Prepare asset unlock operation
  logger.log("Preparing unlock to investor2...");
  receipt = await notoAsset
    .using(paladin2)
    .prepareUnlock(investor1, {
      lockId,
      from: investor1,
      recipients: [{ to: investor2, amount: assetAmount }],
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;
  receipt = await paladin2.ptx.getTransactionReceiptFull(receipt.id);

  domainReceipt = receipt?.domainReceipt as INotoDomainReceipt | undefined;
  const assetUnlockParams = domainReceipt?.lockInfo?.unlockParams;
  const assetUnlockCall = domainReceipt?.lockInfo?.unlockCall;
  if (assetUnlockParams === undefined || assetUnlockCall === undefined) {
    logger.error("No unlock data found in domain receipt");
    return false;
  }

  // Lock the cash for the swap
  logger.log("Locking cash amount from investor2...");
  const investor2Address = await investor2.address();
  receipt = await zetoCash
    .using(paladin3)
    .lock(investor2, {
      amount: cashAmount,
      delegate: investor2Address,
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;
  
  // Poll for the lock operation to be fully settled
  logger.log("Waiting for lock operation to settle...");
  let lockedStateId: string | undefined;
  const pollStartTime = Date.now();
  while (Date.now() - pollStartTime < LONG_POLL_TIMEOUT) {
    const lockedStates = await paladin3.ptx.getStateReceipt(receipt.id);
    const confirmedLockedState = lockedStates?.confirmed?.find(
      (state) => state.data["locked"]
    );
    if (confirmedLockedState) {
      lockedStateId = confirmedLockedState.id;
      break;
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }

  if (lockedStateId === undefined) {
    logger.error(`Timed out after ${LONG_POLL_TIMEOUT / 1000}s waiting for locked state in state receipt`);
    return false;
  }
  logger.log(`Locked state ID: ${lockedStateId}`);

  // Prepare cash transfer
  logger.log("Preparing cash transfer...");
  const txID = await zetoCash.using(paladin3).prepareTransferLocked(investor2, {
    lockedInputs: [lockedStateId],
    delegate: investor2.lookup,
    transfers: [
      {
        to: investor1,
        amount: cashAmount,
        data: "0x",
      },
    ],
  }).id;
  
  logger.log(`Prepared transaction ID: ${txID}`);
  const preparedCashTransfer = await paladin3.pollForPreparedTransaction(txID, LONG_POLL_TIMEOUT);
  if (!preparedCashTransfer) {
    logger.error(`Failed to get prepared transaction for ID: ${txID}`);
    return false;
  }
  
  logger.log("Cash transfer preparation successful!");

  const encodedCashTransfer = await encodeZetoTransfer(preparedCashTransfer);
  logger.log(`Encoded cash transfer length: ${encodedCashTransfer.length}`);

  // Create an atom for the swap
  logger.log("Creating atom...");
  logger.log(`Asset unlock call length: ${assetUnlockCall.length}`);
  logger.log(`Encoded cash transfer length: ${encodedCashTransfer.length}`);
  
  const atom = await atomFactory.create(cashIssuer, [
    {
      contractAddress: notoAsset.address,
      callData: assetUnlockCall,
    },
    {
      contractAddress: zetoCash.address,
      callData: encodedCashTransfer,
    },
  ]);
  if (!checkDeploy(atom)) return false;
  
  logger.log(`Atom created successfully at address: ${atom.address}`);

  // Approve asset unlock operation
  logger.log("Approving asset leg...");
  receipt = await notoAsset
    .using(paladin2)
    .delegateLock(investor1, {
      lockId,
      unlock: assetUnlockParams,
      delegate: atom.address,
      data: "0x",
    })
    .waitForReceipt(LONG_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;

  // Approve cash transfer operation
  logger.log("Approving cash leg...");
  receipt = await zetoCash
    .using(paladin3)
    .delegateLock(investor2, {
      utxos: [lockedStateId],
      delegate: atom.address,
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;

  // execute the swap
  logger.log("Performing swap...");
  receipt = await atom.using(paladin3).execute(investor2);
  if (!checkReceipt(receipt)) return false;

  // Add a delay to ensure the swap operation is fully settled
  await new Promise((resolve) => setTimeout(resolve, 3000));

  let finalAssetBalanceInvestor1: NotoBalanceOfResult | undefined;
  let finalAssetBalanceInvestor2: NotoBalanceOfResult | undefined;
  let finalCashBalanceInvestor1: NotoBalanceOfResult | undefined;
  let finalCashBalanceInvestor2: NotoBalanceOfResult | undefined;

  // get final balances
  // it can take some time for the balances to update, so loop until all balances are >0
  const startTime = Date.now();
  logger.log("Waiting for balances to settle after swap...");
  while (true) {  
    finalAssetBalanceInvestor1 = await notoAsset.using(paladin2).balanceOf(investor1, { account: investor1.lookup });
    finalAssetBalanceInvestor2 = await notoAsset.using(paladin3).balanceOf(investor2, { account: investor2.lookup });
    finalCashBalanceInvestor1 = await zetoCash.using(paladin2).balanceOf(investor1, { account: investor1.lookup });
    finalCashBalanceInvestor2 = await zetoCash.using(paladin3).balanceOf(investor2, { account: investor2.lookup });
    
    logger.log(`Current balances - Asset: I1=${finalAssetBalanceInvestor1.totalBalance}, I2=${finalAssetBalanceInvestor2.totalBalance}, Cash: I1=${finalCashBalanceInvestor1.totalBalance}, I2=${finalCashBalanceInvestor2.totalBalance}`);
    
    if (finalAssetBalanceInvestor1.totalBalance !== "0" &&
        finalAssetBalanceInvestor2.totalBalance !== "0" &&
        finalCashBalanceInvestor1.totalBalance !== "0" &&
        finalCashBalanceInvestor2.totalBalance !== "0") {
      logger.log("All balances are non-zero, proceeding...");
      break;
    }
    // if LONG_POLL_TIMEOUT ms passed from the beginning of the loop than fail the test
    if (Date.now() - startTime > LONG_POLL_TIMEOUT) {
      logger.error(`Failed to get final balances after ${LONG_POLL_TIMEOUT / 1000} seconds`);
      return false;
    }

    await new Promise(resolve => setTimeout(resolve, POLL_INTERVAL));
  }

  // Save contract data to file for later use
  const contractData : ContractData = {
    atomFactoryAddress: atomFactory.address,
    zetoCashAddress: zetoCash.address,
    notoAssetAddress: notoAsset.address,
    issuerGroupId: issuerGroup.group.id,
    issuerGroupAddress: issuerGroup.address,
    trackerAddress: tracker.address,
    atomAddress: atom.address,
    swapDetails: {
      assetAmount: assetAmount,
      cashAmount: cashAmount,
      lockId: lockId,
      lockedStateId: lockedStateId,
      assetUnlockCall: assetUnlockCall,
      encodedCashTransfer: encodedCashTransfer
    },
    finalBalances: {
      asset: {
        investor1: {
          totalBalance: finalAssetBalanceInvestor1?.totalBalance ?? "0",
          totalStates: finalAssetBalanceInvestor1?.totalStates ?? "0",
          overflow: finalAssetBalanceInvestor1?.overflow ?? false
        },
        investor2: {
          totalBalance: finalAssetBalanceInvestor2?.totalBalance ?? "0",
          totalStates: finalAssetBalanceInvestor2?.totalStates ?? "0",
          overflow: finalAssetBalanceInvestor2?.overflow ?? false
        }
      },
      cash: {
        investor1: {
          totalBalance: finalCashBalanceInvestor1?.totalBalance ?? "0",
          totalStates: finalCashBalanceInvestor1?.totalStates ?? "0",
          overflow: finalCashBalanceInvestor1?.overflow ?? false
        },
        investor2: {
          totalBalance: finalCashBalanceInvestor2?.totalBalance ?? "0",
          totalStates: finalCashBalanceInvestor2?.totalStates ?? "0",
          overflow: finalCashBalanceInvestor2?.overflow ?? false
        }
      }
    },
    participants: {
      cashIssuer: cashIssuer.lookup,
      assetIssuer: assetIssuer.lookup,
      investor1: investor1.lookup,
      investor2: investor2.lookup
    },
    timestamp: new Date().toISOString()
  };

  // Use command-line argument for data directory if provided, otherwise use default
  const dataDir = getCachePath();
  if (!fs.existsSync(dataDir)) {
    fs.mkdirSync(dataDir, { recursive: true });
  }

  const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
  const dataFile = path.join(dataDir, `contract-data-${timestamp}.json`);
  fs.writeFileSync(dataFile, JSON.stringify(contractData, null, 2));
  logger.log(`Contract data saved to ${dataFile}`);

  return true;
}

if (require.main === module) {
  main()
    .then((success: boolean) => {
      process.exit(success ? 0 : 1);
    })
    .catch((err) => {
      console.error("Exiting with uncaught error");
      console.error(err);
      process.exit(1);
    });
}
