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
  NotoFactory,
} from "@lfdecentralizedtrust/paladin-sdk";
import * as fs from 'fs';
import * as path from 'path';
import { nodeConnections, findLatestContractDataFile, getCachePath, DEFAULT_POLL_TIMEOUT } from "paladin-example-common";

const logger = console;

export interface ContractData {
  tokenAddress: string;
  notary: string;
  notaryMode: string;
  mintAmount: number;
  transferToNode2Amount: number;
  transferToNode3Amount: number;
  mintTransactionHash: string;
  transferToNode2TransactionHash: string;
  transferToNode3TransactionHash: string;
  finalBalances: {
    node1: {
      totalBalance: string;
      totalStates: string;
      overflow: boolean;
    };
    node2: {
      totalBalance: string;
      totalStates: string;
      overflow: boolean;
    };
    node3: {
      totalBalance: string;
      totalStates: string;
      overflow: boolean;
    };
  };
  node1Verifier: string;
  node2Verifier: string;
  node3Verifier: string;
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
  const verifiers = clients.map((client, i) => client.getVerifiers(`user@${nodeConnections[i].id}`)[0]);

  const [paladinClientNode1, paladinClientNode2, paladinClientNode3] = clients;
  const [verifierNode1, verifierNode2, verifierNode3] = verifiers;

  // STEP 1: Load the saved contract data
  logger.log("STEP 1: Loading saved contract data...");
  // Use command-line argument for data directory if provided, otherwise use default
  const dataDir = getCachePath();
  const dataFile = findLatestContractDataFile(dataDir);
  
  if (!dataFile) {
    logger.error(`STEP 1: No contract data files found in ${dataDir}`);
    logger.error("Please run the original script first to deploy the contract and save the data.");
    return false;
  }

  const contractData: ContractData = JSON.parse(fs.readFileSync(dataFile, 'utf8'));
  logger.log(`STEP 1: Loaded contract data from ${dataFile}`);
  
  // Print cached data summary
  logger.log("\n=== CACHED DATA SUMMARY ===");
  logger.log(`Data File: ${dataFile}`);
  logger.log(`Timestamp: ${contractData.timestamp}`);
  logger.log(`Token Address: ${contractData.tokenAddress}`);
  logger.log(`Notary: ${contractData.notary}`);
  logger.log(`Notary Mode: ${contractData.notaryMode}`);
  logger.log(`Mint Amount: ${contractData.mintAmount}`);
  logger.log(`Transfer to Node2: ${contractData.transferToNode2Amount}`);
  logger.log(`Transfer to Node3: ${contractData.transferToNode3Amount}`);
  logger.log(`Node1 Total Balance: ${contractData.finalBalances.node1.totalBalance}`);
  logger.log(`Node2 Total Balance: ${contractData.finalBalances.node2.totalBalance}`);
  logger.log(`Node3 Total Balance: ${contractData.finalBalances.node3.totalBalance}`);
  logger.log(`Mint TX Hash: ${contractData.mintTransactionHash}`);
  logger.log(`Transfer2 TX Hash: ${contractData.transferToNode2TransactionHash}`);
  logger.log(`Transfer3 TX Hash: ${contractData.transferToNode3TransactionHash}`);
  logger.log("=============================\n");

  // STEP 2: Get verifiers and recreate token connection
  logger.log("STEP 2: Recreating token connection...");
  // Import NotoInstance from the SDK
  const { NotoInstance } = await import("@lfdecentralizedtrust/paladin-sdk");
  const cashToken = new NotoInstance(paladinClientNode1, contractData.tokenAddress);
  
  if (!cashToken) {
    logger.error("STEP 2: Failed to retrieve token!");
    return false;
  }

  logger.log("STEP 2: Token connection recreated successfully!");

  // STEP 3: Verify current balances match saved data
  logger.log("STEP 3: Verifying current balances...");
  try {
    const currentBalanceNode1 = await cashToken.balanceOf(verifierNode1, {
      account: verifierNode1.lookup,
    });

    const currentBalanceNode2 = await cashToken.balanceOf(verifierNode1, {
      account: verifierNode2.lookup,
    });

    const currentBalanceNode3 = await cashToken.balanceOf(verifierNode1, {
      account: verifierNode3.lookup,
    });

    logger.log(`STEP 3: Current balances:`);
    logger.log(`Node1: ${currentBalanceNode1.totalBalance} units, ${currentBalanceNode1.totalStates} states, overflow: ${currentBalanceNode1.overflow}`);
    logger.log(`Node2: ${currentBalanceNode2.totalBalance} units, ${currentBalanceNode2.totalStates} states, overflow: ${currentBalanceNode2.overflow}`);
    logger.log(`Node3: ${currentBalanceNode3.totalBalance} units, ${currentBalanceNode3.totalStates} states, overflow: ${currentBalanceNode3.overflow}`);

    // Verify balances match saved data
    if (currentBalanceNode1.totalBalance !== contractData.finalBalances.node1.totalBalance) {
      logger.error(`STEP 3: ERROR - Node1 balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.node1.totalBalance}`);
      logger.error(`Found: ${currentBalanceNode1.totalBalance}`);
      return false;
    }

    if (currentBalanceNode2.totalBalance !== contractData.finalBalances.node2.totalBalance) {
      logger.error(`STEP 3: ERROR - Node2 balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.node2.totalBalance}`);
      logger.error(`Found: ${currentBalanceNode2.totalBalance}`);
      return false;
    }

    if (currentBalanceNode3.totalBalance !== contractData.finalBalances.node3.totalBalance) {
      logger.error(`STEP 3: ERROR - Node3 balance does not match saved data!`);
      logger.error(`Expected: ${contractData.finalBalances.node3.totalBalance}`);
      logger.error(`Found: ${currentBalanceNode3.totalBalance}`);
      return false;
    }

    logger.log("STEP 3: Balance verification successful!");

  } catch (error) {
    logger.error("STEP 3: Failed to retrieve balances!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 4: Test token functionality by performing a new transfer
  logger.log("STEP 4: Testing token functionality with a new transfer...");
  try {
    const testTransferAmount = 50;
    logger.log(`STEP 4: Transferring ${testTransferAmount} units from Node1 to Node2...`);
    
    const testTransferReceipt = await cashToken
      .transfer(verifierNode1, {
        to: verifierNode2,
        amount: testTransferAmount,
        data: "0x",
      })
      .waitForReceipt(DEFAULT_POLL_TIMEOUT);

    if (!testTransferReceipt?.transactionHash) {
      logger.error("STEP 4: Test transfer failed!");
      return false;
    }

    logger.log("STEP 4: Test transfer completed successfully!");

    // Verify the transfer worked by checking new balances
    const newBalanceNode1 = await cashToken.balanceOf(verifierNode1, {
      account: verifierNode1.lookup,
    });

    const newBalanceNode2 = await cashToken.balanceOf(verifierNode1, {
      account: verifierNode2.lookup,
    });

    const  newBalanceNode3 = await cashToken.balanceOf(verifierNode1, {
      account: verifierNode3.lookup,
    });

    const expectedNewBalanceNode1 = Number(contractData.finalBalances.node1.totalBalance) - testTransferAmount;
    const expectedNewBalanceNode2 = Number(contractData.finalBalances.node2.totalBalance) + testTransferAmount;

    if (Number(newBalanceNode1.totalBalance) !== expectedNewBalanceNode1) {
      logger.error(`STEP 4: ERROR - Node1 balance after test transfer is incorrect!`);
      logger.error(`Expected: ${expectedNewBalanceNode1}`);
      logger.error(`Found: ${newBalanceNode1.totalBalance}`);
      return false;
    }

    if (Number(newBalanceNode2.totalBalance) !== expectedNewBalanceNode2) {
      logger.error(`STEP 4: ERROR - Node2 balance after test transfer is incorrect!`);
      logger.error(`Expected: ${expectedNewBalanceNode2}`);
      logger.error(`Found: ${newBalanceNode2.totalBalance}`);
      return false;
    }

    logger.log("STEP 4: Test transfer verification successful!");
    logger.log(`New Node1 balance: ${newBalanceNode1.totalBalance}`);
    logger.log(`New Node2 balance: ${newBalanceNode2.totalBalance}`);
    logger.log(`New Node3 balance: ${newBalanceNode3.totalBalance}`);
    logger.log(`Test transfer transaction hash: ${testTransferReceipt.transactionHash}`);
    
    // save the new balance in a new file
    const newContractData: ContractData = {
      ...contractData,
      finalBalances: {
        ...contractData.finalBalances,
        node1: { ...contractData.finalBalances.node1, totalBalance: newBalanceNode1.totalBalance.toString() },
        node2: { ...contractData.finalBalances.node2, totalBalance: newBalanceNode2.totalBalance.toString() },
        node3: { ...contractData.finalBalances.node3, totalBalance: newBalanceNode3.totalBalance.toString() },
      },
    };

    const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
    const newDataFile = path.join(dataDir, `contract-data-${timestamp}.json`);
    fs.writeFileSync(newDataFile, JSON.stringify(newContractData, null, 2));
    logger.log(`New contract data saved to ${newDataFile}`);

  } catch (error) {
    logger.error("STEP 4: Token functionality test failed!");
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