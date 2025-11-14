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
  TransactionType,
} from "@lfdecentralizedtrust/paladin-sdk";
import storageJson from "./abis/Storage.json";
import * as fs from 'fs';
import * as path from 'path';
import { nodeConnections, getCachePath, DEFAULT_POLL_TIMEOUT } from "paladin-example-common";

const logger = console;

async function main(): Promise<boolean> {
  // --- Initialization from Imported Config ---
  if (nodeConnections.length < 1) {
    logger.error("The environment config must provide at least 1 node for this scenario.");
    return false;
  }
  
  logger.log("Initializing Paladin client from the environment configuration...");
  const paladin = new PaladinClient(nodeConnections[0].clientOptions);
  const [owner] = paladin.getVerifiers(`owner@${nodeConnections[0].id}`);

  // Step 1: Deploy the Storage contract
  logger.log("Step 1: Deploying the Storage contract...");
  const deploymentTxID = await paladin.ptx.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: storageJson.abi,
    bytecode: storageJson.bytecode,
    from: owner.lookup,
    data: {},
  });

  // Wait for deployment receipt
  const deploymentReceipt = await paladin.pollForReceipt(deploymentTxID, DEFAULT_POLL_TIMEOUT);
  if (!deploymentReceipt?.contractAddress) {
    logger.error("Deployment failed!");
    return false;
  }
  
  // Validate deployment was successful
  if (!deploymentReceipt.success) {
    logger.error("Deployment transaction failed!");
    return false;
  }
  
  logger.log("Step 1: Storage contract deployed successfully!");

  // Step 2: Store a value in the contract
  const valueToStore = 125; // Example value to store
  logger.log(`Step 2: Storing value "${valueToStore}" in the contract...`);
  const storeTxID = await paladin.ptx.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: storageJson.abi,
    function: "store",
    from: owner.lookup,
    to: deploymentReceipt.contractAddress,
    data: { num: valueToStore },
  });

  // Wait for the store transaction receipt
  const storeReceipt = await paladin.pollForReceipt(storeTxID, DEFAULT_POLL_TIMEOUT);
  if (!storeReceipt?.transactionHash) {
    logger.error("Failed to store value in the contract!");
    return false;
  }
  
  // Validate store transaction was successful
  if (!storeReceipt.success) {
    logger.error("Store transaction failed!");
    return false;
  }
  
  logger.log("Step 2: Value stored successfully!" );

  // Step 3: Retrieve the stored value from the contract
  logger.log("Step 3: Retrieving the stored value...");
  const retrieveResult = await paladin.ptx.call({
    type: TransactionType.PUBLIC,
    abi: storageJson.abi,
    function: "retrieve",
    from: owner.lookup,
    to: deploymentReceipt.contractAddress,
    data: {},
  });

  // Validate the retrieved value
  const retrievedValue = retrieveResult["value"];
  if (retrievedValue !== valueToStore.toString()) {
    logger.error(`Retrieved value "${retrievedValue}" does not match stored value "${valueToStore}"!`);
    return false;
  }

  logger.log(`Step 3: Value retrieved successfully! Retrieved value: "${retrievedValue}"`);

  // Save contract data to file for later use
  const contractData = {
    contractAddress: deploymentReceipt.contractAddress,
    storedValue: valueToStore,
    retrievedValue: retrievedValue,
    storeTransactionHash: storeReceipt.transactionHash,
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

  logger.log("All steps completed successfully!");

  return true;
}

// Entry point
if (require.main === module) {
  main()
    .then((success: boolean) => {
      process.exit(success ? 0 : 1);
    })
    .catch((err) => {
      logger.error("Exiting with uncaught error");
      logger.error(err);
      process.exit(1);
    });
}
