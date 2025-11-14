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
} from "@lfdecentralizedtrust/paladin-sdk";
import * as fs from 'fs';
import * as path from 'path';
import { nodeConnections, findLatestContractDataFile, getCachePath } from "paladin-example-common";

const logger = console;

export interface ContractData {
  notoCashAddress: string;
  notoBondAddress: string;
  issuerCustodianGroupId: string;
  issuerCustodianGroupAddress: string;
  investorCustodianGroupId: string;
  investorCustodianGroupAddress: string;
  bondTrackerAddress: string;
  bondTrackerPublicAddress: string;
  bondSubscriptionAddress: string;
  atomFactoryAddress: string;
  atomAddress: string;
  bondDetails: {
    issueDate: number;
    maturityDate: number;
    faceValue: number;
    discountPrice: number;
    minimumDenomination: number;
    bondUnits: number;
    cashAmount: number;
  };
  lockDetails: {
    cashLockId: string;
    bondLockId: string;
    cashUnlockCall: string;
    assetUnlockCall: string;
  };
  finalBalances: {
    cash: {
      investor: {
        totalBalance: string;
        totalStates: string;
        overflow: boolean;
      };
      custodian: {
        totalBalance: string;
        totalStates: string;
        overflow: boolean;
      };
    };
    bond: {
      investor: {
        totalBalance: string;
        totalStates: string;
        overflow: boolean;
      };
      custodian: {
        totalBalance: string;
        totalStates: string;
        overflow: boolean;
      };
    };
  };
  participants: {
    cashIssuer: string;
    bondIssuer: string;
    bondCustodian: string;
    investor: string;
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

  const cachePath = getCachePath();
  // Use command-line argument for data directory if provided, otherwise use default
  const dataFile = findLatestContractDataFile(cachePath);
  
  if (!dataFile) {
    logger.error(`STEP 1: No contract data files found in ${dataFile}`);
    logger.error("Please run the original script first to deploy the contracts and save the data.");
    return false;
  }

  const contractData: ContractData = JSON.parse(fs.readFileSync(dataFile, 'utf8'));
  logger.log(`STEP 1: Loaded contract data from ${dataFile}`);

  // Print cached data summary
  logger.log("\n=== CACHED DATA SUMMARY ===");
  logger.log(`Data File: ${dataFile}`);
  logger.log(`Timestamp: ${contractData.timestamp}`);
  logger.log(`Noto Cash Address: ${contractData.notoCashAddress}`);
  logger.log(`Noto Bond Address: ${contractData.notoBondAddress}`);
  logger.log(`Issuer-Custodian Group ID: ${contractData.issuerCustodianGroupId}`);
  logger.log(`Investor-Custodian Group ID: ${contractData.investorCustodianGroupId}`);
  logger.log(`Bond Tracker Address: ${contractData.bondTrackerAddress}`);
  logger.log(`Bond Subscription Address: ${contractData.bondSubscriptionAddress}`);
  logger.log(`Atom Factory Address: ${contractData.atomFactoryAddress}`);
  logger.log(`Atom Address: ${contractData.atomAddress}`);
  logger.log(`Bond Units: ${contractData.bondDetails.bondUnits}`);
  logger.log(`Cash Amount: ${contractData.bondDetails.cashAmount}`);
  logger.log("=============================\n");

  // STEP 2: Get verifiers and recreate contract connections
  logger.log("STEP 2: Recreating contract connections...");
  const [cashIssuer, bondIssuer] = paladin1.getVerifiers(
    "cashIssuer@node1",
    "bondIssuer@node1"
  );
  const [bondCustodian] = paladin2.getVerifiers("bondCustodian@node2");
  const [investor] = paladin3.getVerifiers("investor@node3");

  // Import necessary classes from the SDK
  const { NotoInstance } = await import("@lfdecentralizedtrust/paladin-sdk");
  
  // Recreate contract instances
  const notoCash = new NotoInstance(paladin1, contractData.notoCashAddress);
  const notoBond = new NotoInstance(paladin1, contractData.notoBondAddress);
  
  // Recreate privacy group connections
  const penteFactory = new PenteFactory(paladin1, "pente");
  const issuerCustodianGroup = await penteFactory.resumePrivacyGroup({
    id: contractData.issuerCustodianGroupId,
  });

  const investorCustodianGroup = await penteFactory.using(paladin3).resumePrivacyGroup({
    id: contractData.investorCustodianGroupId,
  });

  if (!issuerCustodianGroup) {
    logger.error("STEP 2: Failed to retrieve issuer-custodian privacy group!");
    return false;
  }

  if (!investorCustodianGroup) {
    logger.error("STEP 2: Failed to retrieve investor-custodian privacy group!");
    return false;
  }

  logger.log("STEP 2: Contract connections recreated successfully!");

  // STEP 3: Verify cash token balances
  logger.log("STEP 3: Verifying cash token balances...");
  try {
    const currentCashBalanceInvestor = await notoCash
      .using(paladin3)
      .balanceOf(investor, { account: investor.lookup });

    const currentCashBalanceCustodian = await notoCash
      .using(paladin2)
      .balanceOf(bondCustodian, { account: bondCustodian.lookup });

    logger.log(`STEP 3: Current cash balances:`);
    logger.log(`Investor: ${currentCashBalanceInvestor.totalBalance} units, ${currentCashBalanceInvestor.totalStates} states, overflow: ${currentCashBalanceInvestor.overflow}`);
    logger.log(`Custodian: ${currentCashBalanceCustodian.totalBalance} units, ${currentCashBalanceCustodian.totalStates} states, overflow: ${currentCashBalanceCustodian.overflow}`);

    // Verify balances match saved data
    if (currentCashBalanceInvestor.totalBalance !== contractData.finalBalances.cash.investor.totalBalance) {
      logger.error(`STEP 3: ERROR - Investor cash balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.cash.investor.totalBalance}`);
      logger.error(`Found: ${currentCashBalanceInvestor.totalBalance}`);
      return false;
    }

    if (currentCashBalanceCustodian.totalBalance !== contractData.finalBalances.cash.custodian.totalBalance) {
      logger.error(`STEP 3: ERROR - Custodian cash balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.cash.custodian.totalBalance}`);
      logger.error(`Found: ${currentCashBalanceCustodian.totalBalance}`);
      return false;
    }

    logger.log("STEP 3: Cash balance verification successful!");

  } catch (error) {
    logger.error("STEP 3: Failed to retrieve cash balances!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 4: Verify bond token balances
  logger.log("STEP 4: Verifying bond token balances...");
  try {
    const currentBondBalanceInvestor = await notoBond
      .using(paladin3)
      .balanceOf(investor, { account: investor.lookup });

    const currentBondBalanceCustodian = await notoBond
      .using(paladin2)
      .balanceOf(bondCustodian, { account: bondCustodian.lookup });

    logger.log(`STEP 4: Current bond balances:`);
    logger.log(`Investor: ${currentBondBalanceInvestor.totalBalance} units, ${currentBondBalanceInvestor.totalStates} states, overflow: ${currentBondBalanceInvestor.overflow}`);
    logger.log(`Custodian: ${currentBondBalanceCustodian.totalBalance} units, ${currentBondBalanceCustodian.totalStates} states, overflow: ${currentBondBalanceCustodian.overflow}`);

    // Verify balances match saved data
    if (currentBondBalanceInvestor.totalBalance !== contractData.finalBalances.bond.investor.totalBalance) {
      logger.error(`STEP 4: ERROR - Investor bond balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.bond.investor.totalBalance}`);
      logger.error(`Found: ${currentBondBalanceInvestor.totalBalance}`);
      return false;
    }

    if (currentBondBalanceCustodian.totalBalance !== contractData.finalBalances.bond.custodian.totalBalance) {
      logger.error(`STEP 4: ERROR - Custodian bond balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.bond.custodian.totalBalance}`);
      logger.error(`Found: ${currentBondBalanceCustodian.totalBalance}`);
      return false;
    }

    logger.log("STEP 4: Bond balance verification successful!");

  } catch (error) {
    logger.error("STEP 4: Failed to retrieve bond balances!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 5: Test token functionality with new operations
  logger.log("STEP 5: Testing token functionality with new operations...");
  try {
    // Test cash token functionality by checking if we can query balances
    logger.log("STEP 5: Testing cash token query functionality...");
    const testCashBalance = await notoCash
      .using(paladin3)
      .balanceOf(investor, { account: investor.lookup });
    
    if (!testCashBalance || testCashBalance.totalBalance === undefined) {
      logger.error("STEP 5: Cash token query functionality test failed!");
      return false;
    }
    logger.log("STEP 5: Cash token query functionality test successful!");

    // Test bond token functionality by checking if we can query balances
    logger.log("STEP 5: Testing bond token query functionality...");
    const testBondBalance = await notoBond
      .using(paladin3)
      .balanceOf(investor, { account: investor.lookup });
    
    if (!testBondBalance || testBondBalance.totalBalance === undefined) {
      logger.error("STEP 5: Bond token query functionality test failed!");
      return false;
    }
    logger.log("STEP 5: Bond token query functionality test successful!");

    // Test privacy group accessibility
    logger.log("STEP 5: Testing privacy group accessibility...");
    if (!issuerCustodianGroup || !issuerCustodianGroup.group || !issuerCustodianGroup.group.id) {
      logger.error("STEP 5: Issuer-custodian privacy group accessibility test failed!");
      return false;
    }
    if (!investorCustodianGroup || !investorCustodianGroup.group || !investorCustodianGroup.group.id) {
      logger.error("STEP 5: Investor-custodian privacy group accessibility test failed!");
      return false;
    }
    logger.log("STEP 5: Privacy group accessibility test successful!");

  } catch (error) {
    logger.error("STEP 5: Token functionality test failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 6: Verify bond details are preserved
  logger.log("STEP 6: Verifying bond details are preserved...");
  try {
    logger.log(`STEP 6: Bond details verification:`);
    logger.log(`Issue Date: ${contractData.bondDetails.issueDate}`);
    logger.log(`Maturity Date: ${contractData.bondDetails.maturityDate}`);
    logger.log(`Face Value: ${contractData.bondDetails.faceValue}`);
    logger.log(`Discount Price: ${contractData.bondDetails.discountPrice}`);
    logger.log(`Minimum Denomination: ${contractData.bondDetails.minimumDenomination}`);
    logger.log(`Bond Units: ${contractData.bondDetails.bondUnits}`);
    logger.log(`Cash Amount: ${contractData.bondDetails.cashAmount}`);

    // Verify that the bond details are properly formatted
    if (!contractData.bondDetails.issueDate || !contractData.bondDetails.maturityDate) {
      logger.error("STEP 6: ERROR - Bond details are missing critical information!");
      return false;
    }

    if (contractData.bondDetails.bondUnits <= 0 || contractData.bondDetails.cashAmount <= 0) {
      logger.error("STEP 6: ERROR - Bond amounts are invalid!");
      return false;
    }

    logger.log("STEP 6: Bond details verification successful!");

  } catch (error) {
    logger.error("STEP 6: Bond details verification failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 7: Verify lock details are preserved
  logger.log("STEP 7: Verifying lock details are preserved...");
  try {
    logger.log(`STEP 7: Lock details verification:`);
    logger.log(`Cash Lock ID: ${contractData.lockDetails.cashLockId}`);
    logger.log(`Bond Lock ID: ${contractData.lockDetails.bondLockId}`);
    logger.log(`Cash Unlock Call: ${contractData.lockDetails.cashUnlockCall.substring(0, 50)}...`);
    logger.log(`Asset Unlock Call: ${contractData.lockDetails.assetUnlockCall.substring(0, 50)}...`);

    // Verify that the lock details are properly formatted
    if (!contractData.lockDetails.cashLockId || !contractData.lockDetails.bondLockId) {
      logger.error("STEP 7: ERROR - Lock details are missing critical information!");
      return false;
    }

    if (!contractData.lockDetails.cashUnlockCall || !contractData.lockDetails.assetUnlockCall) {
      logger.error("STEP 7: ERROR - Unlock call data is missing!");
      return false;
    }

    logger.log("STEP 7: Lock details verification successful!");

  } catch (error) {
    logger.error("STEP 7: Lock details verification failed!");
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