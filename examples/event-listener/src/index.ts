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
  ITransactionReceipt,
  PaladinWebSocketClient,
  PenteFactory,
  TransactionType,
} from "@lfdecentralizedtrust/paladin-sdk";
import { nanoid } from "nanoid";
import { checkDeploy, getCachePath, waitForDomainReceipt, DEFAULT_POLL_TIMEOUT } from "paladin-example-common";
import helloWorldJson from "./abis/HelloWorld.json";
import * as fs from 'fs';
import * as path from 'path';
import { ContractData } from "./tests/data-persistence";
import { nodeConnections } from "paladin-example-common";

const logger = console;

async function main(): Promise<boolean> {
  // --- Initialization from Imported Config ---
  if (nodeConnections.length < 1) {
    logger.error("The environment config must provide at least 1 node for this scenario.");
    return false;
  }
  
  logger.log("Initializing Paladin client from the environment configuration...");
  const paladin = new PaladinClient(nodeConnections[0].clientOptions);
  const [verifierNode1] = paladin.getVerifiers(`group_member@${nodeConnections[0].id}`);

  // Create a privacy group for Node1 alone
  logger.log("Creating a privacy group for Node1...");
  const penteFactory = new PenteFactory(paladin, "pente");
  const memberPrivacyGroup = await penteFactory
    .newPrivacyGroup({
      members: [verifierNode1],
      evmVersion: "shanghai",
      externalCallsEnabled: true,
    })
    .waitForDeploy(DEFAULT_POLL_TIMEOUT);
  if (!checkDeploy(memberPrivacyGroup)) return false;

  // Deploy a smart contract within the privacy group
  logger.log("Deploying a smart contract to the privacy group...");
  const deploy = memberPrivacyGroup.deploy({
    abi: helloWorldJson.abi,
    bytecode: helloWorldJson.bytecode,
    from: verifierNode1.lookup,
  });
  const receipt = await deploy.waitForReceipt(DEFAULT_POLL_TIMEOUT);
  const contractAddress = await deploy.waitForDeploy(DEFAULT_POLL_TIMEOUT);
  if (!receipt || !contractAddress) {
    logger.error("Failed to deploy the contract. No address returned.");
    return false;
  }
  const lastSequence = receipt.sequence;
  logger.log(
    `Contract deployed successfully! Address: ${contractAddress} Sequence: ${lastSequence}`
  );

  // Store the ABI first to ensure event decoding works
  logger.log("Storing contract ABI for event decoding...");
  await paladin.ptx.storeABI(helloWorldJson.abi);

  // Create a new listener (deleting one if it already exists)
  logger.log("Creating a receipt listener...");
  try {
    await paladin.ptx.deleteReceiptListener("example-event-listener");
  } catch (err) {
    // Ignore error if listener doesn't exist
    logger.log("No existing listener to delete");
  }
  
  await paladin.ptx.createReceiptListener({
    name: "example-event-listener",
    filters: {
      type: TransactionType.PRIVATE,
      domain: "pente",
      sequenceAbove: lastSequence,
    },
    options: {
      domainReceipts: true,
      incompleteStateReceiptBehavior: "block_contract",
    },
  });

  const name = nanoid(10);
  let received = false;
  let receivedEventData: any = null;
  let receivedReceiptId: string | null = null;

  // Create a websocket client for node1
  const processReceipt = async (receipt: ITransactionReceipt) => {
    if (receipt.domain === undefined) {
      return;
    }
    logger.log(
      `Processing receipt ${receipt.id} (sequence: ${receipt.sequence})`
    );
    
    // Use the common helper to wait for domain receipt
    const domainReceipt = await waitForDomainReceipt(paladin, receipt.domain, receipt.id, {
      logger: logger,
      maxAttempts: 20,
      baseDelay: 500
    });
    
    if (!domainReceipt) {
      return; // Helper already logged the failure
    }
    
    if ("receipt" in domainReceipt) {
      // Skip if this receipt is not for our contract
      if (domainReceipt.receipt.to !== contractAddress) {
        logger.log(
          `Skipping receipt ${receipt.id} - not for our contract (to: ${domainReceipt.receipt.to})`
        );
        return;
      }
      logger.log(
        `Processing contract receipt ${receipt.id} (to: ${domainReceipt.receipt.to})`
      );
      for (const log of domainReceipt.receipt.logs ?? []) {
        try {
          const decoded = await paladin.ptx.decodeEvent(log.topics, log.data);
          const message = decoded?.data?.message;
          if (message?.indexOf(name) !== -1) {
            logger.log(`Received event data: ${JSON.stringify(decoded?.data)}`);
            received = true;
            receivedEventData = decoded?.data;
            receivedReceiptId = receipt.id;
          }
        } catch (decodeError) {
          logger.log(`Failed to decode event: ${decodeError}`);
          // Continue processing other logs
        }
      }
    }
  };
  const wsClient = new PaladinWebSocketClient(
    {
      url: "ws://127.0.0.1:31549",
      subscriptions: [{ type: "receipts", name: "example-event-listener" }],
    },
    async (sender, event) => {
      if (
        event.method === "ptx_subscription" &&
        "receipts" in event.params.result
      ) {
        logger.log(
          `Received receipt batch with ${event.params.result.receipts.length} receipts`
        );
        for (const receipt of event.params.result.receipts) {
          // Process each transaction receipt
          await processReceipt(receipt);
        }

        // Ack the receipt batch
        logger.log(`Acknowledging receipt batch`);
        sender.ack(event.params.subscription);
      }
    }
  );

  // Wait for WebSocket connection to be established
  logger.log("Waiting for WebSocket connection...");
  await new Promise((resolve) => setTimeout(resolve, 2000));

  const sayHelloMethod = helloWorldJson.abi.find(
    (abi) => abi.name === "sayHello"
  );
  if (!sayHelloMethod) return false;

  // Call the sayHello function
  logger.log(`Saying hello to '${name}'...`);
  const txId = await memberPrivacyGroup.sendTransaction({
    methodAbi: sayHelloMethod,
    from: verifierNode1.lookup,
    to: contractAddress,
    data: { name },
  }).id;
  logger.log(`Transaction sent with ID: ${txId}`);

  // Wait to receive the receipt
  const attempts = 10;
  const delay = 500;
  for (let i = 0; i < attempts; i++) {
    if (received) {
      logger.log(
        `Successfully received welcome message after ${i + 1} attempts`
      );
      break;
    }
    await new Promise((resolve) => setTimeout(resolve, delay));
  }
  if (!received) {
    logger.error("Failed to receive the receipt.");
    return false;
  }

  // Wait for event data to be properly captured
  logger.log("Waiting for event data to be captured...");
  const startTime = Date.now();
  
  while (!receivedEventData || !receivedReceiptId) {
    await new Promise((resolve) => setTimeout(resolve, 500)); // Reduced polling interval
    
    // If DEFAULT_POLL_TIMEOUT passed from the beginning of the loop then fail the test
    if (Date.now() - startTime > DEFAULT_POLL_TIMEOUT) {
      logger.error(`Failed to capture event data after ${DEFAULT_POLL_TIMEOUT/1000} seconds`);
      logger.error(`received: ${received}, receivedEventData: ${!!receivedEventData}, receivedReceiptId: ${!!receivedReceiptId}`);
      return false;
    }
  }
  logger.log("Event data captured successfully!");

  // Add a small delay to ensure any additional receipts are processed
  logger.log(`Waiting for any additional receipts to be processed...`);
  await new Promise((resolve) => setTimeout(resolve, 1000));

  await wsClient.close();

  // Save contract data to file for later use
  const contractData: ContractData = {
    privacyGroupId: memberPrivacyGroup.group.id,
    privacyGroupAddress: memberPrivacyGroup.address,
    contractAddress: contractAddress,
    listenerName: "example-event-listener",
    lastSequence: lastSequence,
    eventDetails: {
      name: name,
      receivedEventData: receivedEventData,
      receivedReceiptId: receivedReceiptId,
      transactionId: txId,
    },
    listenerConfig: {
      type: TransactionType.PRIVATE,
      domain: "pente",
      sequenceAbove: lastSequence,
      domainReceipts: true,
      incompleteStateReceiptBehavior: "block_contract"
    },
    websocketConfig: {
      url: "ws://127.0.0.1:31549",
      subscriptions: ["example-event-listener"]
    },
    participant: {
      verifierNode1: verifierNode1.lookup
    },
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

  return true;
}

if (require.main === module) {
  main()
    .then((success: boolean) => {
      process.exit(success ? 0 : 1);
    })
    .catch((err) => {
      logger.error("Exiting due to an uncaught error:", err);
      process.exit(1);
    });
}
