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
import helloWorldJson from "./abis/HelloWorld.json";
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

  // Retrieve the verifier for the owner account

  // STEP 1: Deploy the HelloWorld contract
  logger.log("STEP 1: Deploying the HelloWorld contract...");
  const deploymentTxID = await paladin.ptx.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: helloWorldJson.abi,
    bytecode: helloWorldJson.bytecode,
    from: owner.lookup,
    data: {},
  });

  // Wait for the deployment receipt
  const deploymentReceipt = await paladin.pollForReceipt(deploymentTxID, DEFAULT_POLL_TIMEOUT, true);
  if (!deploymentReceipt?.contractAddress) {
    logger.error("STEP 1: Deployment failed!");
    return false;
  }
  logger.log("STEP 1: HelloWorld contract deployed successfully!");

  // STEP 2: Call the sayHello function
  logger.log("STEP 2: Calling the sayHello function...");
  const name = "John"; // Example name for the greeting

  const sayHelloTxID = await paladin.ptx.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: helloWorldJson.abi,
    function: "sayHello",
    from: owner.lookup,
    to: deploymentReceipt.contractAddress,
    data: {
      name: name,
    },
  });

  if (!sayHelloTxID) {
    logger.error("STEP 2: Function call failed!");
    return false;
  }

  // Wait for the function call receipt
  const functionReceipt = await paladin.pollForReceipt(sayHelloTxID, DEFAULT_POLL_TIMEOUT, true);
  if (!functionReceipt?.transactionHash) {
    logger.error("STEP 2: Receipt retrieval failed!");
    return false;
  }
  
  // Validate the transaction was successful
  if (!functionReceipt.success) {
    logger.error("STEP 2: Transaction failed!");
    return false;
  }
  logger.log("STEP 2: sayHello function executed successfully!");

  // STEP 3: Retrieve and verify the emitted event
  logger.log("STEP 3: Retrieving and verifying emitted events...");
  const events = await paladin.bidx.decodeTransactionEvents(
    functionReceipt.transactionHash,
    helloWorldJson.abi,
    "pretty=true",
  );

  // Extract the event message and validate its content
  const message = events[0].data["message"];
  const expectedOutput = `Welcome to Paladin, ${name}`;
  if (message !== expectedOutput) {
    logger.error(`STEP 3: ERROR - Event data does not match the expected output! message: "${message}"`);
    return false;
  }
  logger.log("STEP 3: Events verified successfully!");

  // Save contract address and message to file for later use
  const contractData = {
    contractAddress: deploymentReceipt.contractAddress,
    message: message,
    transactionHash: functionReceipt.transactionHash,
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

  // Log the final message to the console
  logger.log("\n", message, "\n");

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
