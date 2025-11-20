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
  NotoFactory,
  PenteFactory,
  ZetoFactory,
} from "@lfdecentralizedtrust/paladin-sdk";
import * as fs from 'fs';
import * as path from 'path';
import { nodeConnections, findLatestContractDataFile, getCachePath } from "paladin-example-common";

const logger = console;

export interface ContractData {
  atomFactoryAddress: string;
  zetoCashAddress: string;
  notoAssetAddress: string;
  issuerGroupId: string;
  issuerGroupAddress: string;
  trackerAddress: string;
  atomAddress: string;
  swapDetails: {
    assetAmount: number;
    cashAmount: number;
    lockId: string;
    lockedStateId: string;
    assetUnlockCall: string;
    encodedCashTransfer: string;
  };
  finalBalances: {
    asset: {
      investor1: {
        totalBalance: string;
        totalStates: string;
        overflow: boolean;
      };
      investor2: {
        totalBalance: string;
        totalStates: string;
        overflow: boolean;
      };
    };
    cash: {
      investor1: {
        totalBalance: string;
        totalStates: string;
        overflow: boolean;
      };
      investor2: {
        totalBalance: string;
        totalStates: string;
        overflow: boolean;
      };
    };
  };
  participants: {
    cashIssuer: string;
    assetIssuer: string;
    investor1: string;
    investor2: string;
  };
  timestamp: string;
}

async function main(): Promise<boolean> {
  // --- Initialization from Imported Config ---
  if (nodeConnections.length < 3) {
    logger.error("The environment config must provide at least 3 nodes for this scenario.");
    return false;
  }
  
  logger.log("Initializing Paladin clients from the environment configuration...");
  const clients = nodeConnections.map(node => new PaladinClient(node.clientOptions));
  const [paladin1, paladin2, paladin3] = clients;

  // STEP 1: Load the saved contract data
  logger.log("STEP 1: Loading saved contract data...");
  // Use command-line argument for data directory if provided, otherwise use default
  const dataDir =getCachePath();
  const dataFile = findLatestContractDataFile(dataDir);
  
  if (!dataFile) {
    logger.error(`STEP 1: No contract data files found in ${dataDir}`);
    logger.error("Please run the original script first to deploy the contracts and save the data.");
    return false;
  }

  const contractData: ContractData = JSON.parse(fs.readFileSync(dataFile, 'utf8'));
  logger.log(`STEP 1: Loaded contract data from ${dataFile}`);

  // Print cached data summary
  logger.log("\n=== CACHED DATA SUMMARY ===");
  logger.log(`Data File: ${dataFile}`);
  logger.log(`Timestamp: ${contractData.timestamp}`);
  logger.log(`Atom Factory Address: ${contractData.atomFactoryAddress}`);
  logger.log(`Zeto Cash Address: ${contractData.zetoCashAddress}`);
  logger.log(`Noto Asset Address: ${contractData.notoAssetAddress}`);
  logger.log(`Issuer Group ID: ${contractData.issuerGroupId}`);
  logger.log(`Issuer Group Address: ${contractData.issuerGroupAddress}`);
  logger.log(`Tracker Address: ${contractData.trackerAddress}`);
  logger.log(`Atom Address: ${contractData.atomAddress}`);
  logger.log(`Asset Amount: ${contractData.swapDetails.assetAmount}`);
  logger.log(`Cash Amount: ${contractData.swapDetails.cashAmount}`);
  logger.log("=============================\n");

  // STEP 2: Get verifiers and recreate contract connections
  logger.log("STEP 2: Recreating contract connections...");
  const [cashIssuer, assetIssuer] = paladin1.getVerifiers(
    "cashIssuer@node1",
    "assetIssuer@node1"
  );
  const [investor1] = paladin2.getVerifiers("investor1@node2");
  const [investor2] = paladin3.getVerifiers("investor2@node3");

  // Import necessary classes from the SDK
  const { ZetoInstance, NotoInstance } = await import("@lfdecentralizedtrust/paladin-sdk");
  
  // Recreate contract instances
  const zetoCash = new ZetoInstance(paladin1, contractData.zetoCashAddress);
  const notoAsset = new NotoInstance(paladin1, contractData.notoAssetAddress);
  
  // Recreate privacy group connection
  const penteFactory = new PenteFactory(paladin1, "pente");
  const issuerGroup = await penteFactory.resumePrivacyGroup({
    id: contractData.issuerGroupId,
    // id: contractData.issuerGroupAddress,

  });

  if (!issuerGroup) {
    logger.error("STEP 2: Failed to retrieve privacy group!");
    return false;
  }

  logger.log("STEP 2: Contract connections recreated successfully!");

  // STEP 3: Verify asset token balances
  logger.log("STEP 3: Verifying asset token balances...");
  try {
    const currentAssetBalanceInvestor1 = await notoAsset.using(paladin2).balanceOf(investor1, { account: investor1.lookup });
    const currentAssetBalanceInvestor2 = await notoAsset.using(paladin3).balanceOf(investor2, { account: investor2.lookup });

    logger.log(`STEP 3: Current asset balances:`);
    logger.log(`Investor1: ${currentAssetBalanceInvestor1.totalBalance} units, ${currentAssetBalanceInvestor1.totalStates} states, overflow: ${currentAssetBalanceInvestor1.overflow}`);
    logger.log(`Investor2: ${currentAssetBalanceInvestor2.totalBalance} units, ${currentAssetBalanceInvestor2.totalStates} states, overflow: ${currentAssetBalanceInvestor2.overflow}`);

    // Verify balances match saved data
    if (currentAssetBalanceInvestor1.totalBalance !== contractData.finalBalances.asset.investor1.totalBalance) {
      logger.error(`STEP 3: ERROR - Investor1 asset balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.asset.investor1.totalBalance}`);
      logger.error(`Found: ${currentAssetBalanceInvestor1.totalBalance}`);
      return false;
    }

    if (currentAssetBalanceInvestor2.totalBalance !== contractData.finalBalances.asset.investor2.totalBalance) {
      logger.error(`STEP 3: ERROR - Investor2 asset balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.asset.investor2.totalBalance}`);
      logger.error(`Found: ${currentAssetBalanceInvestor2.totalBalance}`);
      return false;
    }

    logger.log("STEP 3: Asset balance verification successful!");

  } catch (error) {
    logger.error("STEP 3: Failed to retrieve asset balances!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 4: Verify cash token balances
  logger.log("STEP 4: Verifying cash token balances...");
  try {
    const currentCashBalanceInvestor1 = await zetoCash.using(paladin2).balanceOf(investor1, { account: investor1.lookup });
    const currentCashBalanceInvestor2 = await zetoCash.using(paladin3).balanceOf(investor2, { account: investor2.lookup });

    logger.log(`STEP 4: Current cash balances:`);
    logger.log(`Investor1: ${currentCashBalanceInvestor1.totalBalance} units, ${currentCashBalanceInvestor1.totalStates} states, overflow: ${currentCashBalanceInvestor1.overflow}`);
    logger.log(`Investor2: ${currentCashBalanceInvestor2.totalBalance} units, ${currentCashBalanceInvestor2.totalStates} states, overflow: ${currentCashBalanceInvestor2.overflow}`);

    // Verify balances match saved data
    if (currentCashBalanceInvestor1.totalBalance !== contractData.finalBalances.cash.investor1.totalBalance) {
      logger.error(`STEP 4: ERROR - Investor1 cash balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.cash.investor1.totalBalance}`);
      logger.error(`Found: ${currentCashBalanceInvestor1.totalBalance}`);
      return false;
    }

    if (currentCashBalanceInvestor2.totalBalance !== contractData.finalBalances.cash.investor2.totalBalance) {
      logger.error(`STEP 4: ERROR - Investor2 cash balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.cash.investor2.totalBalance}`);
      logger.error(`Found: ${currentCashBalanceInvestor2.totalBalance}`);
      return false;
    }

    logger.log("STEP 4: Cash balance verification successful!");

  } catch (error) {
    logger.error("STEP 4: Failed to retrieve cash balances!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 5: Test token functionality with new operations
  logger.log("STEP 5: Testing token functionality with new operations...");
  try {
    // Test asset token functionality by checking if we can query balances
    logger.log("STEP 5: Testing asset token query functionality...");
    const testAssetBalance = await notoAsset
      .using(paladin2)
      .balanceOf(investor1, { account: investor1.lookup });
    
    if (!testAssetBalance || testAssetBalance.totalBalance === undefined) {
      logger.error("STEP 5: Asset token query functionality test failed!");
      return false;
    }
    logger.log("STEP 5: Asset token query functionality test successful!");

    // Test cash token functionality by checking if we can query balances
    logger.log("STEP 5: Testing cash token query functionality...");
    const testCashBalance = await zetoCash.using(paladin2).balanceOf(investor1, { account: investor1.lookup });
    const testCashBalance2 = await zetoCash.using(paladin3).balanceOf(investor2, { account: investor2.lookup });
    
    if (!testCashBalance || testCashBalance.totalBalance === undefined) {
      logger.error("STEP 5: Cash token query functionality test failed!");
      return false;
    }
    logger.log("STEP 5: Cash token query functionality test successful!");

    // Test privacy group accessibility
    logger.log("STEP 5: Testing privacy group accessibility...");
    if (!issuerGroup || !issuerGroup.group || !issuerGroup.group.id) {
      logger.error("STEP 5: Privacy group accessibility test failed!");
      return false;
    }
    logger.log("STEP 5: Privacy group accessibility test successful!");

  } catch (error) {
    logger.error("STEP 5: Token functionality test failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 6: Verify swap details are preserved
  logger.log("STEP 6: Verifying swap details are preserved...");
  try {
    logger.log(`STEP 6: Swap details verification:`);
    logger.log(`Asset amount: ${contractData.swapDetails.assetAmount}`);
    logger.log(`Cash amount: ${contractData.swapDetails.cashAmount}`);
    logger.log(`Lock ID: ${contractData.swapDetails.lockId}`);
    logger.log(`Locked state ID: ${contractData.swapDetails.lockedStateId}`);
    logger.log(`Asset unlock call: ${contractData.swapDetails.assetUnlockCall.substring(0, 50)}...`);
    logger.log(`Encoded cash transfer: ${contractData.swapDetails.encodedCashTransfer.substring(0, 50)}...`);

    // Verify that the swap details are properly formatted
    if (!contractData.swapDetails.lockId || !contractData.swapDetails.lockedStateId) {
      logger.error("STEP 6: ERROR - Swap details are missing critical information!");
      return false;
    }

    if (!contractData.swapDetails.assetUnlockCall || !contractData.swapDetails.encodedCashTransfer) {
      logger.error("STEP 6: ERROR - Swap call data is missing!");
      return false;
    }

    logger.log("STEP 6: Swap details verification successful!");

  } catch (error) {
    logger.error("STEP 6: Swap details verification failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  logger.log("\nSUCCESS: Verification completed!");

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