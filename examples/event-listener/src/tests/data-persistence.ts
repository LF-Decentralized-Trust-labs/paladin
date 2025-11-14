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
  PenteFactory,
  TransactionType,
} from "@lfdecentralizedtrust/paladin-sdk";
import * as fs from 'fs';
import * as path from 'path';
import helloWorldJson from "../abis/HelloWorld.json";
import { nodeConnections, findLatestContractDataFile, getCachePath, DEFAULT_POLL_TIMEOUT } from "paladin-example-common";

const logger = console;


export interface ContractData {
  privacyGroupId: string;
  privacyGroupAddress: string;
  contractAddress: string;
  listenerName: string;
  lastSequence: number;
  eventDetails: {
    name: string;
    receivedEventData: any;
    receivedReceiptId: string | null;
    transactionId: string;
  };
  listenerConfig: {
    type: TransactionType;
    domain: string;
    sequenceAbove: number;
    domainReceipts: boolean;
    incompleteStateReceiptBehavior: string;
  };
  websocketConfig: {
    url: string;
    subscriptions: string[];
  };
  participant: {
    verifierNode1: string;
  };
  timestamp: string;
}

async function main(): Promise<boolean> {
  // --- Initialization from Imported Config ---
  if (nodeConnections.length < 1) {
    logger.error("The environment config must provide at least 1 node for this scenario.");
    return false;
  }
  
  logger.log("Initializing Paladin client from the environment configuration...");
  const paladin = new PaladinClient(nodeConnections[0].clientOptions);

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
  logger.log(`Privacy Group ID: ${contractData.privacyGroupId}`);
  logger.log(`Privacy Group Address: ${contractData.privacyGroupAddress}`);
  logger.log(`Contract Address: ${contractData.contractAddress}`);
  logger.log(`Listener Name: ${contractData.listenerName}`);
  logger.log(`Last Sequence: ${contractData.lastSequence}`);
  logger.log(`Event Name: ${contractData.eventDetails.name}`);
  logger.log(`Transaction ID: ${contractData.eventDetails.transactionId}`);
  logger.log(`Received Receipt ID: ${contractData.eventDetails.receivedReceiptId}`);
  logger.log("=============================\n");

  // STEP 2: Get verifier and recreate privacy group connection
  logger.log("STEP 2: Recreating privacy group connection...");
  const [verifierNode1] = paladin.getVerifiers(`member@${nodeConnections[0].id}`);

  // Recreate privacy group connection
  const penteFactory = new PenteFactory(paladin, "pente");
  const memberPrivacyGroup = await penteFactory.resumePrivacyGroup({
    id: contractData.privacyGroupId,
  });

  if (!memberPrivacyGroup) {
    logger.error("STEP 2: Failed to retrieve privacy group!");
    return false;
  }

  logger.log("STEP 2: Privacy group connection recreated successfully!");

  // STEP 3: Verify privacy group accessibility
  logger.log("STEP 3: Verifying privacy group accessibility...");
  try {
    if (!memberPrivacyGroup.group || !memberPrivacyGroup.group.id) {
      logger.error("STEP 3: Privacy group accessibility test failed!");
      return false;
    }

    if (memberPrivacyGroup.group.id !== contractData.privacyGroupId) {
      logger.error("STEP 3: Privacy group ID does not match saved data!");
      logger.error(`Expected: ${contractData.privacyGroupId}`);
      logger.error(`Found: ${memberPrivacyGroup.group.id}`);
      return false;
    }

    if (memberPrivacyGroup.address !== contractData.privacyGroupAddress) {
      logger.error("STEP 3: Privacy group address does not match saved data!");
      logger.error(`Expected: ${contractData.privacyGroupAddress}`);
      logger.error(`Found: ${memberPrivacyGroup.address}`);
      return false;
    }

    logger.log("STEP 3: Privacy group accessibility verification successful!");

  } catch (error) {
    logger.error("STEP 3: Privacy group accessibility test failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 4: Verify contract is still accessible
  logger.log("STEP 4: Verifying contract accessibility...");
  try {
    // Test if we can query the contract
    const sayHelloMethod = helloWorldJson.abi.find(
      (abi) => abi.name === "sayHello"
    );
    if (!sayHelloMethod) {
      logger.error("STEP 4: Could not find sayHello method in ABI!");
      return false;
    }

    // Try to call the contract (read-only)
    const callResult = await memberPrivacyGroup.call({
      methodAbi: sayHelloMethod,
      from: verifierNode1.lookup,
      to: contractData.contractAddress,
      data: { name: "test" },
    });

    if (!callResult) {
      logger.error("STEP 4: Contract call failed!");
      return false;
    }

    logger.log("STEP 4: Contract accessibility verification successful!");

  } catch (error) {
    logger.error("STEP 4: Contract accessibility test failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 5: Verify event listener configuration
  logger.log("STEP 5: Verifying event listener configuration...");
  try {
    logger.log(`STEP 5: Listener configuration verification:`);
    logger.log(`Listener Name: ${contractData.listenerName}`);
    logger.log(`Type: ${contractData.listenerConfig.type}`);
    logger.log(`Domain: ${contractData.listenerConfig.domain}`);
    logger.log(`Sequence Above: ${contractData.listenerConfig.sequenceAbove}`);
    logger.log(`Domain Receipts: ${contractData.listenerConfig.domainReceipts}`);
    logger.log(`Incomplete State Receipt Behavior: ${contractData.listenerConfig.incompleteStateReceiptBehavior}`);

    // Verify that the listener configuration is properly formatted
    if (!contractData.listenerName || !contractData.listenerConfig.domain) {
      logger.error("STEP 5: ERROR - Listener configuration is missing critical information!");
      return false;
    }

    if (contractData.listenerConfig.sequenceAbove < 0) {
      logger.error("STEP 5: ERROR - Invalid sequence number!");
      return false;
    }

    logger.log("STEP 5: Event listener configuration verification successful!");

  } catch (error) {
    logger.error("STEP 5: Event listener configuration verification failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 6: Verify websocket configuration
  logger.log("STEP 6: Verifying websocket configuration...");
  try {
    logger.log(`STEP 6: Websocket configuration verification:`);
    logger.log(`URL: ${contractData.websocketConfig.url}`);
    logger.log(`Subscriptions: ${contractData.websocketConfig.subscriptions.join(', ')}`);

    // Verify that the websocket configuration is properly formatted
    if (!contractData.websocketConfig.url || !contractData.websocketConfig.subscriptions.length) {
      logger.error("STEP 6: ERROR - Websocket configuration is missing critical information!");
      return false;
    }

    if (!contractData.websocketConfig.subscriptions.includes(contractData.listenerName)) {
      logger.error("STEP 6: ERROR - Listener name not found in subscriptions!");
      return false;
    }

    logger.log("STEP 6: Websocket configuration verification successful!");

  } catch (error) {
    logger.error("STEP 6: Websocket configuration verification failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 7: Verify event details are preserved
  logger.log("STEP 7: Verifying event details are preserved...");
  try {
    logger.log(`STEP 7: Event details verification:`);
    logger.log(`Event Name: ${contractData.eventDetails.name}`);
    logger.log(`Transaction ID: ${contractData.eventDetails.transactionId}`);
    logger.log(`Received Receipt ID: ${contractData.eventDetails.receivedReceiptId}`);
    logger.log(`Received Event Data: ${JSON.stringify(contractData.eventDetails.receivedEventData)}`);

    // Verify that the event details are properly formatted
    if (!contractData.eventDetails.name || !contractData.eventDetails.transactionId) {
      logger.error("STEP 7: ERROR - Event details are missing critical information!");
      return false;
    }

    if (!contractData.eventDetails.receivedReceiptId) {
      logger.error("STEP 7: ERROR - Received receipt ID is missing!");
      return false;
    }

    // Check if event data is available
    if (!contractData.eventDetails.receivedEventData) {
      logger.warn("STEP 7: WARNING - Received event data is undefined. This might indicate the event was not properly captured.");
      logger.warn("STEP 7: This could be due to timing issues in the original run. The verification will continue with available data.");
      
      // Continue verification with available data
      logger.log("STEP 7: Event details verification completed (with warning about missing event data)");
    } else {
      // Verify the event data contains the expected message
      const message = contractData.eventDetails.receivedEventData.message;
      if (!message || !message.includes(contractData.eventDetails.name)) {
        logger.error("STEP 7: ERROR - Event message does not contain the expected name!");
        return false;
      }
      logger.log("STEP 7: Event details verification successful!");
    }

  } catch (error) {
    logger.error("STEP 7: Event details verification failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 8: Test contract functionality with new event
  logger.log("STEP 8: Testing contract functionality with new event...");
  try {
    const sayHelloMethod = helloWorldJson.abi.find(
      (abi) => abi.name === "sayHello"
    );
    if (!sayHelloMethod) {
      logger.error("STEP 8: Could not find sayHello method in ABI!");
      return false;
    }

    // Generate a new test name
    const testName = `verify-test-${Date.now()}`;
    logger.log(`STEP 8: Testing with new name: "${testName}"`);

    // Call the sayHello function
    const txId = await memberPrivacyGroup.sendTransaction({
      methodAbi: sayHelloMethod,
      from: verifierNode1.lookup,
      to: contractData.contractAddress,
      data: { name: testName },
    }).id;

    if (!txId) {
      logger.error("STEP 8: Failed to send test transaction!");
      return false;
    }

    logger.log(`STEP 8: Test transaction sent with ID: ${txId}`);

    // Try to get the transaction receipt
    const receipt = await paladin.pollForReceipt(txId, DEFAULT_POLL_TIMEOUT);
    if (!receipt) {
      logger.error("STEP 8: Failed to get test transaction receipt!");
      return false;
    }

    logger.log(`STEP 8: Test transaction receipt received: ${receipt.id}`);
    logger.log("STEP 8: Contract functionality test successful!");

  } catch (error) {
    logger.error("STEP 8: Contract functionality test failed!");
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