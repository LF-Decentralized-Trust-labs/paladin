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
import { checkDeploy, DEFAULT_POLL_TIMEOUT } from "paladin-example-common";
import storageJson from "./abis/Storage.json";
import { PrivateStorage } from "./helpers/storage";
import * as fs from 'fs';
import * as path from 'path';
import { nodeConnections, getCachePath } from "paladin-example-common";

const logger = console;

async function main(): Promise<boolean> {
  // --- Initialization from Imported Config ---
  if (nodeConnections.length < 3) {
    logger.error("The environment config must provide at least 3 nodes for this scenario.");
    return false;
  }
  
  logger.log("Initializing Paladin clients from the environment configuration...");
  const clients = nodeConnections.map(node => new PaladinClient(node.clientOptions));
  const [paladinNode1, paladinNode2, paladinNode3] = clients;

  const [memberNode1] = paladinNode1.getVerifiers(`member@${nodeConnections[0].id}`);
  const [memberNode2] = paladinNode2.getVerifiers(`member@${nodeConnections[1].id}`);
  const [outsiderNode1] = paladinNode1.getVerifiers(`outsider@${nodeConnections[0].id}`);
  const [outsiderNode3] = paladinNode3.getVerifiers(`outsider@${nodeConnections[2].id}`);

  // Step 1: Create a privacy group for members
  logger.log("Creating a privacy group for member@node1 and member@node2...");
  const penteFactory = new PenteFactory(paladinNode1, "pente");
  const memberPrivacyGroup = await penteFactory.newPrivacyGroup({
    members: [memberNode1, memberNode2],
    evmVersion: "shanghai",
    externalCallsEnabled: true,
  }).waitForDeploy(DEFAULT_POLL_TIMEOUT);
  if (!checkDeploy(memberPrivacyGroup)) return false;

  logger.log(`Privacy group created, ID: ${memberPrivacyGroup?.group.id}`);

  // Step 2: Deploy a smart contract within the privacy group
  logger.log("Deploying a smart contract to the privacy group...");
  const contractAddress = await memberPrivacyGroup.deploy({
    abi: storageJson.abi,
    bytecode: storageJson.bytecode,
    from: memberNode1.lookup,
  }).waitForDeploy(DEFAULT_POLL_TIMEOUT);
  if (!contractAddress) {
    logger.error("Failed to deploy the contract. No address returned.");
    return false;
  }

  logger.log(`Contract deployed successfully! Address: ${contractAddress}`);

  // Step 3: Use the deployed contract for private storage
  const privateStorageContract = new PrivateStorage(
    memberPrivacyGroup,
    contractAddress
  );

  // Store a value in the contract
  const valueToStore = 125; // Example value to store
  logger.log(`Storing a value "${valueToStore}" in the contract...`);
  const storeReceipt = await privateStorageContract.sendTransaction({
    from: memberNode1.lookup,
    function: "store",
    data: { num: valueToStore },
  }).waitForReceipt(DEFAULT_POLL_TIMEOUT);
  
  // Validate store transaction was successful
  if (!storeReceipt?.success) {
    logger.error("Store transaction failed!");
    return false;
  }
  
  logger.log(
    "Value stored successfully! Transaction hash:",
    storeReceipt?.transactionHash
  );

  // Retrieve the value as member@node1
  logger.log("member@node1 retrieving the value from the contract...");
  const retrievedValueNode1 = await privateStorageContract.call({
    from: memberNode1.lookup,
    function: "retrieve",
  });
  
  // Validate the retrieved value
  if (retrievedValueNode1["value"] !== valueToStore.toString()) {
    logger.error(`Value retrieval validation failed for member@node1! Expected: "${valueToStore}", Retrieved: "${retrievedValueNode1["value"]}"`);
    return false;
  }
  
  logger.log(
    "member@node1 retrieved the value successfully:",
    retrievedValueNode1["value"]
  );

  // Retrieve the value as member@node2
  logger.log("member@node2 retrieving the value from the contract...");
  const retrievedValueNode2 = await privateStorageContract
    .using(paladinNode2)
    .call({
      from: memberNode2.lookup,
      function: "retrieve",
    });
    
  // Validate the retrieved value
  if (retrievedValueNode2["value"] !== valueToStore.toString()) {
    logger.error(`Value retrieval validation failed for member@node2! Expected: "${valueToStore}", Retrieved: "${retrievedValueNode2["value"]}"`);
    return false;
  }
  
  logger.log(
    "member@node2 retrieved the value successfully:",
    retrievedValueNode2["value"]
  );


    // Attempt to retrieve the value as outsider@node1 (non-member on same node as a member)
    try {
      logger.log("outsider@node1 attempting to retrieve the value...");
      await privateStorageContract.using(paladinNode1).call({
        from: outsiderNode1.lookup,
        function: "retrieve",
      });
      logger.error(
        "outsider@node1 should not have access to the privacy group!"
      );
      return false;
    } catch (error) {
      logger.log(
        "outsider@node1 correctly denied access to the privacy group!"
      );
    }

  // Attempt to retrieve the value as outsider@node3 (non-member on node with no members)
  try {
    logger.log("outsider@node3 attempting to retrieve the value...");
    await privateStorageContract.using(paladinNode3).call({
      from: outsiderNode3.lookup,
      function: "retrieve",
    });
    logger.error(
      "outsider@node3 should not have access to the privacy group!"
    );
    return false;
  } catch (error) {
    logger.log(
      "outsider@node3 correctly denied access to the privacy group!"
    );
  }

  // Save contract data to file for later use
  const contractData = {
    privacyGroupId: memberPrivacyGroup?.group.id,
    contractAddress: contractAddress,
    storedValue: valueToStore,
    retrievedValueNode1: retrievedValueNode1["value"],
    retrievedValueNode2: retrievedValueNode2["value"],
    storeTransactionHash: storeReceipt?.transactionHash,
    memberAtNode1Verifier: memberNode1.lookup,
    memberAtNode2Verifier: memberNode2.lookup,
    outsiderAtNode1Verifier: outsiderNode1.lookup,
    outsideAtNode3Verifier: outsiderNode3.lookup,
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

// Execute the main function when this file is run directly
if (require.main === module) {
  main()
    .then((success: boolean) => {
      process.exit(success ? 0 : 1); // Exit with status 0 for success, 1 for failure
    })
    .catch((err) => {
      logger.error("Exiting due to an uncaught error:", err);
      process.exit(1); // Exit with status 1 for any uncaught errors
    });
}
