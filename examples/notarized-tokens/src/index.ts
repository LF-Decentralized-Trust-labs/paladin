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
import { ContractData } from "./tests/data-persistence";
import { nodeConnections, getCachePath, DEFAULT_POLL_TIMEOUT } from "paladin-example-common";
import assert from "assert";

const logger = console;

async function main(): Promise<boolean> {
  // --- Initialization from Imported Config ---
  if (nodeConnections.length < 3) {
    logger.error("The environment config must provide at least 3 nodes for this scenario.");
    return false;
  }
  
  logger.log("Initializing Paladin clients from the environment configuration...");
  const clients = nodeConnections.map(node => new PaladinClient(node.clientOptions));
  const [paladinClientNode1, paladinClientNode2, paladinClientNode3] = clients;

  const [verifierNode1] = paladinClientNode1.getVerifiers(`user@${nodeConnections[0].id}`);
  const [verifierNode2] = paladinClientNode2.getVerifiers(`user@${nodeConnections[1].id}`);
  const [verifierNode3] = paladinClientNode3.getVerifiers(`user@${nodeConnections[2].id}`);

  const mintAmount = 2000;
  const transferToNode2Amount = 1000;
  const transferToNode3Amount = 800;

  // Step 1: Deploy a Noto token to represent cash
  logger.log("Step 1: Deploying a Noto cash token...");
  const notoFactory = new NotoFactory(paladinClientNode1, "noto");
  const cashToken = await notoFactory
    .newNoto(verifierNode1, {
      name: "NOTO",
      symbol: "NOTO",
      notary: verifierNode1,
      notaryMode: "basic",
    })
    .waitForDeploy(DEFAULT_POLL_TIMEOUT);
  if (!cashToken) {
    logger.error("Failed to deploy the Noto cash token!");
    return false;
  }
  logger.log("Noto cash token deployed successfully!");

 

  // Step 2: Mint cash tokens
  logger.log(`Step 2: Minting ${mintAmount} units of cash to Node1...`);
  const mintReceipt = await cashToken
    .mint(verifierNode1, {
      to: verifierNode1,
      amount: mintAmount,
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!mintReceipt) {
    logger.error("Failed to mint cash tokens!");
    return false;
  }
  
  // Validate mint transaction was successful
  if (!mintReceipt.success) {
    logger.error("Mint transaction failed!");
    return false;
  }

  // test that minted amount is correct
  const balance = await cashToken.balanceOf(verifierNode1, {
    account: verifierNode1.lookup,
  });
  logger.log(`Balance of the token: ${balance.totalBalance}`);
  assert(balance.totalBalance === mintAmount.toString(), `Balance of the token should be ${mintAmount}`);

  logger.log(`Successfully minted ${mintAmount} units of cash to Node1!`);
  let balanceNode1 = await cashToken.balanceOf(verifierNode1, {
    account: verifierNode1.lookup,
  });
  
  // Validate the balance was updated correctly
  if (balanceNode1.totalBalance !== mintAmount.toString()) {
    logger.error(`Mint validation failed! Expected balance: ${mintAmount}, Actual balance: ${balanceNode1.totalBalance}`);
    return false;
  }
  
  logger.log(
    `Node1 State: ${balanceNode1.totalBalance} units of cash, ${balanceNode1.totalStates} states, overflow: ${balanceNode1.overflow}`
  );

  // Step 3: Transfer cash to Node2
  logger.log("Step 3: Transferring 1000 units of cash from Node1 to Node2...");
  const transferToNode2 = await cashToken
    .transfer(verifierNode1, {
      to: verifierNode2,
      amount: transferToNode2Amount,
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!transferToNode2) {
    logger.error("Failed to transfer cash to Node2!");
    return false;
  }
  
  // Validate transfer transaction was successful
  if (!transferToNode2.success) {
    logger.error("Transfer to Node2 transaction failed!");
    return false;
  }
  
 
  logger.log(`Successfully transferred ${transferToNode2Amount} units of cash to Node2!`);
  let balanceNode2 = await cashToken.balanceOf(verifierNode1, {
    account: verifierNode2.lookup,
  });
  
  // Validate the balance was updated correctly
  if (balanceNode2.totalBalance !== transferToNode2Amount.toString()) {
    logger.error(`Transfer to Node2 validation failed! Expected balance: ${transferToNode2Amount}, Actual balance: ${balanceNode2.totalBalance}`);
    return false;
  }

  logger.log(
    `Node2 State: ${balanceNode2.totalBalance} units of cash, ${balanceNode2.totalStates} states, overflow: ${balanceNode2.overflow}`
  );

  // Step 4: Transfer cash to Node3 from Node2
  logger.log("Step 4: Transferring 800 units of cash from Node2 to Node3...");
  const transferToNode3 = await cashToken
    .using(paladinClientNode2)
    .transfer(verifierNode2, {
      to: verifierNode3,
      amount: transferToNode3Amount,
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!transferToNode3) {
    logger.error("Failed to transfer cash to Node3!");
    return false;
  }
  
  // Validate transfer transaction was successful
  if (!transferToNode3.success) {
    logger.error("Transfer to Node3 transaction failed!");
    return false;
  }
  
  logger.log(`Successfully transferred ${transferToNode3Amount} units of cash to Node3!`);
  let balanceNode3 = await cashToken.balanceOf(verifierNode1, {
    account: verifierNode3.lookup,
  });
  
  // Validate the balance was updated correctly
  if (balanceNode3.totalBalance !== transferToNode3Amount.toString()) {
    logger.error(`Transfer to Node3 validation failed! Expected balance: ${transferToNode3Amount}, Actual balance: ${balanceNode3.totalBalance}`);
    return false;
  }
  
  logger.log(
    `Node3 State: ${balanceNode3.totalBalance} units of cash, ${balanceNode3.totalStates} states, overflow: ${balanceNode3.overflow}`
  );

  // Validate final balances after all operations
  const finalBalanceNode1 = await cashToken.balanceOf(verifierNode1, {
    account: verifierNode1.lookup,
  });
  const finalBalanceNode2 = await cashToken.balanceOf(verifierNode1, {
    account: verifierNode2.lookup,
  });
  const finalBalanceNode3 = await cashToken.balanceOf(verifierNode1, {
    account: verifierNode3.lookup,
  });

  // Validate final balances match expected values
  const expectedBalanceNode1 = mintAmount - transferToNode2Amount;
  const expectedBalanceNode2 = transferToNode2Amount - transferToNode3Amount;
  const expectedBalanceNode3 = transferToNode3Amount;

  if (finalBalanceNode1.totalBalance !== expectedBalanceNode1.toString()) {
    logger.error(`Final balance validation failed for Node1! Expected: ${expectedBalanceNode1}, Actual: ${finalBalanceNode1.totalBalance}`);
    return false;
  }
  if (finalBalanceNode2.totalBalance !== expectedBalanceNode2.toString()) {
    logger.error(`Final balance validation failed for Node2! Expected: ${expectedBalanceNode2}, Actual: ${finalBalanceNode2.totalBalance}`);
    return false;
  }
  if (finalBalanceNode3.totalBalance !== expectedBalanceNode3.toString()) {
    logger.error(`Final balance validation failed for Node3! Expected: ${expectedBalanceNode3}, Actual: ${finalBalanceNode3.totalBalance}`);
    return false;
  }

  logger.log(`Final balances - Node1: ${finalBalanceNode1.totalBalance}, Node2: ${finalBalanceNode2.totalBalance}, Node3: ${finalBalanceNode3.totalBalance}`);

  // Save contract data to file for later use
  // should be of type
  const contractData : ContractData =  { 
    tokenAddress: cashToken.address,
    notary: verifierNode1.lookup,
    notaryMode: "basic",
    mintAmount: mintAmount,
    transferToNode2Amount: transferToNode2Amount,
    transferToNode3Amount: transferToNode3Amount,
    mintTransactionHash: mintReceipt?.transactionHash,
    transferToNode2TransactionHash: transferToNode2?.transactionHash,
    transferToNode3TransactionHash: transferToNode3?.transactionHash,
    finalBalances: {
      node1: {
        totalBalance: finalBalanceNode1.totalBalance,
        totalStates: finalBalanceNode1.totalStates,
        overflow: finalBalanceNode1.overflow
      },
      node2: {
        totalBalance: finalBalanceNode2.totalBalance,
        totalStates: finalBalanceNode2.totalStates,
        overflow: finalBalanceNode2.overflow
      },
      node3: {
        totalBalance: finalBalanceNode3.totalBalance,
        totalStates: finalBalanceNode3.totalStates,
        overflow: finalBalanceNode3.overflow
      }
    },
    node1Verifier: verifierNode1.lookup,
    node2Verifier: verifierNode2.lookup,
    node3Verifier: verifierNode3.lookup,
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

  // All steps completed successfully
  logger.log("All operations completed successfully!");
  return true;
}

// Execute the main function if this file is run directly
if (require.main === module) {
  main()
    .then((success: boolean) => {
      process.exit(success ? 0 : 1); // Exit with 0 for success, 1 for failure
    })
    .catch((err) => {
      logger.error("Exiting due to an uncaught error:", err);
      process.exit(1); // Exit with status 1 for any uncaught errors
    });
}
