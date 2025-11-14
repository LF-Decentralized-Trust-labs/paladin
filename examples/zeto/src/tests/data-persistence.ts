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
  ZetoFactory,
} from "@lfdecentralizedtrust/paladin-sdk";
import * as fs from 'fs';
import * as path from 'path';
import { nodeConnections, findLatestContractDataFile, getCachePath, DEFAULT_POLL_TIMEOUT } from "paladin-example-common";

const logger = console;

export interface ContractData {
  zetoCBDC1Address: string;
  zetoCBDC2Address: string;
  erc20Address: string;
  tokenName: string;
  useCase1: {
    mintAmounts: number[];
    transferAmount: number;
    finalBalances: {
      bank1: {
        totalBalance: string;
        totalStates: string;
        overflow: boolean;
      };
      bank2: {
        totalBalance: string;
        totalStates: string;
        overflow: boolean;
      };
    };
  };
  useCase2: {
    erc20MintAmount: number;
    erc20ApproveAmount: number;
    depositAmount: number;
    transferAmount: number;
    withdrawAmount: number;
    finalBalances: {
      bank1: {
        totalBalance: string;
        totalStates: string;
        overflow: boolean;
      };
      bank2: {
        totalBalance: string;
        totalStates: string;
        overflow: boolean;
      };
    };
  };
  cbdcIssuer: string;
  bank1: string;
  bank2: string;
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
  const [bank1] = paladin2.getVerifiers(`bank1@${nodeConnections[0].id}`);
  const [bank2] = paladin3.getVerifiers(`bank2@${nodeConnections[1].id}`);

  // STEP 1: Load the saved contract data
  logger.log("STEP 1: Loading saved contract data...");
  // Use command-line argument for data directory if provided, otherwise use default
  const dataDir = getCachePath();
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
  logger.log(`Zeto CBDC1 Address: ${contractData.zetoCBDC1Address}`);
  logger.log(`Zeto CBDC2 Address: ${contractData.zetoCBDC2Address}`);
  logger.log(`ERC20 Address: ${contractData.erc20Address}`);
  logger.log(`Token Name: ${contractData.tokenName}`);
  logger.log(`CBDC Issuer: ${contractData.cbdcIssuer}`);
  logger.log(`Bank1: ${contractData.bank1}`);
  logger.log(`Bank2: ${contractData.bank2}`);
  logger.log(`Use Case 1 - Transfer Amount: ${contractData.useCase1.transferAmount}`);
  logger.log(`Use Case 2 - Deposit Amount: ${contractData.useCase2.depositAmount}`);
  logger.log(`Use Case 2 - Transfer Amount: ${contractData.useCase2.transferAmount}`);
  logger.log(`Use Case 2 - Withdraw Amount: ${contractData.useCase2.withdrawAmount}`);
  logger.log("=============================\n");

  // STEP 2: Get verifiers and recreate token connections
  logger.log("STEP 2: Recreating token connections...");

  const zetoFactory = new ZetoFactory(paladin3, "zeto");
  
  // Import ZetoInstance from the SDK
  const { ZetoInstance } = await import("@lfdecentralizedtrust/paladin-sdk");
  const zetoCBDC1 = new ZetoInstance(paladin3, contractData.zetoCBDC1Address);
  const zetoCBDC2 = new ZetoInstance(paladin3, contractData.zetoCBDC2Address);

  logger.log("STEP 2: Token connections recreated successfully!");

  // STEP 3: Verify Use Case 1 - Private minting balances
  logger.log("STEP 3: Verifying Use Case 1 (Private minting) balances...");
  try {
    const currentBank1Balance = await zetoCBDC1
      .using(paladin1)
      .balanceOf(bank1, { account: bank1.lookup });

    const currentBank2Balance = await zetoCBDC1
      .using(paladin2)
      .balanceOf(bank2, { account: bank2.lookup });

    logger.log(`STEP 3: Current Use Case 1 balances:`);
    logger.log(`Bank1: ${currentBank1Balance.totalBalance} units, ${currentBank1Balance.totalStates} states, overflow: ${currentBank1Balance.overflow}`);
    logger.log(`Bank2: ${currentBank2Balance.totalBalance} units, ${currentBank2Balance.totalStates} states, overflow: ${currentBank2Balance.overflow}`);

    // Verify balances match saved data exactly
    if (currentBank1Balance.totalBalance !== contractData.useCase1.finalBalances.bank1.totalBalance) {
      logger.error(`STEP 3: ERROR - Bank1 balance does not match saved data!`);
      logger.error(`Expected: ${contractData.useCase1.finalBalances.bank1.totalBalance}`);
      logger.error(`Found: ${currentBank1Balance.totalBalance}`);
      return false;
    }

    if (currentBank2Balance.totalBalance !== contractData.useCase1.finalBalances.bank2.totalBalance) {
      logger.error(`STEP 3: ERROR - Bank2 balance does not match saved data!`);
      logger.error(`Expected: ${contractData.useCase1.finalBalances.bank2.totalBalance}`);
      logger.error(`Found: ${currentBank2Balance.totalBalance}`);
      return false;
    }

    logger.log("STEP 3: Use Case 1 balance verification successful!");

  } catch (error) {
    logger.error("STEP 3: Failed to retrieve Use Case 1 balances!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 4: Verify Use Case 2 - Public minting balances
  logger.log("STEP 4: Verifying Use Case 2 (Public minting) balances...");
  try {
    const currentBank1BalanceUC2 = await zetoCBDC2
      .using(paladin1)
      .balanceOf(bank1, { account: bank1.lookup });

    const currentBank2BalanceUC2 = await zetoCBDC2
      .using(paladin2)
      .balanceOf(bank2, { account: bank2.lookup });

    logger.log(`STEP 4: Current Use Case 2 balances:`);
    logger.log(`Bank1: ${currentBank1BalanceUC2.totalBalance} units, ${currentBank1BalanceUC2.totalStates} states, overflow: ${currentBank1BalanceUC2.overflow}`);
    logger.log(`Bank2: ${currentBank2BalanceUC2.totalBalance} units, ${currentBank2BalanceUC2.totalStates} states, overflow: ${currentBank2BalanceUC2.overflow}`);

    // Verify balances match saved data exactly
    if (currentBank1BalanceUC2.totalBalance !== contractData.useCase2.finalBalances.bank1.totalBalance) {
      logger.error(`STEP 4: ERROR - Bank1 Use Case 2 balance does not match saved data!`);
      logger.error(`Expected: ${contractData.useCase2.finalBalances.bank1.totalBalance}`);
      logger.error(`Found: ${currentBank1BalanceUC2.totalBalance}`);
      return false;
    }

    if (currentBank2BalanceUC2.totalBalance !== contractData.useCase2.finalBalances.bank2.totalBalance) {
      logger.error(`STEP 4: ERROR - Bank2 Use Case 2 balance does not match saved data!`);
      logger.error(`Expected: ${contractData.useCase2.finalBalances.bank2.totalBalance}`);
      logger.error(`Found: ${currentBank2BalanceUC2.totalBalance}`);
      return false;
    }

    logger.log("STEP 4: Use Case 2 balance verification successful!");

  } catch (error) {
    logger.error("STEP 4: Failed to retrieve Use Case 2 balances!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 5: Test token functionality with new transfers
  logger.log("STEP 5: Testing token functionality with new transfers...");
  try {
    // Save the current state BEFORE running tests
    const initialStateBank1 = await zetoCBDC1
      .using(paladin1)
      .balanceOf(bank1, { account: bank1.lookup });
    const initialStateBank2 = await zetoCBDC1
      .using(paladin2)
      .balanceOf(bank2, { account: bank2.lookup });

    const testTransferAmount = 50;
    logger.log(`STEP 5: Testing transfer of ${testTransferAmount} units from Bank1 to Bank2 in Use Case 1...`);
    logger.log(`Initial Bank1: ${initialStateBank1.totalBalance}, Initial Bank2: ${initialStateBank2.totalBalance}`);
    
    const testTransferReceipt = await zetoCBDC1
      .using(paladin1)
      .transfer(bank1, {
        transfers: [
          {
            to: bank2,
            amount: testTransferAmount,
            data: "0x",
          },
        ],
      })
      .waitForReceipt(DEFAULT_POLL_TIMEOUT);

    if (!testTransferReceipt?.transactionHash) {
      logger.error("STEP 5: Test transfer failed!");
      return false;
    }

    // Add a small delay to ensure state is settled
    await new Promise((resolve) => setTimeout(resolve, 2000));

    logger.log("STEP 5: Test transfer completed successfully!");

    // Verify the transfer worked by checking new balances
    const newBank1Balance = await zetoCBDC1
      .using(paladin1)
      .balanceOf(bank1, { account: bank1.lookup });

    const newBank2Balance = await zetoCBDC1
      .using(paladin2)
      .balanceOf(bank2, { account: bank2.lookup });

    const expectedNewBank1Balance = Number(initialStateBank1.totalBalance) - testTransferAmount;
    const expectedNewBank2Balance = Number(initialStateBank2.totalBalance) + testTransferAmount;

    if (Number(newBank1Balance.totalBalance) !== expectedNewBank1Balance) {
      logger.error(`STEP 5: ERROR - Bank1 balance after test transfer is incorrect!`);
      logger.error(`Expected: ${expectedNewBank1Balance}`);
      logger.error(`Found: ${newBank1Balance.totalBalance}`);
      return false;
    }

    if (Number(newBank2Balance.totalBalance) !== expectedNewBank2Balance) {
      logger.error(`STEP 5: ERROR - Bank2 balance after test transfer is incorrect!`);
      logger.error(`Expected: ${expectedNewBank2Balance}`);
      logger.error(`Found: ${newBank2Balance.totalBalance}`);
      return false;
    }

    logger.log("STEP 5: Test transfer verification successful!");
    logger.log(`New Bank1 balance: ${newBank1Balance.totalBalance}`);
    logger.log(`New Bank2 balance: ${newBank2Balance.totalBalance}`);
    logger.log(`Test transfer transaction hash: ${testTransferReceipt.transactionHash}`);

  } catch (error) {
    logger.error("STEP 5: Token functionality test failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 6: Verify the original saved state is still accessible (but don't try to restore it)
  logger.log("STEP 6: Verifying original saved state is still accessible...");
  try {
    // Just verify we can still read the balances - they may have changed due to our tests
    const currentBank1Balance = await zetoCBDC1
      .using(paladin1)
      .balanceOf(bank1, { account: bank1.lookup });
    const currentBank2Balance = await zetoCBDC1
      .using(paladin2)
      .balanceOf(bank2, { account: bank2.lookup });

    logger.log(`STEP 6: Current balances after tests:`);
    logger.log(`Bank1: ${currentBank1Balance.totalBalance} units, ${currentBank1Balance.totalStates} states`);
    logger.log(`Bank2: ${currentBank2Balance.totalBalance} units, ${currentBank2Balance.totalStates} states`);
    logger.log(`Original saved Bank1: ${contractData.useCase1.finalBalances.bank1.totalBalance}`);
    logger.log(`Original saved Bank2: ${contractData.useCase1.finalBalances.bank2.totalBalance}`);

    logger.log("STEP 6: State accessibility verification successful!");

    // save the new balance in a new file
    const newContractData: ContractData = {
      ...contractData,
      useCase1: {
        ...contractData.useCase1,
        finalBalances: {
          ...contractData.useCase1.finalBalances,
          bank1: { ...contractData.useCase1.finalBalances.bank1, totalBalance: currentBank1Balance.totalBalance.toString() },
          bank2: { ...contractData.useCase1.finalBalances.bank2, totalBalance: currentBank2Balance.totalBalance.toString() },
        },
      },
    };

    const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
    const newDataFile = path.join(dataDir, `contract-data-${timestamp}.json`);
    fs.writeFileSync(newDataFile, JSON.stringify(newContractData, null, 2));
    logger.log(`New contract data saved to ${newDataFile}`);

  } catch (error) {
    logger.error("STEP 6: State accessibility verification failed!");
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