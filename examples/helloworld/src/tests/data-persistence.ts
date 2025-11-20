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
import PaladinClient from "@lfdecentralizedtrust/paladin-sdk";
import helloWorldJson from "../abis/HelloWorld.json";
import * as fs from 'fs';
import { DEFAULT_POLL_TIMEOUT, nodeConnections, findLatestContractDataFile, getCachePath } from "paladin-example-common";

const logger = console;

interface ContractData {
  contractAddress: string;
  message: string;
  transactionHash: string;
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
  const owner = paladin.getVerifiers(`owner@${nodeConnections[0].id}`)[0];


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
  logger.log(`Contract Address: ${contractData.contractAddress}`);
  logger.log(`Original Message: "${contractData.message}"`);
  logger.log(`Transaction Hash: ${contractData.transactionHash}`);
  logger.log("=============================\n");

  // STEP 2: Get historical events by transaction hash
  logger.log("STEP 2: Getting historical events for the transaction...");
  
  try {
    // Get indexed events for this specific transaction
    const events = await paladin.bidx.getTransactionEventsByHash(contractData.transactionHash);

    if (events.length === 0) {
      logger.error("STEP 2: No historical events found for this transaction!");
      return false;
    }

    logger.log(`STEP 2: Found ${events.length} historical events for the transaction`);

    // STEP 3: Decode the specific transaction events to verify the message
    logger.log("STEP 3: Decoding historical transaction events...");
    const decodedEvents = await paladin.bidx.decodeTransactionEvents(
      contractData.transactionHash,
      helloWorldJson.abi,
      "pretty=true"
    );

    if (decodedEvents.length === 0) {
      logger.error("STEP 3: No decoded events found for the transaction!");
      return false;
    }

    logger.log(`STEP 3: Successfully decoded ${decodedEvents.length} events`);

    // STEP 4: Verify the event data matches what we expect
    logger.log("STEP 4: Verifying event data...");
    const historicalMessage = decodedEvents[0].data["message"];
    
    if (historicalMessage !== contractData.message) {
      logger.error(`STEP 4: ERROR - Historical event data does not match saved data!`);
      logger.error(`Expected: "${contractData.message}"`);
      logger.error(`Found: "${historicalMessage}"`);
      return false;
    }

    logger.log("STEP 4: Event data verification successful!");
    logger.log(`Historical message: "${historicalMessage}"`);

    // STEP 5: Additional verification - query transaction receipts
    logger.log("STEP 5: Querying transaction receipts...");
    const receipts = await paladin.ptx.queryTransactionReceipts({
      limit: 10,
      sort: ["sequence"],
      equal: [
        {
          field: "transactionHash",
          value: contractData.transactionHash
        }
      ]
    });

    if (receipts.length === 0) {
      logger.error("STEP 5: No transaction receipts found");
      return false;
    }  
    logger.log(`STEP 5: Found ${receipts.length} transaction receipts`);
    const receipt = receipts[0];
    logger.log(`Transaction ID: ${receipt.id}`);
    logger.log(`Success: ${receipt.success}`);
    logger.log(`Block Number: ${receipt.blockNumber}`);

    // STEP 6: Verify we can still call the contract (test callability)
    logger.log("STEP 6: Verifying contract is still callable...");
    
    try {
      // Try to call the sayHello function again with a different name
      const newName = "Arthur";
      logger.log(`STEP 6: Calling sayHello with name: "${newName}"`);
      
      const sayHelloTxID = await paladin.ptx.sendTransaction({
        type: "public" as any,
        abi: helloWorldJson.abi,
        function: "sayHello",
        from: owner.lookup,
        to: contractData.contractAddress,
        data: {
          name: newName,
        },
      });

      if (!sayHelloTxID) {
        logger.error("STEP 6: Function call failed!");
        return false;
      }

      // Wait for the function call receipt
      const functionReceipt = await paladin.pollForReceipt(sayHelloTxID, DEFAULT_POLL_TIMEOUT, true);
      if (!functionReceipt?.transactionHash) {
        logger.error("STEP 6: Receipt retrieval failed!");
        return false;
      }
      logger.log("STEP 6: New sayHello function call executed successfully!");

      // Decode the new event to verify it worked
      const newEvents = await paladin.bidx.decodeTransactionEvents(
        functionReceipt.transactionHash,
        helloWorldJson.abi,
        "pretty=true"
      );

      if (newEvents.length === 0) {
        logger.error("STEP 6: No events found in new transaction!");
        return false;
      }

      const newMessage = newEvents[0].data["message"];
      const expectedNewOutput = `Welcome to Paladin, ${newName}`;
      
      if (newMessage !== expectedNewOutput) {
        logger.error(`STEP 6: ERROR - New event data does not match expected output!`);
        logger.error(`Expected: "${expectedNewOutput}"`);
        logger.error(`Found: "${newMessage}"`);
        return false;
      }

      logger.log("STEP 6: Contract is still callable and working correctly!");
      logger.log(`New message: "${newMessage}"`);
      logger.log(`New transaction hash: ${functionReceipt.transactionHash}`);
      
    } catch (error) {
      logger.error("STEP 6: Contract call failed!");
      logger.error(`Error: ${error}`);
      return false;
    }

    logger.log("\nSUCCESS: verification completed!");

    return true;

  } catch (error) {
    logger.error("Error during historical verification:");
    logger.error(error);
    return false;
  }
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