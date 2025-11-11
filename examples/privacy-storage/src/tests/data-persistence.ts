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
} from "@lfdecentralizedtrust/paladin-sdk";
import { DEFAULT_POLL_TIMEOUT } from "paladin-example-common";
import { PrivateStorage } from "../helpers/storage";
import * as fs from 'fs';
import { nodeConnections, findLatestContractDataFile, getCachePath } from "paladin-example-common";

const logger = console;

interface ContractData {
  privacyGroupId: string;
  contractAddress: string;
  storedValue: number;
  retrievedValueNode1: string;
  retrievedValueNode2: string;
  storeTransactionHash: string;
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
  const [paladinNode1, paladinNode2, paladinNode3] = clients;
  const [verifierNode1] = paladinNode1.getVerifiers(`member@${nodeConnections[0].id}`);
  const [verifierNode2] = paladinNode2.getVerifiers(`member@${nodeConnections[1].id}`);
  const [verifierNode3] = paladinNode3.getVerifiers(`outsider@${nodeConnections[2].id}`);

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
  logger.log(`Contract Address: ${contractData.contractAddress}`);
  logger.log(`Stored Value: ${contractData.storedValue}`);
  logger.log(`Node1 Retrieved Value: ${contractData.retrievedValueNode1}`);
  logger.log(`Node2 Retrieved Value: ${contractData.retrievedValueNode2}`);
  logger.log(`Store TX Hash: ${contractData.storeTransactionHash}`);
  logger.log("=============================\n");

  // STEP 2: Get verifiers and recreate privacy group connection
  logger.log("STEP 2: Recreating privacy group connection...");

  const penteFactory = new PenteFactory(paladinNode1, "pente");
  const memberPrivacyGroup = await penteFactory.resumePrivacyGroup({
    id: contractData.privacyGroupId,
  });
  
  if (!memberPrivacyGroup) {
    logger.error("STEP 2: Failed to retrieve privacy group!");
    return false;
  }

  logger.log("STEP 2: Privacy group connection recreated successfully!");

  // STEP 3: Recreate private storage contract instance
  logger.log("STEP 3: Recreating private storage contract instance...");
  const privateStorageContract = new PrivateStorage(
    memberPrivacyGroup,
    contractData.contractAddress
  );
  logger.log("STEP 3: Private storage contract instance created successfully!");

  // STEP 4: Verify the stored value is still accessible by Node1
  logger.log("STEP 4: Verifying Node1 can still access the stored value...");
  try {
    const retrievedValueNode1 = await privateStorageContract.call({
      from: verifierNode1.lookup,
      function: "retrieve",
    });

    const currentValueNode1 = retrievedValueNode1["value"];
    logger.log(`STEP 4: Node1 current retrieved value: "${currentValueNode1}"`);

    if (currentValueNode1 !== contractData.retrievedValueNode1) {
      logger.error(`STEP 4: ERROR - Node1 current value does not match saved value!`);
      logger.error(`Expected: "${contractData.retrievedValueNode1}"`);
      logger.error(`Found: "${currentValueNode1}"`);
      return false;
    }

    logger.log("STEP 4: Node1 value verification successful!");

  } catch (error) {
    logger.error("STEP 4: Node1 failed to retrieve stored value!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 5: Verify the stored value is still accessible by Node2
  logger.log("STEP 5: Verifying Node2 can still access the stored value...");
  try {
    const retrievedValueNode2 = await privateStorageContract
      .using(paladinNode2)
      .call({
        from: verifierNode2.lookup,
        function: "retrieve",
      });

    const currentValueNode2 = retrievedValueNode2["value"];
    logger.log(`STEP 5: Node2 current retrieved value: "${currentValueNode2}"`);

    if (currentValueNode2 !== contractData.retrievedValueNode2) {
      logger.error(`STEP 5: ERROR - Node2 current value does not match saved value!`);
      logger.error(`Expected: "${contractData.retrievedValueNode2}"`);
      logger.error(`Found: "${currentValueNode2}"`);
      return false;
    }

    logger.log("STEP 5: Node2 value verification successful!");

  } catch (error) {
    logger.error("STEP 5: Node2 failed to retrieve stored value!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 6: Verify Node3 (outsider) still cannot access the data
  logger.log("STEP 6: Verifying Node3 (outsider) still cannot access the data...");
  try {
    await privateStorageContract.using(paladinNode3).call({
      from: verifierNode3.lookup,
      function: "retrieve",
    });
    logger.error("STEP 6: ERROR - Node3 (outsider) should not have access to the privacy group!");
    return false;
  } catch (error) {
    logger.log("STEP 6: Expected behavior - Node3 (outsider) cannot retrieve the data from the privacy group. Access denied.");
  }

  // STEP 7: Test contract functionality by storing a new value
  logger.log("STEP 7: Testing contract functionality with a new value...");
  try {
    const newValueToStore = 999;
    logger.log(`STEP 7: Storing new value "${newValueToStore}" in the contract...`);
    
    const storeReceipt = await privateStorageContract.sendTransaction({
      from: verifierNode1.lookup,
      function: "store",
      data: { num: newValueToStore },
    }).waitForReceipt(DEFAULT_POLL_TIMEOUT);

    if (!storeReceipt?.transactionHash) {
      logger.error("STEP 7: Function call failed!");
      return false;
    }

    logger.log("STEP 7: New value stored successfully!");

    // Verify the new value was stored correctly by Node1
    logger.log("STEP 7: Verifying Node1 can retrieve the new value...");
    const newRetrieveResultNode1 = await privateStorageContract.call({
      from: verifierNode1.lookup,
      function: "retrieve",
    });

    const newRetrievedValueNode1 = newRetrieveResultNode1["value"];
    if (newRetrievedValueNode1 !== newValueToStore.toString()) {
      logger.error(`STEP 7: ERROR - Node1 new value verification failed!`);
      logger.error(`Expected: "${newValueToStore}"`);
      logger.error(`Found: "${newRetrievedValueNode1}"`);
      return false;
    }

    // Verify the new value was stored correctly by Node2
    logger.log("STEP 7: Verifying Node2 can retrieve the new value...");
    const newRetrieveResultNode2 = await privateStorageContract
      .using(paladinNode2)
      .call({
        from: verifierNode2.lookup,
        function: "retrieve",
      });

    const newRetrievedValueNode2 = newRetrieveResultNode2["value"];
    if (newRetrievedValueNode2 !== newValueToStore.toString()) {
      logger.error(`STEP 7: ERROR - Node2 new value verification failed!`);
      logger.error(`Expected: "${newValueToStore}"`);
      logger.error(`Found: "${newRetrievedValueNode2}"`);
      return false;
    }

    logger.log("STEP 7: New value verification successful for both Node1 and Node2!");
    logger.log(`New stored value: "${newRetrievedValueNode1}"`);
    logger.log(`New transaction hash: ${storeReceipt.transactionHash}`);

  } catch (error) {
    logger.error("STEP 7: Contract functionality test failed!");
    logger.error(`Error: ${error}`);
    return false;
  }

  // STEP 8: Restore the original value
  logger.log("STEP 8: Restoring the original value...");
  try {
    logger.log(`STEP 8: Restoring original value "${contractData.storedValue}"...`);
    
    const restoreReceipt = await privateStorageContract.sendTransaction({
      from: verifierNode1.lookup,
      function: "store",
      data: { num: contractData.storedValue },
    }).waitForReceipt(DEFAULT_POLL_TIMEOUT);

    if (!restoreReceipt?.transactionHash) {
      logger.error("STEP 8: Function call failed!");
      return false;
    }

    logger.log("STEP 8: Original value restored successfully!");

    // Final verification by Node1
    const finalRetrieveResultNode1 = await privateStorageContract.call({
      from: verifierNode1.lookup,
      function: "retrieve",
    });

    const finalValueNode1 = finalRetrieveResultNode1["value"];
    if (finalValueNode1 !== contractData.retrievedValueNode1) {
      logger.error(`STEP 8: ERROR - Node1 final value verification failed!`);
      logger.error(`Expected: "${contractData.retrievedValueNode1}"`);
      logger.error(`Found: "${finalValueNode1}"`);
      return false;
    }

    // Final verification by Node2
    const finalRetrieveResultNode2 = await privateStorageContract
      .using(paladinNode2)
      .call({
        from: verifierNode2.lookup,
        function: "retrieve",
      });

    const finalValueNode2 = finalRetrieveResultNode2["value"];
    if (finalValueNode2 !== contractData.retrievedValueNode2) {
      logger.error(`STEP 8: ERROR - Node2 final value verification failed!`);
      logger.error(`Expected: "${contractData.retrievedValueNode2}"`);
      logger.error(`Found: "${finalValueNode2}"`);
      return false;
    }

    logger.log("STEP 8: Original value restoration verification successful for both Node1 and Node2!");

  } catch (error) {
    logger.error("STEP 8: Value restoration failed!");
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